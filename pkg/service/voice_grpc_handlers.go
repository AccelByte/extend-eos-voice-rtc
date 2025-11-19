package service

import (
	"context"
	"encoding/json"
	"strings"

	sessionpb "extend-eos-voice-rtc/pkg/pb/accelbyte-asyncapi/session/session/v1"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Game session event handlers.
type GameSessionCreatedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionCreatedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

type GameSessionJoinedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionJoinedEventServiceServer
	logger *logrus.Entry
}

type GameSessionMembersChangedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionMembersChangedEventServiceServer
	logger *logrus.Entry
}

type GameSessionKickedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionKickedEventServiceServer
	logger *logrus.Entry
}

type GameSessionEndedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionEndedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

// Party event handlers.
type PartyCreatedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyCreatedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

type PartyJoinedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyJoinedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

type PartyMembersChangedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyMembersChangedEventServiceServer
	logger *logrus.Entry
}

type PartyLeaveHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyLeaveEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

type PartyKickedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyKickedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

type PartyDeletedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyDeletedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

type PartyRejoinedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyRejoinedEventServiceServer
	orchestrator voiceOrchestrator
	logger       *logrus.Entry
}

func NewGameSessionCreatedServer(o voiceOrchestrator, logger *logrus.Entry) *GameSessionCreatedHandler {
	return &GameSessionCreatedHandler{
		UnimplementedMpv2SessionHistoryGameSessionCreatedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionCreatedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewGameSessionJoinedServer(_ voiceOrchestrator, logger *logrus.Entry) *GameSessionJoinedHandler {
	return &GameSessionJoinedHandler{
		UnimplementedMpv2SessionHistoryGameSessionJoinedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionJoinedEventServiceServer{},
		logger: logger,
	}
}

func NewGameSessionMembersChangedServer(_ voiceOrchestrator, logger *logrus.Entry) *GameSessionMembersChangedHandler {
	return &GameSessionMembersChangedHandler{
		UnimplementedMpv2SessionHistoryGameSessionMembersChangedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionMembersChangedEventServiceServer{},
		logger: logger,
	}
}

func NewGameSessionKickedServer(_ voiceOrchestrator, logger *logrus.Entry) *GameSessionKickedHandler {
	return &GameSessionKickedHandler{
		UnimplementedMpv2SessionHistoryGameSessionKickedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionKickedEventServiceServer{},
		logger: logger,
	}
}

func NewGameSessionEndedServer(o voiceOrchestrator, logger *logrus.Entry) *GameSessionEndedHandler {
	return &GameSessionEndedHandler{
		UnimplementedMpv2SessionHistoryGameSessionEndedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionEndedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewPartyCreatedServer(o voiceOrchestrator, logger *logrus.Entry) *PartyCreatedHandler {
	return &PartyCreatedHandler{
		UnimplementedMpv2SessionHistoryPartyCreatedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyCreatedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewPartyJoinedServer(o voiceOrchestrator, logger *logrus.Entry) *PartyJoinedHandler {
	return &PartyJoinedHandler{
		UnimplementedMpv2SessionHistoryPartyJoinedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyJoinedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewPartyMembersChangedServer(_ voiceOrchestrator, logger *logrus.Entry) *PartyMembersChangedHandler {
	return &PartyMembersChangedHandler{
		UnimplementedMpv2SessionHistoryPartyMembersChangedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyMembersChangedEventServiceServer{},
		logger: logger,
	}
}

func NewPartyLeaveServer(o voiceOrchestrator, logger *logrus.Entry) *PartyLeaveHandler {
	return &PartyLeaveHandler{
		UnimplementedMpv2SessionHistoryPartyLeaveEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyLeaveEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewPartyKickedServer(o voiceOrchestrator, logger *logrus.Entry) *PartyKickedHandler {
	return &PartyKickedHandler{
		UnimplementedMpv2SessionHistoryPartyKickedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyKickedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewPartyDeletedServer(o voiceOrchestrator, logger *logrus.Entry) *PartyDeletedHandler {
	return &PartyDeletedHandler{
		UnimplementedMpv2SessionHistoryPartyDeletedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyDeletedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func NewPartyRejoinedServer(o voiceOrchestrator, logger *logrus.Entry) *PartyRejoinedHandler {
	return &PartyRejoinedHandler{
		UnimplementedMpv2SessionHistoryPartyRejoinedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyRejoinedEventServiceServer{},
		orchestrator: o,
		logger:       logger,
	}
}

func (h *GameSessionCreatedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionCreatedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "game session created", event.GetSessionId(), event.GetPayload(), "game session created handler: received payload")
	if err := h.orchestrator.HandleGameSessionCreated(ctx, event.GetSessionId(), event.GetPayload().GetMessage()); err != nil {
		return nil, status.Errorf(codes.Internal, "handle game session created failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *GameSessionJoinedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionJoinedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "game session joined", event.GetSessionId(), event.GetPayload(), "game session joined handler: received payload")
	return &emptypb.Empty{}, nil
}

func (h *GameSessionMembersChangedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionMembersChangedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "game session members changed", event.GetSessionId(), event.GetPayload(), "game session members changed handler: received payload")
	return &emptypb.Empty{}, nil
}

func (h *GameSessionKickedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionKickedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "game session kicked", event.GetSessionId(), event.GetPayload(), "game session kicked handler: received payload")
	return &emptypb.Empty{}, nil
}

func (h *GameSessionEndedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionEndedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "game session ended", event.GetSessionId(), event.GetPayload(), "game session ended handler: received payload")
	payload := ""
	if event.GetPayload() != nil {
		payload = event.GetPayload().GetMessage()
	}
	if err := h.orchestrator.HandleGameSessionEnded(ctx, event.GetSessionId(), payload); err != nil {
		return nil, status.Errorf(codes.Internal, "handle game session ended failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyCreatedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyCreatedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party created", event.GetSessionId(), event.GetPayload(), "party created handler: received payload")
	payload := event.GetPayload()
	userIDs := userIDsFromPayload(payload)
	snapshot := ""
	if payload != nil {
		snapshot = payload.GetMessage()
	}
	if len(userIDs) == 0 && snapshot == "" {
		logNotificationPayload(h.logger, logrus.WarnLevel, "party created", event.GetSessionId(), payload, "party created handler: payload missing user IDs")
	}
	if err := h.orchestrator.HandlePartyCreated(ctx, event.GetSessionId(), snapshot, userIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party created failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyJoinedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyJoinedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party joined", event.GetSessionId(), event.GetPayload(), "party joined handler: received payload")
	payload := event.GetPayload()
	userIDs := userIDsFromPayload(payload)
	if len(userIDs) == 0 {
		userIDs = partyUserIDsFromSnapshot(h.logger, "party joined handler", event.GetSessionId(), payload)
	}
	if err := h.orchestrator.HandlePartyMembersJoined(ctx, event.GetSessionId(), userIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party joined failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyMembersChangedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyMembersChangedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party members changed", event.GetSessionId(), event.GetPayload(), "party members changed handler: received payload")
	return &emptypb.Empty{}, nil
}

func (h *PartyLeaveHandler) OnMessage(ctx context.Context, event *sessionpb.PartyLeaveEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party leave", event.GetSessionId(), event.GetPayload(), "party leave handler: received payload")
	payload := event.GetPayload()
	userIDs := userIDsFromPayload(payload)
	if len(userIDs) == 0 {
		userIDs = partyUserIDsFromSnapshot(h.logger, "party leave handler", event.GetSessionId(), payload)
	}
	if err := h.orchestrator.HandlePartyMembersRemoved(ctx, event.GetSessionId(), userIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party leave failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyKickedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyKickedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party kicked", event.GetSessionId(), event.GetPayload(), "party kicked handler: received payload")
	payload := event.GetPayload()
	userIDs := userIDsFromPayload(payload)
	if len(userIDs) == 0 {
		userIDs = partyUserIDsFromSnapshot(h.logger, "party kicked handler", event.GetSessionId(), payload)
	}
	if err := h.orchestrator.HandlePartyMembersRemoved(ctx, event.GetSessionId(), userIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party kicked failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyDeletedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyDeletedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party deleted", event.GetSessionId(), event.GetPayload(), "party deleted handler: received payload")
	payload := event.GetPayload()
	userIDs := userIDsFromPayload(payload)
	if len(userIDs) == 0 {
		userIDs = partyMembersFromSnapshot(h.logger, "party deleted handler", event.GetSessionId(), payload)
	}
	if err := h.orchestrator.HandlePartyMembersRemoved(ctx, event.GetSessionId(), userIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party deleted failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyRejoinedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyRejoinedEvent) (*emptypb.Empty, error) {
	logNotificationPayload(h.logger, logrus.DebugLevel, "party rejoined", event.GetSessionId(), event.GetPayload(), "party rejoined handler: received payload")
	payload := event.GetPayload()
	userIDs := userIDsFromPayload(payload)
	if len(userIDs) == 0 {
		userIDs = partyUserIDsFromSnapshot(h.logger, "party rejoined handler", event.GetSessionId(), payload)
	}
	if err := h.orchestrator.HandlePartyMembersJoined(ctx, event.GetSessionId(), userIDs); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party rejoined failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func userIDsFromPayload(payload *sessionpb.NotificationPayload) []string {
	if payload == nil {
		return nil
	}
	if len(payload.GetUserIds()) == 0 {
		return nil
	}
	return append([]string(nil), payload.GetUserIds()...)
}

func partyUserIDsFromSnapshot(logger *logrus.Entry, eventLabel, sessionID string, payload *sessionpb.NotificationPayload) []string {
	if payload == nil {
		return nil
	}
	message := strings.TrimSpace(payload.GetMessage())
	if message == "" {
		return nil
	}
	requester, err := requesterUserIDFromSnapshot(message)
	if err != nil {
		if logger != nil {
			logger.WithError(err).WithField("sessionId", sessionID).Warn(eventLabel + ": snapshot decode failed")
		}
		return nil
	}
	if requester == "" {
		if logger != nil {
			logger.WithField("sessionId", sessionID).Warn(eventLabel + ": snapshot missing requester user ID")
		}
		return nil
	}
	if logger != nil {
		logger.WithFields(logrus.Fields{
			"sessionId": sessionID,
			"userId":    requester,
		}).Debug(eventLabel + ": derived user ID from snapshot")
	}
	return []string{requester}
}

func partyMembersFromSnapshot(logger *logrus.Entry, eventLabel, sessionID string, payload *sessionpb.NotificationPayload) []string {
	if payload == nil {
		return nil
	}
	members, err := parsePartySnapshotMembers(payload.GetMessage())
	if err != nil {
		if logger != nil {
			logger.WithError(err).WithField("sessionId", sessionID).Warn(eventLabel + ": snapshot decode failed")
		}
		return nil
	}
	if len(members) == 0 {
		if logger != nil {
			logger.WithField("sessionId", sessionID).Warn(eventLabel + ": snapshot missing members")
		}
		return nil
	}
	if logger != nil {
		logger.WithFields(logrus.Fields{
			"sessionId":          sessionID,
			"derivedMemberCount": len(members),
		}).Debug(eventLabel + ": derived members from snapshot")
	}
	return members
}

func requesterUserIDFromSnapshot(encoded string) (string, error) {
	data, err := decodeSnapshotEnvelope(encoded)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	var envelope partyLifecycleEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	return strings.TrimSpace(envelope.RequesterUserID), nil
}

type partyLifecycleEnvelope struct {
	RequesterUserID string                 `json:"requesterUserID"`
	Payload         partyLifecycleSnapshot `json:"payload"`
}

type partyLifecycleSnapshot struct {
	Members []partyLifecycleMember `json:"Members"`
}

type partyLifecycleMember struct {
	ID             string `json:"ID"`
	Status         string `json:"Status"`
	StatusV2       string `json:"StatusV2"`
	PreviousStatus string `json:"PreviousStatus"`
}

func logNotificationPayload(logger *logrus.Entry, level logrus.Level, eventName, sessionID string, payload *sessionpb.NotificationPayload, message string) {
	if logger == nil {
		return
	}

	fields := logrus.Fields{
		"event": eventName,
	}
	if sessionID != "" {
		fields["sessionId"] = sessionID
	}

	switch {
	case payload == nil:
		fields["payload"] = "<nil>"
	default:
		bytes, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(payload)
		if err != nil {
			logger.WithError(err).WithFields(fields).Warn(message + " (marshal failed)")
			return
		}
		fields["payload"] = string(bytes)
	}

	entry := logger.WithFields(fields)
	if level == logrus.WarnLevel {
		entry.Warn(message)
		return
	}
	entry.Debug(message)
}
