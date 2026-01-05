// Copyright (c) 2023-2025 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package service

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVoiceError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *VoiceError
		expected string
	}{
		{
			name:     "user not in party error",
			err:      ErrUserNotInParty,
			expected: "40301: You must join a party before calling this endpoint.",
		},
		{
			name:     "user not in session error",
			err:      ErrUserNotInSession,
			expected: "40302: You must join a session before calling this endpoint.",
		},
		{
			name:     "epic account not linked error",
			err:      ErrEpicAccountNotLinked,
			expected: "40303: Your account is not linked to an Epic Games account.",
		},
		{
			name:     "user not in team error",
			err:      ErrUserNotInTeam,
			expected: "40304: You are not assigned to a team in this session.",
		},
		{
			name:     "epic authentication error",
			err:      ErrEpicAuthentication,
			expected: "50001: Epic Games authentication failed.",
		},
		{
			name:     "epic rtc api error",
			err:      ErrEpicRTCAPI,
			expected: "50002: Epic RTC API error.",
		},
		{
			name:     "accelbyte api error",
			err:      ErrAccelByteAPI,
			expected: "50003: AccelByte API error.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("Error() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVoiceError_ToGRPCStatus(t *testing.T) {
	tests := []struct {
		name         string
		err          *VoiceError
		expectedCode codes.Code
	}{
		{
			name:         "40301 maps to PermissionDenied",
			err:          ErrUserNotInParty,
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "40302 maps to PermissionDenied",
			err:          ErrUserNotInSession,
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "40304 maps to PermissionDenied",
			err:          ErrUserNotInTeam,
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "40303 maps to FailedPrecondition",
			err:          ErrEpicAccountNotLinked,
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "50001 maps to Internal",
			err:          ErrEpicAuthentication,
			expectedCode: codes.Internal,
		},
		{
			name:         "50002 maps to Internal",
			err:          ErrEpicRTCAPI,
			expectedCode: codes.Internal,
		},
		{
			name:         "50003 maps to Internal",
			err:          ErrAccelByteAPI,
			expectedCode: codes.Internal,
		},
		{
			name: "unknown code maps to Unknown",
			err: &VoiceError{
				Code:    "99999",
				Message: "Unknown error",
			},
			expectedCode: codes.Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := tt.err.ToGRPCStatus()
			st, ok := status.FromError(grpcErr)
			if !ok {
				t.Errorf("ToGRPCStatus() did not return a status error")
				return
			}
			if st.Code() != tt.expectedCode {
				t.Errorf("ToGRPCStatus() code = %v, want %v", st.Code(), tt.expectedCode)
			}
			if st.Message() != tt.err.Message {
				t.Errorf("ToGRPCStatus() message = %v, want %v", st.Message(), tt.err.Message)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name         string
		baseErr      *VoiceError
		wrappedErr   error
		expectedCode codes.Code
		expectMsg    string
	}{
		{
			name:         "wrap with nil error",
			baseErr:      ErrEpicRTCAPI,
			wrappedErr:   nil,
			expectedCode: codes.Internal,
			expectMsg:    "Epic RTC API error.",
		},
		{
			name:         "wrap with actual error",
			baseErr:      ErrEpicRTCAPI,
			wrappedErr:   errors.New("connection timeout"),
			expectedCode: codes.Internal,
			expectMsg:    "Epic RTC API error.: connection timeout",
		},
		{
			name:         "wrap AccelByte API error",
			baseErr:      ErrAccelByteAPI,
			wrappedErr:   errors.New("session not found"),
			expectedCode: codes.Internal,
			expectMsg:    "AccelByte API error.: session not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := WrapError(tt.baseErr, tt.wrappedErr)
			st, ok := status.FromError(grpcErr)
			if !ok {
				t.Errorf("WrapError() did not return a status error")
				return
			}
			if st.Code() != tt.expectedCode {
				t.Errorf("WrapError() code = %v, want %v", st.Code(), tt.expectedCode)
			}
			if st.Message() != tt.expectMsg {
				t.Errorf("WrapError() message = %v, want %v", st.Message(), tt.expectMsg)
			}
		})
	}
}
