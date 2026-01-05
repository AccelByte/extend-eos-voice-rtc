// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"

	pb "extend-eos-voice-rtc/pkg/pb"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"
)

type MyServiceServerImpl struct {
	pb.UnimplementedVoiceServer
	tokenRepo   repository.TokenRepository
	configRepo  repository.ConfigRepository
	refreshRepo repository.RefreshTokenRepository
}

func NewMyServiceServer(
	tokenRepo repository.TokenRepository,
	configRepo repository.ConfigRepository,
	refreshRepo repository.RefreshTokenRepository,
) *MyServiceServerImpl {
	return &MyServiceServerImpl{
		tokenRepo:   tokenRepo,
		configRepo:  configRepo,
		refreshRepo: refreshRepo,
	}
}

// HealthCheck returns the health status of the service
func (s *MyServiceServerImpl) HealthCheck(
	ctx context.Context, req *pb.HealthCheckRequest,
) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status: "ok",
	}, nil
}

// TODO: Implement your voice RTC service methods here
