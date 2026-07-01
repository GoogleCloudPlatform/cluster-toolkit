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
	"os"
	"testing"
)

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "Nil Error",
			err:      nil,
			expected: "",
		},
		{
			name:     "Permission Denied",
			err:      os.ErrPermission,
			expected: ErrTypePermissionDenied,
		},
		{
			name:     "File Not Exist",
			err:      os.ErrNotExist,
			expected: ErrTypeFileNotFound,
		},
		{
			name:     "Context Deadline Exceeded",
			err:      context.DeadlineExceeded,
			expected: ErrTypeTimeout,
		},
		{
			name:     "Context Canceled",
			err:      context.Canceled,
			expected: ErrTypeCanceled,
		},
		{
			name:     "Text Match Validation",
			err:      errors.New("some invalid configuration provided"),
			expected: ErrTypeValidation,
		},
		{
			name:     "Text Match Network",
			err:      errors.New("failed to dial tcp: connection refused"),
			expected: ErrTypeNetwork,
		},
		{
			name:     "Text Match Permission",
			err:      errors.New("server responded with 403 forbidden"),
			expected: ErrTypePermissionDenied,
		},
		{
			name:     "Text Match Not Found",
			err:      errors.New("resource not found"),
			expected: ErrTypeFileNotFound,
		},
		{
			name:     "Unknown Error",
			err:      errors.New("something went entirely wrong"),
			expected: ErrTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := categorizeError(tt.err); got != tt.expected {
				t.Errorf("categorizeError() = %v, want %v", got, tt.expected)
			}
		})
	}
}
