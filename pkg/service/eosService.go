// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"extend-eos-voice-rtc/pkg/common"
	pb "extend-eos-voice-rtc/pkg/pb"

	"github.com/AccelByte/accelbyte-go-sdk/lobby-sdk/pkg/lobbyclient/notification"
	"github.com/AccelByte/accelbyte-go-sdk/lobby-sdk/pkg/lobbyclientmodels"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/lobby"
	sessionclientmodels "github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclientmodels"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EpicClientInterface defines the interface for Epic Games API client
type EpicClientInterface interface {
	CreateRoomToken(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error)
	RemoveParticipant(ctx context.Context, roomID string, puid string) error
	GetEpicPUID(ctx context.Context, userIDs []string) (map[string]string, error)
}

// SessionValidatorInterface defines the interface for session validation
type SessionValidatorInterface interface {
	ValidateUserInParty(ctx context.Context, userID string, partyID string) error
	ValidateUserInSession(ctx context.Context, userID string, sessionID string) error
	GetUserTeamInSession(ctx context.Context, userID string, sessionID string) (string, error)
	GetEpicPUID(ctx context.Context, userID string) (string, error)
	GetSession(ctx context.Context, sessionID string) (*sessionclientmodels.ApimodelsGameSessionResponse, error)
}

// EOSVoiceServiceServerImpl implements the EOS Voice RTC service
type EOSVoiceServiceServerImpl struct {
	pb.UnimplementedVoiceServer
	tokenRepo   repository.TokenRepository
	configRepo  repository.ConfigRepository
	refreshRepo repository.RefreshTokenRepository
	epicClient  EpicClientInterface
	validator   SessionValidatorInterface
	logger      *slog.Logger
}

// NewEOSVoiceServiceServer creates a new EOS Voice service server
func NewEOSVoiceServiceServer(
	tokenRepo repository.TokenRepository,
	configRepo repository.ConfigRepository,
	refreshRepo repository.RefreshTokenRepository,
	epicClient *EpicGamesClient,
	validator *SessionValidator,
	logger *slog.Logger,
) *EOSVoiceServiceServerImpl {
	return &EOSVoiceServiceServerImpl{
		tokenRepo:   tokenRepo,
		configRepo:  configRepo,
		refreshRepo: refreshRepo,
		epicClient:  epicClient,
		validator:   validator,
		logger:      logger,
	}
}

// HealthCheck returns the health status of the service
func (s *EOSVoiceServiceServerImpl) HealthCheck(
	ctx context.Context, req *pb.HealthCheckRequest,
) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status: "ok",
	}, nil
}

func (s *EOSVoiceServiceServerImpl) resolvePUID(ctx context.Context, userID string, providedPUID string) (string, error) {
	if providedPUID != "" {
		trimmed := strings.TrimSpace(providedPUID)
		if trimmed == "" {
			return "", status.Error(codes.InvalidArgument, "puid must not be empty")
		}
		return trimmed, nil
	}

	return s.validator.GetEpicPUID(ctx, userID)
}

// GeneratePartyToken generates a voice token for a party voice channel
func (s *EOSVoiceServiceServerImpl) GeneratePartyToken(
	ctx context.Context, req *pb.PartyTokenRequest,
) (*pb.EOSTokenResponse, error) {
	// Step 1: Extract party_id from request (already in req.PartyId from path parameter)
	partyID := req.PartyId
	if partyID == "" {
		s.logger.Error("party_id is required")
		return nil, status.Error(codes.InvalidArgument, "party_id is required")
	}

	// Step 2: Extract hardMuted from query string (already in req.HardMuted)
	hardMuted := req.HardMuted

	// Step 3: Extract user ID from authorization token
	userID, err := common.GetUserIDFromContext(ctx)
	if err != nil {
		s.logger.Error("failed to extract user ID from context", "error", err)
		return nil, err
	}

	// Step 4: Extract client IP from HTTP headers
	clientIP := common.ExtractClientIP(ctx)

	s.logger.Info("generating party voice token",
		"userID", userID,
		"partyID", partyID,
		"hardMuted", hardMuted,
		"clientIP", clientIP)

	// Step 5: Validate user is in party (fail fast)
	if err := s.validator.ValidateUserInParty(ctx, userID, partyID); err != nil {
		s.logger.Error("user not in party", "userID", userID, "partyID", partyID, "error", err)
		return nil, err
	}

	// Step 6: Get Epic PUID (fail fast if not linked)
	puid, err := s.resolvePUID(ctx, userID, req.Puid)

	if err != nil {
		s.logger.Error("failed to get Epic PUID", "userID", userID, "error", err)
		return nil, err
	}

	// Step 7: Generate room ID: partyID + ":Voice"
	roomID := partyID + ":Voice"

	// Step 8: Generate token via Epic RTC API
	participants := []RTCParticipant{
		{
			PUID:      puid,
			ClientIP:  clientIP,
			HardMuted: hardMuted,
		},
	}

	tokenResp, err := s.epicClient.CreateRoomToken(ctx, roomID, participants)
	if err != nil {
		s.logger.Error("failed to create room token", "roomID", roomID, "error", err)
		return nil, ParseHTTPError(err, "epic_rtc")
	}

	// Extract the token for the user
	if len(tokenResp.Participants) == 0 {
		s.logger.Error("no participant token in response", "roomID", roomID)
		return nil, ErrEpicRTCAPI.ToGRPCStatus()
	}

	participantToken := tokenResp.Participants[0]

	// Return response
	return &pb.EOSTokenResponse{
		ClientBaseUrl: tokenResp.ClientBaseURL,
		Token:         participantToken.Token,
		ChannelType:   pb.ChannelType_PARTY,
		RoomId:        roomID,
	}, nil
}

