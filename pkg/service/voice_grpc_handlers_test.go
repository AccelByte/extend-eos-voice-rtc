package service

import (
	"context"
	"testing"

	sessionpb "extend-eos-voice-rtc/pkg/pb/accelbyte-asyncapi/session/session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockOrchestrator struct {
	gameSessionCreatedCalls int
	gameSessionEndedCalls   int
	partyCreatedCalls       int
	partyJoinCalls          int
	partyRemoveCalls        int
	lastSession             string
	lastMessage             string
	lastUserIDs             []string

	gameCreatedErr error
	gameEndedErr   error
	partyCreateErr error
	partyJoinErr   error
	partyRemoveErr error
}

func (m *mockOrchestrator) HandleGameSessionCreated(_ context.Context, sessionID string, snapshot string) error {
	m.gameSessionCreatedCalls++
	m.lastSession = sessionID
	m.lastMessage = snapshot
	return m.gameCreatedErr
}

func (m *mockOrchestrator) HandleGameSessionEnded(_ context.Context, sessionID string) error {
	m.gameSessionEndedCalls++
	m.lastSession = sessionID
	return m.gameEndedErr
}

func (m *mockOrchestrator) HandlePartyCreated(_ context.Context, partyID string, userIDs []string) error {
	m.partyCreatedCalls++
	m.lastSession = partyID
	m.lastUserIDs = userIDs
	return m.partyCreateErr
}

func (m *mockOrchestrator) HandlePartyMembersJoined(_ context.Context, partyID string, userIDs []string) error {
	m.partyJoinCalls++
	m.lastSession = partyID
	m.lastUserIDs = userIDs
	return m.partyJoinErr
}

func (m *mockOrchestrator) HandlePartyMembersRemoved(_ context.Context, partyID string, userIDs []string) error {
	m.partyRemoveCalls++
	m.lastSession = partyID
	m.lastUserIDs = userIDs
	return m.partyRemoveErr
}

func TestGameSessionHandlersInvokeOrchestrator(t *testing.T) {
	mock := &mockOrchestrator{}

	created := NewGameSessionCreatedServer(mock)
	payload := &sessionpb.NotificationPayload{Message: "c29tZQ=="} // "some"
	if _, err := created.OnMessage(context.Background(), &sessionpb.GameSessionCreatedEvent{SessionId: "session-1", Payload: payload}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.gameSessionCreatedCalls != 1 || mock.lastSession != "session-1" || mock.lastMessage != "c29tZQ==" {
		t.Fatalf("game session created not invoked: %+v", mock)
	}

	joined := NewGameSessionJoinedServer(mock)
	if _, err := joined.OnMessage(context.Background(), &sessionpb.GameSessionJoinedEvent{}); err != nil {
		t.Fatalf("joined handler should not error: %v", err)
	}
	if mock.gameSessionCreatedCalls != 1 {
		t.Fatalf("joined handler should be a noop")
	}

	ended := NewGameSessionEndedServer(mock)
	mock.gameEndedErr = status.Error(codes.DataLoss, "boom")
	_, err := ended.OnMessage(context.Background(), &sessionpb.GameSessionEndedEvent{SessionId: "session-2"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error, got %v", err)
	}
	if mock.gameSessionEndedCalls != 1 || mock.lastSession != "session-2" {
		t.Fatalf("game session ended not invoked: %+v", mock)
	}
}

func TestPartyHandlersInvokeOrchestrator(t *testing.T) {
	mock := &mockOrchestrator{}

	created := NewPartyCreatedServer(mock)
	if _, err := created.OnMessage(context.Background(), &sessionpb.PartyCreatedEvent{SessionId: "party-1", Payload: &sessionpb.NotificationPayload{UserIds: []string{"p1"}}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.partyCreatedCalls != 1 || mock.lastSession != "party-1" || len(mock.lastUserIDs) != 1 || mock.lastUserIDs[0] != "p1" {
		t.Fatalf("party created not invoked: %+v", mock)
	}

	joined := NewPartyJoinedServer(mock)
	payload := &sessionpb.NotificationPayload{UserIds: []string{"user-1", "user-2"}}
	if _, err := joined.OnMessage(context.Background(), &sessionpb.PartyJoinedEvent{SessionId: "party-2", Payload: payload}); err != nil {
		t.Fatalf("join handler error: %v", err)
	}
	if mock.partyJoinCalls != 1 || mock.lastSession != "party-2" || len(mock.lastUserIDs) != 2 {
		t.Fatalf("party join not invoked as expected: %+v", mock)
	}

	mock.partyRemoveErr = status.Error(codes.Internal, "remove failed")
	leave := NewPartyLeaveServer(mock)
	_, err := leave.OnMessage(context.Background(), &sessionpb.PartyLeaveEvent{SessionId: "party-3", Payload: &sessionpb.NotificationPayload{UserIds: []string{"gone"}}})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal code on remove, got %v", err)
	}
	if mock.partyRemoveCalls != 1 || mock.lastSession != "party-3" || mock.lastUserIDs[0] != "gone" {
		t.Fatalf("party remove state incorrect: %+v", mock)
	}

	kicked := NewPartyKickedServer(mock)
	if _, err := kicked.OnMessage(context.Background(), &sessionpb.PartyKickedEvent{SessionId: "party-4"}); err == nil {
		t.Fatalf("expected error due to previous failure")
	}

	membersChanged := NewPartyMembersChangedServer(mock)
	if _, err := membersChanged.OnMessage(context.Background(), &sessionpb.PartyMembersChangedEvent{}); err != nil {
		t.Fatalf("members changed handler should be noop: %v", err)
	}
	if mock.partyJoinCalls != 1 || mock.partyRemoveCalls != 2 {
		t.Fatalf("members changed handler should not call orchestrator: %+v", mock)
	}
}
