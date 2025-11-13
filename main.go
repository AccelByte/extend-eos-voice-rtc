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
	sessionsvc "github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/session"
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
	environment     = "production"
	id              = int64(1)
	metricsEndpoint = "/metrics"
	metricsPort     = 8080
	grpcServerPort  = 6565
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
	voiceBaseURL := common.GetEnv("EOS_VOICE_BASE_URL", "https://api.epicgames.dev/rtc/")
	voiceDeploymentID := common.GetEnv("EOS_VOICE_DEPLOYMENT_ID", "")
	voiceTokenURL := common.GetEnv("EOS_VOICE_TOKEN_URL", "https://api.epicgames.dev/epic/oauth/v2/token")
	voiceClientID := common.GetEnv("EOS_VOICE_CLIENT_ID", "")
	voiceClientSecret := common.GetEnv("EOS_VOICE_CLIENT_SECRET", "")
	voiceScope := common.GetEnv("EOS_VOICE_SCOPE", "basic_profile friends_list presence")
	voiceTopic := common.GetEnv("EOS_VOICE_NOTIFICATION_TOPIC", "EOS_VOICE")
	if voiceDeploymentID == "" {
		logrus.Fatal("EOS_VOICE_DEPLOYMENT_ID environment variable is required")
	}
	if voiceClientID == "" || voiceClientSecret == "" {
		logrus.Fatal("EOS_VOICE_CLIENT_ID and EOS_VOICE_CLIENT_SECRET environment variables are required")
	}
	voiceHTTPClient, err := voiceclient.New(voiceclient.Config{
		BaseURL:      voiceBaseURL,
		DeploymentID: voiceDeploymentID,
		TokenURL:     voiceTokenURL,
		ClientID:     voiceClientID,
		ClientSecret: voiceClientSecret,
		Scope:        voiceScope,
	})
	if err != nil {
		logrus.Fatalf("failed to configure EOS voice client: %v", err)
	}

	sessionClient := factory.NewSessionClient(configRepo)
	gameSessionService := sessionsvc.GameSessionService{
		Client:           sessionClient,
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}
	partyService := sessionsvc.PartyService{
		Client:           sessionClient,
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}
	notificationService := lobbysvc.NotificationService{
		Client:           factory.NewLobbyClient(configRepo),
		ConfigRepository: configRepo,
		TokenRepository:  tokenRepo,
	}

	voiceProcessor, err := service.NewVoiceEventProcessor(service.VoiceProcessorConfig{
		Namespace:           absNamespace,
		NotificationTopic:   voiceTopic,
		VoiceClient:         voiceHTTPClient,
		GameSessionService:  gameSessionService,
		PartyService:        partyService,
		NotificationService: notificationService,
		Logger:              logrusLogger.WithField("component", "voice"),
	})
	if err != nil {
		logrus.Fatalf("failed to initialize voice processor: %v", err)
	}

	sessionpb.RegisterMpv2SessionHistoryGameSessionCreatedEventServiceServer(s, service.NewGameSessionCreatedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryGameSessionJoinedEventServiceServer(s, service.NewGameSessionJoinedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryGameSessionMembersChangedEventServiceServer(s, service.NewGameSessionMembersChangedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryGameSessionKickedEventServiceServer(s, service.NewGameSessionKickedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryGameSessionEndedEventServiceServer(s, service.NewGameSessionEndedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryPartyCreatedEventServiceServer(s, service.NewPartyCreatedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryPartyJoinedEventServiceServer(s, service.NewPartyJoinedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryPartyMembersChangedEventServiceServer(s, service.NewPartyMembersChangedServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryPartyLeaveEventServiceServer(s, service.NewPartyLeaveServer(voiceProcessor))
	sessionpb.RegisterMpv2SessionHistoryPartyKickedEventServiceServer(s, service.NewPartyKickedServer(voiceProcessor))

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
