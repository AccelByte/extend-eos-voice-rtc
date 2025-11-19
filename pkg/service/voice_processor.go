package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"extend-eos-voice-rtc/pkg/voiceclient"

	lobbyNotification "github.com/AccelByte/accelbyte-go-sdk/lobby-sdk/pkg/lobbyclient/notification"
	lobbyModels "github.com/AccelByte/accelbyte-go-sdk/lobby-sdk/pkg/lobbyclientmodels"
	"github.com/sirupsen/logrus"
)

const (
	externalAccountBatchSize = 16
	identityProviderOpenID   = "openid"
	notificationTypeTeam     = "team-session"
	notificationTypeGame     = "game-session"
	notificationTypeParty    = "party-session"
)

type (
	voiceOrchestrator interface {
		HandleGameSessionCreated(ctx context.Context, sessionID string, snapshot string) error
		HandleGameSessionEnded(ctx context.Context, sessionID string, snapshot string) error
		HandlePartyCreated(ctx context.Context, partyID string, snapshot string, userIDs []string) error
		HandlePartyMembersJoined(ctx context.Context, partyID string, userIDs []string) error
		HandlePartyMembersRemoved(ctx context.Context, partyID string, userIDs []string) error
	}

	voiceRoomClient interface {
		CreateRoomTokens(ctx context.Context, roomID string, participants []voiceclient.Participant) (*voiceclient.CreateRoomTokenResponse, error)
		RemoveParticipant(ctx context.Context, roomID, productUserID string) error
		QueryExternalAccounts(ctx context.Context, identityProviderID string, accountIDs []string) (map[string]string, error)
	}

	lobbyNotifier interface {
		SendSpecificUserFreeformNotificationV1AdminShort(params *lobbyNotification.SendSpecificUserFreeformNotificationV1AdminParams) error
	}
)

// VoiceEventProcessor orchestrates EOS voice rooms based on session events.
type VoiceEventProcessor struct {
	namespace           string
	topicName           string
	voiceClient         voiceRoomClient
	notificationService lobbyNotifier
	logger              *logrus.Entry
	enableTeamVoice     bool
	enableGameVoice     bool
}

var _ voiceOrchestrator = (*VoiceEventProcessor)(nil)

// VoiceProcessorConfig bundles dependencies for VoiceEventProcessor initialization.
type VoiceProcessorConfig struct {
	Namespace           string
	NotificationTopic   string
	VoiceClient         voiceRoomClient
	NotificationService lobbyNotifier
	Logger              *logrus.Entry
	EnableTeamVoice     bool
	EnableGameVoice     bool
}

