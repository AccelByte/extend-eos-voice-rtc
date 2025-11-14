//go:build !integration
// +build !integration

package voiceclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing config")
	}

	_, err = New(Config{BaseURL: "http://example.com", DeploymentID: "", TokenURL: "http://example.com/token", ClientID: "id", ClientSecret: "secret"})
	if err == nil {
		t.Fatal("expected error for missing deployment ID")
	}
}

func TestCreateRoomTokensAndRemoval(t *testing.T) {
	var tokenRequests int32
	tokenSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("unexpected error parsing form: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "client_credentials" {
			t.Fatalf("unexpected grant_type: %s", got)
		}
		atomic.AddInt32(&tokenRequests, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer tokenSrv.Close()

	var lastAuth string
	voiceSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roomId":        "session:Voice",
				"clientBaseUrl": "wss://media",
				"deploymentId":  "dep",
				"participants": []map[string]any{{
					"puid":  "player",
					"token": "room-token",
				}},
			})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer voiceSrv.Close()

	cfg := Config{
		BaseURL:      voiceSrv.URL,
		DeploymentID: "dep",
		TokenURL:     tokenSrv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   &http.Client{Timeout: time.Second},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error constructing client: %v", err)
	}

	resp, err := client.CreateRoomTokens(context.Background(), "session:Voice", []Participant{{ProductUserID: "player"}})
	if err != nil {
		t.Fatalf("unexpected error creating tokens: %v", err)
	}
	if resp == nil || resp.ClientBaseURL != "wss://media" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if lastAuth != "Bearer test-token" {
		t.Fatalf("expected bearer token, got %q", lastAuth)
	}

	if err := client.RemoveParticipant(context.Background(), "session:Voice", "player"); err != nil {
		t.Fatalf("unexpected error removing participant: %v", err)
	}

	if atomic.LoadInt32(&tokenRequests) != 1 {
		t.Fatalf("token endpoint called unexpected times: %d", tokenRequests)
	}
}

func TestTokenProviderFetchesToken(t *testing.T) {
	tokenSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "simple-token",
			"expires_in":   120,
		})
	}))
	defer tokenSrv.Close()

	client, err := New(Config{
		BaseURL:      "http://example",
		DeploymentID: "dep",
		TokenURL:     tokenSrv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing client: %v", err)
	}

	tok, err := client.tokens.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error retrieving token: %v", err)
	}
	if tok != "simple-token" {
		t.Fatalf("expected simple-token, got %q", tok)
	}
}

func TestTokenProviderUsesRefreshToken(t *testing.T) {
	var calls int32
	refreshValue := "refresh-1"
	tokenSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			if got := r.FormValue("grant_type"); got != "client_credentials" {
				t.Fatalf("expected client_credentials grant, got %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "token-1",
				"refresh_token": refreshValue,
				"expires_in":    3600,
			})
		case 2:
			if got := r.FormValue("grant_type"); got != "refresh_token" {
				t.Fatalf("expected refresh_token grant, got %s", got)
			}
			if got := r.FormValue("refresh_token"); got != refreshValue {
				t.Fatalf("unexpected refresh token payload: %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "token-2",
				"refresh_token": "refresh-2",
				"expires_in":    3600,
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer tokenSrv.Close()

	client, err := New(Config{
		BaseURL:      "http://example",
		DeploymentID: "dep",
		TokenURL:     tokenSrv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing client: %v", err)
	}

	tok, err := client.tokens.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error retrieving token: %v", err)
	}
	if tok != "token-1" {
		t.Fatalf("unexpected initial token %q", tok)
	}

	client.tokens.mu.Lock()
	client.tokens.expiresAt = time.Now().Add(-time.Minute)
	client.tokens.mu.Unlock()

	refreshed, err := client.tokens.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error fetching refreshed token: %v", err)
	}
	if refreshed != "token-2" {
		t.Fatalf("unexpected refreshed token %q", refreshed)
	}

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("unexpected token endpoint calls: %d", got)
	}

	client.tokens.mu.Lock()
	defer client.tokens.mu.Unlock()
	if client.tokens.refresh != "refresh-2" {
		t.Fatalf("refresh token not updated, got %q", client.tokens.refresh)
	}
}

func TestQueryExternalAccounts(t *testing.T) {
	tokenSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token",
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	var capturedQuery url.Values
	apiSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		if _, err := w.Write([]byte(`{"ids":{"user-1":"puid-1"}}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer apiSrv.Close()

	client, err := New(Config{
		BaseURL:      apiSrv.URL,
		DeploymentID: "dep",
		TokenURL:     tokenSrv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing client: %v", err)
	}

	mapping, err := client.QueryExternalAccounts(context.Background(), "openId", []string{"user-1", ""})
	if err != nil {
		t.Fatalf("unexpected error querying external accounts: %v", err)
	}
	if puid := mapping["user-1"]; puid != "puid-1" {
		t.Fatalf("unexpected mapping: %+v", mapping)
	}
	if got := capturedQuery.Get("identityProviderId"); got != "openId" {
		t.Fatalf("unexpected identity provider: %s", got)
	}
	if entries := capturedQuery["accountId"]; len(entries) != 1 || entries[0] != "user-1" {
		t.Fatalf("unexpected account IDs: %v", capturedQuery["accountId"])
	}
}

func newHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping test: unable to listen on loopback: %v", err)
	}
	server := &httptest.Server{
		Listener: l,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}

func TestTokenProviderRefreshFallback(t *testing.T) {
	var calls int32
	tokenSrv := newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		switch atomic.AddInt32(&calls, 1) {
		case 1:
			if got := r.FormValue("grant_type"); got != "client_credentials" {
				t.Fatalf("expected client_credentials, got %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "token-initial",
				"refresh_token": "refresh-initial",
				"expires_in":    3600,
			})
		case 2:
			if got := r.FormValue("grant_type"); got != "refresh_token" {
				t.Fatalf("expected refresh_token grant, got %s", got)
			}
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid"})
		case 3:
			if got := r.FormValue("grant_type"); got != "client_credentials" {
				t.Fatalf("expected fallback client_credentials, got %s", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "token-fallback",
				"refresh_token": "refresh-fallback",
				"expires_in":    3600,
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer tokenSrv.Close()

	client, err := New(Config{
		BaseURL:      "http://example",
		DeploymentID: "dep",
		TokenURL:     tokenSrv.URL,
		ClientID:     "id",
		ClientSecret: "secret",
		HTTPClient:   &http.Client{Timeout: time.Second},
	})
	if err != nil {
		t.Fatalf("unexpected error constructing client: %v", err)
	}

	if _, err := client.tokens.Token(context.Background()); err != nil {
		t.Fatalf("unexpected error retrieving initial token: %v", err)
	}

	client.tokens.mu.Lock()
	client.tokens.expiresAt = time.Now().Add(-time.Minute)
	client.tokens.mu.Unlock()

	tok, err := client.tokens.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error retrieving fallback token: %v", err)
	}
	if tok != "token-fallback" {
		t.Fatalf("unexpected fallback token %q", tok)
	}

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("unexpected number of token calls: %d", got)
	}
}