// GenerateSessionToken generates voice tokens for session and/or team channels
func (s *EOSVoiceServiceServerImpl) GenerateSessionToken(
	ctx context.Context, req *pb.SessionTokenRequest,
) (*pb.SessionTokenResponse, error) {
	// Validate: at least one must be true
	if !req.Team && !req.Session {
		return nil, status.Error(codes.InvalidArgument,
			"cannot generate voice token: both team and session flags are set to false, at least one must be set to true")
	}

	// Validate session_id
	sessionID := req.SessionId
	if sessionID == "" {
		s.logger.Error("session_id is required")
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	// Extract user ID from authorization token
	userID, err := common.GetUserIDFromContext(ctx)
	if err != nil {
		s.logger.Error("failed to extract user ID from context", "error", err)
		return nil, err
	}

	// Extract client IP from HTTP headers
	clientIP := common.ExtractClientIP(ctx)
	hardMuted := req.HardMuted

	s.logger.Info("generating session voice tokens",
		"userID", userID,
		"sessionID", sessionID,
		"team", req.Team,
		"session", req.Session,
		"hardMuted", hardMuted,
		"clientIP", clientIP)

	// Validate user in session (common for both team and session tokens)
	if err := s.validator.ValidateUserInSession(ctx, userID, sessionID); err != nil {
		s.logger.Error("user not in session", "userID", userID, "sessionID", sessionID, "error", err)
		return nil, err
	}

	// Get Epic PUID (common for both)
	puid, err := s.resolvePUID(ctx, userID, req.Puid)
	if err != nil {
		s.logger.Error("failed to get Epic PUID", "userID", userID, "error", err)
		return nil, err
	}

	// Initialize response with empty array
	response := &pb.SessionTokenResponse{
		Tokens: []*pb.EOSTokenResponse{},
	}

	// Track if any token generation failed
	var teamErr, sessionErr error

	// Generate team token if requested
	if req.Team {
		teamToken, err := s.generateTeamTokenInternal(ctx, sessionID, userID, puid, clientIP, hardMuted)
		if err != nil {
			s.logger.Error("failed to generate team token", "error", err)
			teamErr = err
			// Continue to try session token if requested
		} else {
			response.Tokens = append(response.Tokens, teamToken)
		}
	}

	// Generate session token if requested
	if req.Session {
		sessionToken, err := s.generateSessionTokenInternal(ctx, sessionID, puid, clientIP, hardMuted)
		if err != nil {
			s.logger.Error("failed to generate session token", "error", err)
			sessionErr = err
			// Continue - might have team token already
		} else {
			response.Tokens = append(response.Tokens, sessionToken)
		}
	}

	// Check results
	if len(response.Tokens) == 0 {
		// Both failed or no tokens generated
		if teamErr != nil && sessionErr != nil {
			return nil, status.Errorf(codes.Internal,
				"failed to generate both team and session tokens: team=%v, session=%v", teamErr, sessionErr)
		} else if teamErr != nil {
			return nil, teamErr
		} else if sessionErr != nil {
			return nil, sessionErr
		}
		return nil, status.Error(codes.Internal, "failed to generate any tokens")
	}

	// Partial or full success
	if teamErr != nil || sessionErr != nil {
		s.logger.Warn("partial success generating tokens",
			"teamSuccess", teamErr == nil && req.Team,
			"sessionSuccess", sessionErr == nil && req.Session,
			"tokensGenerated", len(response.Tokens))
	}

	return response, nil
}

// generateTeamTokenInternal generates a team token (internal helper)
func (s *EOSVoiceServiceServerImpl) generateTeamTokenInternal(
	ctx context.Context, sessionID, userID, puid, clientIP string, hardMuted bool,
) (*pb.EOSTokenResponse, error) {
	// Get user's team ID
	teamID, err := s.validator.GetUserTeamInSession(ctx, userID, sessionID)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition,
			"failed to get user team: %v", err)
	}

	// Generate room ID: sessionID + ":" + teamID
	roomID := sessionID + ":" + teamID

	// Generate token via Epic RTC API
	participants := []RTCParticipant{
		{
			PUID:      puid,
			ClientIP:  clientIP,
			HardMuted: hardMuted,
		},
	}

	tokenResp, err := s.epicClient.CreateRoomToken(ctx, roomID, participants)
	if err != nil {
		return nil, ParseHTTPError(err, "epic_rtc")
	}

	if len(tokenResp.Participants) == 0 {
		return nil, ErrEpicRTCAPI.ToGRPCStatus()
	}

	participantToken := tokenResp.Participants[0]

	return &pb.EOSTokenResponse{
		ClientBaseUrl: tokenResp.ClientBaseURL,
		Token:         participantToken.Token,
		ChannelType:   pb.ChannelType_TEAM,
		RoomId:        roomID,
	}, nil
}

