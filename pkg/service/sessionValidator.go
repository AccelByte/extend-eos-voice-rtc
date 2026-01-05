// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"context"
	"log/slog"

	"extend-eos-voice-rtc/pkg/common"

	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/factory"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/repository"
	"github.com/AccelByte/accelbyte-go-sdk/services-api/pkg/service/session"
	"github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclient/game_session"
	"github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclient/party"
	sessionclientmodels "github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclientmodels"
)

type sessionService interface {
	GetGameSessionShort(input *game_session.GetGameSessionParams) (*sessionclientmodels.ApimodelsGameSessionResponse, error)
	AdminQueryPartiesShort(input *party.AdminQueryPartiesParams) (*sessionclientmodels.ApimodelsPartyQueryResponse, error)
}

type sessionServiceAdapter struct {
	gameSession *session.GameSessionService
	party       *session.PartyService
}

func (s *sessionServiceAdapter) GetGameSessionShort(input *game_session.GetGameSessionParams) (*sessionclientmodels.ApimodelsGameSessionResponse, error) {
	return s.gameSession.GetGameSessionShort(input)
}

func (s *sessionServiceAdapter) AdminQueryPartiesShort(input *party.AdminQueryPartiesParams) (*sessionclientmodels.ApimodelsPartyQueryResponse, error) {
	return s.party.AdminQueryPartiesShort(input)
}

// SessionValidator handles validation of user membership in sessions and parties
type SessionValidator struct {
	sessionService sessionService
	configRepo     repository.ConfigRepository
	tokenRepo      repository.TokenRepository
	epicClient     EpicClientInterface
	logger         *slog.Logger
}

// NewSessionValidator creates a new session validator
func NewSessionValidator(
	configRepo repository.ConfigRepository,
	tokenRepo repository.TokenRepository,
	epicClient EpicClientInterface,
	logger *slog.Logger,
) *SessionValidator {
	sessionClient := factory.NewSessionClient(configRepo)
	return &SessionValidator{
		sessionService: &sessionServiceAdapter{
			gameSession: &session.GameSessionService{
				Client:           sessionClient,
				ConfigRepository: configRepo,
				TokenRepository:  tokenRepo,
			},
			party: &session.PartyService{
				Client:           sessionClient,
				ConfigRepository: configRepo,
				TokenRepository:  tokenRepo,
			},
		},
		configRepo: configRepo,
		tokenRepo:  tokenRepo,
		epicClient: epicClient,
		logger:     logger,
	}
}

// ValidateUserInParty validates that a user is a member of the specified party
func (v *SessionValidator) ValidateUserInParty(ctx context.Context, userID string, partyID string) error {
	namespace := common.GetEnv("AB_NAMESPACE", "accelbyte")

	params := &party.AdminQueryPartiesParams{
		Namespace: namespace,
		PartyID:   &partyID,
	}

	partyResp, err := v.sessionService.AdminQueryPartiesShort(params)
	if err != nil {
		v.logger.Error("failed to get party details", "partyID", partyID, "error", err)
		return ParseHTTPError(err, "party")
	}

	if partyResp == nil {
		v.logger.Error("party not found", "partyID", partyID)
		return ErrPartyNotFound.ToGRPCStatus()
	}

	if len(partyResp.Data) == 0 {
		v.logger.Error("party not found", "partyID", partyID)
		return ErrPartyNotFound.ToGRPCStatus()
	}

	// Check if user is a member of the party
	for _, member := range partyResp.Data[0].Members {
		if member.ID != nil && *member.ID == userID {
			return nil
		}
	}

	v.logger.Warn("user not in party", "userID", userID, "partyID", partyID)
	return ErrUserNotInParty.ToGRPCStatus()
}

// ValidateUserInSession validates that a user is a member of the specified session
func (v *SessionValidator) ValidateUserInSession(ctx context.Context, userID string, sessionID string) error {
	sessionResp, err := v.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Check if user is a member of the session
	for _, member := range sessionResp.Members {
		if member.ID != nil && *member.ID == userID {
			return nil
		}
	}

	v.logger.Warn("user not in session", "userID", userID, "sessionID", sessionID)
	return ErrUserNotInSession.ToGRPCStatus()
}

// GetSession fetches session details for the given session ID
func (v *SessionValidator) GetSession(ctx context.Context, sessionID string) (*sessionclientmodels.ApimodelsGameSessionResponse, error) {
	namespace := common.GetEnv("AB_NAMESPACE", "accelbyte")

	params := &game_session.GetGameSessionParams{
		Namespace: namespace,
		SessionID: sessionID,
	}

	sessionResp, err := v.sessionService.GetGameSessionShort(params)
	if err != nil {
		v.logger.Error("failed to get session details", "sessionID", sessionID, "error", err)
		return nil, ParseHTTPError(err, "session")
	}

	if sessionResp == nil {
		v.logger.Error("session not found", "sessionID", sessionID)
		return nil, ErrSessionNotFound.ToGRPCStatus()
	}

	return sessionResp, nil
}

// GetUserTeamInSession extracts the team ID from user's session membership
func (v *SessionValidator) GetUserTeamInSession(ctx context.Context, userID string, sessionID string) (string, error) {
	namespace := common.GetEnv("AB_NAMESPACE", "accelbyte")

	params := &game_session.GetGameSessionParams{
		Namespace: namespace,
		SessionID: sessionID,
	}

	sessionResp, err := v.sessionService.GetGameSessionShort(params)
	if err != nil {
		v.logger.Error("failed to get session details", "sessionID", sessionID, "error", err)
		return "", ParseHTTPError(err, "session")
	}

	if sessionResp == nil {
		v.logger.Error("session not found", "sessionID", sessionID)
		return "", ErrSessionNotFound.ToGRPCStatus()
	}

	// Find user in members and get their team ID
	for _, member := range sessionResp.Members {
		if member.ID != nil && *member.ID == userID {
			// Check if Teams array exists and user is assigned to a team
			if len(sessionResp.Teams) > 0 {
				// Find which team the user belongs to
				for _, team := range sessionResp.Teams {
					if team.UserIDs != nil {
						for _, teamMemberID := range team.UserIDs {
							if teamMemberID == userID {
								if team.TeamID != "" {
									return team.TeamID, nil
								}
							}
						}
					}
				}
			}

			// User is in session but not assigned to any team
			v.logger.Warn("user in session but not in team", "userID", userID, "sessionID", sessionID)
			return "", ErrUserNotInTeam.ToGRPCStatus()
		}
	}

	// User not found in session
	v.logger.Warn("user not in session", "userID", userID, "sessionID", sessionID)
	return "", ErrUserNotInSession.ToGRPCStatus()
}

// GetEpicPUID maps AccelByte User ID to Epic PUID via external account mapping
func (v *SessionValidator) GetEpicPUID(ctx context.Context, userID string) (string, error) {
	// Call Epic Connect API to get PUID
	puids, err := v.epicClient.GetEpicPUID(ctx, []string{userID})
	if err != nil {
		v.logger.Error("failed to query Epic external accounts", "userID", userID, "error", err)
		return "", ParseHTTPError(err, "epic_puid")
	}

	// Check if the user has a linked Epic account
	puid, exists := puids[userID]
	if !exists || puid == "" {
		v.logger.Warn("Epic account not linked", "userID", userID)
		return "", ErrEpicAccountNotLinked.ToGRPCStatus()
	}

	return puid, nil
}
