// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	pb "extend-eos-voice-rtc/pkg/pb"

	sessionclientmodels "github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclientmodels"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockSessionValidator is a mock implementation of SessionValidator
type mockSessionValidator struct {
	validateUserInPartyFunc   func(ctx context.Context, userID string, partyID string) error
	validateUserInSessionFunc func(ctx context.Context, userID string, sessionID string) error
	getUserTeamInSessionFunc  func(ctx context.Context, userID string, sessionID string) (string, error)
	getEpicPUIDFunc           func(ctx context.Context, userID string) (string, error)
	getSessionFunc            func(ctx context.Context, sessionID string) (*sessionclientmodels.ApimodelsGameSessionResponse, error)
}

func (m *mockSessionValidator) ValidateUserInParty(ctx context.Context, userID string, partyID string) error {
	if m.validateUserInPartyFunc != nil {
		return m.validateUserInPartyFunc(ctx, userID, partyID)
	}
	return nil
}

func (m *mockSessionValidator) ValidateUserInSession(ctx context.Context, userID string, sessionID string) error {
	if m.validateUserInSessionFunc != nil {
		return m.validateUserInSessionFunc(ctx, userID, sessionID)
	}
	return nil
}

func (m *mockSessionValidator) GetUserTeamInSession(ctx context.Context, userID string, sessionID string) (string, error) {
	if m.getUserTeamInSessionFunc != nil {
		return m.getUserTeamInSessionFunc(ctx, userID, sessionID)
	}
	return "team-1", nil
}

func (m *mockSessionValidator) GetEpicPUID(ctx context.Context, userID string) (string, error) {
	if m.getEpicPUIDFunc != nil {
		return m.getEpicPUIDFunc(ctx, userID)
	}
	return "epic-puid-123", nil
}

func (m *mockSessionValidator) GetSession(ctx context.Context, sessionID string) (*sessionclientmodels.ApimodelsGameSessionResponse, error) {
	if m.getSessionFunc != nil {
		return m.getSessionFunc(ctx, sessionID)
	}
	return &sessionclientmodels.ApimodelsGameSessionResponse{}, nil
}

// mockEpicGamesClient is a mock implementation of EpicGamesClient
type mockEpicGamesClient struct {
	createRoomTokenFunc   func(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error)
	removeParticipantFunc func(ctx context.Context, roomID string, puid string) error
	getEpicPUIDFunc       func(ctx context.Context, userIDs []string) (map[string]string, error)
}

func (m *mockEpicGamesClient) CreateRoomToken(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
	if m.createRoomTokenFunc != nil {
		return m.createRoomTokenFunc(ctx, roomID, participants)
	}
	return &CreateRoomTokenResponse{
		RoomID:        roomID,
		ClientBaseURL: "wss://voice.epicgames.com",
		Participants: []RTCParticipantResponse{
			{
				PUID:      participants[0].PUID,
				Token:     "mock-voice-token",
				HardMuted: participants[0].HardMuted,
			},
		},
		DeploymentID: "test-deployment",
	}, nil
}

func (m *mockEpicGamesClient) RemoveParticipant(ctx context.Context, roomID string, puid string) error {
	if m.removeParticipantFunc != nil {
		return m.removeParticipantFunc(ctx, roomID, puid)
	}
	return nil
}

func (m *mockEpicGamesClient) GetEpicPUID(ctx context.Context, userIDs []string) (map[string]string, error) {
	if m.getEpicPUIDFunc != nil {
		return m.getEpicPUIDFunc(ctx, userIDs)
	}
	result := make(map[string]string)
	for _, userID := range userIDs {
		result[userID] = "epic-puid-" + userID
	}
	return result, nil
}

