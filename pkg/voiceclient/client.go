package voiceclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config defines the parameters required to communicate with the EOS Voice API.
type Config struct {
	BaseURL      string
	DeploymentID string
	TokenURL     string
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
}

// Participant defines the request payload for the create room token call.
type Participant struct {
	ProductUserID string
	ClientIP      string
	HardMuted     bool
}

// CreateRoomTokenResponse models the successful create-room-token response.
type CreateRoomTokenResponse struct {
	RoomID        string                       `json:"roomId"`
	ClientBaseURL string                       `json:"clientBaseUrl"`
	Participants  []CreateRoomTokenParticipant `json:"participants"`
	DeploymentID  string                       `json:"deploymentId"`
}

// CreateRoomTokenParticipant carries token information per participant.
type CreateRoomTokenParticipant struct {
	ProductUserID string `json:"puid"`
	Token         string `json:"token"`
	HardMuted     bool   `json:"hardMuted"`
}

// Client is a thin wrapper around the EOS Voice Web API.
type Client struct {
	baseURL      *url.URL
	deploymentID string
	httpClient   *http.Client
	tokens       *tokenProvider
}

// New creates a new voice client instance capable of minting its own access tokens.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("voiceclient: base URL is required")
	}
	if strings.TrimSpace(cfg.DeploymentID) == "" {
		return nil, fmt.Errorf("voiceclient: deployment ID is required")
	}
	if strings.TrimSpace(cfg.TokenURL) == "" {
		return nil, fmt.Errorf("voiceclient: token URL is required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, fmt.Errorf("voiceclient: client ID and secret are required")
	}

	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: invalid base URL: %w", err)
	}
	base.Path = "/"
	base.RawQuery = ""
	base.Fragment = ""

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	provider, err := newTokenProvider(tokenProviderConfig{
		TokenURL:     cfg.TokenURL,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		DeploymentID: cfg.DeploymentID,
		HTTPClient:   httpClient,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:      base,
		deploymentID: cfg.DeploymentID,
		httpClient:   httpClient,
		tokens:       provider,
	}, nil
}