// generateSessionTokenInternal generates a session token (internal helper)
func (s *EOSVoiceServiceServerImpl) generateSessionTokenInternal(
	ctx context.Context, sessionID, puid, clientIP string, hardMuted bool,
) (*pb.EOSTokenResponse, error) {
	// Generate room ID: sessionID + ":Voice"
	roomID := sessionID + ":Voice"

	// Generate token via Epic RTC API
	participants := []RTCParticipant{
		{
			PUID:      puid,
			ClientIP:  clientIP,
			HardMuted: hardMuted,
		},
	}

	tokenResp, err := s.epicClient.CreateRoomToken(ctx, roomID, participants)
	if err != nil {
		return nil, ParseHTTPError(err, "epic_rtc")
	}

	if len(tokenResp.Participants) == 0 {
		return nil, ErrEpicRTCAPI.ToGRPCStatus()
	}

	participantToken := tokenResp.Participants[0]

	return &pb.EOSTokenResponse{
		ClientBaseUrl: tokenResp.ClientBaseURL,
		Token:         participantToken.Token,
		ChannelType:   pb.ChannelType_SESSION,
		RoomId:        roomID,
	}, nil
}

// GenerateAdminSessionToken generates voice tokens for all users in a session
func (s *EOSVoiceServiceServerImpl) GenerateAdminSessionToken(
	ctx context.Context, req *pb.AdminSessionTokenRequest,
) (*pb.AdminSessionTokenResponse, error) {
	sessionID := req.SessionId
	if sessionID == "" {
		s.logger.Error("session_id is required")
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	if !req.Team && !req.Session {
		return nil, status.Error(codes.InvalidArgument, "team or session must be true")
	}

	hardMuted := req.HardMuted
	notify := req.Notify
	allowPendingUsers := req.AllowPendingUsers

	sessionResp, err := s.validator.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Filter members based on allow_pending_users flag
	userIDs := make([]string, 0, len(sessionResp.Members))
	for _, member := range sessionResp.Members {
		if member == nil || member.ID == nil || *member.ID == "" {
			continue
		}

		// Check if member has a valid StatusV2
		if member.StatusV2 == nil {
			s.logger.Warn("member missing StatusV2, skipping",
				"userID", *member.ID, "sessionID", sessionID)
			continue
		}

		status := *member.StatusV2

		// Filter based on allow_pending_users flag
		if allowPendingUsers {
			// Include: INVITED, JOINED, CONNECTED
			if status == "INVITED" || status == "JOINED" || status == "CONNECTED" {
				userIDs = append(userIDs, *member.ID)
			} else {
				s.logger.Debug("excluding member with status",
					"userID", *member.ID, "status", status, "sessionID", sessionID)
			}
		} else {
			// Default: Only include JOINED, CONNECTED (exclude INVITED)
			if status == "JOINED" || status == "CONNECTED" {
				userIDs = append(userIDs, *member.ID)
			} else {
				s.logger.Debug("excluding member with status",
					"userID", *member.ID, "status", status, "sessionID", sessionID)
			}
		}
	}

	if len(userIDs) == 0 {
		if allowPendingUsers {
			return nil, status.Error(codes.FailedPrecondition,
				"session has no valid members with status INVITED, JOINED, or CONNECTED")
		}
		return nil, status.Error(codes.FailedPrecondition,
			"session has no valid members with status JOINED or CONNECTED. Set allow_pending_users=true to include users with status INVITED")
	}

	clientIP := common.ExtractClientIP(ctx)
	if sessionResp.DSInformation != nil && sessionResp.DSInformation.Server != nil && sessionResp.DSInformation.Server.IP != "" {
		clientIP = sessionResp.DSInformation.Server.IP
	}

	s.logger.Debug("generating admin session voice tokens",
		"sessionID", sessionID,
		"team", req.Team,
		"session", req.Session,
		"hardMuted", hardMuted,
		"notify", notify,
		"allowPendingUsers", allowPendingUsers,
		"clientIP", clientIP,
		"totalMembers", len(sessionResp.Members),
		"eligibleUserCount", len(userIDs))

	puids, err := s.epicClient.GetEpicPUID(ctx, userIDs)
	if err != nil {
		s.logger.Error("failed to get Epic PUIDs", "error", err)
		return nil, ParseHTTPError(err, "epic_puid")
	}

	tokens := make([]*pb.AdminUserTokenResponse, 0)

	if req.Session {
		roomID := sessionID + ":Voice"
		roomTokens, err := s.createRoomTokens(ctx, roomID, userIDs, puids, clientIP, hardMuted, pb.ChannelType_SESSION)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, roomTokens...)
	}

	if req.Team {
		if len(sessionResp.Teams) == 0 {
			return nil, status.Error(codes.FailedPrecondition, "session has no teams")
		}

		for _, team := range sessionResp.Teams {
			if team == nil || team.TeamID == "" || len(team.UserIDs) == 0 {
				continue
			}

			roomID := sessionID + ":" + team.TeamID
			roomTokens, err := s.createRoomTokens(ctx, roomID, team.UserIDs, puids, clientIP, hardMuted, pb.ChannelType_TEAM)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, roomTokens...)
		}
	}

	if notify {
		notificationSvc := lobby.NotificationService{
			Client:           factory.NewLobbyClient(s.configRepo),
			ConfigRepository: s.configRepo,
			TokenRepository:  s.tokenRepo,
		}

		// Group tokens by user ID
		tokensByUser := make(map[string][]*pb.EOSTokenResponse)
		for _, tokenEntry := range tokens {
			if tokenEntry == nil || tokenEntry.UserId == "" || tokenEntry.VoiceToken == nil {
				continue
			}
			tokensByUser[tokenEntry.UserId] = append(tokensByUser[tokenEntry.UserId], tokenEntry.VoiceToken)
		}

		// Send one notification per user with all their tokens
		for userID, voiceTokens := range tokensByUser {
			if err := s.sendVoiceTokenNotification(ctx, &notificationSvc, userID, voiceTokens); err != nil {
				s.logger.Error("failed to send voice token notification", "userID", userID, "error", err)
			}
		}
	}

	return &pb.AdminSessionTokenResponse{Tokens: tokens}, nil
}

