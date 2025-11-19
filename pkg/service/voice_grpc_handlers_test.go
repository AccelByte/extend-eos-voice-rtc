package service

import (
	"context"
	"testing"

	sessionpb "extend-eos-voice-rtc/pkg/pb/accelbyte-asyncapi/session/session/v1"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
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

func (m *mockOrchestrator) HandleGameSessionEnded(_ context.Context, sessionID string, snapshot string) error {
	m.gameSessionEndedCalls++
	m.lastSession = sessionID
	m.lastMessage = snapshot
	return m.gameEndedErr
}

func (m *mockOrchestrator) HandlePartyCreated(_ context.Context, partyID string, snapshot string, userIDs []string) error {
	m.partyCreatedCalls++
	m.lastSession = partyID
	m.lastMessage = snapshot
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

	created := NewGameSessionCreatedServer(mock, nil)
	payload := &sessionpb.NotificationPayload{Message: "c29tZQ=="} // "some"
	if _, err := created.OnMessage(context.Background(), &sessionpb.GameSessionCreatedEvent{SessionId: "session-1", Payload: payload}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.gameSessionCreatedCalls != 1 || mock.lastSession != "session-1" || mock.lastMessage != "c29tZQ==" {
		t.Fatalf("game session created not invoked: %+v", mock)
	}

	joined := NewGameSessionJoinedServer(mock, nil)
	if _, err := joined.OnMessage(context.Background(), &sessionpb.GameSessionJoinedEvent{}); err != nil {
		t.Fatalf("joined handler should not error: %v", err)
	}
	if mock.gameSessionCreatedCalls != 1 {
		t.Fatalf("joined handler should be a noop")
	}

	ended := NewGameSessionEndedServer(mock, nil)
	mock.gameEndedErr = status.Error(codes.DataLoss, "boom")
	_, err := ended.OnMessage(context.Background(), &sessionpb.GameSessionEndedEvent{
		SessionId: "session-2",
		Payload:   &sessionpb.NotificationPayload{Message: "snapshot"},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error, got %v", err)
	}
	if mock.gameSessionEndedCalls != 1 || mock.lastSession != "session-2" || mock.lastMessage != "snapshot" {
		t.Fatalf("game session ended not invoked: %+v", mock)
	}
}

func TestPartyHandlersInvokeOrchestrator(t *testing.T) {
	mock := &mockOrchestrator{}

	created := NewPartyCreatedServer(mock, nil)
	payload := &sessionpb.NotificationPayload{UserIds: []string{"p1"}, Message: "snapshot"}
	if _, err := created.OnMessage(context.Background(), &sessionpb.PartyCreatedEvent{SessionId: "party-1", Payload: payload}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.partyCreatedCalls != 1 || mock.lastSession != "party-1" || len(mock.lastUserIDs) != 1 || mock.lastUserIDs[0] != "p1" || mock.lastMessage != "snapshot" {
		t.Fatalf("party created not invoked: %+v", mock)
	}

	joined := NewPartyJoinedServer(mock, nil)
	joinPayload := &sessionpb.NotificationPayload{UserIds: []string{"user-1", "user-2"}}
	if _, err := joined.OnMessage(context.Background(), &sessionpb.PartyJoinedEvent{SessionId: "party-2", Payload: joinPayload}); err != nil {
		t.Fatalf("join handler error: %v", err)
	}
	if mock.partyJoinCalls != 1 || mock.lastSession != "party-2" || len(mock.lastUserIDs) != 2 {
		t.Fatalf("party join not invoked as expected: %+v", mock)
	}

	mock.partyJoinErr = nil
	mock.partyJoinCalls = 0
	snapshotPayload := &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "req-1")}
	if _, err := joined.OnMessage(context.Background(), &sessionpb.PartyJoinedEvent{SessionId: "party-2", Payload: snapshotPayload}); err != nil {
		t.Fatalf("join handler snapshot error: %v", err)
	}
	if mock.partyJoinCalls != 1 || mock.lastUserIDs[0] != "req-1" {
		t.Fatalf("party join snapshot fallback failed: %+v", mock)
	}

	mock.partyRemoveErr = status.Error(codes.Internal, "remove failed")
	leave := NewPartyLeaveServer(mock, nil)
	_, err := leave.OnMessage(context.Background(), &sessionpb.PartyLeaveEvent{SessionId: "party-3", Payload: &sessionpb.NotificationPayload{UserIds: []string{"gone"}}})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal code on remove, got %v", err)
	}
	if mock.partyRemoveCalls != 1 || mock.lastSession != "party-3" || mock.lastUserIDs[0] != "gone" {
		t.Fatalf("party remove state incorrect: %+v", mock)
	}

	mock.partyRemoveErr = nil
	if _, err := leave.OnMessage(context.Background(), &sessionpb.PartyLeaveEvent{SessionId: "party-3", Payload: &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "req-leave")}}); err != nil {
		t.Fatalf("leave snapshot fallback failed: %v", err)
	}

	kicked := NewPartyKickedServer(mock, nil)
	mock.partyRemoveErr = status.Error(codes.Internal, "kicked remove failed")
	if _, err := kicked.OnMessage(context.Background(), &sessionpb.PartyKickedEvent{SessionId: "party-4"}); err == nil {
		t.Fatalf("expected error due to previous failure")
	}
	mock.partyRemoveErr = nil
	if _, err := kicked.OnMessage(context.Background(), &sessionpb.PartyKickedEvent{SessionId: "party-4", Payload: &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "req-kick")}}); err != nil {
		t.Fatalf("kicked snapshot fallback failed: %v", err)
	}

	expectedJoins := mock.partyJoinCalls
	expectedRemovals := mock.partyRemoveCalls

	membersChanged := NewPartyMembersChangedServer(mock, nil)
	if _, err := membersChanged.OnMessage(context.Background(), &sessionpb.PartyMembersChangedEvent{}); err != nil {
		t.Fatalf("members changed handler should be noop: %v", err)
	}
	if mock.partyJoinCalls != expectedJoins || mock.partyRemoveCalls != expectedRemovals {
		t.Fatalf("members changed handler should not call orchestrator: %+v", mock)
	}
}

