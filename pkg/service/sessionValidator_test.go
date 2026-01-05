// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclient/game_session"
	"github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclient/party"
	sessionclientmodels "github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclientmodels"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errDummy = errors.New("dummy error")

type fakeSessionService struct {
	partyResp   *sessionclientmodels.ApimodelsPartyQueryResponse
	partyErr    error
	sessionResp *sessionclientmodels.ApimodelsGameSessionResponse
	sessionErr  error
}

func (f *fakeSessionService) AdminQueryPartiesShort(_ *party.AdminQueryPartiesParams) (*sessionclientmodels.ApimodelsPartyQueryResponse, error) {
	return f.partyResp, f.partyErr
}

func (f *fakeSessionService) GetGameSessionShort(_ *game_session.GetGameSessionParams) (*sessionclientmodels.ApimodelsGameSessionResponse, error) {
	return f.sessionResp, f.sessionErr
}

type fakeEpicClient struct {
	puids map[string]string
	err   error
}

func (f *fakeEpicClient) CreateRoomToken(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
	return nil, nil
}

func (f *fakeEpicClient) RemoveParticipant(ctx context.Context, roomID string, puid string) error {
	return nil
}

func (f *fakeEpicClient) GetEpicPUID(ctx context.Context, userIDs []string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.puids, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestSessionValidatorValidateUserInParty(t *testing.T) {
	userID := "user-1"
	partyID := "party-1"

	tests := []struct {
		name         string
		sessionSvc   sessionService
		expectedCode codes.Code
		expectError  bool
	}{
		{
			name:         "party service error",
			sessionSvc:   &fakeSessionService{partyErr: errDummy},
			expectedCode: codes.Internal,
			expectError:  true,
		},
		{
			name:         "party not found",
			sessionSvc:   &fakeSessionService{partyResp: nil},
			expectedCode: codes.NotFound,
			expectError:  true,
		},
		{
			name: "party response without data",
			sessionSvc: &fakeSessionService{
				partyResp: &sessionclientmodels.ApimodelsPartyQueryResponse{Data: []*sessionclientmodels.ApimodelsPartySessionResponse{}},
			},
			expectedCode: codes.NotFound,
			expectError:  true,
		},
		{
			name: "user not in party",
			sessionSvc: &fakeSessionService{
				partyResp: &sessionclientmodels.ApimodelsPartyQueryResponse{
					Data: []*sessionclientmodels.ApimodelsPartySessionResponse{
						{Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: stringPtr("other-user")}}},
					},
				},
			},
			expectedCode: codes.PermissionDenied,
			expectError:  true,
		},
		{
			name: "user in party",
			sessionSvc: &fakeSessionService{
				partyResp: &sessionclientmodels.ApimodelsPartyQueryResponse{
					Data: []*sessionclientmodels.ApimodelsPartySessionResponse{
						{Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: &userID}}},
					},
				},
			},
			expectError: false,
		},
	}

	validator := &SessionValidator{
		sessionService: nil,
		epicClient:     &fakeEpicClient{},
		logger:         testLogger(),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator.sessionService = tt.sessionSvc
			err := validator.ValidateUserInParty(context.Background(), userID, partyID)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error")
				}
				st := status.Convert(err)
				if st.Code() != tt.expectedCode {
					t.Fatalf("expected code %v got %v", tt.expectedCode, st.Code())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSessionValidatorValidateUserInSession(t *testing.T) {
	userID := "user-1"

	tests := []struct {
		name           string
		sessionService sessionService
		expectedCode   codes.Code
		expectError    bool
	}{
		{
			name:           "session service error",
			sessionService: &fakeSessionService{sessionErr: errDummy},
			expectedCode:   codes.Internal,
			expectError:    true,
		},
		{
			name:           "session not found",
			sessionService: &fakeSessionService{sessionResp: nil},
			expectedCode:   codes.NotFound,
			expectError:    true,
		},
		{
			name: "user not in session",
			sessionService: &fakeSessionService{
				sessionResp: &sessionclientmodels.ApimodelsGameSessionResponse{
					Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: stringPtr("other-user")}},
				},
			},
			expectedCode: codes.PermissionDenied,
			expectError:  true,
		},
		{
			name: "user in session",
			sessionService: &fakeSessionService{
				sessionResp: &sessionclientmodels.ApimodelsGameSessionResponse{
					Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: &userID}},
				},
			},
			expectError: false,
		},
	}

	validator := &SessionValidator{
		sessionService: nil,
		epicClient:     &fakeEpicClient{},
		logger:         testLogger(),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator.sessionService = tt.sessionService
			err := validator.ValidateUserInSession(context.Background(), userID, "session-1")
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error")
				}
				st := status.Convert(err)
				if st.Code() != tt.expectedCode {
					t.Fatalf("expected code %v got %v", tt.expectedCode, st.Code())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSessionValidatorGetUserTeamInSession(t *testing.T) {
	userID := "user-1"
	sessionID := "session-1"

	tests := []struct {
		name           string
		sessionService sessionService
		expectedCode   codes.Code
		expectedTeam   string
		expectError    bool
	}{
		{
			name:           "session service error",
			sessionService: &fakeSessionService{sessionErr: errDummy},
			expectedCode:   codes.Internal,
			expectError:    true,
		},
		{
			name:           "session not found",
			sessionService: &fakeSessionService{sessionResp: nil},
			expectedCode:   codes.NotFound,
			expectError:    true,
		},
		{
			name: "user not in session",
			sessionService: &fakeSessionService{
				sessionResp: &sessionclientmodels.ApimodelsGameSessionResponse{
					Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: stringPtr("other-user")}},
				},
			},
			expectedCode: codes.PermissionDenied,
			expectError:  true,
		},
		{
			name: "user in session but not in team",
			sessionService: &fakeSessionService{
				sessionResp: &sessionclientmodels.ApimodelsGameSessionResponse{
					Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: &userID}},
					Teams: []*sessionclientmodels.ModelsTeam{
						{TeamID: "team-1", UserIDs: []string{"other-user"}},
					},
				},
			},
			expectedCode: codes.PermissionDenied,
			expectError:  true,
		},
		{
			name: "user in team",
			sessionService: &fakeSessionService{
				sessionResp: &sessionclientmodels.ApimodelsGameSessionResponse{
					Members: []*sessionclientmodels.ApimodelsUserResponse{{ID: &userID}},
					Teams: []*sessionclientmodels.ModelsTeam{
						{TeamID: "team-1", UserIDs: []string{userID}},
					},
				},
			},
			expectedTeam: "team-1",
			expectError:  false,
		},
	}

	validator := &SessionValidator{
		sessionService: nil,
		epicClient:     &fakeEpicClient{},
		logger:         testLogger(),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator.sessionService = tt.sessionService
			teamID, err := validator.GetUserTeamInSession(context.Background(), userID, sessionID)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error")
				}
				st := status.Convert(err)
				if st.Code() != tt.expectedCode {
					t.Fatalf("expected code %v got %v", tt.expectedCode, st.Code())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if teamID != tt.expectedTeam {
				t.Fatalf("expected team %s got %s", tt.expectedTeam, teamID)
			}
		})
	}
}