// createTestContext creates a context with authorization metadata
func createTestContext(userID string) context.Context {
	// Create JWT token: header.payload.signature
	// Payload: {"sub":"user-123","namespace":"test"}
	// Base64URL: eyJzdWIiOiJ1c2VyLTEyMyIsIm5hbWVzcGFjZSI6InRlc3QifQ
	payload := "eyJzdWIiOiJ1c2VyLTEyMyIsIm5hbWVzcGFjZSI6InRlc3QifQ"
	token := "header." + payload + ".signature"

	md := metadata.MD{
		"authorization": []string{"Bearer " + token},
	}
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestGeneratePartyToken(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name                string
		request             *pb.PartyTokenRequest
		ctx                 context.Context
		mockValidator       *mockSessionValidator
		mockEpicClient      *mockEpicGamesClient
		expectError         bool
		expectedCode        codes.Code
		expectedChannelType pb.ChannelType
	}{
		{
			name: "successfully generate party voice token",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-123",
				HardMuted: false,
			},
			mockValidator:       &mockSessionValidator{},
			mockEpicClient:      &mockEpicGamesClient{},
			expectError:         false,
			expectedChannelType: pb.ChannelType_PARTY,
		},
		{
			name: "successfully generate party voice token with hard muted",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-456",
				HardMuted: true,
			},
			mockValidator:       &mockSessionValidator{},
			mockEpicClient:      &mockEpicGamesClient{},
			expectError:         false,
			expectedChannelType: pb.ChannelType_PARTY,
		},
		{
			name: "provided puid bypasses lookup",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-777",
				HardMuted: false,
				Puid:      "epic-puid-1",
			},
			mockValidator: &mockSessionValidator{
				getEpicPUIDFunc: func(ctx context.Context, userID string) (string, error) {
					return "", errors.New("lookup should not be called")
				},
			},
			mockEpicClient: &mockEpicGamesClient{
				createRoomTokenFunc: func(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
					if len(participants) != 1 || participants[0].PUID != "epic-puid-1" {
						t.Fatalf("expected provided puid, got %v", participants)
					}
					return &CreateRoomTokenResponse{
						RoomID:        roomID,
						ClientBaseURL: "wss://voice.epicgames.com",
						Participants: []RTCParticipantResponse{
							{PUID: participants[0].PUID, Token: "mock-voice-token", HardMuted: participants[0].HardMuted},
						},
						DeploymentID: "test-deployment",
					}, nil
				},
			},
			expectError:         false,
			expectedChannelType: pb.ChannelType_PARTY,
		},
		{
			name: "blank puid returns error",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-888",
				HardMuted: false,
				Puid:      "   ",
			},
			ctx:            createTestContext("user-123"),
			mockValidator:  &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.InvalidArgument,
		},

		{
			name: "missing party_id returns error",
			request: &pb.PartyTokenRequest{
				PartyId:   "",
				HardMuted: false,
			},
			ctx:            createTestContext("user-123"),
			mockValidator:  &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.InvalidArgument,
		},
		{
			name: "user not in party returns error",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-999",
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				validateUserInPartyFunc: func(ctx context.Context, userID string, partyID string) error {
					return ErrUserNotInParty.ToGRPCStatus()
				},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.PermissionDenied,
		},
		{
			name: "epic account not linked returns error",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-123",
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				getEpicPUIDFunc: func(ctx context.Context, userID string) (string, error) {
					return "", ErrEpicAccountNotLinked.ToGRPCStatus()
				},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.FailedPrecondition,
		},
		{
			name: "missing auth context returns error",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-123",
				HardMuted: false,
			},
			ctx:            context.Background(),
			mockValidator:  &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.Unauthenticated,
		},
		{
			name: "epic client create token error",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-123",
				HardMuted: false,
			},
			ctx:           createTestContext("user-123"),
			mockValidator: &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{
				createRoomTokenFunc: func(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
					return nil, errors.New("create token failed")
				},
			},
			expectError:  true,
			expectedCode: codes.Internal,
		},
		{
			name: "epic client returns empty participants",
			request: &pb.PartyTokenRequest{
				PartyId:   "party-123",
				HardMuted: false,
			},
			ctx:           createTestContext("user-123"),
			mockValidator: &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{
				createRoomTokenFunc: func(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
					return &CreateRoomTokenResponse{Participants: []RTCParticipantResponse{}}, nil
				},
			},
			expectError:  true,
			expectedCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &EOSVoiceServiceServerImpl{
				tokenRepo:   nil,
				configRepo:  nil,
				refreshRepo: nil,
				epicClient:  tt.mockEpicClient,
				validator:   tt.mockValidator,
				logger:      logger,
			}

			ctx := tt.ctx
			if ctx == nil {
				ctx = createTestContext("user-123")
			}
			resp, err := service.GeneratePartyToken(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Errorf("GeneratePartyToken() expected error, got nil")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("GeneratePartyToken() error is not a status error")
					return
				}
				if st.Code() != tt.expectedCode {
					t.Errorf("GeneratePartyToken() error code = %v, want %v", st.Code(), tt.expectedCode)
				}
			} else {
				if err != nil {
					t.Errorf("GeneratePartyToken() unexpected error: %v", err)
					return
				}
				if resp.ChannelType != tt.expectedChannelType {
					t.Errorf("GeneratePartyToken() ChannelType = %v, want %v", resp.ChannelType, tt.expectedChannelType)
				}
				if resp.RoomId != tt.request.PartyId+":Voice" {
					t.Errorf("GeneratePartyToken() RoomId = %v, want %v", resp.RoomId, tt.request.PartyId+":Voice")
				}
			}
		})
	}
}

