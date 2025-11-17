package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"

	"extend-eos-voice-rtc/pkg/voiceclient"

	lobbyNotification "github.com/AccelByte/accelbyte-go-sdk/lobby-sdk/pkg/lobbyclient/notification"
	sessionGame "github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclient/game_session"
	sessionModels "github.com/AccelByte/accelbyte-go-sdk/session-sdk/pkg/sessionclientmodels"
	"github.com/sirupsen/logrus"
)

func TestNewVoiceEventProcessorValidation(t *testing.T) {
	if _, err := NewVoiceEventProcessor(VoiceProcessorConfig{}); err == nil {
		t.Fatalf("expected error when voice client missing")
	}
	if _, err := NewVoiceEventProcessor(VoiceProcessorConfig{VoiceClient: &fakeVoiceClient{}}); err == nil {
		t.Fatalf("expected error when notification service missing")
	}
	if _, err := NewVoiceEventProcessor(VoiceProcessorConfig{
		VoiceClient:         &fakeVoiceClient{},
		NotificationService: &fakeNotificationService{},
	}); err == nil {
		t.Fatalf("expected error when game session service missing")
	}
	cfg := VoiceProcessorConfig{
		Namespace:           "ns",
		VoiceClient:         &fakeVoiceClient{},
		GameSessionService:  &fakeGameSessionService{},
		NotificationService: &fakeNotificationService{},
	}
	processor, err := NewVoiceEventProcessor(cfg)
	if err != nil {
		t.Fatalf("expected valid processor: %v", err)
	}
	if processor.topicName != "EOS_VOICE" {
		t.Fatalf("expected default topic, got %s", processor.topicName)
	}
}