func TestSessionValidatorGetEpicPUID(t *testing.T) {
	userID := "user-1"

	tests := []struct {
		name         string
		epicClient   EpicClientInterface
		expectedCode codes.Code
		expectedPUID string
		expectError  bool
	}{
		{
			name:         "epic client error",
			epicClient:   &fakeEpicClient{err: errDummy},
			expectedCode: codes.Internal,
			expectError:  true,
		},
		{
			name:         "account not linked",
			epicClient:   &fakeEpicClient{puids: map[string]string{}},
			expectedCode: codes.FailedPrecondition,
			expectError:  true,
		},
		{
			name:         "success",
			epicClient:   &fakeEpicClient{puids: map[string]string{userID: "puid-1"}},
			expectedPUID: "puid-1",
			expectError:  false,
		},
	}

	validator := &SessionValidator{
		sessionService: nil,
		logger:         testLogger(),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator.epicClient = tt.epicClient
			puid, err := validator.GetEpicPUID(context.Background(), userID)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error")
				}
				st := status.Convert(err)
				if st.Code() != tt.expectedCode {
					t.Fatalf("expected code %v got %v", tt.expectedCode, st.Code())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if puid != tt.expectedPUID {
				t.Fatalf("expected puid %s got %s", tt.expectedPUID, puid)
			}
		})
	}
}

func stringPtr(value string) *string {
	return &value
}