func TestGenerateSessionToken(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name                 string
		request              *pb.SessionTokenRequest
		ctx                  context.Context
		mockValidator        *mockSessionValidator
		mockEpicClient       *mockEpicGamesClient
		expectError          bool
		expectedCode         codes.Code
		expectedTokenCount   int
		expectedChannelTypes []pb.ChannelType
	}{
		{
			name: "successfully generate session token only",
			request: &pb.SessionTokenRequest{
				SessionId: "session-123",
				Team:      false,
				Session:   true,
				HardMuted: false,
			},
			ctx:                  createTestContext("user-123"),
			mockValidator:        &mockSessionValidator{},
			mockEpicClient:       &mockEpicGamesClient{},
			expectError:          false,
			expectedTokenCount:   1,
			expectedChannelTypes: []pb.ChannelType{pb.ChannelType_SESSION},
		},
		{
			name: "successfully generate team token only",
			request: &pb.SessionTokenRequest{
				SessionId: "session-456",
				Team:      true,
				Session:   false,
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				getUserTeamInSessionFunc: func(ctx context.Context, userID string, sessionID string) (string, error) {
					return "team-789", nil
				},
			},
			mockEpicClient:       &mockEpicGamesClient{},
			expectError:          false,
			expectedTokenCount:   1,
			expectedChannelTypes: []pb.ChannelType{pb.ChannelType_TEAM},
		},
		{
			name: "successfully generate both team and session tokens",
			request: &pb.SessionTokenRequest{
				SessionId: "session-789",
				Team:      true,
				Session:   true,
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				getUserTeamInSessionFunc: func(ctx context.Context, userID string, sessionID string) (string, error) {
					return "team-abc", nil
				},
			},
			mockEpicClient:       &mockEpicGamesClient{},
			expectError:          false,
			expectedTokenCount:   2,
			expectedChannelTypes: []pb.ChannelType{pb.ChannelType_TEAM, pb.ChannelType_SESSION},
		},
		{
			name: "both flags false returns error",
			request: &pb.SessionTokenRequest{
				SessionId: "session-999",
				Team:      false,
				Session:   false,
				HardMuted: false,
			},
			ctx:            createTestContext("user-123"),
			mockValidator:  &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.InvalidArgument,
		},
		{
			name: "missing session_id returns error",
			request: &pb.SessionTokenRequest{
				SessionId: "",
				Team:      false,
				Session:   true,
				HardMuted: false,
			},
			ctx:            createTestContext("user-123"),
			mockValidator:  &mockSessionValidator{},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.InvalidArgument,
		},
		{
			name: "user not in session returns error",
			request: &pb.SessionTokenRequest{
				SessionId: "session-999",
				Team:      false,
				Session:   true,
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				validateUserInSessionFunc: func(ctx context.Context, userID string, sessionID string) error {
					return ErrUserNotInSession.ToGRPCStatus()
				},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.PermissionDenied,
		},
		{
			name: "team requested but user has no team returns error",
			request: &pb.SessionTokenRequest{
				SessionId: "session-123",
				Team:      true,
				Session:   false,
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				getUserTeamInSessionFunc: func(ctx context.Context, userID string, sessionID string) (string, error) {
					return "", ErrUserNotInTeam.ToGRPCStatus()
				},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.FailedPrecondition,
		},
		{
			name: "partial success - team succeeds, session fails",
			request: &pb.SessionTokenRequest{
				SessionId: "session-partial",
				Team:      true,
				Session:   true,
				HardMuted: false,
			},
			ctx: createTestContext("user-123"),
			mockValidator: &mockSessionValidator{
				getUserTeamInSessionFunc: func(ctx context.Context, userID string, sessionID string) (string, error) {
					return "team-123", nil
				},
			},
			mockEpicClient: &mockEpicGamesClient{
				createRoomTokenFunc: func(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
					// Team token succeeds, session token fails
					if strings.Contains(roomID, ":team-123") {
						return &CreateRoomTokenResponse{
							RoomID:        roomID,
							ClientBaseURL: "wss://voice.epicgames.com",
							Participants: []RTCParticipantResponse{
								{PUID: participants[0].PUID, Token: "team-token", HardMuted: participants[0].HardMuted},
							},
						}, nil
					}
					return nil, errors.New("session token creation failed")
				},
			},
			expectError:          false,
			expectedTokenCount:   1,
			expectedChannelTypes: []pb.ChannelType{pb.ChannelType_TEAM},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &EOSVoiceServiceServerImpl{
				tokenRepo:   nil,
				configRepo:  nil,
				refreshRepo: nil,
				epicClient:  tt.mockEpicClient,
				validator:   tt.mockValidator,
				logger:      logger,
			}

			ctx := tt.ctx
			if ctx == nil {
				ctx = createTestContext("user-123")
			}
			resp, err := service.GenerateSessionToken(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Errorf("GenerateSessionToken() expected error, got nil")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("GenerateSessionToken() error is not a status error")
					return
				}
				if st.Code() != tt.expectedCode {
					t.Errorf("GenerateSessionToken() error code = %v, want %v", st.Code(), tt.expectedCode)
				}
			} else {
				if err != nil {
					t.Errorf("GenerateSessionToken() unexpected error: %v", err)
					return
				}
				if len(resp.Tokens) != tt.expectedTokenCount {
					t.Errorf("GenerateSessionToken() token count = %v, want %v", len(resp.Tokens), tt.expectedTokenCount)
					return
				}
				// Verify channel types
				for i, expectedType := range tt.expectedChannelTypes {
					if i >= len(resp.Tokens) {
						t.Errorf("GenerateSessionToken() missing token at index %d", i)
						continue
					}
					if resp.Tokens[i].ChannelType != expectedType {
						t.Errorf("GenerateSessionToken() token[%d].ChannelType = %v, want %v", i, resp.Tokens[i].ChannelType, expectedType)
					}
				}
			}
		})
	}
}
func TestRevokeToken(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		request        *pb.RevokeTokenRequest
		mockEpicClient *mockEpicGamesClient
		expectError    bool
		expectedCode   codes.Code
	}{
		{
			name: "successfully revoke voice tokens",
			request: &pb.RevokeTokenRequest{
				RoomId:  "party-123:Voice",
				UserIds: []string{"user-1", "user-2"},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    false,
		},
		{
			name: "missing room_id returns error",
			request: &pb.RevokeTokenRequest{
				RoomId:  "",
				UserIds: []string{"user-1"},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.InvalidArgument,
		},
		{
			name: "missing user_ids returns error",
			request: &pb.RevokeTokenRequest{
				RoomId:  "party-123:Voice",
				UserIds: []string{},
			},
			mockEpicClient: &mockEpicGamesClient{},
			expectError:    true,
			expectedCode:   codes.InvalidArgument,
		},
		{
			name: "epic client get puids error",
			request: &pb.RevokeTokenRequest{
				RoomId:  "party-123:Voice",
				UserIds: []string{"user-1"},
			},
			mockEpicClient: &mockEpicGamesClient{
				getEpicPUIDFunc: func(ctx context.Context, userIDs []string) (map[string]string, error) {
					return nil, errors.New("puid error")
				},
			},
			expectError:  true,
			expectedCode: codes.Internal,
		},
		{
			name: "epic client remove participant error",
			request: &pb.RevokeTokenRequest{
				RoomId:  "party-123:Voice",
				UserIds: []string{"user-1"},
			},
			mockEpicClient: &mockEpicGamesClient{
				removeParticipantFunc: func(ctx context.Context, roomID string, puid string) error {
					return errors.New("remove error")
				},
			},
			expectError:  true,
			expectedCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &EOSVoiceServiceServerImpl{
				tokenRepo:   nil,
				configRepo:  nil,
				refreshRepo: nil,
				epicClient:  tt.mockEpicClient,
				validator:   nil,
				logger:      logger,
			}

			ctx := context.Background()
			resp, err := service.RevokeToken(ctx, tt.request)

			if tt.expectError {
				if err == nil {
					t.Errorf("RevokeToken() expected error, got nil")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("RevokeToken() error is not a status error")
					return
				}
				if st.Code() != tt.expectedCode {
					t.Errorf("RevokeToken() error code = %v, want %v", st.Code(), tt.expectedCode)
				}
			} else {
				if err != nil {
					t.Errorf("RevokeToken() unexpected error: %v", err)
					return
				}
				if resp == nil {
					t.Errorf("RevokeToken() returned nil response")
				}
			}
		})
	}
}