func TestCreateParticipantsSendsNotifications(t *testing.T) {
	t.Helper()
	client := &fakeVoiceClient{
		queryResult: map[string]string{
			"user-1": "puid-1",
			"user-2": "puid-2",
		},
		createResponse: &voiceclient.CreateRoomTokenResponse{
			RoomID:        "session-1:blue",
			ClientBaseURL: "wss://voice",
			Participants: []voiceclient.CreateRoomTokenParticipant{
				{ProductUserID: "puid-1", Token: "token-1"},
				{ProductUserID: "puid-2", Token: "token-2"},
			},
		},
	}
	notifier := &fakeNotificationService{}
	processor := &VoiceEventProcessor{
		namespace:           "ns",
		topicName:           "topic",
		voiceClient:         client,
		gameSessionService:  &fakeGameSessionService{},
		notificationService: notifier,
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	err := processor.createParticipants(context.Background(), "session-1", "session-1:blue", []string{"user-1", "user-2"}, notificationTypeGame)
	if err != nil {
		t.Fatalf("createParticipants returned error: %v", err)
	}
	if len(client.createCalls) != 1 {
		t.Fatalf("expected one createRoomTokens call, got %d", len(client.createCalls))
	}
	created := client.createCalls[0]
	if created.roomID != "session-1:blue" || len(created.participants) != 2 {
		t.Fatalf("unexpected create call: %+v", created)
	}
	if len(notifier.sent) != 2 {
		t.Fatalf("expected two notifications, got %d", len(notifier.sent))
	}
	for _, rec := range notifier.sent {
		var payload map[string]string
		if err := json.Unmarshal([]byte(rec.message), &payload); err != nil {
			t.Fatalf("unable to decode notification payload: %v", err)
		}
		if payload["token"] == "" || payload["teamId"] != "blue" {
			t.Fatalf("unexpected notification payload: %+v", payload)
		}
	}
}

func TestCreateParticipantsSkipsMissingIDs(t *testing.T) {
	client := &fakeVoiceClient{
		queryResult: map[string]string{},
		createResponse: &voiceclient.CreateRoomTokenResponse{
			RoomID: "unused",
		},
	}
	processor := &VoiceEventProcessor{
		namespace:           "ns",
		topicName:           "topic",
		voiceClient:         client,
		gameSessionService:  &fakeGameSessionService{},
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	if err := processor.createParticipants(context.Background(), "session-1", "session-1:0", []string{"user-1"}, notificationTypeGame); err != nil {
		t.Fatalf("expected no error when EOS IDs missing: %v", err)
	}
	if len(client.createCalls) != 0 {
		t.Fatalf("expected no createRoomTokens calls, got %d", len(client.createCalls))
	}
}

func TestRemoveParticipantsRevokesTokens(t *testing.T) {
	client := &fakeVoiceClient{
		queryResult: map[string]string{
			"user-1": "puid-1",
			"user-2": "puid-2",
		},
	}
	processor := &VoiceEventProcessor{
		voiceClient:         client,
		gameSessionService:  &fakeGameSessionService{},
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	if err := processor.removeParticipants(context.Background(), "session-1", "room-1", []string{"user-1", "user-2"}); err != nil {
		t.Fatalf("removeParticipants returned error: %v", err)
	}
	if len(client.removeCalls) != 2 {
		t.Fatalf("expected two remove calls, got %d", len(client.removeCalls))
	}
}

func TestFetchEOSIDsDeduplicatesAndBatches(t *testing.T) {
	client := &fakeVoiceClient{queryResult: map[string]string{}}
	for i := 0; i < externalAccountBatchSize+5; i++ {
		id := fmt.Sprintf("user-%d", i)
		client.queryResult[id] = fmt.Sprintf("puid-%d", i)
	}
	processor := &VoiceEventProcessor{
		voiceClient:         client,
		gameSessionService:  &fakeGameSessionService{},
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	var ids []string
	for i := 0; i < externalAccountBatchSize+5; i++ {
		ids = append(ids, fmt.Sprintf("user-%d", i))
	}
	ids = append(ids, "user-1") // duplicate

	result, err := processor.fetchEOSIDs(context.Background(), ids)
	if err != nil {
		t.Fatalf("fetchEOSIDs returned error: %v", err)
	}
	if len(result) != externalAccountBatchSize+5 {
		t.Fatalf("expected %d ids, got %d", externalAccountBatchSize+5, len(result))
	}
	if len(client.queryBatches) != 2 {
		t.Fatalf("expected batching, got %d batches", len(client.queryBatches))
	}
}

func TestBuildGameSessionRoomMemberships(t *testing.T) {
	sessionID := "session-1"
	user1 := "user-1"
	user2 := "user-2"
	member1Status := "JOINED"
	member2Status := "LEFT"
	session := &sessionModels.ApimodelsGameSessionResponse{
		ID: &sessionID,
		Members: []*sessionModels.ApimodelsUserResponse{
			{ID: &user1, Status: &member1Status},
			{ID: &user2, Status: &member2Status},
		},
		Teams: []*sessionModels.ModelsTeam{
			{TeamID: "alpha", UserIDs: []string{user1, user2}},
		},
	}

	result := buildGameSessionRoomMemberships(session)
	if len(result) != 1 {
		t.Fatalf("expected one room, got %d", len(result))
	}
	users := result["session-1:alpha"]
	if len(users) != 1 || users[0] != "user-1" {
		t.Fatalf("room members not filtered: %+v", users)
	}
}

func TestBuildGameSessionRoomMembershipsFromSnapshot(t *testing.T) {
	snapshot := &gameSessionSnapshot{
		ID: "session-1",
		Teams: []gameSessionTeam{
			{TeamID: "", UserIDs: []string{"user-1", "user-2"}},
		},
		Members: []gameSessionMember{
			{ID: "user-1", Status: "JOINED"},
			{ID: "user-2", Status: "LEFT"},
		},
	}
	result := buildGameSessionRoomMembershipsFromSnapshot(snapshot)
	users := result["session-1:0"]
	if len(users) != 1 || users[0] != "user-1" {
		t.Fatalf("unexpected snapshot members: %+v", users)
	}
}

func TestHelperFunctions(t *testing.T) {
	if !isActiveStatus(ptr("CONNECTED"), ptr("")) {
		t.Fatalf("connected status should be active")
	}
	if isActiveStatus(ptr("LEFT"), ptr("")) {
		t.Fatalf("left status should be inactive")
	}
	if got := partyVoiceRoomID("party-1"); got != "party-1:Voice" {
		t.Fatalf("unexpected room id %s", got)
	}
	if got := defaultGameSessionRoomID("session-1"); got != "session-1:0" {
		t.Fatalf("unexpected default room %s", got)
	}
	members := []*sessionModels.ApimodelsUserResponse{
		{ID: ptr("user-1"), Status: ptr("JOINED")},
		{ID: ptr("user-2"), Status: ptr("LEFT")},
	}
	active := filterActiveMembers(members)
	if len(active) != 1 || active[0] != "user-1" {
		t.Fatalf("filterActiveMembers failed: %+v", active)
	}
	items := []string{"b", "a", "a"}
	unique := uniqueStrings(items)
	sort.Strings(unique)
	if len(unique) != 2 || unique[0] != "a" || unique[1] != "b" {
		t.Fatalf("uniqueStrings failed: %+v", unique)
	}
	batches := batchStrings([]string{"a", "b", "c"}, 2)
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 1 {
		t.Fatalf("batchStrings failed: %+v", batches)
	}
	inverted := invertMap(map[string]string{"a": "1", "b": ""})
	if len(inverted) != 1 || inverted["1"] != "a" {
		t.Fatalf("invertMap failed: %+v", inverted)
	}
}

func TestHandleGameSessionEndedRevokesMembers(t *testing.T) {
	sessionID := "session-1"
	memberStatus := "JOINED"
	session := &sessionModels.ApimodelsGameSessionResponse{
		ID: &sessionID,
		Members: []*sessionModels.ApimodelsUserResponse{
			{ID: ptr("user-1"), Status: &memberStatus},
			{ID: ptr("user-2"), Status: &memberStatus},
		},
		Teams: []*sessionModels.ModelsTeam{
			{TeamID: "alpha", UserIDs: []string{"user-1"}},
			{TeamID: "beta", UserIDs: []string{"user-2"}},
		},
	}
	service := &fakeGameSessionService{resp: session}
	client := &fakeVoiceClient{
		queryResult: map[string]string{
			"user-1": "puid-1",
			"user-2": "puid-2",
		},
	}
	processor := &VoiceEventProcessor{
		namespace:           "ns",
		topicName:           "topic",
		gameSessionService:  service,
		voiceClient:         client,
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	if err := processor.HandleGameSessionEnded(context.Background(), sessionID); err != nil {
		t.Fatalf("handle ended returned error: %v", err)
	}
	if len(client.removeCalls) != 2 {
		t.Fatalf("expected 2 revocations, got %d", len(client.removeCalls))
	}
}

func TestHandleGameSessionEndedWhenSessionMissing(t *testing.T) {
	service := &fakeGameSessionService{
		err: &sessionGame.GetGameSessionNotFound{},
	}
	processor := &VoiceEventProcessor{
		namespace:           "ns",
		topicName:           "topic",
		gameSessionService:  service,
		voiceClient:         &fakeVoiceClient{},
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	if err := processor.HandleGameSessionEnded(context.Background(), "missing"); err != nil {
		t.Fatalf("expected nil error when session missing, got %v", err)
	}
}

func TestHandleGameSessionCreated(t *testing.T) {
	client := &fakeVoiceClient{
		queryResult: map[string]string{"user-1": "puid-1"},
		createResponse: &voiceclient.CreateRoomTokenResponse{
			RoomID:        "session-10:0",
			ClientBaseURL: "wss://voice",
			Participants: []voiceclient.CreateRoomTokenParticipant{
				{ProductUserID: "puid-1", Token: "token-1"},
			},
		},
	}
	processor := &VoiceEventProcessor{
		namespace:           "ns",
		topicName:           "topic",
		voiceClient:         client,
		gameSessionService:  &fakeGameSessionService{},
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}
	snapshot := gameSessionSnapshotEnvelope{
		Payload: gameSessionSnapshot{
			ID: "session-10",
			Members: []gameSessionMember{
				{ID: "user-1", Status: "JOINED"},
			},
		},
	}
	data, _ := json.Marshal(snapshot)
	encoded := base64.StdEncoding.EncodeToString(data)
	if err := processor.HandleGameSessionCreated(context.Background(), "session-10", encoded); err != nil {
		t.Fatalf("handle game session created returned error: %v", err)
	}
	if len(client.createCalls) != 1 {
		t.Fatalf("expected create participants, got %d calls", len(client.createCalls))
	}
}

func TestHandlePartyLifecycle(t *testing.T) {
	client := &fakeVoiceClient{
		queryResult: map[string]string{
			"user-1": "puid-1",
			"user-2": "puid-2",
		},
		createResponse: &voiceclient.CreateRoomTokenResponse{
			RoomID:        "party-1:Voice",
			ClientBaseURL: "wss://voice",
			Participants: []voiceclient.CreateRoomTokenParticipant{
				{ProductUserID: "puid-1", Token: "token-1"},
				{ProductUserID: "puid-2", Token: "token-2"},
			},
		},
	}
	processor := &VoiceEventProcessor{
		namespace:           "ns",
		topicName:           "topic",
		voiceClient:         client,
		gameSessionService:  &fakeGameSessionService{},
		notificationService: &fakeNotificationService{},
		logger:              logrus.New().WithField("component", "voice-test"),
	}

	if err := processor.HandlePartyCreated(context.Background(), "party-1", []string{"user-1"}); err != nil {
		t.Fatalf("handle party created error: %v", err)
	}
	if err := processor.HandlePartyMembersJoined(context.Background(), "party-1", []string{"user-2"}); err != nil {
		t.Fatalf("handle party join error: %v", err)
	}
	if err := processor.HandlePartyMembersRemoved(context.Background(), "party-1", []string{"user-1"}); err != nil {
		t.Fatalf("handle party remove error: %v", err)
	}
	if len(client.createCalls) != 2 {
		t.Fatalf("expected create calls for initial and joined members, got %d", len(client.createCalls))
	}
	if len(client.removeCalls) == 0 {
		t.Fatalf("expected revoke calls for removed members")
	}
}

func TestIsGameSessionNotFound(t *testing.T) {
	if !isGameSessionNotFound(&sessionGame.GetGameSessionNotFound{}) {
		t.Fatalf("expected true for session not found type")
	}
	if isGameSessionNotFound(errors.New("boom")) {
		t.Fatalf("unexpected true for generic error")
	}
}

func ptr[T any](v T) *T {
	return &v
}

type fakeVoiceClient struct {
	queryResult  map[string]string
	queryErr     error
	queryBatches [][]string

	createResponse *voiceclient.CreateRoomTokenResponse
	createErr      error
	createCalls    []struct {
		roomID       string
		participants []voiceclient.Participant
	}

	removeErr   error
	removeCalls []struct {
		roomID        string
		productUserID string
	}
}

func (f *fakeVoiceClient) QueryExternalAccounts(_ context.Context, _ string, accountIDs []string) (map[string]string, error) {
	f.queryBatches = append(f.queryBatches, append([]string(nil), accountIDs...))
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	result := make(map[string]string, len(accountIDs))
	for _, id := range accountIDs {
		if puid, ok := f.queryResult[id]; ok {
			result[id] = puid
		}
	}
	return result, nil
}

func (f *fakeVoiceClient) CreateRoomTokens(_ context.Context, roomID string, participants []voiceclient.Participant) (*voiceclient.CreateRoomTokenResponse, error) {
	f.createCalls = append(f.createCalls, struct {
		roomID       string
		participants []voiceclient.Participant
	}{
		roomID:       roomID,
		participants: append([]voiceclient.Participant(nil), participants...),
	})
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createResponse, nil
}

func (f *fakeVoiceClient) RemoveParticipant(_ context.Context, roomID, productUserID string) error {
	f.removeCalls = append(f.removeCalls, struct {
		roomID        string
		productUserID string
	}{roomID: roomID, productUserID: productUserID})
	if f.removeErr != nil {
		return f.removeErr
	}
	return nil
}

type fakeNotificationService struct {
	sent []notificationRecord
	err  error
}

type notificationRecord struct {
	userID    string
	namespace string
	message   string
}

func (f *fakeNotificationService) SendSpecificUserFreeformNotificationV1AdminShort(params *lobbyNotification.SendSpecificUserFreeformNotificationV1AdminParams) error {
	if f.err != nil {
		return f.err
	}
	record := notificationRecord{
		userID:    params.UserID,
		namespace: params.Namespace,
	}
	if params.Body != nil && params.Body.Message != nil {
		record.message = *params.Body.Message
	}
	f.sent = append(f.sent, record)
	return nil
}

type fakeGameSessionService struct {
	resp      *sessionModels.ApimodelsGameSessionResponse
	err       error
	requested []string
}

func (f *fakeGameSessionService) GetGameSessionShort(params *sessionGame.GetGameSessionParams) (*sessionModels.ApimodelsGameSessionResponse, error) {
	f.requested = append(f.requested, params.SessionID)
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}
