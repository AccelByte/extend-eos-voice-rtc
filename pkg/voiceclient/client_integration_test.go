//go:build integration

package voiceclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	integrationBaseURL  = "https://api.epicgames.dev"
	integrationTokenURL = "	https://api.epicgames.dev/auth/v1/oauth/token"
)

// TestCreateRoomTokensAgainstEOS performs a live request against the EOS Voice API.
//
// The test is gated behind the `integration` build tag and requires the following
// environment variables to be set with valid credentials:
//   - EOS_IT_DEPLOYMENT_ID       (deployment identifier for your title)
//   - EOS_IT_CLIENT_ID           (OAuth client ID)
//   - EOS_IT_CLIENT_SECRET       (OAuth client secret)
//   - EOS_IT_ROOM_ID             (room identifier the title is allowed to create/update)
//   - EOS_IT_PRODUCT_USER_ID     (EOS Product User ID that should receive the room token)
//
// The EOS Voice and token endpoints default to the production values but can be
// overridden via `EOS_IT_BASE_URL` and `EOS_IT_TOKEN_URL`
//
// When any variable is missing the test is skipped. Because the call mutates live
// voice state, point the parameters at an isolated deployment or disposable room.
func TestCreateRoomTokensAgainstEOS(t *testing.T) {
	required := []string{
		"EOS_IT_DEPLOYMENT_ID",
		"EOS_IT_CLIENT_ID",
		"EOS_IT_CLIENT_SECRET",
		"EOS_IT_ROOM_ID",
		"EOS_IT_PRODUCT_USER_ID",
		"EOS_IT_ACCOUNT_ID",
	}

	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Skipf("skipping EOS integration test, %s not set", key)
		}
	}

	cfg := Config{
		BaseURL:      envOrDefault("EOS_IT_BASE_URL", integrationBaseURL),
		DeploymentID: os.Getenv("EOS_IT_DEPLOYMENT_ID"),
		TokenURL:     envOrDefault("EOS_IT_TOKEN_URL", integrationTokenURL),
		ClientID:     os.Getenv("EOS_IT_CLIENT_ID"),
		ClientSecret: os.Getenv("EOS_IT_CLIENT_SECRET"),
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to construct voice client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	roomID := os.Getenv("EOS_IT_ROOM_ID")
	expectedPUID := os.Getenv("EOS_IT_PRODUCT_USER_ID")
	accountID := os.Getenv("EOS_IT_ACCOUNT_ID")

	voiceToken, err := client.tokens.Token(ctx)
	if err != nil {
		t.Fatalf("failed to fetch voice token: %v", err)
	}
	fmt.Printf("[integration] Voice token (len=%d): %s\n", len(voiceToken), voiceToken)

	ids, err := client.QueryExternalAccounts(ctx, "openid", []string{accountID})
	if err != nil {
		t.Fatalf("QueryExternalAccounts failed: %v", err)
	}
	resolved := ids[accountID]
	if resolved == "" {
		t.Fatalf("QueryExternalAccounts returned empty ProductUserID for %s", accountID)
	}
	if expectedPUID != "" && expectedPUID != resolved {
		t.Fatalf("resolved ProductUserID mismatch: expected %s, got %s", expectedPUID, resolved)
	}
	fmt.Printf("[integration] QueryExternalAccounts resolved account %s -> PUID %s\n", accountID, resolved)

	resp, err := client.CreateRoomTokens(ctx, roomID, []Participant{{
		ProductUserID: resolved,
	}})
	if err != nil {
		t.Fatalf("CreateRoomTokens failed: %v", err)
	}
	if resp == nil || resp.RoomID == "" || len(resp.Participants) == 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Participants[0].Token == "" {
		t.Fatalf("expected participant token in response: %+v", resp.Participants[0])
	}
	fmt.Printf("[integration] CreateRoomTokens issued token: %s\n", resp.Participants[0].Token)

	if os.Getenv("EOS_IT_SKIP_REMOVE") == "1" {
		fmt.Println("[integration] EOS_IT_SKIP_REMOVE=1, skipping removal")
		return
	}

	if err := client.RemoveParticipant(ctx, roomID, resolved); err != nil {
		t.Fatalf("RemoveParticipant failed: %v", err)
	}
	fmt.Printf("[integration] RemoveParticipant succeeded for %s in room %s\n", resolved, roomID)
}

func TestObtainAccessTokenAgainstEOS(t *testing.T) {
	required := []string{
		"EOS_IT_DEPLOYMENT_ID",
		"EOS_IT_CLIENT_ID",
		"EOS_IT_CLIENT_SECRET",
	}

	for _, key := range required {
		if os.Getenv(key) == "" {
			t.Skipf("skipping EOS token integration test, %s not set", key)
		}
	}

	cfg := Config{
		BaseURL:      envOrDefault("EOS_IT_BASE_URL", integrationBaseURL),
		DeploymentID: os.Getenv("EOS_IT_DEPLOYMENT_ID"),
		TokenURL:     envOrDefault("EOS_IT_TOKEN_URL", integrationTokenURL),
		ClientID:     os.Getenv("EOS_IT_CLIENT_ID"),
		ClientSecret: os.Getenv("EOS_IT_CLIENT_SECRET"),
		HTTPClient:   &http.Client{Timeout: 10 * time.Second},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to construct voice client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tok, err := client.tokens.Token(ctx)
	if err != nil {
		t.Fatalf("token acquisition failed: %v", err)
	}
	if tok == "" {
		t.Fatal("received empty token from EOS")
	}

	fmt.Printf("[integration] Voice token (len=%d): %s\n", len(tok), tok)
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return strings.TrimSpace(fallback)
}
