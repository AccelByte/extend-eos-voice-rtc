// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package main

import (
	"context"
	"extend-eos-voice-rtc/pkg/common"
	"extend-eos-voice-rtc/pkg/service"
	"extend-eos-voice-rtc/pkg/voiceclient"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/iam"
	lobbysvc "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/lobby"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	sessionpb "extend-eos-voice-rtc/pkg/pb/accelbyte-asyncapi/session/session/v1"

	sdkAuth "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/utils/auth"
	prometheusGrpc "github.com/grpc-ecosystem/go-grpc-prometheus"
	prometheusCollectors "github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	environment          = "production"
	id                   = int64(1)
	metricsEndpoint      = "/metrics"
	metricsPort          = 8080
	grpcServerPort       = 6565
	defaultVoiceBaseURL  = "https://api.epicgames.dev"
	defaultVoiceTokenURL = "https://api.epicgames.dev/auth/v1/oauth/token"
)

var (
	serviceName = common.GetEnv("OTEL_SERVICE_NAME", "ExtendEventHandlerGoServerDocker")
	logLevelStr = common.GetEnv("LOG_LEVEL", logrus.InfoLevel.String())
)

func main() {
	logrus.Infof("starting app server..")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logrusLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		logrusLevel = logrus.InfoLevel
	}

	logrusLogger := logrus.New()
	logrusLogger.SetLevel(logrusLevel)

	loggingOptions := []logging.Option{
		logging.WithLogOnEvents(logging.StartCall, logging.FinishCall, logging.PayloadReceived, logging.PayloadSent),
		logging.WithFieldsFromContext(func(ctx context.Context) logging.Fields {
			if span := trace.SpanContextFromContext(ctx); span.IsSampled() {
				return logging.Fields{"traceID", span.TraceID().String()}
			}

			return nil
		}),
		logging.WithLevels(logging.DefaultClientCodeToLevel),
		logging.WithDurationField(logging.DurationToDurationField),
	}
	unaryServerInterceptors := []grpc.UnaryServerInterceptor{
		prometheusGrpc.UnaryServerInterceptor,
		logging.UnaryServerInterceptor(common.InterceptorLogger(logrusLogger), loggingOptions...),
	}
	streamServerInterceptors := []grpc.StreamServerInterceptor{
		prometheusGrpc.StreamServerInterceptor,
		logging.StreamServerInterceptor(common.InterceptorLogger(logrusLogger), loggingOptions...),
	}

	// Preparing the IAM authorization
	var tokenRepo repository.TokenRepository = sdkAuth.DefaultTokenRepositoryImpl()
	var configRepo repository.ConfigRepository = sdkAuth.DefaultConfigRepositoryImpl()
	var refreshRepo repository.RefreshTokenRepository = &sdkAuth.RefreshTokenImpl{AutoRefresh: true, RefreshRate: 0.8}

	// Create gRPC Server
	s := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(unaryServerInterceptors...),
		grpc.ChainStreamInterceptor(streamServerInterceptors...),
	)

	// Configure IAM authorization
	oauthService := iam.OAuth20Service{
		Client:                 factory.NewIamClient(configRepo),
		ConfigRepository:       configRepo,
		TokenRepository:        tokenRepo,
		RefreshTokenRepository: refreshRepo,
	}
	clientId := configRepo.GetClientId()
	clientSecret := configRepo.GetClientSecret()
	err = oauthService.LoginClient(&clientId, &clientSecret)
	if err != nil {
		logrus.Fatalf("Error unable to login using clientId and clientSecret: %v", err)
	}

	// Configure voice orchestration
	absNamespace := common.GetEnv("AB_NAMESPACE", "accelbyte")
	voiceDeploymentID := common.GetEnv("EOS_VOICE_DEPLOYMENT_ID", "")
	voiceClientID := common.GetEnv("EOS_VOICE_CLIENT_ID", "")
	voiceClientSecret := common.GetEnv("EOS_VOICE_CLIENT_SECRET", "")
	voiceTopic := common.GetEnv("EOS_VOICE_NOTIFICATION_TOPIC", "EOS_VOICE")
	enablePartyVoice := common.GetEnvBool("ENABLE_PARTY_VOICE", true)
	enableTeamVoice := common.GetEnvBool("ENABLE_TEAM_VOICE", true)
	enableGameVoice := common.GetEnvBool("ENABLE_GAME_VOICE", false)
	anyGameVoice := enableTeamVoice || enableGameVoice
	if voiceDeploymentID == "" {
		logrus.Fatal("EOS_VOICE_DEPLOYMENT_ID environment variable is required")
	}
	if voiceClientID == "" || voiceClientSecret == "" {
		logrus.Fatal("EOS_VOICE_CLIENT_ID and EOS_VOICE_CLIENT_SECRET environment variables are required")
	}
	voiceHTTPClient, err := voiceclient.New(voiceclient.Config{
		BaseURL:      defaultVoiceBaseURL,
		DeploymentID: voiceDeploymentID,
		TokenURL:     defaultVoiceTokenURL,
		ClientID:     voiceClientID,
		ClientSecret: voiceClientSecret,
	})
	if err != nil {
		logrus.Fatalf("failed to configure EOS voice client: %v", err)
	}

	notificationService := lobbysvc.NotificationService{
		Client:           factory.NewLobbyClient(configRepo),
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}

	voiceLogger := logrusLogger.WithField("component", "voice")
	voiceProcessor, err := service.NewVoiceEventProcessor(service.VoiceProcessorConfig{
		Namespace:           absNamespace,
		NotificationTopic:   voiceTopic,
		VoiceClient:         voiceHTTPClient,
		NotificationService: &notificationService,
		Logger:              voiceLogger,
		EnableTeamVoice:     enableTeamVoice,
		EnableGameVoice:     enableGameVoice,
	})
	if err != nil {
		logrus.Fatalf("failed to initialize voice processor: %v", err)
	}

	if anyGameVoice {
		sessionpb.RegisterMpv2SessionHistoryGameSessionCreatedEventServiceServer(s, service.NewGameSessionCreatedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryGameSessionJoinedEventServiceServer(s, service.NewGameSessionJoinedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryGameSessionMembersChangedEventServiceServer(s, service.NewGameSessionMembersChangedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryGameSessionKickedEventServiceServer(s, service.NewGameSessionKickedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryGameSessionEndedEventServiceServer(s, service.NewGameSessionEndedServer(voiceProcessor, voiceLogger))
		if enableTeamVoice {
			logrusLogger.Info("team-based game session events enabled")
		} else {
			logrusLogger.Warn("team-based game session events disabled")
		}
		if enableGameVoice {
			logrusLogger.Info("session-wide game session events enabled")
		} else {
			logrusLogger.Warn("session-wide game session events disabled")
		}
	} else {
		logrusLogger.Warn("game session events disabled")
	}
	if enablePartyVoice {
		sessionpb.RegisterMpv2SessionHistoryPartyCreatedEventServiceServer(s, service.NewPartyCreatedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryPartyJoinedEventServiceServer(s, service.NewPartyJoinedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryPartyMembersChangedEventServiceServer(s, service.NewPartyMembersChangedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryPartyLeaveEventServiceServer(s, service.NewPartyLeaveServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryPartyKickedEventServiceServer(s, service.NewPartyKickedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryPartyDeletedEventServiceServer(s, service.NewPartyDeletedServer(voiceProcessor, voiceLogger))
		sessionpb.RegisterMpv2SessionHistoryPartyRejoinedEventServiceServer(s, service.NewPartyRejoinedServer(voiceProcessor, voiceLogger))
		logrusLogger.Info("party events enabled")
	} else {
		logrusLogger.Warn("party events disabled")
	}
	if !enablePartyVoice && !anyGameVoice {
		logrusLogger.Warn("ENABLE_PARTY_VOICE, ENABLE_TEAM_VOICE, and ENABLE_GAME_VOICE are all false; no events will be processed")
	}

	// Enable gRPC Reflection
	reflection.Register(s)
	logrus.Infof("gRPC reflection enabled")

	// Enable gRPC Health GetEventInfo
	grpc_health_v1.RegisterHealthServer(s, health.NewServer())

	prometheusGrpc.Register(s)

	// Register Prometheus Metrics
	prometheusRegistry := prometheus.NewRegistry()
	prometheusRegistry.MustRegister(
		prometheusCollectors.NewGoCollector(),
		prometheusCollectors.NewProcessCollector(prometheusCollectors.ProcessCollectorOpts{}),
		prometheusGrpc.DefaultServerMetrics,
	)

	go func() {
		http.Handle(metricsEndpoint, promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{}))
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", metricsPort), nil))
	}()
	logrus.Infof("serving prometheus metrics at: (:%d%s)", metricsPort, metricsEndpoint)

	// Save Tracer Provider
	tracerProvider, err := common.NewTracerProvider(serviceName, environment, id)
	if err != nil {
		logrus.Fatalf("failed to create tracer provider: %v", err)

		return
	}
	otel.SetTracerProvider(tracerProvider)
	defer func(ctx context.Context) {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			logrus.Fatal(err)
		}
	}(ctx)
	logrus.Infof("set tracer provider: (name: %s environment: %s id: %d)", serviceName, environment, id)

	// Save Text Map Propagator
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			b3.New(),
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	logrus.Infof("set text map propagator")

	// Start gRPC Server
	logrus.Infof("starting gRPC server..")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcServerPort))
	if err != nil {
		logrus.Fatalf("failed to listen to tcp:%d: %v", grpcServerPort, err)

		return
	}
	go func() {
		if err = s.Serve(lis); err != nil {
			logrus.Fatalf("failed to run gRPC server: %v", err)

			return
		}
	}()
	logrus.Infof("gRPC server started")
	logrus.Infof("app server started")

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	logrus.Infof("signal received")
}

// no extra helpers required beyond boolean env toggles