// CreateRoomTokens issues create room token requests for the provided participants.
func (c *Client) CreateRoomTokens(ctx context.Context, roomID string, participants []Participant) (*CreateRoomTokenResponse, error) {
	if len(participants) == 0 {
		return nil, fmt.Errorf("voiceclient: at least one participant required")
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, fmt.Errorf("voiceclient: room ID is required")
	}

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: obtain token: %w", err)
	}

	reqBody := struct {
		Participants []struct {
			ProductUserID string `json:"puid"`
			ClientIP      string `json:"clientIP,omitempty"`
			HardMuted     bool   `json:"hardMuted"`
		} `json:"participants"`
	}{}

	for _, p := range participants {
		reqBody.Participants = append(reqBody.Participants, struct {
			ProductUserID string `json:"puid"`
			ClientIP      string `json:"clientIP,omitempty"`
			HardMuted     bool   `json:"hardMuted"`
		}{
			ProductUserID: p.ProductUserID,
			ClientIP:      strings.TrimSpace(p.ClientIP),
			HardMuted:     p.HardMuted,
		})
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: marshal payload: %w", err)
	}

	endpoint := fmt.Sprintf("rtc/v1/%s/room/%s", url.PathEscape(c.deploymentID), url.PathEscape(roomID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.makeURL(endpoint), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("voiceclient: create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("voiceclient: create room token %s failed: %d %s", roomID, resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var response CreateRoomTokenResponse
	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("voiceclient: decode response: %w", err)
	}

	return &response, nil
}

// RemoveParticipant removes a participant from a room, revoking their token.
func (c *Client) RemoveParticipant(ctx context.Context, roomID, productUserID string) error {
	if strings.TrimSpace(roomID) == "" || strings.TrimSpace(productUserID) == "" {
		return fmt.Errorf("voiceclient: room ID and product user ID are required")
	}

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return fmt.Errorf("voiceclient: obtain token: %w", err)
	}

	endpoint := fmt.Sprintf("rtc/v1/%s/room/%s/participants/%s", url.PathEscape(c.deploymentID), url.PathEscape(roomID), url.PathEscape(productUserID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.makeURL(endpoint), nil)
	if err != nil {
		return fmt.Errorf("voiceclient: create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("voiceclient: execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("voiceclient: remove participant %s from room %s failed: %d %s", productUserID, roomID, resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	return nil
}

// QueryExternalAccounts retrieves EOS Product User IDs for the provided external account IDs.
func (c *Client) QueryExternalAccounts(ctx context.Context, identityProviderID string, accountIDs []string) (map[string]string, error) {
	identityProviderID = strings.TrimSpace(identityProviderID)
	result := make(map[string]string, len(accountIDs))
	if len(accountIDs) == 0 {
		return result, nil
	}
	if identityProviderID == "" {
		return nil, fmt.Errorf("voiceclient: identity provider ID is required")
	}

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: obtain token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.makeURL("user/v1/accounts"), nil)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: create request: %w", err)
	}

	query := req.URL.Query()
	for _, id := range accountIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		query.Add("accountId", trimmed)
	}
	if len(query["accountId"]) == 0 {
		return result, nil
	}
	query.Set("identityProviderId", identityProviderID)
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voiceclient: execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("voiceclient: query external accounts failed: %d %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var data struct {
		IDs map[string]string `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("voiceclient: decode external accounts response: %w", err)
	}
	for k, v := range data.IDs {
		if strings.TrimSpace(v) == "" {
			continue
		}
		result[k] = v
	}

	return result, nil
}

func (c *Client) makeURL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	clone := *c.baseURL
	clone.Path = path
	clone.RawQuery = ""
	clone.Fragment = ""
	return clone.String()
}

// tokenProvider acquires and caches OAuth tokens for EOS.
type tokenProvider struct {
	clientID     string
	clientSecret string
	deploymentID string
	tokenURL     string
	httpClient   *http.Client

	mu        sync.Mutex
	value     string
	expiresAt time.Time
	refresh   string
}

type tokenProviderConfig struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	DeploymentID string
	HTTPClient   *http.Client
}

func newTokenProvider(cfg tokenProviderConfig) (*tokenProvider, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &tokenProvider{
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		deploymentID: cfg.DeploymentID,
		tokenURL:     cfg.TokenURL,
		httpClient:   cfg.HTTPClient,
	}, nil
}

func (p *tokenProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	if p.value != "" && time.Now().Add(30*time.Second).Before(p.expiresAt) {
		value := p.value
		p.mu.Unlock()
		return value, nil
	}
	refreshToken := p.refresh
	p.mu.Unlock()

	var combinedErr error
	if strings.TrimSpace(refreshToken) != "" {
		if err := p.obtainToken(ctx, url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {refreshToken},
			"deployment_id": {p.deploymentID},
		}); err == nil {
			return p.currentToken()
		} else {
			combinedErr = err
		}
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("deployment_id", p.deploymentID)

	if err := p.obtainToken(ctx, form); err != nil {
		if combinedErr != nil {
			return "", errors.Join(combinedErr, err)
		}
		return "", err
	}

	return p.currentToken()
}

func (p *tokenProvider) currentToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.value == "" {
		return "", fmt.Errorf("voiceclient: token provider not initialised")
	}
	return p.value, nil
}

func (p *tokenProvider) obtainToken(ctx context.Context, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("voiceclient: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	credentials := base64.StdEncoding.EncodeToString([]byte(p.clientID + ":" + p.clientSecret))
	req.Header.Set("Authorization", "Basic "+credentials)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("voiceclient: execute token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("voiceclient: token request failed: %d %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("voiceclient: decode token response: %w", err)
	}
	if strings.TrimSpace(body.AccessToken) == "" {
		return fmt.Errorf("voiceclient: token response missing access_token")
	}
	if !strings.EqualFold(body.TokenType, "bearer") && body.TokenType != "" {
		return fmt.Errorf("voiceclient: unsupported token type %q", body.TokenType)
	}

	expires := time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	if body.ExpiresIn == 0 {
		expires = time.Now().Add(5 * time.Minute)
	}

	p.mu.Lock()
	p.value = body.AccessToken
	p.expiresAt = expires
	if strings.TrimSpace(body.RefreshToken) != "" {
		p.refresh = body.RefreshToken
	} else if form.Get("grant_type") == "client_credentials" {
		p.refresh = ""
	}
	p.mu.Unlock()

	return nil
}
