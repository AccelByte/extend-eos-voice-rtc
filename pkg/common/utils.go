// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func GetEnvInt(key string, fallback int) int {
	str := GetEnv(key, strconv.Itoa(fallback))
	val, err := strconv.Atoi(str)
	if err != nil {
		return fallback
	}

	return val
}

func GetBasePath() string {
	basePath := os.Getenv("BASE_PATH")
	if basePath == "" {
		slog.Error("BASE_PATH envar is not set or empty")
		os.Exit(1)
	}
	if !strings.HasPrefix(basePath, "/") {
		slog.Error("BASE_PATH envar is invalid, no leading '/' found. Valid example: /basePath")
		os.Exit(1)
	}

	return basePath
}

// ExtractClientIP extracts the client IP address from gRPC metadata
// Priority order: X-Forwarded-For (first IP), X-Real-IP, CF-Connecting-IP
func ExtractClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	// Try X-Forwarded-For first (take the first IP in the list)
	if ips := md.Get("x-forwarded-for"); len(ips) > 0 {
		// X-Forwarded-For can contain multiple IPs (client, proxy1, proxy2)
		// First IP is the original client
		parts := strings.Split(ips[0], ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Try X-Real-IP (nginx)
	if ips := md.Get("x-real-ip"); len(ips) > 0 {
		return ips[0]
	}

	// Try CF-Connecting-IP (Cloudflare)
	if ips := md.Get("cf-connecting-ip"); len(ips) > 0 {
		return ips[0]
	}

	return ""
}

// JWTClaims represents JWT token claims
type JWTClaims struct {
	Sub       string `json:"sub"`       // Subject (User ID)
	Namespace string `json:"namespace"` // Namespace
	// Add other claims as needed
}

// GetUserIDFromContext extracts the user ID from the JWT token in the gRPC metadata
func GetUserIDFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "metadata is missing")
	}

	authHeaders, ok := md["authorization"]
	if !ok || len(authHeaders) == 0 {
		return "", status.Error(codes.Unauthenticated, "authorization header is missing")
	}

	// Extract token from "Bearer <token>"
	token := strings.TrimPrefix(authHeaders[0], "Bearer ")

	// JWT tokens are in format: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", status.Error(codes.Unauthenticated, "invalid token format")
	}

	// Decode the payload (second part)
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", status.Error(codes.Unauthenticated, "failed to decode token payload")
	}

	// Parse claims
	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", status.Error(codes.Unauthenticated, "failed to parse token claims")
	}

	if claims.Sub == "" {
		return "", status.Error(codes.Unauthenticated, "user ID not found in token")
	}

	return claims.Sub, nil
}
