// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
)

const (
	ErrTypePermissionDenied = "PermissionDenied"
	ErrTypeFileNotFound     = "FileNotFound"
	ErrTypeValidation       = "ValidationError"
	ErrTypeNetwork          = "NetworkError"
	ErrTypeTimeout          = "TimeoutError"
	ErrTypeCanceled         = "CanceledError"
	ErrTypeUnknown          = "Unknown"
)

// categorizeError maps an error to a broad, PII-safe category.
func categorizeError(err error) string {
	if err == nil {
		return ""
	}

	// Standard Go error checks
	if errors.Is(err, os.ErrPermission) {
		return ErrTypePermissionDenied
	}
	if errors.Is(err, os.ErrNotExist) {
		return ErrTypeFileNotFound
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTypeTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrTypeCanceled
	}

	// Check for networking/timeout errors specifically
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrTypeTimeout
		}
		return ErrTypeNetwork
	}

	// Fallback string matching on safe keywords
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "permission denied") || strings.Contains(errMsg, "403 forbidden") || strings.Contains(errMsg, "access denied") {
		return ErrTypePermissionDenied
	}
	if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "404") {
		return ErrTypeFileNotFound
	}
	if strings.Contains(errMsg, "validation failed") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "malformed") {
		return ErrTypeValidation
	}
	if strings.Contains(errMsg, "timeout") {
		return ErrTypeTimeout
	}
	if strings.Contains(errMsg, "network") || strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "dial tcp") {
		return ErrTypeNetwork
	}

	return ErrTypeUnknown
}
