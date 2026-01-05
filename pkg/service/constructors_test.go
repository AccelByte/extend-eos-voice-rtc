// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"extend-eos-voice-rtc/pkg/service/mocks"

	"github.com/AccelByte/accelbyte-go-sdk/iam-sdk/pkg/iamclientmodels"
	"go.uber.org/mock/gomock"
)

type fakeConfigRepo struct {
	baseURL string
}

func (f *fakeConfigRepo) GetClientId() string       { return "client" }
func (f *fakeConfigRepo) GetClientSecret() string   { return "secret" }
func (f *fakeConfigRepo) GetJusticeBaseUrl() string { return f.baseURL }

type fakeTokenRepo struct{}

func (f *fakeTokenRepo) Store(accessToken interface{}) error {
	return nil
}

func (f *fakeTokenRepo) GetToken() (*iamclientmodels.OauthmodelTokenResponseV3, error) {
	return &iamclientmodels.OauthmodelTokenResponseV3{}, nil
}

func (f *fakeTokenRepo) RemoveToken() error {
	return nil
}

func (f *fakeTokenRepo) TokenIssuedTimeUTC() time.Time {
	return time.Now().UTC()
}

type fakeRefreshRepo struct{}

func (f *fakeRefreshRepo) DisableAutoRefresh() bool { return true }
func (f *fakeRefreshRepo) GetRefreshRate() float64  { return 0.8 }
func (f *fakeRefreshRepo) SetRefreshIsRunningInBackground(b bool) {
}

type fakeEpicClientLite struct{}

func (f *fakeEpicClientLite) CreateRoomToken(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
	return nil, nil
}
func (f *fakeEpicClientLite) RemoveParticipant(ctx context.Context, roomID string, puid string) error {
	return nil
}
func (f *fakeEpicClientLite) GetEpicPUID(ctx context.Context, userIDs []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestNewEOSVoiceServiceServer(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tokenRepo := mocks.NewMockTokenRepository(ctrl)
	configRepo := mocks.NewMockConfigRepository(ctrl)
	refreshRepo := mocks.NewMockRefreshTokenRepository(ctrl)

	epicClient := &EpicGamesClient{}
	sessionValidator := &SessionValidator{}

	server := NewEOSVoiceServiceServer(tokenRepo, configRepo, refreshRepo, epicClient, sessionValidator, logger)

	if server.tokenRepo != tokenRepo {
		t.Fatalf("expected token repository to be set")
	}
	if server.configRepo != configRepo {
		t.Fatalf("expected config repository to be set")
	}
	if server.refreshRepo != refreshRepo {
		t.Fatalf("expected refresh repository to be set")
	}
	if server.epicClient != epicClient {
		t.Fatalf("expected epic client to be set")
	}
	if server.validator != sessionValidator {
		t.Fatalf("expected validator to be set")
	}
}

func TestNewSessionValidator(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	configRepo := &fakeConfigRepo{baseURL: "http://example.com"}
	tokenRepo := &fakeTokenRepo{}
	epicClient := &fakeEpicClientLite{}

	validator := NewSessionValidator(configRepo, tokenRepo, epicClient, logger)

	if validator.sessionService == nil {
		t.Fatalf("expected session service to be initialized")
	}
	if validator.configRepo != configRepo {
		t.Fatalf("expected config repo to be set")
	}
	if validator.tokenRepo != tokenRepo {
		t.Fatalf("expected token repo to be set")
	}
	if validator.epicClient != epicClient {
		t.Fatalf("expected epic client to be set")
	}
}

func TestNewMyServiceServerAndHealthCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tokenRepo := mocks.NewMockTokenRepository(ctrl)
	configRepo := mocks.NewMockConfigRepository(ctrl)
	refreshRepo := mocks.NewMockRefreshTokenRepository(ctrl)

	server := NewMyServiceServer(tokenRepo, configRepo, refreshRepo)
	if server == nil {
		t.Fatalf("expected server")
	}

	resp, err := server.HealthCheck(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected ok status")
	}
}
