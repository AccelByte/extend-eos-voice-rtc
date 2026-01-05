// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VoiceError represents a custom error type for voice service
type VoiceError struct {
	Code    string
	Message string
}

func (e *VoiceError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ToGRPCStatus converts VoiceError to gRPC status error
func (e *VoiceError) ToGRPCStatus() error {
	var code codes.Code
	switch e.Code {
	case "40301", "40302", "40304":
		code = codes.PermissionDenied
	case "40303":
		code = codes.FailedPrecondition
	case "40401", "40402", "40403":
		code = codes.NotFound
	case "42901":
		code = codes.ResourceExhausted
	case "50001", "50002", "50003":
		code = codes.Internal
	default:
		code = codes.Unknown
	}
	return status.Error(code, e.Message)
}

// Predefined voice errors
var (
	// 403 Permission errors
	ErrUserNotInParty = &VoiceError{
		Code:    "40301",
		Message: "You must join a party before calling this endpoint.",
	}
	ErrUserNotInSession = &VoiceError{
		Code:    "40302",
		Message: "You must join a session before calling this endpoint.",
	}
	ErrEpicAccountNotLinked = &VoiceError{
		Code:    "40303",
		Message: "Your account is not linked to an Epic Games account.",
	}
	ErrUserNotInTeam = &VoiceError{
		Code:    "40304",
		Message: "You are not assigned to a team in this session.",
	}

	// 404 Not Found errors
	ErrPartyNotFound = &VoiceError{
		Code:    "40401",
		Message: "Party not found.",
	}
	ErrSessionNotFound = &VoiceError{
		Code:    "40402",
		Message: "Session not found.",
	}
	ErrUserNotFound = &VoiceError{
		Code:    "40403",
		Message: "User not found.",
	}

	// 429 Rate Limit errors
	ErrRateLimitExceeded = &VoiceError{
		Code:    "42901",
		Message: "Rate limit exceeded. Please try again later.",
	}

	// 500 Internal errors
	ErrEpicAuthentication = &VoiceError{
		Code:    "50001",
		Message: "Epic Games authentication failed.",
	}
	ErrEpicRTCAPI = &VoiceError{
		Code:    "50002",
		Message: "Epic RTC API error.",
	}
	ErrAccelByteAPI = &VoiceError{
		Code:    "50003",
		Message: "AccelByte API error.",
	}
)

// WrapError wraps an error with additional context and returns a gRPC status error
func WrapError(baseErr *VoiceError, err error) error {
	if err == nil {
		return baseErr.ToGRPCStatus()
	}
	wrappedErr := &VoiceError{
		Code:    baseErr.Code,
		Message: fmt.Sprintf("%s: %v", baseErr.Message, err),
	}
	return wrappedErr.ToGRPCStatus()
}

// ParseHTTPError converts HTTP error to appropriate VoiceError based on status code
func ParseHTTPError(err error, context string) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Check for 404 Not Found
	if strings.Contains(errStr, "status 404") || strings.Contains(errStr, "not found") || strings.Contains(errStr, "NotFound") {
		switch context {
		case "party":
			return ErrPartyNotFound.ToGRPCStatus()
		case "session":
			return ErrSessionNotFound.ToGRPCStatus()
		case "user", "epic_puid":
			return ErrUserNotFound.ToGRPCStatus()
		default:
			return status.Error(codes.NotFound, errStr)
		}
	}

	// Check for 403 Forbidden
	if strings.Contains(errStr, "status 403") || strings.Contains(errStr, "forbidden") || strings.Contains(errStr, "Forbidden") {
		return status.Error(codes.PermissionDenied, errStr)
	}

	// Check for 429 Rate Limit
	if strings.Contains(errStr, "status 429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "too many requests") {
		return ErrRateLimitExceeded.ToGRPCStatus()
	}

	// Check for 400 Bad Request
	if strings.Contains(errStr, "status 400") || strings.Contains(errStr, "bad request") || strings.Contains(errStr, "BadRequest") {
		return status.Error(codes.InvalidArgument, errStr)
	}

	// Check for 401 Unauthorized
	if strings.Contains(errStr, "status 401") || strings.Contains(errStr, "unauthorized") || strings.Contains(errStr, "Unauthorized") {
		return status.Error(codes.Unauthenticated, errStr)
	}

	// Default to Internal for 5xx or unknown errors
	if context == "epic_rtc" || context == "epic_puid" {
		return WrapError(ErrEpicRTCAPI, err)
	}
	return WrapError(ErrAccelByteAPI, err)
}