// NewVoiceEventProcessor builds a VoiceEventProcessor with sane defaults.
func NewVoiceEventProcessor(cfg VoiceProcessorConfig) (*VoiceEventProcessor, error) {
	if cfg.VoiceClient == nil {
		return nil, errors.New("voice processor: voice client is required")
	}
	if cfg.NotificationService == nil {
		return nil, errors.New("voice processor: notification service is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.New().WithField("component", "voice-processor")
	}
	topic := cfg.NotificationTopic
	if topic == "" {
		topic = "EOS_VOICE"
	}

	return &VoiceEventProcessor{
		namespace:           cfg.Namespace,
		topicName:           topic,
		voiceClient:         cfg.VoiceClient,
		notificationService: cfg.NotificationService,
		logger:              logger,
		enableTeamVoice:     cfg.EnableTeamVoice,
		enableGameVoice:     cfg.EnableGameVoice,
	}, nil
}

// HandleGameSessionCreated generates fresh voice tokens for every active player in the session.
func (p *VoiceEventProcessor) HandleGameSessionCreated(ctx context.Context, sessionID string, encodedSnapshot string) error {
	if sessionID == "" {
		return errors.New("handle game session created: session ID is required")
	}

	if !p.enableTeamVoice && !p.enableGameVoice {
		p.logger.WithField("sessionId", sessionID).Debug("handle game session created: game voice disabled")
		return nil
	}

	snapshot, err := decodeGameSessionSnapshot(encodedSnapshot)
	if err != nil {
		p.logger.WithError(err).WithField("sessionId", sessionID).Warn("handle game session created: snapshot decode failed, skipping")
		return nil
	}
	if snapshot == nil {
		p.logger.WithField("sessionId", sessionID).Warn("handle game session created: empty snapshot membership, skipping")
		return nil
	}

	if p.enableTeamVoice {
		roomMembers := buildGameSessionRoomMembershipsFromSnapshot(snapshot)
		if len(roomMembers) == 0 {
			p.logger.WithField("sessionId", sessionID).Warn("handle game session created: empty snapshot membership, skipping")
		} else {
			for roomID, members := range roomMembers {
				if err := p.createParticipants(ctx, sessionID, roomID, members, notificationTypeTeam); err != nil {
					p.logger.WithError(err).WithFields(logrus.Fields{
						"sessionId": sessionID,
						"roomId":    roomID,
					}).Error("handle game session created: failed to create participants")
				}
			}
			p.logger.WithFields(logrus.Fields{
				"sessionId": sessionID,
				"roomCount": len(roomMembers),
				"participantCount": func() int {
					count := 0
					for _, members := range roomMembers {
						count += len(members)
					}
					return count
				}(),
			}).Debug("handle game session created: snapshot decoded")
		}
	}

	if p.enableGameVoice {
		if len(snapshot.Teams) == 0 && p.enableTeamVoice {
			p.logger.WithField("sessionId", sessionID).Debug("handle game session created: skipping session-wide room because snapshot has no teams")
		} else {
			allMembers := buildGameSessionMembersFromSnapshot(snapshot)
			if len(allMembers) == 0 {
				p.logger.WithField("sessionId", sessionID).Warn("handle game session created: no active members for session-wide room")
			} else {
				roomID := defaultGameSessionRoomID(snapshot.ID)
				if err := p.createParticipants(ctx, sessionID, roomID, allMembers, notificationTypeGame); err != nil {
					p.logger.WithError(err).WithFields(logrus.Fields{
						"sessionId": sessionID,
						"roomId":    roomID,
					}).Error("handle game session created: failed to create session-wide participants")
				} else {
					p.logger.WithFields(logrus.Fields{
						"sessionId":        sessionID,
						"roomId":           roomID,
						"participantCount": len(allMembers),
					}).Debug("handle game session created: processed session-wide members")
				}
			}
		}
	}

	return nil
}

// HandleGameSessionEnded revokes every participant token for the supplied session.
func (p *VoiceEventProcessor) HandleGameSessionEnded(ctx context.Context, sessionID string, encodedSnapshot string) error {
	if sessionID == "" {
		return errors.New("handle game session ended: session ID is required")
	}

	if !p.enableTeamVoice && !p.enableGameVoice {
		p.logger.WithField("sessionId", sessionID).Debug("handle game session ended: game voice disabled")
		return nil
	}

	snapshot, err := decodeGameSessionSnapshot(encodedSnapshot)
	if err != nil {
		p.logger.WithError(err).WithField("sessionId", sessionID).Warn("handle game session ended: snapshot decode failed, skipping")
		return nil
	}
	if snapshot == nil {
		p.logger.WithField("sessionId", sessionID).Warn("handle game session ended: empty snapshot membership, skipping")
		return nil
	}

	if p.enableTeamVoice {
		roomMembers := buildGameSessionRoomMembershipsFromSnapshot(snapshot)
		for roomID, members := range roomMembers {
			if err := p.removeParticipants(ctx, sessionID, roomID, members); err != nil {
				p.logger.WithError(err).WithFields(logrus.Fields{
					"sessionId": sessionID,
					"roomId":    roomID,
				}).Warn("handle game session ended: failed to revoke participants")
			}
		}
	}

	if p.enableGameVoice {
		if len(snapshot.Teams) == 0 && p.enableTeamVoice {
			p.logger.WithField("sessionId", sessionID).Debug("handle game session ended: skipping session-wide teardown because no teams were present")
		} else {
			allMembers := buildGameSessionMembersFromSnapshot(snapshot)
			if len(allMembers) == 0 {
				p.logger.WithField("sessionId", sessionID).Debug("handle game session ended: no session-wide members to revoke")
			} else {
				roomID := defaultGameSessionRoomID(snapshot.ID)
				if err := p.removeParticipants(ctx, sessionID, roomID, allMembers); err != nil {
					p.logger.WithError(err).WithFields(logrus.Fields{
						"sessionId": sessionID,
						"roomId":    roomID,
					}).Warn("handle game session ended: failed to revoke session-wide participants")
				}
			}
		}
	}

	return nil
}

// HandlePartyCreated ensures an initial voice token exists for each active member.
func (p *VoiceEventProcessor) HandlePartyCreated(ctx context.Context, partyID string, snapshot string, userIDs []string) error {
	if partyID == "" {
		return errors.New("handle party created: party ID is required")
	}

	members := uniqueStrings(userIDs)
	if len(members) == 0 {
		derived, err := parsePartySnapshotMembers(snapshot)
		if err != nil {
			p.logger.WithError(err).WithField("partyId", partyID).Warn("handle party created: snapshot decode failed, skipping")
			return nil
		}
		members = uniqueStrings(derived)
		if len(members) == 0 {
			p.logger.WithField("partyId", partyID).Warn("handle party created: unable to resolve user IDs, skipping")
			return nil
		}
		p.logger.WithField("partyId", partyID).Debug("handle party created: derived user IDs from snapshot")
	}

	roomID := partyVoiceRoomID(partyID)
	if err := p.createParticipants(ctx, partyID, roomID, members, notificationTypeParty); err != nil {
		return fmt.Errorf("handle party created: create participants: %w", err)
	}
	p.logger.WithFields(logrus.Fields{
		"partyId":          partyID,
		"participantCount": len(members),
	}).Debug("handle party created: processed user IDs")

	return nil
}

// HandlePartyMembersJoined issues new party tokens for the provided members.
func (p *VoiceEventProcessor) HandlePartyMembersJoined(ctx context.Context, partyID string, userIDs []string) error {
	if partyID == "" {
		return errors.New("handle party members joined: party ID is required")
	}

	roomID := partyVoiceRoomID(partyID)
	return p.createParticipants(ctx, partyID, roomID, userIDs, notificationTypeParty)
}

// HandlePartyMembersRemoved revokes party tokens for the provided members.
func (p *VoiceEventProcessor) HandlePartyMembersRemoved(ctx context.Context, partyID string, userIDs []string) error {
	if partyID == "" {
		return errors.New("handle party members removed: party ID is required")
	}
	roomID := partyVoiceRoomID(partyID)
	return p.removeParticipants(ctx, partyID, roomID, userIDs)
}

func (p *VoiceEventProcessor) createParticipants(ctx context.Context, sessionID, roomID string, userIDs []string, notificationType string) error {
	if len(userIDs) == 0 {
		p.logger.WithFields(logrus.Fields{
			"sessionId": sessionID,
			"roomId":    roomID,
		}).Debug("create participants: no user IDs provided")
		return nil
	}

	if notificationType == "" {
		notificationType = notificationTypeTeam
	}

	p.logger.WithFields(logrus.Fields{
		"sessionId":          sessionID,
		"roomId":             roomID,
		"notificationType":   notificationType,
		"requestedUserCount": len(userIDs),
	}).Debug("create participants: resolving EOS IDs")

	eosMap, err := p.fetchEOSIDs(ctx, userIDs)
	if err != nil {
		return err
	}

	participants := make([]voiceclient.Participant, 0, len(userIDs))
	abToEos := make(map[string]string, len(userIDs))
	missing := 0
	for _, uid := range userIDs {
		eosID := eosMap[uid]
		if eosID == "" {
			p.logger.WithFields(logrus.Fields{
				"sessionId": sessionID,
				"roomId":    roomID,
				"userId":    uid,
			}).Warn("create participants: missing EOS ID, skipping")
			missing++
			continue
		}
		participants = append(participants, voiceclient.Participant{
			ProductUserID: eosID,
			HardMuted:     false,
		})
		abToEos[uid] = eosID
	}
	p.logger.WithFields(logrus.Fields{
		"sessionId":            sessionID,
		"roomId":               roomID,
		"notificationType":     notificationType,
		"resolvedParticipants": len(participants),
		"skippedUsers":         missing,
	}).Debug("create participants: resolved participant identities")

	if len(participants) == 0 {
		p.logger.WithFields(logrus.Fields{
			"sessionId":        sessionID,
			"roomId":           roomID,
			"notificationType": notificationType,
		}).Warn("create participants: no participants resolved after EOS lookup")
		return nil
	}

	p.logger.WithFields(logrus.Fields{
		"sessionId":        sessionID,
		"roomId":           roomID,
		"notificationType": notificationType,
		"participantCount": len(participants),
	}).Debug("create participants: requesting voice tokens")
	resp, err := p.voiceClient.CreateRoomTokens(ctx, roomID, participants)
	if err != nil {
		return err
	}
	p.logger.WithFields(logrus.Fields{
		"sessionId":             sessionID,
		"roomId":                resp.RoomID,
		"notificationType":      notificationType,
		"requestedParticipants": len(participants),
		"tokenCount":            len(resp.Participants),
		"clientBaseUrl":         resp.ClientBaseURL,
	}).Debug("create participants: created voice tokens")

	// Map EOS -> AccelByte user ID for notifications.
	eosToUserID := invertMap(abToEos)

	for _, participant := range resp.Participants {
		userID := eosToUserID[participant.ProductUserID]
		if userID == "" {
			continue
		}
		notification := map[string]string{
			"type":          notificationType,
			"sessionId":     sessionID,
			"roomId":        resp.RoomID,
			"clientBaseUrl": resp.ClientBaseURL,
			"token":         participant.Token,
		}
		if notificationType == notificationTypeTeam {
			if parts := strings.SplitN(resp.RoomID, ":", 2); len(parts) == 2 {
				notification["teamId"] = parts[1]
			}
		}
		message, err := json.Marshal(notification)
		if err != nil {
			p.logger.WithError(err).WithField("userId", userID).Warn("create participants: marshal notification failed")
			continue
		}
		if err := p.sendFreeformNotification(ctx, userID, string(message)); err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"userId": userID,
			}).Warn("create participants: send notification failed")
			continue
		}
		p.logger.WithFields(logrus.Fields{
			"userId":           userID,
			"sessionId":        sessionID,
			"roomId":           resp.RoomID,
			"notificationType": notificationType,
			"tokenLength":      len(participant.Token),
			"clientBaseUrl":    resp.ClientBaseURL,
		}).Debug("create participants: notification dispatched")
	}

	return nil
}

