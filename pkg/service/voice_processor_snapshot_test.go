package service

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseGameSessionSnapshot(t *testing.T) {
	payload := `{"payload":{"ID":"session123","Members":[{"ID":"userA","Status":"JOINED","StatusV2":"JOINED"}],"Teams":[]}}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	p := &VoiceEventProcessor{}
	rooms, err := p.parseGameSessionSnapshot(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected one room, got %d", len(rooms))
	}
	users := rooms["session123:0"]
	if len(users) != 1 || users[0] != "userA" {
		t.Fatalf("unexpected users: %+v", users)
	}
}

func TestParseGameSessionSnapshotInvalid(t *testing.T) {
	p := &VoiceEventProcessor{}
	if _, err := p.parseGameSessionSnapshot("not-base64"); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
}

func TestParseGameSessionSnapshotMissingID(t *testing.T) {
	payload := `{"payload":{"Members":[{"ID":"userA","Status":"JOINED","StatusV2":"JOINED"}],"Teams":[]}}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	p := &VoiceEventProcessor{}
	if _, err := p.parseGameSessionSnapshot(encoded); err == nil || !strings.Contains(err.Error(), "missing session ID") {
		t.Fatalf("expected missing session id error, got %v", err)
	}
}

func TestParseGameSessionSnapshotInlineJSON(t *testing.T) {
	p := &VoiceEventProcessor{}
	payload := `{"payload":{"ID":"inline-1","Members":[{"ID":"userInline","Status":"JOINED","StatusV2":"JOINED"}],"Teams":[{"teamID":"red","userIDs":["userInline"]}]}}`
	rooms, err := p.parseGameSessionSnapshot(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	users := rooms["inline-1:red"]
	if len(users) != 1 || users[0] != "userInline" {
		t.Fatalf("unexpected inline snapshot users: %+v", users)
	}
}

func TestParseGameSessionSnapshotEmpty(t *testing.T) {
	p := &VoiceEventProcessor{}
	rooms, err := p.parseGameSessionSnapshot("  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rooms != nil {
		t.Fatalf("expected nil rooms, got %v", rooms)
	}
}

func TestParsePartySnapshotMembers(t *testing.T) {
	payload := `{"payload":{"ID":"party123","Members":[{"ID":"good","Status":"JOINED","StatusV2":"JOINED"},{"ID":"stale","Status":"LEFT","StatusV2":"LEFT"},{"ID":"","Status":"JOINED"}]}}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	members, err := parsePartySnapshotMembers(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 || members[0] != "good" {
		t.Fatalf("unexpected party members: %+v", members)
	}
}

func TestParsePartySnapshotMembersInlineJSON(t *testing.T) {
	payload := `{"payload":{"ID":"partyInline","Members":[{"ID":"keep","Status":"JOINED","StatusV2":"JOINED"}]}}`
	members, err := parsePartySnapshotMembers(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 || members[0] != "keep" {
		t.Fatalf("expected keep member, got %+v", members)
	}
}

func TestParsePartySnapshotMembersMissingID(t *testing.T) {
	payload := `{"payload":{"Members":[{"ID":"missing","Status":"JOINED","StatusV2":"JOINED"}]}}`
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	if _, err := parsePartySnapshotMembers(encoded); err == nil || !strings.Contains(err.Error(), "missing session ID") {
		t.Fatalf("expected missing id error, got %v", err)
	}
}

func TestParsePartySnapshotMembersInvalid(t *testing.T) {
	if _, err := parsePartySnapshotMembers("???"); err == nil {
		t.Fatalf("expected error for invalid payload")
	}
}

func TestBuildGameSessionMembersFromSnapshot(t *testing.T) {
	snapshot := &gameSessionSnapshot{
		ID: "session-agg",
		Members: []gameSessionMember{
			{ID: "user-active", Status: "JOINED"},
			{ID: "user-stale", Status: "LEFT"},
			{ID: "", Status: "JOINED"},
		},
	}
	members := buildGameSessionMembersFromSnapshot(snapshot)
	if len(members) != 1 || members[0] != "user-active" {
		t.Fatalf("unexpected aggregated members: %+v", members)
	}
}
