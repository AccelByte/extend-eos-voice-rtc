//go:build !integration
// +build !integration

package voiceclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error for missing config")
	}
	if _, err := New(Config{BaseURL: "http://example.com", DeploymentID: "", TokenURL: "http://example.com/token", ClientID: "id", ClientSecret: "secret"}); err == nil {
		t.Fatal("expected error for missing deployment ID")
	}
	if _, err := New(Config{BaseURL: "http://example.com", DeploymentID: "dep", TokenURL: "", ClientID: "id", ClientSecret: "secret"}); err == nil {
		t.Fatal("expected error for missing token URL")
	}
	if _, err := New(Config{BaseURL: "http://example.com", DeploymentID: "dep", TokenURL: "http://token", ClientID: "", ClientSecret: ""}); err == nil {
		t.Fatal("expected error for missing client credentials")
	}
	if _, err := New(Config{BaseURL: "://bad", DeploymentID: "dep", TokenURL: "http://token", ClientID: "id", ClientSecret: "secret"}); err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

func TestNewCreatesClient(t *testing.T) {
	t.Parallel()
	client, err := New(Config{
		BaseURL:      "https://example.com/api",
		DeploymentID: "dep",
		TokenURL:     "https://auth/token",
		ClientID:     "id",
		ClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("unexpected error constructing client: %v", err)
	}
	if client.baseURL.Path != "/" {
		t.Fatalf("expected sanitized path, got %s", client.baseURL.Path)
	}
	if client.deploymentID != "dep" {
		t.Fatalf("unexpected deployment id: %s", client.deploymentID)
	}
}

func TestCreateRoomTokensAndRemoval(t *testing.T) {
	t.Parallel()
	transport := newQueueTransport(t,
		func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer stub-token" {
				t.Fatalf("unexpected auth header: %s", got)
			}
			var payload struct {
				Participants []struct {
					ProductUserID string `json:"puid"`
				} `json:"participants"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("unable to decode payload: %v", err)
			}
			if len(payload.Participants) != 1 || payload.Participants[0].ProductUserID != "player" {
				t.Fatalf("unexpected payload: %+v", payload.Participants)
			}
			return jsonResponse(t, http.StatusOK, map[string]any{
				"roomId":        "session:Voice",
				"clientBaseUrl": "wss://media",
				"deploymentId":  "dep",
				"participants": []map[string]any{{
					"puid":  "player",
					"token": "room-token",
				}},
			}), nil
		},
		func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodDelete {
				t.Fatalf("expected DELETE, got %s", req.Method)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil))}, nil
		},
	)
	client := newTestClient(t, transport)
	client.tokens.value = "stub-token"
	client.tokens.expiresAt = time.Now().Add(time.Hour)

	resp, err := client.CreateRoomTokens(context.Background(), "session:Voice", []Participant{{ProductUserID: "player"}})
	if err != nil {
		t.Fatalf("unexpected error creating tokens: %v", err)
	}
	if resp == nil || resp.ClientBaseURL != "wss://media" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	if err := client.RemoveParticipant(context.Background(), "session:Voice", "player"); err != nil {
		t.Fatalf("unexpected error removing participant: %v", err)
	}
	if transport.CallCount() != 2 {
		t.Fatalf("expected two HTTP calls, got %d", transport.CallCount())
	}
}

func TestCreateRoomTokensValidatesInput(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, newQueueTransport(t))
	if _, err := client.CreateRoomTokens(context.Background(), "", nil); err == nil {
		t.Fatal("expected error when participants missing")
	}
	if _, err := client.CreateRoomTokens(context.Background(), "room", nil); err == nil {
		t.Fatal("expected error when participants slice empty")
	}
}

func TestRemoveParticipantValidatesInput(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, newQueueTransport(t))
	if err := client.RemoveParticipant(context.Background(), "", ""); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestQueryExternalAccounts(t *testing.T) {
	t.Parallel()
	transport := newQueueTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/user/v1/accounts" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if got := req.URL.Query()["accountId"]; len(got) != 1 || got[0] != "user-1" {
			t.Fatalf("unexpected account query: %v", got)
		}
		if req.URL.Query().Get("identityProviderId") != "openId" {
			t.Fatalf("missing identity provider")
		}
		return jsonResponse(t, http.StatusOK, map[string]any{"ids": map[string]string{"user-1": "puid-1"}}), nil
	})
	client := newTestClient(t, transport)
	client.tokens.value = "stub-token"
	client.tokens.expiresAt = time.Now().Add(time.Hour)

	mapping, err := client.QueryExternalAccounts(context.Background(), "openId", []string{"user-1", ""})
	if err != nil {
		t.Fatalf("unexpected error querying accounts: %v", err)
	}
	if mapping["user-1"] != "puid-1" {
		t.Fatalf("unexpected mapping: %+v", mapping)
	}
}

func TestQueryExternalAccountsSkipsEmptyIDs(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, newQueueTransport(t))
	client.tokens.value = "stub-token"
	client.tokens.expiresAt = time.Now().Add(time.Hour)

	result, err := client.QueryExternalAccounts(context.Background(), "openid", []string{"", " "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestTokenProviderFetchesToken(t *testing.T) {
	t.Parallel()
	var calls int32
	provider := newTestTokenProvider(t, func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if req.FormValue("grant_type") != "client_credentials" {
			t.Fatalf("unexpected grant type: %s", req.FormValue("grant_type"))
		}
		return jsonResponse(t, http.StatusOK, map[string]any{
			"access_token": "simple-token",
			"expires_in":   120,
		}), nil
	})

	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error retrieving token: %v", err)
	}
	if token != "simple-token" {
		t.Fatalf("unexpected token: %s", token)
	}
	if calls != 1 {
		t.Fatalf("expected single token request, got %d", calls)
	}
}

func TestTokenProviderUsesRefreshToken(t *testing.T) {
	t.Parallel()
	var calls int32
	refresh := "refresh-1"
	provider := newTestTokenProvider(t,
		func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := req.FormValue("grant_type"); got != "client_credentials" {
				t.Fatalf("expected client_credentials, got %s", got)
			}
			return jsonResponse(t, http.StatusOK, map[string]any{
				"access_token":  "token-1",
				"refresh_token": refresh,
				"expires_in":    1,
			}), nil
		},
		func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := req.FormValue("grant_type"); got != "refresh_token" {
				t.Fatalf("expected refresh_token, got %s", got)
			}
			if got := req.FormValue("refresh_token"); got != refresh {
				t.Fatalf("unexpected refresh token payload: %s", got)
			}
			return jsonResponse(t, http.StatusOK, map[string]any{
				"access_token":  "token-2",
				"refresh_token": "refresh-2",
				"expires_in":    3600,
			}), nil
		},
	)

	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error retrieving token: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("unexpected initial token %s", token)
	}

	provider.mu.Lock()
	provider.expiresAt = time.Now().Add(-time.Second)
	provider.mu.Unlock()

	refreshed, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error refreshing token: %v", err)
	}
	if refreshed != "token-2" {
		t.Fatalf("unexpected refreshed token %s", refreshed)
	}
	if calls != 2 {
		t.Fatalf("expected two token calls, got %d", calls)
	}
}

func TestTokenProviderRefreshFallback(t *testing.T) {
	t.Parallel()
	var calls int32
	provider := newTestTokenProvider(t,
		func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			return jsonResponse(t, http.StatusOK, map[string]any{
				"access_token":  "token-initial",
				"refresh_token": "refresh-initial",
				"expires_in":    1,
			}), nil
		},
		func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			return jsonResponse(t, http.StatusBadRequest, map[string]string{"error": "invalid"}), nil
		},
		func(req *http.Request) (*http.Response, error) {
			atomic.AddInt32(&calls, 1)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			return jsonResponse(t, http.StatusOK, map[string]any{
				"access_token":  "token-fallback",
				"refresh_token": "refresh-fallback",
				"expires_in":    3600,
			}), nil
		},
	)

	if _, err := provider.Token(context.Background()); err != nil {
		t.Fatalf("unexpected error retrieving initial token: %v", err)
	}

	provider.mu.Lock()
	provider.expiresAt = time.Now().Add(-time.Second)
	provider.mu.Unlock()

	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error retrieving fallback token: %v", err)
	}
	if token != "token-fallback" {
		t.Fatalf("unexpected fallback token: %s", token)
	}
	if calls != 3 {
		t.Fatalf("expected three token calls, got %d", calls)
	}
}

type queueTransport struct {
	t          *testing.T
	mu         sync.Mutex
	responders []func(*http.Request) (*http.Response, error)
	calls      int
}

func newQueueTransport(t *testing.T, responders ...func(*http.Request) (*http.Response, error)) *queueTransport {
	return &queueTransport{t: t, responders: responders}
}

func (q *queueTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.calls++
	if len(q.responders) == 0 {
		q.t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
	}
	handler := q.responders[0]
	q.responders = q.responders[1:]
	return handler(req)
}

func (q *queueTransport) CallCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.calls
}

func newTestClient(t *testing.T, transport *queueTransport) *Client {
	t.Helper()
	base, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	return &Client{
		baseURL:      base,
		deploymentID: "dep",
		httpClient:   &http.Client{Transport: transport, Timeout: time.Second},
		tokens: &tokenProvider{
			httpClient: transportClient(transport),
			tokenURL:   "https://auth/token",
		},
	}
}

func newTestTokenProvider(t *testing.T, responders ...func(*http.Request) (*http.Response, error)) *tokenProvider {
	t.Helper()
	transport := newQueueTransport(t, responders...)
	provider, err := newTokenProvider(tokenProviderConfig{
		TokenURL:     "https://auth/token",
		ClientID:     "id",
		ClientSecret: "secret",
		DeploymentID: "dep",
		HTTPClient:   &http.Client{Transport: transport, Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing token provider: %v", err)
	}
	return provider
}

func transportClient(rt http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: rt,
		Timeout:   time.Second,
	}
}

func jsonResponse(t *testing.T, status int, payload any) *http.Response {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(data)),
		Header:     make(http.Header),
	}
}