func (p *VoiceEventProcessor) removeParticipants(ctx context.Context, sessionID, roomID string, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}

	eosMap, err := p.fetchEOSIDs(ctx, userIDs)
	if err != nil {
		return err
	}

	for _, uid := range userIDs {
		eosID := eosMap[uid]
		if eosID == "" {
			continue
		}
		if err := p.voiceClient.RemoveParticipant(ctx, roomID, eosID); err != nil {
			p.logger.WithError(err).WithFields(logrus.Fields{
				"sessionId": sessionID,
				"roomId":    roomID,
				"userId":    uid,
			}).Warn("remove participants: voice removal failed")
		}
	}

	return nil
}

func (p *VoiceEventProcessor) sendFreeformNotification(ctx context.Context, userID, message string) error {
	body := &lobbyModels.ModelFreeFormNotificationRequestV1{
		Message:   &message,
		TopicName: &p.topicName,
	}
	params := lobbyNotification.NewSendSpecificUserFreeformNotificationV1AdminParamsWithContext(ctx)
	params.Namespace = p.namespace
	params.UserID = userID
	params.Body = body

	return p.notificationService.SendSpecificUserFreeformNotificationV1AdminShort(params)
}

func (p *VoiceEventProcessor) fetchEOSIDs(ctx context.Context, userIDs []string) (map[string]string, error) {
	result := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	unique := uniqueStrings(userIDs)
	batches := batchStrings(unique, externalAccountBatchSize)

	for _, batch := range batches {
		lookup, err := p.voiceClient.QueryExternalAccounts(ctx, identityProviderOpenID, batch)
		if err != nil {
			return nil, fmt.Errorf("fetch EOS IDs: %w", err)
		}
		for userID, eosID := range lookup {
			if strings.TrimSpace(eosID) == "" {
				continue
			}
			result[userID] = eosID
		}
	}

	return result, nil
}

