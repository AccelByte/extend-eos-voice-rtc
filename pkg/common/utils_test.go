// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string][]string
		expected string
	}{
		{
			name: "extract from X-Forwarded-For with single IP",
			metadata: map[string][]string{
				"x-forwarded-for": {"192.168.1.100"},
			},
			expected: "192.168.1.100",
		},
		{
			name: "extract from X-Forwarded-For with multiple IPs",
			metadata: map[string][]string{
				"x-forwarded-for": {"192.168.1.100, 10.0.0.1, 172.16.0.1"},
			},
			expected: "192.168.1.100",
		},
		{
			name: "extract from X-Real-IP when X-Forwarded-For is missing",
			metadata: map[string][]string{
				"x-real-ip": {"192.168.1.200"},
			},
			expected: "192.168.1.200",
		},
		{
			name: "extract from CF-Connecting-IP when others are missing",
			metadata: map[string][]string{
				"cf-connecting-ip": {"192.168.1.300"},
			},
			expected: "192.168.1.300",
		},
		{
			name: "X-Forwarded-For takes priority over X-Real-IP",
			metadata: map[string][]string{
				"x-forwarded-for": {"192.168.1.100"},
				"x-real-ip":       {"192.168.1.200"},
			},
			expected: "192.168.1.100",
		},
		{
			name: "X-Real-IP takes priority over CF-Connecting-IP",
			metadata: map[string][]string{
				"x-real-ip":        {"192.168.1.200"},
				"cf-connecting-ip": {"192.168.1.300"},
			},
			expected: "192.168.1.200",
		},
		{
			name:     "return empty string when no metadata",
			metadata: map[string][]string{},
			expected: "",
		},
		{
			name: "return empty string when no IP headers",
			metadata: map[string][]string{
				"other-header": {"value"},
			},
			expected: "",
		},
		{
			name: "handle whitespace in X-Forwarded-For",
			metadata: map[string][]string{
				"x-forwarded-for": {"  192.168.1.100  , 10.0.0.1"},
			},
			expected: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.MD(tt.metadata)
			ctx := metadata.NewIncomingContext(context.Background(), md)

			got := ExtractClientIP(ctx)
			if got != tt.expected {
				t.Errorf("ExtractClientIP() = %v, want %v", got, tt.expected)
			}
		})
	}

	t.Run("return empty string when context has no metadata", func(t *testing.T) {
		ctx := context.Background()
		got := ExtractClientIP(ctx)
		if got != "" {
			t.Errorf("ExtractClientIP() = %v, want empty string", got)
		}
	})
}

func TestGetUserIDFromContext(t *testing.T) {
	// Create a valid JWT token payload for testing
	// Format: {"sub":"user-123","namespace":"test-namespace"}
	// Base64URL encoded: eyJzdWIiOiJ1c2VyLTEyMyIsIm5hbWVzcGFjZSI6InRlc3QtbmFtZXNwYWNlIn0
	validPayload := "eyJzdWIiOiJ1c2VyLTEyMyIsIm5hbWVzcGFjZSI6InRlc3QtbmFtZXNwYWNlIn0"
	validToken := "header." + validPayload + ".signature"

	// Token with empty sub
	// Format: {"sub":"","namespace":"test-namespace"}
	// Base64URL encoded: eyJzdWIiOiIiLCJuYW1lc3BhY2UiOiJ0ZXN0LW5hbWVzcGFjZSJ9
	emptySubPayload := "eyJzdWIiOiIiLCJuYW1lc3BhY2UiOiJ0ZXN0LW5hbWVzcGFjZSJ9"
	emptySubToken := "header." + emptySubPayload + ".signature"

	tests := []struct {
		name         string
		metadata     map[string][]string
		expectedUser string
		expectError  bool
		expectedCode codes.Code
	}{
		{
			name: "successfully extract user ID",
			metadata: map[string][]string{
				"authorization": {"Bearer " + validToken},
			},
			expectedUser: "user-123",
			expectError:  false,
		},
		{
			name: "token without Bearer prefix",
			metadata: map[string][]string{
				"authorization": {validToken},
			},
			expectedUser: "user-123",
			expectError:  false,
		},
		{
			name:         "missing metadata",
			metadata:     nil,
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:         "missing authorization header",
			metadata:     map[string][]string{},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name: "empty authorization header",
			metadata: map[string][]string{
				"authorization": {},
			},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name: "invalid token format - only 2 parts",
			metadata: map[string][]string{
				"authorization": {"Bearer header.payload"},
			},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name: "invalid token format - only 1 part",
			metadata: map[string][]string{
				"authorization": {"Bearer justonepart"},
			},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name: "invalid base64 payload",
			metadata: map[string][]string{
				"authorization": {"Bearer header.!!!invalid!!!.signature"},
			},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name: "invalid JSON in payload",
			metadata: map[string][]string{
				"authorization": {"Bearer header.bm90anNvbg.signature"}, // "notjson" in base64
			},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name: "empty sub in token",
			metadata: map[string][]string{
				"authorization": {"Bearer " + emptySubToken},
			},
			expectError:  true,
			expectedCode: codes.Unauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx context.Context
			if tt.metadata != nil {
				md := metadata.MD(tt.metadata)
				ctx = metadata.NewIncomingContext(context.Background(), md)
			} else {
				ctx = context.Background()
			}

			userID, err := GetUserIDFromContext(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("GetUserIDFromContext() expected error, got nil")
					return
				}
				st, ok := status.FromError(err)
				if !ok {
					t.Errorf("GetUserIDFromContext() error is not a status error")
					return
				}
				if st.Code() != tt.expectedCode {
					t.Errorf("GetUserIDFromContext() error code = %v, want %v", st.Code(), tt.expectedCode)
				}
			} else {
				if err != nil {
					t.Errorf("GetUserIDFromContext() unexpected error: %v", err)
					return
				}
				if userID != tt.expectedUser {
					t.Errorf("GetUserIDFromContext() = %v, want %v", userID, tt.expectedUser)
				}
			}
		})
	}
}
