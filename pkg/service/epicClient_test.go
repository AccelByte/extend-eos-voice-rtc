// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"net/http/httptest"
	"net/http"
	"os"
	"testing"
	"time"
)

// mockRoundTripper is a mock HTTP transport for testing
type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestEpicGamesClient_GetEpicPUID(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		userIDs        []string
		mockResponse   *QueryExternalAccountsResponse
		mockStatusCode int
		expectError    bool
		expectedPUIDs  map[string]string
	}{
		{
			name:    "successfully get PUIDs for single user",
			userIDs: []string{"user-123"},
			mockResponse: &QueryExternalAccountsResponse{
				IDs: map[string]string{
					"user-123": "epic-puid-123",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
			expectedPUIDs: map[string]string{
				"user-123": "epic-puid-123",
			},
		},
		{
			name:    "successfully get PUIDs for multiple users",
			userIDs: []string{"user-123", "user-456"},
			mockResponse: &QueryExternalAccountsResponse{
				IDs: map[string]string{
					"user-123": "epic-puid-123",
					"user-456": "epic-puid-456",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
			expectedPUIDs: map[string]string{
				"user-123": "epic-puid-123",
				"user-456": "epic-puid-456",
			},
		},
		{
			name:    "user not linked to Epic returns empty map",
			userIDs: []string{"user-unlinked"},
			mockResponse: &QueryExternalAccountsResponse{
				IDs: map[string]string{},
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
			expectedPUIDs:  map[string]string{},
		},
		{
			name:           "API error returns error",
			userIDs:        []string{"user-123"},
			mockResponse:   nil,
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP client
			mockTransport := &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					// Verify request
					if req.URL.Query().Get("identityProviderId") != "openid" {
						t.Errorf("Expected identityProviderId=openid, got %s", req.URL.Query().Get("identityProviderId"))
					}

					// Create response
					var body []byte
					if tt.mockResponse != nil {
						body, _ = json.Marshal(tt.mockResponse)
					}

					return &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(bytes.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				},
			}

			client := &EpicGamesClient{
				config: &EpicConfig{
					BaseURL:      "https://api.epicgames.dev",
					ClientID:     "test-client",
					ClientSecret: "test-secret",
					DeploymentID: "test-deployment",
				},
				httpClient: &http.Client{
					Transport: mockTransport,
				},
				accessToken: "mock-token",
				logger:      logger,
			}

			puids, err := client.GetEpicPUID(context.Background(), tt.userIDs)

			if tt.expectError {
				if err == nil {
					t.Errorf("GetEpicPUID() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("GetEpicPUID() unexpected error: %v", err)
					return
				}
				if len(puids) != len(tt.expectedPUIDs) {
					t.Errorf("GetEpicPUID() returned %d PUIDs, want %d", len(puids), len(tt.expectedPUIDs))
					return
				}
				for userID, expectedPUID := range tt.expectedPUIDs {
					if puids[userID] != expectedPUID {
						t.Errorf("GetEpicPUID() PUID for %s = %s, want %s", userID, puids[userID], expectedPUID)
					}
				}
			}
		})
	}
}

func TestEpicGamesClient_CreateRoomToken(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		roomID         string
		participants   []RTCParticipant
		mockResponse   *CreateRoomTokenResponse
		mockStatusCode int
		expectError    bool
	}{
		{
			name:   "successfully create room token",
			roomID: "party-123:Voice",
			participants: []RTCParticipant{
				{
					PUID:      "epic-puid-123",
					ClientIP:  "192.168.1.1",
					HardMuted: false,
				},
			},
			mockResponse: &CreateRoomTokenResponse{
				RoomID:        "party-123:Voice",
				ClientBaseURL: "wss://voice.epicgames.com",
				Participants: []RTCParticipantResponse{
					{
						PUID:      "epic-puid-123",
						Token:     "voice-token-xyz",
						HardMuted: false,
					},
				},
				DeploymentID: "test-deployment",
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:   "create room token with hard muted",
			roomID: "session-456:team-1",
			participants: []RTCParticipant{
				{
					PUID:      "epic-puid-456",
					ClientIP:  "10.0.0.1",
					HardMuted: true,
				},
			},
			mockResponse: &CreateRoomTokenResponse{
				RoomID:        "session-456:team-1",
				ClientBaseURL: "wss://voice.epicgames.com",
				Participants: []RTCParticipantResponse{
					{
						PUID:      "epic-puid-456",
						Token:     "voice-token-abc",
						HardMuted: true,
					},
				},
				DeploymentID: "test-deployment",
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:   "API error returns error",
			roomID: "error-room",
			participants: []RTCParticipant{
				{
					PUID:      "epic-puid-999",
					ClientIP:  "192.168.1.1",
					HardMuted: false,
				},
			},
			mockResponse:   nil,
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP client
			mockTransport := &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					// Verify request method and headers
					if req.Method != "POST" {
						t.Errorf("Expected POST request, got %s", req.Method)
					}
					if req.Header.Get("Content-Type") != "application/json" {
						t.Errorf("Expected Content-Type: application/json")
					}

					// Create response
					var body []byte
					if tt.mockResponse != nil {
						body, _ = json.Marshal(tt.mockResponse)
					}

					return &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(bytes.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				},
			}

			client := &EpicGamesClient{
				config: &EpicConfig{
					BaseURL:      "https://api.epicgames.dev",
					ClientID:     "test-client",
					ClientSecret: "test-secret",
					DeploymentID: "test-deployment",
				},
				httpClient: &http.Client{
					Transport: mockTransport,
				},
				accessToken: "mock-token",
				logger:      logger,
			}

			resp, err := client.CreateRoomToken(context.Background(), tt.roomID, tt.participants)

			if tt.expectError {
				if err == nil {
					t.Errorf("CreateRoomToken() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CreateRoomToken() unexpected error: %v", err)
					return
				}
				if resp.RoomID != tt.mockResponse.RoomID {
					t.Errorf("CreateRoomToken() RoomID = %s, want %s", resp.RoomID, tt.mockResponse.RoomID)
				}
				if resp.ClientBaseURL != tt.mockResponse.ClientBaseURL {
					t.Errorf("CreateRoomToken() ClientBaseURL = %s, want %s", resp.ClientBaseURL, tt.mockResponse.ClientBaseURL)
				}
				if len(resp.Participants) != len(tt.mockResponse.Participants) {
					t.Errorf("CreateRoomToken() participants count = %d, want %d", len(resp.Participants), len(tt.mockResponse.Participants))
				}
			}
		})
	}
}

func TestEpicGamesClient_RemoveParticipant(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tests := []struct {
		name           string
		roomID         string
		puid           string
		mockStatusCode int
		expectError    bool
	}{
		{
			name:           "successfully remove participant",
			roomID:         "party-123:Voice",
			puid:           "epic-puid-123",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "successfully remove participant with 200 OK",
			roomID:         "session-456:Voice",
			puid:           "epic-puid-456",
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "API error returns error",
			roomID:         "error-room",
			puid:           "epic-puid-999",
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP client
			mockTransport := &mockRoundTripper{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					// Verify request method
					if req.Method != "DELETE" {
						t.Errorf("Expected DELETE request, got %s", req.Method)
					}

					return &http.Response{
						StatusCode: tt.mockStatusCode,
						Body:       io.NopCloser(bytes.NewReader([]byte{})),
						Header:     make(http.Header),
					}, nil
				},
			}

			client := &EpicGamesClient{
				config: &EpicConfig{
					BaseURL:      "https://api.epicgames.dev",
					ClientID:     "test-client",
					ClientSecret: "test-secret",
					DeploymentID: "test-deployment",
				},
				httpClient: &http.Client{
					Transport: mockTransport,
				},
				accessToken: "mock-token",
				logger:      logger,
			}

			err := client.RemoveParticipant(context.Background(), tt.roomID, tt.puid)

			if tt.expectError {
				if err == nil {
					t.Errorf("RemoveParticipant() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("RemoveParticipant() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestEpicGamesClient_GetAccessToken(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client := &EpicGamesClient{
		config: &EpicConfig{
			BaseURL:      "https://api.epicgames.dev",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			DeploymentID: "test-deployment",
		},
		accessToken: "test-access-token",
		logger:      logger,
	}

	token := client.GetAccessToken()
	if token != "test-access-token" {
		t.Errorf("GetAccessToken() = %s, want %s", token, "test-access-token")
	}
}

func TestEpicGamesClient_Shutdown(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, cancel := context.WithCancel(context.Background())

	client := &EpicGamesClient{
		config: &EpicConfig{
			BaseURL:      "https://api.epicgames.dev",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			DeploymentID: "test-deployment",
		},
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	// Start a goroutine that checks if context is cancelled
	done := make(chan bool)
	go func() {
		<-client.ctx.Done()
		done <- true
	}()

	// Shutdown the client
	client.Shutdown()

	// Wait for context cancellation or timeout
	select {
	case <-done:
		// Context was cancelled as expected
	case <-time.After(1 * time.Second):
		t.Error("Shutdown() did not cancel context within timeout")
	}
}

func TestEpicGamesClient_GetConfig(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	expectedConfig := &EpicConfig{
		BaseURL:      "https://api.epicgames.dev",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		DeploymentID: "test-deployment",
	}

	client := &EpicGamesClient{
		config: expectedConfig,
		logger: logger,
	}

	config := client.GetConfig()
	if config != expectedConfig {
		t.Error("GetConfig() did not return the correct config")
	}
	if config.BaseURL != expectedConfig.BaseURL {
		t.Errorf("GetConfig().BaseURL = %s, want %s", config.BaseURL, expectedConfig.BaseURL)
	}
	if config.ClientID != expectedConfig.ClientID {
		t.Errorf("GetConfig().ClientID = %s, want %s", config.ClientID, expectedConfig.ClientID)
	}
}

func TestNewEpicGamesClientMissingEnv(t *testing.T) {
	t.Setenv("EPIC_CLIENT_ID", "")
	t.Setenv("EPIC_CLIENT_SECRET", "")
	t.Setenv("EPIC_DEPLOYMENT_ID", "")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if _, err := NewEpicGamesClient(logger); err == nil {
		t.Fatalf("expected error for missing epic env vars")
	}
}

func TestNewEpicGamesClientSuccess(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	expiresAt := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request")
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatalf("expected authorization header")
		}

		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		if values.Get("grant_type") != "client_credentials" {
			t.Fatalf("expected grant_type client_credentials")
		}
		if values.Get("deployment_id") != "deployment-1" {
			t.Fatalf("expected deployment_id")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"token-1","token_type":"bearer","expires_at":"` + expiresAt + `"}`))
	}))
	defer server.Close()

	t.Setenv("EPIC_BASE_URL", server.URL)
	t.Setenv("EPIC_CLIENT_ID", "client-1")
	t.Setenv("EPIC_CLIENT_SECRET", "secret-1")
	t.Setenv("EPIC_DEPLOYMENT_ID", "deployment-1")

	client, err := NewEpicGamesClient(logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.GetAccessToken() == "" {
		t.Fatalf("expected access token to be set")
	}
	client.Shutdown()
}

func TestEpicGamesClientAuthenticateError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mockTransport := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString("error")),
				Header:     make(http.Header),
			}, nil
		},
	}

	client := &EpicGamesClient{
		config: &EpicConfig{
			BaseURL:      "https://api.epicgames.dev",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			DeploymentID: "test-deployment",
		},
		httpClient: &http.Client{Transport: mockTransport},
		logger:     logger,
	}

	if err := client.authenticate(); err == nil {
		t.Fatalf("expected authenticate error")
	}
}