func buildGameSessionRoomMembershipsFromSnapshot(snapshot *gameSessionSnapshot) map[string][]string {
	result := map[string][]string{}
	if snapshot == nil || snapshot.ID == "" {
		return result
	}

	memberLookup := map[string]gameSessionMember{}
	for _, member := range snapshot.Members {
		if member.ID == "" {
			continue
		}
		memberLookup[member.ID] = member
	}

	if len(snapshot.Teams) == 0 {
		var active []string
		for _, member := range snapshot.Members {
			if member.ID == "" {
				continue
			}
			if isActiveStatus(&member.Status, &member.StatusV2) {
				active = append(active, member.ID)
			}
		}
		roomID := defaultGameSessionRoomID(snapshot.ID)
		result[roomID] = active
		return result
	}

	for idx, team := range snapshot.Teams {
		teamID := strings.TrimSpace(team.TeamID)
		if teamID == "" {
			teamID = fmt.Sprintf("%d", idx)
		}
		roomID := fmt.Sprintf("%s:%s", snapshot.ID, teamID)
		var users []string
		for _, uid := range team.UserIDs {
			member := memberLookup[uid]
			if uid == "" {
				continue
			}
			if isActiveStatus(&member.Status, &member.StatusV2) {
				users = append(users, uid)
			}
		}
		result[roomID] = users
	}

	return result
}