func TestGameSessionPassiveHandlers(t *testing.T) {
	mock := &mockOrchestrator{}
	membersChanged := NewGameSessionMembersChangedServer(mock, nil)
	if _, err := membersChanged.OnMessage(context.Background(), &sessionpb.GameSessionMembersChangedEvent{}); err != nil {
		t.Fatalf("members changed should not error: %v", err)
	}
	kicked := NewGameSessionKickedServer(mock, nil)
	if _, err := kicked.OnMessage(context.Background(), &sessionpb.GameSessionKickedEvent{}); err != nil {
		t.Fatalf("kicked handler should not error: %v", err)
	}
}

func TestPartyUserIDsFromSnapshot(t *testing.T) {
	logger, hook := test.NewNullLogger()
	entry := logrus.NewEntry(logger)

	if ids := partyUserIDsFromSnapshot(entry, "label", "party", nil); ids != nil {
		t.Fatalf("expected nil ids for nil payload")
	}

	validPayload := &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "requester-1")}
	ids := partyUserIDsFromSnapshot(entry, "label", "party", validPayload)
	if len(ids) != 1 || ids[0] != "requester-1" {
		t.Fatalf("unexpected ids for valid snapshot: %+v", ids)
	}

	if ids := partyUserIDsFromSnapshot(entry, "label", "party", &sessionpb.NotificationPayload{Message: "??"}); ids != nil {
		t.Fatalf("expected nil ids for invalid snapshot")
	}

	missingRequester := &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "")}
	if ids := partyUserIDsFromSnapshot(entry, "label", "party", missingRequester); ids != nil {
		t.Fatalf("expected nil ids when requester missing")
	}

	if hook == nil || len(hook.Entries) == 0 {
		t.Fatalf("expected logs to be recorded for snapshot parsing scenarios")
	}
}

