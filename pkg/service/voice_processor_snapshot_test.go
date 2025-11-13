package service

import (
	"encoding/base64"
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