func buildGameSessionMembersFromSnapshot(snapshot *gameSessionSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var members []string
	for _, member := range snapshot.Members {
		if member.ID == "" {
			continue
		}
		if isActiveStatus(&member.Status, &member.StatusV2) {
			members = append(members, member.ID)
		}
	}
	return uniqueStrings(members)
}

func isActiveStatus(statusPtr, statusV2Ptr *string) bool {
	var status, statusV2 string
	if statusPtr != nil {
		status = strings.ToUpper(strings.TrimSpace(*statusPtr))
	}
	if statusV2Ptr != nil {
		statusV2 = strings.ToUpper(strings.TrimSpace(*statusV2Ptr))
	}

	switch {
	case status == "JOINED" || status == "CONNECTED":
		return true
	case statusV2 == "JOINED" || statusV2 == "CONNECTED":
		return true
	default:
		return false
	}
}

func partyVoiceRoomID(sessionID string) string {
	return fmt.Sprintf("%s:Voice", sessionID)
}

func defaultGameSessionRoomID(sessionID string) string {
	return fmt.Sprintf("%s:0", sessionID)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var result []string
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func batchStrings(values []string, size int) [][]string {
	if size <= 0 {
		size = 1
	}
	var batches [][]string
	for len(values) > size {
		batch := append([]string(nil), values[:size]...)
		batches = append(batches, batch)
		values = values[size:]
	}
	if len(values) > 0 {
		batches = append(batches, append([]string(nil), values...))
	}
	return batches
}

func invertMap(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if v == "" {
			continue
		}
		result[v] = k
	}
	return result
}

func (p *VoiceEventProcessor) parseGameSessionSnapshot(encoded string) (map[string][]string, error) {
	snapshot, err := decodeGameSessionSnapshot(encoded)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, nil
	}
	return buildGameSessionRoomMembershipsFromSnapshot(snapshot), nil
}

func decodeGameSessionSnapshot(encoded string) (*gameSessionSnapshot, error) {
	data, err := decodeSnapshotEnvelope(encoded)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var envelope gameSessionSnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if envelope.Payload.ID == "" {
		return nil, errors.New("snapshot missing session ID")
	}
	return &envelope.Payload, nil
}

func parsePartySnapshotMembers(encoded string) ([]string, error) {
	data, err := decodeSnapshotEnvelope(encoded)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var envelope gameSessionSnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal party snapshot: %w", err)
	}
	if envelope.Payload.ID == "" {
		return nil, errors.New("party snapshot missing session ID")
	}
	var members []string
	for _, member := range envelope.Payload.Members {
		if member.ID == "" {
			continue
		}
		if isActiveStatus(&member.Status, &member.StatusV2) {
			members = append(members, member.ID)
		}
	}
	return members, nil
}

func decodeSnapshotEnvelope(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		return []byte(trimmed), nil
	}
	data, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(trimmed)
		if err != nil {
			return nil, fmt.Errorf("decode snapshot: %w", err)
		}
	}
	return data, nil
}

type gameSessionSnapshotEnvelope struct {
	Payload gameSessionSnapshot `json:"payload"`
}

type gameSessionSnapshot struct {
	ID      string              `json:"ID"`
	Members []gameSessionMember `json:"Members"`
	Teams   []gameSessionTeam   `json:"Teams"`
}

type gameSessionMember struct {
	ID       string `json:"ID"`
	Status   string `json:"Status"`
	StatusV2 string `json:"StatusV2"`
}

type gameSessionTeam struct {
	TeamID  string   `json:"teamID"`
	UserIDs []string `json:"userIDs"`
}
