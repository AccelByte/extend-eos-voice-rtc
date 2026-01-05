// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"extend-eos-voice-rtc/pkg/common"
)

// EpicConfig holds Epic Games API configuration
type EpicConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	DeploymentID string
}

// EpicTokenResponse represents the OAuth token response from Epic Games
type EpicTokenResponse struct {
	AccessToken    string   `json:"access_token"`
	TokenType      string   `json:"token_type"`
	ExpiresAt      string   `json:"expires_at"`
	ExpiresIn      int      `json:"expires_in"`
	Features       []string `json:"features"`
	OrganizationID string   `json:"organization_id"`
	ProductID      string   `json:"product_id"`
	SandboxID      string   `json:"sandbox_id"`
	DeploymentID   string   `json:"deployment_id"`
}

// EpicGamesClient handles Epic Games API authentication and requests
type EpicGamesClient struct {
	config      *EpicConfig
	httpClient  *http.Client
	accessToken string
	expiresAt   time.Time
	tokenMutex  sync.RWMutex
	logger      *slog.Logger
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewEpicGamesClient creates a new Epic Games API client
func NewEpicGamesClient(logger *slog.Logger) (*EpicGamesClient, error) {
	config := &EpicConfig{
		BaseURL:      common.GetEnv("EPIC_BASE_URL", "https://api.epicgames.dev"),
		ClientID:     common.GetEnv("EPIC_CLIENT_ID", ""),
		ClientSecret: common.GetEnv("EPIC_CLIENT_SECRET", ""),
		DeploymentID: common.GetEnv("EPIC_DEPLOYMENT_ID", ""),
	}

	if config.ClientID == "" || config.ClientSecret == "" || config.DeploymentID == "" {
		return nil, fmt.Errorf("EPIC_CLIENT_ID, EPIC_CLIENT_SECRET, and EPIC_DEPLOYMENT_ID must be set")
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &EpicGamesClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initial authentication
	if err := client.authenticate(); err != nil {
		cancel()
		return nil, fmt.Errorf("initial authentication failed: %w", err)
	}

	// Start token refresh goroutine
	client.StartTokenRefresh()

	return client, nil
}

// authenticate performs client credentials flow to get access token
func (c *EpicGamesClient) authenticate() error {
	authURL := fmt.Sprintf("%s/auth/v1/oauth/token", c.config.BaseURL)

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("deployment_id", c.config.DeploymentID)

	req, err := http.NewRequest("POST", authURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.config.ClientID, c.config.ClientSecret)))
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", auth))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp EpicTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Update token with mutex
	c.tokenMutex.Lock()
	c.accessToken = tokenResp.AccessToken
	c.expiresAt, _ = time.Parse(time.RFC3339, tokenResp.ExpiresAt)
	c.tokenMutex.Unlock()

	c.logger.Info("Epic authentication successful", "expiresAt", c.expiresAt)
	return nil
}

// GetAccessToken returns the current access token (thread-safe)
func (c *EpicGamesClient) GetAccessToken() string {
	c.tokenMutex.RLock()
	defer c.tokenMutex.RUnlock()
	return c.accessToken
}

// StartTokenRefresh starts a goroutine to refresh the token periodically
func (c *EpicGamesClient) StartTokenRefresh() {
	go func() {
		// Refresh token every 50 minutes (tokens typically last ~60 minutes)
		ticker := time.NewTicker(50 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := c.authenticate(); err != nil {
					c.logger.Error("failed to refresh Epic token", "error", err)
				} else {
					c.logger.Info("Epic token refreshed successfully")
				}
			case <-c.ctx.Done():
				c.logger.Info("stopping Epic token refresh")
				return
			}
		}
	}()
}

// Shutdown gracefully stops the token refresh goroutine
func (c *EpicGamesClient) Shutdown() {
	if c.cancel != nil {
		c.cancel()
	}
}

// GetConfig returns the Epic configuration
func (c *EpicGamesClient) GetConfig() *EpicConfig {
	return c.config
}

// QueryExternalAccountsResponse represents the response from Epic Connect API
type QueryExternalAccountsResponse struct {
	IDs map[string]string `json:"ids"` // Map of AccelByte User ID to Epic PUID
}

// GetEpicPUID queries the Epic Connect API to map AccelByte User ID to Epic PUID
func (c *EpicGamesClient) GetEpicPUID(ctx context.Context, userIDs []string) (map[string]string, error) {
	queryURL := fmt.Sprintf("%s/user/v1/accounts", c.config.BaseURL)

	// Build query parameters
	params := url.Values{}
	for _, userID := range userIDs {
		params.Add("accountId", userID)
	}
	params.Set("identityProviderId", "openid")

	fullURL := fmt.Sprintf("%s?%s", queryURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.GetAccessToken()))
	req.Header.Set("Accept", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query external accounts failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var accountsResp QueryExternalAccountsResponse
	if err := json.NewDecoder(resp.Body).Decode(&accountsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return accountsResp.IDs, nil
}

// RTCParticipant represents a participant in the RTC room request
type RTCParticipant struct {
	PUID      string `json:"puid"`
	ClientIP  string `json:"clientIp,omitempty"`
	HardMuted bool   `json:"hardMuted"`
}

// CreateRoomTokenRequest represents the request to create a room token
type CreateRoomTokenRequest struct {
	Participants []RTCParticipant `json:"participants"`
}

// RTCParticipantResponse represents a participant in the RTC room response
type RTCParticipantResponse struct {
	PUID      string `json:"puid"`
	Token     string `json:"token"`
	HardMuted bool   `json:"hardMuted"`
}

// CreateRoomTokenResponse represents the response from creating a room token
type CreateRoomTokenResponse struct {
	RoomID        string                   `json:"roomId"`
	ClientBaseURL string                   `json:"clientBaseUrl"`
	Participants  []RTCParticipantResponse `json:"participants"`
	DeploymentID  string                   `json:"deploymentId"`
}

// CreateRoomToken calls the Epic RTC API to create a room token
func (c *EpicGamesClient) CreateRoomToken(ctx context.Context, roomID string, participants []RTCParticipant) (*CreateRoomTokenResponse, error) {
	rtcURL := fmt.Sprintf("%s/rtc/v1/%s/room/%s", c.config.BaseURL, c.config.DeploymentID, roomID)

	requestBody := CreateRoomTokenRequest{
		Participants: participants,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", rtcURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.GetAccessToken()))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create room token failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp CreateRoomTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &tokenResp, nil
}

// RemoveParticipant removes a participant from an RTC room (token revocation)
func (c *EpicGamesClient) RemoveParticipant(ctx context.Context, roomID string, puid string) error {
	rtcURL := fmt.Sprintf("%s/rtc/v1/%s/room/%s/participants/%s",
		c.config.BaseURL, c.config.DeploymentID, roomID, puid)

	req, err := http.NewRequestWithContext(ctx, "DELETE", rtcURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.GetAccessToken()))

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove participant failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