// RevokeToken revokes voice tokens for specified users in a room (admin endpoint)
func (s *EOSVoiceServiceServerImpl) RevokeToken(
	ctx context.Context, req *pb.RevokeTokenRequest,
) (*pb.RevokeTokenResponse, error) {
	s.logger.Info("Start REVOKING")
	// Extract room_id from request (path parameter)
	roomID := req.RoomId
	if roomID == "" {
		s.logger.Error("room_id is required")
		return nil, status.Error(codes.InvalidArgument, "room_id is required")
	}

	// Extract user IDs from request body
	userIDs := req.UserIds
	if len(userIDs) == 0 {
		s.logger.Error("user_ids is required")
		return nil, status.Error(codes.InvalidArgument, "user_ids is required")
	}

	s.logger.Info("revoking voice tokens", "roomID", roomID, "userCount", len(userIDs))

	// Map AccelByte User IDs to Epic PUIDs
	puids, err := s.epicClient.GetEpicPUID(ctx, userIDs)
	if err != nil {
		s.logger.Error("failed to get Epic PUIDs", "error", err)
		return nil, ParseHTTPError(err, "epic_puid")
	}

	// Revoke tokens for each user
	var errors []string
	var notFoundCount int
	for userID, puid := range puids {
		if puid == "" {
			s.logger.Warn("user has no linked Epic account, skipping", "userID", userID)
			continue
		}

		if err := s.epicClient.RemoveParticipant(ctx, roomID, puid); err != nil {
			s.logger.Error("failed to remove participant", "userID", userID, "puid", puid, "error", err)
			errors = append(errors, err.Error())

			// Track if error is 404 (user not in room)
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				notFoundCount++
			}
		} else {
			s.logger.Info("successfully revoked token", "userID", userID, "puid", puid)
		}
	}

	// If there were errors, return appropriate status
	if len(errors) > 0 {
		// If all errors are 404 (user not in room), return NotFound
		if notFoundCount == len(errors) {
			return nil, status.Error(codes.NotFound, "one or more users not found in room")
		}

		// Mixed errors or server errors - return Internal
		return nil, status.Error(codes.Internal, "failed to revoke some tokens")
	}

	// Return empty response for 204 No Content
	return &pb.RevokeTokenResponse{}, nil
}

