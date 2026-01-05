// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package mocks

import (
	"testing"
	"time"

	iamclientmodels "github.com/AccelByte/accelbyte-go-sdk/iam-sdk/pkg/iamclientmodels"
	"go.uber.org/mock/gomock"
)

func TestMockTokenRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockTokenRepository(ctrl)
	mockRepo.EXPECT().GetToken().Return(&iamclientmodels.OauthmodelTokenResponseV3{}, nil)
	mockRepo.EXPECT().RemoveToken().Return(nil)
	mockRepo.EXPECT().Store(gomock.Any()).Return(nil)
	mockRepo.EXPECT().TokenIssuedTimeUTC().Return(time.Now())

	if _, err := mockRepo.GetToken(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mockRepo.RemoveToken(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mockRepo.Store("token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = mockRepo.TokenIssuedTimeUTC()
}

func TestMockConfigRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockConfigRepository(ctrl)
	mockRepo.EXPECT().GetClientId().Return("client")
	mockRepo.EXPECT().GetClientSecret().Return("secret")
	mockRepo.EXPECT().GetJusticeBaseUrl().Return("http://example.com")

	if mockRepo.GetClientId() == "" {
		t.Fatalf("expected client id")
	}
	if mockRepo.GetClientSecret() == "" {
		t.Fatalf("expected client secret")
	}
	if mockRepo.GetJusticeBaseUrl() == "" {
		t.Fatalf("expected base url")
	}
}

func TestMockRefreshTokenRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockRefreshTokenRepository(ctrl)
	mockRepo.EXPECT().DisableAutoRefresh().Return(true)
	mockRepo.EXPECT().GetRefreshRate().Return(0.5)
	mockRepo.EXPECT().SetRefreshIsRunningInBackground(true)

	if !mockRepo.DisableAutoRefresh() {
		t.Fatalf("expected disable auto refresh to return true")
	}
	if mockRepo.GetRefreshRate() != 0.5 {
		t.Fatalf("unexpected refresh rate")
	}
	mockRepo.SetRefreshIsRunningInBackground(true)
}
