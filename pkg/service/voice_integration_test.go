package service

import (
	"context"
	"net"
	"sync"
	"testing"

	sessionpb "extend-eos-voice-rtc/pkg/pb/accelbyte-asyncapi/session/session/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

type orchestratorMock struct {
	mu             sync.Mutex
	lastSession    string
	lastSnapshot   string
	lastUserIDs    []string
	gameCreated    int
	gameEnded      int
	partyCreated   int
	partyJoined    int
	partyRemoved   int
	gameCreateErr  error
	gameEndedErr   error
	partyCreateErr error
	partyJoinErr   error
	partyRemoveErr error
}

func (m *orchestratorMock) HandleGameSessionCreated(_ context.Context, sessionID string, snapshot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSession = sessionID
	m.lastSnapshot = snapshot
	m.gameCreated++
	return m.gameCreateErr
}

func (m *orchestratorMock) HandleGameSessionEnded(_ context.Context, sessionID string, snapshot string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSession = sessionID
	m.lastSnapshot = snapshot
	m.gameEnded++
	return m.gameEndedErr
}

func (m *orchestratorMock) HandlePartyCreated(_ context.Context, partyID string, snapshot string, userIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSession = partyID
	m.lastSnapshot = snapshot
	m.lastUserIDs = userIDs
	m.partyCreated++
	return m.partyCreateErr
}

func (m *orchestratorMock) HandlePartyMembersJoined(_ context.Context, partyID string, userIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSession = partyID
	m.lastUserIDs = userIDs
	m.partyJoined++
	return m.partyJoinErr
}

func (m *orchestratorMock) HandlePartyMembersRemoved(_ context.Context, partyID string, userIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSession = partyID
	m.lastUserIDs = userIDs
	m.partyRemoved++
	return m.partyRemoveErr
}

func startVoiceTestServer(t *testing.T, orchestrator voiceOrchestrator) (*grpc.ClientConn, func()) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()

	sessionpb.RegisterMpv2SessionHistoryGameSessionCreatedEventServiceServer(server, NewGameSessionCreatedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryGameSessionJoinedEventServiceServer(server, NewGameSessionJoinedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryGameSessionMembersChangedEventServiceServer(server, NewGameSessionMembersChangedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryGameSessionKickedEventServiceServer(server, NewGameSessionKickedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryGameSessionEndedEventServiceServer(server, NewGameSessionEndedServer(orchestrator, nil))

	sessionpb.RegisterMpv2SessionHistoryPartyCreatedEventServiceServer(server, NewPartyCreatedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryPartyJoinedEventServiceServer(server, NewPartyJoinedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryPartyMembersChangedEventServiceServer(server, NewPartyMembersChangedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryPartyLeaveEventServiceServer(server, NewPartyLeaveServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryPartyKickedEventServiceServer(server, NewPartyKickedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryPartyDeletedEventServiceServer(server, NewPartyDeletedServer(orchestrator, nil))
	sessionpb.RegisterMpv2SessionHistoryPartyRejoinedEventServiceServer(server, NewPartyRejoinedServer(orchestrator, nil))

	go func() {
		_ = server.Serve(lis)
	}()

	ctx := context.Background()
	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.DialContext(ctx, "",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}

	cleanup := func() {
		conn.Close()
		server.Stop()
		lis.Close()
	}

	return conn, cleanup
}

func TestGameSessionGRPCHandlers(t *testing.T) {
	mock := &orchestratorMock{}
	conn, cleanup := startVoiceTestServer(t, mock)
	defer cleanup()

	createdClient := sessionpb.NewMpv2SessionHistoryGameSessionCreatedEventServiceClient(conn)
	if _, err := createdClient.OnMessage(context.Background(), &sessionpb.GameSessionCreatedEvent{SessionId: "session-A", Payload: &sessionpb.NotificationPayload{Message: "c29tZQ=="}}); err != nil {
		t.Fatalf("unexpected error calling created handler: %v", err)
	}
	mock.mu.Lock()
	if mock.gameCreated != 1 || mock.lastSession != "session-A" || mock.lastSnapshot != "c29tZQ==" {
		t.Fatalf("unexpected orchestrator state: %+v", mock)
	}
	mock.mu.Unlock()

	joinedClient := sessionpb.NewMpv2SessionHistoryGameSessionJoinedEventServiceClient(conn)
	mock.mu.Lock()
	mock.gameCreateErr = status.Error(codes.InvalidArgument, "boom")
	mock.mu.Unlock()
	if _, err := joinedClient.OnMessage(context.Background(), &sessionpb.GameSessionJoinedEvent{SessionId: "session-B"}); err != nil {
		t.Fatalf("joined handler should ignore events: %v", err)
	}

	mock.mu.Lock()
	mock.gameCreateErr = nil
	mock.gameEndedErr = status.Error(codes.NotFound, "gone")
	mock.mu.Unlock()

	endedClient := sessionpb.NewMpv2SessionHistoryGameSessionEndedEventServiceClient(conn)
	_, err := endedClient.OnMessage(context.Background(), &sessionpb.GameSessionEndedEvent{
		SessionId: "session-C",
		Payload:   &sessionpb.NotificationPayload{Message: "snapshot-data"},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error calling ended handler: %v", err)
	}
	mock.mu.Lock()
	if mock.gameEnded != 1 || mock.lastSession != "session-C" || mock.lastSnapshot != "snapshot-data" {
		t.Fatalf("cleanup not invoked: %+v", mock)
	}
	mock.mu.Unlock()
}

func TestPartyGRPCHandlers(t *testing.T) {
	mock := &orchestratorMock{}
	conn, cleanup := startVoiceTestServer(t, mock)
	defer cleanup()

	createdClient := sessionpb.NewMpv2SessionHistoryPartyCreatedEventServiceClient(conn)
	if _, err := createdClient.OnMessage(context.Background(), &sessionpb.PartyCreatedEvent{
		SessionId: "party-A",
		Payload:   &sessionpb.NotificationPayload{UserIds: []string{"p1"}, Message: "snapshot"},
	}); err != nil {
		t.Fatalf("unexpected error calling party created: %v", err)
	}
	mock.mu.Lock()
	if mock.partyCreated != 1 || mock.lastSession != "party-A" || mock.lastSnapshot != "snapshot" || len(mock.lastUserIDs) != 1 || mock.lastUserIDs[0] != "p1" {
		t.Fatalf("unexpected state: %+v", mock)
	}
	mock.mu.Unlock()

	mock.mu.Lock()
	mock.partyJoinErr = status.Error(codes.Internal, "boom")
	mock.mu.Unlock()

	membersClient := sessionpb.NewMpv2SessionHistoryPartyMembersChangedEventServiceClient(conn)
	if _, err := membersClient.OnMessage(context.Background(), &sessionpb.PartyMembersChangedEvent{SessionId: "party-B"}); err != nil {
		t.Fatalf("members changed handler should be noop: %v", err)
	}

	mock.mu.Lock()
	if mock.partyJoined != 0 {
		t.Fatalf("members changed should not touch orchestrator: %+v", mock)
	}
	mock.mu.Unlock()

	joinedClient := sessionpb.NewMpv2SessionHistoryPartyJoinedEventServiceClient(conn)
	_, err := joinedClient.OnMessage(context.Background(), &sessionpb.PartyJoinedEvent{
		SessionId: "party-C",
		Payload:   &sessionpb.NotificationPayload{UserIds: []string{"user-1"}},
	})
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected join handler to propagate error: %v", err)
	}
	mock.mu.Lock()
	if mock.partyJoined != 1 || mock.lastSession != "party-C" || len(mock.lastUserIDs) != 1 {
		t.Fatalf("party join state mismatch: %+v", mock)
	}
	mock.partyJoinErr = nil
	mock.partyRemoveErr = nil
	mock.mu.Unlock()

	leaveClient := sessionpb.NewMpv2SessionHistoryPartyLeaveEventServiceClient(conn)
	if _, err := leaveClient.OnMessage(context.Background(), &sessionpb.PartyLeaveEvent{
		SessionId: "party-D",
		Payload:   &sessionpb.NotificationPayload{UserIds: []string{"user-2"}},
	}); err != nil {
		t.Fatalf("unexpected error removing party member: %v", err)
	}
	mock.mu.Lock()
	if mock.partyRemoved != 1 || mock.lastUserIDs[0] != "user-2" {
		t.Fatalf("party removal state mismatch: %+v", mock)
	}
	mock.mu.Unlock()

	if _, err := joinedClient.OnMessage(context.Background(), &sessionpb.PartyJoinedEvent{
		SessionId: "party-E",
		Payload:   &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "req-join-int")},
	}); err != nil {
		t.Fatalf("unexpected error deriving join snapshot: %v", err)
	}
	mock.mu.Lock()
	if mock.partyJoined != 2 || mock.lastSession != "party-E" || mock.lastUserIDs[0] != "req-join-int" {
		t.Fatalf("party join snapshot state mismatch: %+v", mock)
	}
	mock.mu.Unlock()

	if _, err := leaveClient.OnMessage(context.Background(), &sessionpb.PartyLeaveEvent{
		SessionId: "party-F",
		Payload:   &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "req-leave-int")},
	}); err != nil {
		t.Fatalf("unexpected error deriving leave snapshot: %v", err)
	}
	mock.mu.Lock()
	if mock.partyRemoved != 2 || mock.lastSession != "party-F" || mock.lastUserIDs[0] != "req-leave-int" {
		t.Fatalf("party leave snapshot mismatch: %+v", mock)
	}
	mock.mu.Unlock()

	kickedClient := sessionpb.NewMpv2SessionHistoryPartyKickedEventServiceClient(conn)
	if _, err := kickedClient.OnMessage(context.Background(), &sessionpb.PartyKickedEvent{
		SessionId: "party-G",
		Payload:   &sessionpb.NotificationPayload{Message: partySnapshotMessage(t, "req-kick-int")},
	}); err != nil {
		t.Fatalf("unexpected error deriving kick snapshot: %v", err)
	}
	mock.mu.Lock()
	if mock.partyRemoved != 3 || mock.lastSession != "party-G" || mock.lastUserIDs[0] != "req-kick-int" {
		t.Fatalf("party kick snapshot mismatch: %+v", mock)
	}
	mock.mu.Unlock()
}
