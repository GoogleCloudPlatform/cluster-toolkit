// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"
)

// mockTransport implements http.RoundTripper to intercept HTTP calls
type mockTransport struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestFlush(t *testing.T) {
	// Save the original transport and restore it after tests run
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	tests := []struct {
		name       string
		payload    LogRequest
		mockReturn func(req *http.Request) (*http.Response, error)
	}{
		{
			name: "Success (200 OK)",
			payload: LogRequest{
				LogSourceName: CONCORD,
			},
			mockReturn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"status":"ok"}`)),
					Header:     make(http.Header),
				}, nil
			},
		},
		{
			name: "HTTP Error Response (400 Bad Request)",
			payload: LogRequest{
				LogSourceName: CONCORD,
			},
			mockReturn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(bytes.NewBufferString(`bad request`)),
					Header:     make(http.Header),
				}, nil
			},
		},
		{
			name: "Network Error",
			payload: LogRequest{
				LogSourceName: CONCORD,
			},
			mockReturn: func(req *http.Request) (*http.Response, error) {
				// Simulate a timeout or network-level error
				return nil, errors.New("network timeout")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override the default transport for this specific test case
			http.DefaultTransport = &mockTransport{
				roundTripFunc: tt.mockReturn,
			}

			// Execute Flush. Since Flush logs errors instead of returning them,
			// we are primarily verifying that it doesn't panic and gracefully
			// handles all mocked HTTP responses/errors.
			Flush(tt.payload)
		})
	}
}