func TestLogNotificationPayload(t *testing.T) {
	logger, hook := test.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)
	entry := logrus.NewEntry(logger)

	logNotificationPayload(nil, logrus.InfoLevel, "nil", "", nil, "should noop")

	logNotificationPayload(entry, logrus.WarnLevel, "warn-event", "session", nil, "warn message")
	if hook.LastEntry().Level != logrus.WarnLevel {
		t.Fatalf("expected warn entry, got %v", hook.LastEntry())
	}

	hook.Reset()
	payload := &sessionpb.NotificationPayload{UserIds: []string{"user-1"}}
	logNotificationPayload(entry, logrus.DebugLevel, "debug-event", "", payload, "debug message")
	if hook.LastEntry().Level != logrus.DebugLevel {
		t.Fatalf("expected debug entry, got %v", hook.LastEntry())
	}
	if payloadField, ok := hook.LastEntry().Data["payload"]; !ok || payloadField == "" {
		t.Fatalf("expected payload field to be logged")
	}
}

func TestPartyDeletedHandler(t *testing.T) {
	mock := &mockOrchestrator{}
	logger := logrus.New().WithField("test", "party-deleted")
	handler := NewPartyDeletedServer(mock, logger)

	payload := &sessionpb.NotificationPayload{UserIds: []string{"alpha", "beta"}}
	if _, err := handler.OnMessage(context.Background(), &sessionpb.PartyDeletedEvent{SessionId: "party-1", Payload: payload}); err != nil {
		t.Fatalf("party deleted handler returned error: %v", err)
	}
	if mock.partyRemoveCalls != 1 || mock.lastSession != "party-1" || len(mock.lastUserIDs) != 2 {
		t.Fatalf("party deleted handler did not forward user IDs: %+v", mock)
	}

	mock.partyRemoveCalls = 0
	mock.lastUserIDs = nil
	snapshotPayload := &sessionpb.NotificationPayload{Message: partyMembersSnapshotMessage(t, "party-1", "gamma", "delta")}
	if _, err := handler.OnMessage(context.Background(), &sessionpb.PartyDeletedEvent{SessionId: "party-1", Payload: snapshotPayload}); err != nil {
		t.Fatalf("party deleted snapshot handler returned error: %v", err)
	}
	if mock.partyRemoveCalls != 1 || len(mock.lastUserIDs) != 2 {
		t.Fatalf("party deleted snapshot handling failed: %+v", mock)
	}
}

func TestPartyRejoinedHandler(t *testing.T) {
	mock := &mockOrchestrator{}
	handler := NewPartyRejoinedServer(mock, nil)

	payload := &sessionpb.NotificationPayload{UserIds: []string{"returning"}}
	if _, err := handler.OnMessage(context.Background(), &sessionpb.PartyRejoinedEvent{SessionId: "party-join", Payload: payload}); err != nil {
		t.Fatalf("party rejoined handler error: %v", err)
	}
	if mock.partyJoinCalls != 1 || mock.lastUserIDs[0] != "returning" {
		t.Fatalf("party rejoined handler did not forward payload user IDs: %+v", mock)
	}

	mock.partyJoinCalls = 0
	mock.lastUserIDs = nil
	snapshot := &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "snapshot-user")}
	if _, err := handler.OnMessage(context.Background(), &sessionpb.PartyRejoinedEvent{SessionId: "party-join", Payload: snapshot}); err != nil {
		t.Fatalf("party rejoined snapshot handler error: %v", err)
	}
	if mock.partyJoinCalls != 1 || mock.lastUserIDs[0] != "snapshot-user" {
		t.Fatalf("party rejoined snapshot fallback failed: %+v", mock)
	}
}