func (s *EOSVoiceServiceServerImpl) createRoomTokens(
	ctx context.Context,
	roomID string,
	userIDs []string,
	puids map[string]string,
	clientIP string,
	hardMuted bool,
	channelType pb.ChannelType,
) ([]*pb.AdminUserTokenResponse, error) {
	participants := make([]RTCParticipant, 0, len(userIDs))
	filteredUserIDs := make([]string, 0, len(userIDs))

	for _, userID := range userIDs {
		puid := puids[userID]
		if puid == "" {
			s.logger.Warn("user has no linked Epic account", "userID", userID)
			continue
		}

		participants = append(participants, RTCParticipant{
			PUID:      puid,
			ClientIP:  clientIP,
			HardMuted: hardMuted,
		})
		filteredUserIDs = append(filteredUserIDs, userID)
	}

	if len(participants) == 0 {
		return nil, ErrEpicAccountNotLinked.ToGRPCStatus()
	}

	tokenResp, err := s.epicClient.CreateRoomToken(ctx, roomID, participants)
	if err != nil {
		s.logger.Error("failed to create room token", "roomID", roomID, "error", err)
		return nil, ParseHTTPError(err, "epic_rtc")
	}

	if len(tokenResp.Participants) == 0 {
		s.logger.Error("no participant token in response", "roomID", roomID)
		return nil, ErrEpicRTCAPI.ToGRPCStatus()
	}

	tokenByPUID := make(map[string]string, len(tokenResp.Participants))
	for _, participant := range tokenResp.Participants {
		tokenByPUID[participant.PUID] = participant.Token
	}

	responses := make([]*pb.AdminUserTokenResponse, 0, len(filteredUserIDs))
	for _, userID := range filteredUserIDs {
		puid := puids[userID]
		participantToken := tokenByPUID[puid]
		if participantToken == "" {
			s.logger.Error("missing participant token", "roomID", roomID, "userID", userID, "puid", puid)
			return nil, ErrEpicRTCAPI.ToGRPCStatus()
		}

		voiceToken := &pb.EOSTokenResponse{
			ClientBaseUrl: tokenResp.ClientBaseURL,
			Token:         participantToken,
			ChannelType:   channelType,
			RoomId:        roomID,
		}

		responses = append(responses, &pb.AdminUserTokenResponse{
			UserId:     userID,
			VoiceToken: voiceToken,
		})
	}

	return responses, nil
}

func (s *EOSVoiceServiceServerImpl) sendVoiceTokenNotification(
	ctx context.Context,
	notificationSvc *lobby.NotificationService,
	userID string,
	voiceTokens []*pb.EOSTokenResponse,
) error {
	if notificationSvc == nil {
		return status.Error(codes.Internal, "notification service is not configured")
	}

	// Build array of token objects
	tokenPayloads := make([]map[string]string, 0, len(voiceTokens))
	for _, voiceToken := range voiceTokens {
		tokenPayloads = append(tokenPayloads, map[string]string{
			"ClientBaseUrl": voiceToken.ClientBaseUrl,
			"Token":         voiceToken.Token,
			"ChannelType":   voiceToken.ChannelType.String(),
			"RoomId":        voiceToken.RoomId,
		})
	}

	payload := map[string]interface{}{
		"Tokens": tokenPayloads,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	message := string(payloadBytes)
	topic := "EOS_VOICE"

	params := notification.NewSendSpecificUserFreeformNotificationV1AdminParamsWithContext(ctx)
	params.Namespace = common.GetEnv("AB_NAMESPACE", "accelbyte")
	params.UserID = userID
	params.Body = &lobbyclientmodels.ModelFreeFormNotificationRequestV1{
		Message:   &message,
		TopicName: &topic,
	}

	return notificationSvc.SendSpecificUserFreeformNotificationV1AdminShort(params)
}
