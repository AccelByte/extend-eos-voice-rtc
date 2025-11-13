package service

import (
	"context"

	sessionpb "extend-eos-voice-rtc/pkg/pb/accelbyte-asyncapi/session/session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Game session event handlers.
type GameSessionCreatedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionCreatedEventServiceServer
	orchestrator voiceOrchestrator
}

type GameSessionJoinedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionJoinedEventServiceServer
}

type GameSessionMembersChangedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionMembersChangedEventServiceServer
}

type GameSessionKickedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionKickedEventServiceServer
}

type GameSessionEndedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryGameSessionEndedEventServiceServer
	orchestrator voiceOrchestrator
}

// Party event handlers.
type PartyCreatedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyCreatedEventServiceServer
	orchestrator voiceOrchestrator
}

type PartyJoinedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyJoinedEventServiceServer
	orchestrator voiceOrchestrator
}

type PartyMembersChangedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyMembersChangedEventServiceServer
}

type PartyLeaveHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyLeaveEventServiceServer
	orchestrator voiceOrchestrator
}

type PartyKickedHandler struct {
	sessionpb.UnimplementedMpv2SessionHistoryPartyKickedEventServiceServer
	orchestrator voiceOrchestrator
}

func NewGameSessionCreatedServer(o voiceOrchestrator) *GameSessionCreatedHandler {
	return &GameSessionCreatedHandler{
		UnimplementedMpv2SessionHistoryGameSessionCreatedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionCreatedEventServiceServer{},
		orchestrator: o,
	}
}

func NewGameSessionJoinedServer(_ voiceOrchestrator) *GameSessionJoinedHandler {
	return &GameSessionJoinedHandler{
		UnimplementedMpv2SessionHistoryGameSessionJoinedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionJoinedEventServiceServer{},
	}
}

func NewGameSessionMembersChangedServer(_ voiceOrchestrator) *GameSessionMembersChangedHandler {
	return &GameSessionMembersChangedHandler{
		UnimplementedMpv2SessionHistoryGameSessionMembersChangedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionMembersChangedEventServiceServer{},
	}
}

func NewGameSessionKickedServer(_ voiceOrchestrator) *GameSessionKickedHandler {
	return &GameSessionKickedHandler{
		UnimplementedMpv2SessionHistoryGameSessionKickedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionKickedEventServiceServer{},
	}
}

func NewGameSessionEndedServer(o voiceOrchestrator) *GameSessionEndedHandler {
	return &GameSessionEndedHandler{
		UnimplementedMpv2SessionHistoryGameSessionEndedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryGameSessionEndedEventServiceServer{},
		orchestrator: o,
	}
}

func NewPartyCreatedServer(o voiceOrchestrator) *PartyCreatedHandler {
	return &PartyCreatedHandler{
		UnimplementedMpv2SessionHistoryPartyCreatedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyCreatedEventServiceServer{},
		orchestrator: o,
	}
}

func NewPartyJoinedServer(o voiceOrchestrator) *PartyJoinedHandler {
	return &PartyJoinedHandler{
		UnimplementedMpv2SessionHistoryPartyJoinedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyJoinedEventServiceServer{},
		orchestrator: o,
	}
}

func NewPartyMembersChangedServer(_ voiceOrchestrator) *PartyMembersChangedHandler {
	return &PartyMembersChangedHandler{
		UnimplementedMpv2SessionHistoryPartyMembersChangedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyMembersChangedEventServiceServer{},
	}
}

func NewPartyLeaveServer(o voiceOrchestrator) *PartyLeaveHandler {
	return &PartyLeaveHandler{
		UnimplementedMpv2SessionHistoryPartyLeaveEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyLeaveEventServiceServer{},
		orchestrator: o,
	}
}

func NewPartyKickedServer(o voiceOrchestrator) *PartyKickedHandler {
	return &PartyKickedHandler{
		UnimplementedMpv2SessionHistoryPartyKickedEventServiceServer: sessionpb.UnimplementedMpv2SessionHistoryPartyKickedEventServiceServer{},
		orchestrator: o,
	}
}

func (h *GameSessionCreatedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionCreatedEvent) (*emptypb.Empty, error) {
	if err := h.orchestrator.HandleGameSessionCreated(ctx, event.GetSessionId(), event.GetPayload().GetMessage()); err != nil {
		return nil, status.Errorf(codes.Internal, "handle game session created failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *GameSessionJoinedHandler) OnMessage(ctx context.Context, _ *sessionpb.GameSessionJoinedEvent) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (h *GameSessionMembersChangedHandler) OnMessage(ctx context.Context, _ *sessionpb.GameSessionMembersChangedEvent) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (h *GameSessionKickedHandler) OnMessage(ctx context.Context, _ *sessionpb.GameSessionKickedEvent) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (h *GameSessionEndedHandler) OnMessage(ctx context.Context, event *sessionpb.GameSessionEndedEvent) (*emptypb.Empty, error) {
	if err := h.orchestrator.HandleGameSessionEnded(ctx, event.GetSessionId()); err != nil {
		return nil, status.Errorf(codes.Internal, "handle game session ended failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyCreatedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyCreatedEvent) (*emptypb.Empty, error) {
	if err := h.orchestrator.HandlePartyCreated(ctx, event.GetSessionId(), event.GetPayload().GetUserIds()); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party created failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyJoinedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyJoinedEvent) (*emptypb.Empty, error) {
	if err := h.orchestrator.HandlePartyMembersJoined(ctx, event.GetSessionId(), userIDsFromPayload(event.GetPayload())); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party joined failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyMembersChangedHandler) OnMessage(ctx context.Context, _ *sessionpb.PartyMembersChangedEvent) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (h *PartyLeaveHandler) OnMessage(ctx context.Context, event *sessionpb.PartyLeaveEvent) (*emptypb.Empty, error) {
	if err := h.orchestrator.HandlePartyMembersRemoved(ctx, event.GetSessionId(), userIDsFromPayload(event.GetPayload())); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party leave failed: %v", err)
	}
	return &emptypb.Empty{}, nil
}

func (h *PartyKickedHandler) OnMessage(ctx context.Context, event *sessionpb.PartyKickedEvent) (*emptypb.Empty, error) {
	if err := h.orchestrator.HandlePartyMembersRemoved(ctx, event.GetSessionId(), userIDsFromPayload(event.GetPayload())); err != nil {
		return nil, status.Errorf(codes.Internal, "handle party kicked failed: %v", err)
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