func TestHealthCheck(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	service := &EOSVoiceServiceServerImpl{
		logger: logger,
	}

	ctx := context.Background()
	resp, err := service.HealthCheck(ctx, &pb.HealthCheckRequest{})

	if err != nil {
		t.Errorf("HealthCheck() unexpected error: %v", err)
		return
	}
	if resp.Status != "ok" {
		t.Errorf("HealthCheck() status = %v, want ok", resp.Status)
	}
}

func TestGenerateAdminSessionToken_AllowPendingUsers(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name              string
		allowPendingUsers bool
		sessionID         string
		team              bool
		session           bool
		memberStatuses    []string
		expectedUserCount int
		expectError       bool
		expectedErrorMsg  string
	}{
		{
			name:              "allow_pending_users=true includes INVITED users",
			allowPendingUsers: true,
			sessionID:         "session-1",
			session:           true,
			memberStatuses:    []string{"INVITED", "JOINED", "CONNECTED"},
			expectedUserCount: 3,
			expectError:       false,
		},
		{
			name:              "allow_pending_users=false excludes INVITED users",
			allowPendingUsers: false,
			sessionID:         "session-2",
			session:           true,
			memberStatuses:    []string{"INVITED", "JOINED", "CONNECTED"},
			expectedUserCount: 2,
			expectError:       false,
		},
		{
			name:              "all members INVITED with allow_pending_users=false returns error",
			allowPendingUsers: false,
			sessionID:         "session-3",
			session:           true,
			memberStatuses:    []string{"INVITED", "INVITED"},
			expectedUserCount: 0,
			expectError:       true,
			expectedErrorMsg:  "Set allow_pending_users=true to include users with status INVITED",
		},
		{
			name:              "all members INVITED with allow_pending_users=true succeeds",
			allowPendingUsers: true,
			sessionID:         "session-4",
			session:           true,
			memberStatuses:    []string{"INVITED", "INVITED"},
			expectedUserCount: 2,
			expectError:       false,
		},
		{
			name:              "mixed statuses with allow_pending_users=false",
			allowPendingUsers: false,
			sessionID:         "session-5",
			session:           true,
			memberStatuses:    []string{"INVITED", "JOINED", "LEFT", "KICKED", "CONNECTED"},
			expectedUserCount: 2, // Only JOINED and CONNECTED
			expectError:       false,
		},
		{
			name:              "mixed statuses with allow_pending_users=true",
			allowPendingUsers: true,
			sessionID:         "session-6",
			session:           true,
			memberStatuses:    []string{"INVITED", "JOINED", "LEFT", "KICKED", "CONNECTED"},
			expectedUserCount: 3, // INVITED, JOINED, CONNECTED
			expectError:       false,
		},
		{
			name:              "only inactive members with allow_pending_users=false",
			allowPendingUsers: false,
			sessionID:         "session-7",
			session:           true,
			memberStatuses:    []string{"LEFT", "KICKED", "REJECTED"},
			expectedUserCount: 0,
			expectError:       true,
			expectedErrorMsg:  "Set allow_pending_users=true",
		},
		{
			name:              "only inactive members with allow_pending_users=true",
			allowPendingUsers: true,
			sessionID:         "session-8",
			session:           true,
			memberStatuses:    []string{"LEFT", "KICKED", "REJECTED"},
			expectedUserCount: 0,
			expectError:       true,
			expectedErrorMsg:  "no valid members with status INVITED, JOINED, or CONNECTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock members based on test case
			members := make([]*sessionclientmodels.ApimodelsUserResponse, len(tt.memberStatuses))
			puids := make(map[string]string)

			for i, status := range tt.memberStatuses {
				userID := "user-" + string(rune('1'+i))
				puid := "puid-" + string(rune('1'+i))
				statusCopy := status

				members[i] = &sessionclientmodels.ApimodelsUserResponse{
					ID:       &userID,
					StatusV2: &statusCopy,
				}
				puids[userID] = puid
			}

			// Create mock validator
			mockValidator := &mockSessionValidator{
				getSessionFunc: func(ctx context.Context, sessionID string) (*sessionclientmodels.ApimodelsGameSessionResponse, error) {
					return &sessionclientmodels.ApimodelsGameSessionResponse{
						Members: members,
					}, nil
				},
			}

			// Create mock Epic client with dynamic participant handling
			mockEpicClient := &mockEpicGamesClient{
				getEpicPUIDFunc: func(ctx context.Context, userIDs []string) (map[string]string, error) {
					result := make(map[string]string)
					for _, userID := range userIDs {
						if puid, exists := puids[userID]; exists {
							result[userID] = puid
						}
					}
					return result, nil
				},
				createRoomTokenFunc: func(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
					response := &CreateRoomTokenResponse{
						RoomID:        roomID,
						ClientBaseURL: "wss://voice.epicgames.com",
						Participants:  []RTCParticipantResponse{},
						DeploymentID:  "test-deployment",
					}
					for _, p := range participants {
						response.Participants = append(response.Participants, RTCParticipantResponse{
							PUID:      p.PUID,
							Token:     "token-for-" + p.PUID,
							HardMuted: p.HardMuted,
						})
					}
					return response, nil
				},
			}

			service := &EOSVoiceServiceServerImpl{
				validator:  mockValidator,
				epicClient: mockEpicClient,
				logger:     logger,
			}

			ctx := context.Background()
			req := &pb.AdminSessionTokenRequest{
				SessionId:         tt.sessionID,
				Team:              tt.team,
				Session:           tt.session,
				AllowPendingUsers: tt.allowPendingUsers,
				HardMuted:         false,
				Notify:            false,
			}

			resp, err := service.GenerateAdminSessionToken(ctx, req)

			if tt.expectError {
				if err == nil {
					t.Errorf("GenerateAdminSessionToken() expected error but got none")
					return
				}
				if tt.expectedErrorMsg != "" && !strings.Contains(err.Error(), tt.expectedErrorMsg) {
					t.Errorf("GenerateAdminSessionToken() error = %v, want error containing %v", err.Error(), tt.expectedErrorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateAdminSessionToken() unexpected error: %v", err)
				return
			}

			if len(resp.Tokens) != tt.expectedUserCount {
				t.Errorf("GenerateAdminSessionToken() got %d tokens, want %d", len(resp.Tokens), tt.expectedUserCount)
			}
		})
	}
}
