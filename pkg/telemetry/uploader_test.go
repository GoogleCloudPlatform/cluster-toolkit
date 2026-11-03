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
			t.Setenv(telemetryEndpointEnvVar, "")
			// Override the default transport for this specific test case
			http.DefaultTransport = &mockTransport{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					const wantProd = "https://play.googleapis.com/log?format=json_proto"
					if req.URL.String() != wantProd {
						t.Errorf("default Flush URL = %q, want %q", req.URL.String(), wantProd)
					}
					return tt.mockReturn(req)
				},
			}

			// Execute Flush. Since Flush logs errors instead of returning them,
			// we are primarily verifying that it doesn't panic and gracefully
			// handles all mocked HTTP responses/errors.
			Flush(tt.payload)
		})
	}
}

func TestFlushCustomEndpointOverride(t *testing.T) {
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()

	for _, tc := range []struct {
		name    string
		envURL  string
		wantURL string
	}{
		{
			name:    "IPv4 loopback routes locally and never hits clearcutProdURL",
			envURL:  "http://127.0.0.1:18080/log",
			wantURL: "http://127.0.0.1:18080/log?format=json_proto",
		},
		{
			name:    "localhost routes locally and preserves query params",
			envURL:  "http://localhost:8080?source=test",
			wantURL: "http://localhost:8080?format=json_proto&source=test",
		},
		{
			name:    "IPv6 loopback routes locally",
			envURL:  "http://[::1]:18080/log",
			wantURL: "http://[::1]:18080/log?format=json_proto",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(telemetryEndpointEnvVar, tc.envURL)
			var gotURL string
			http.DefaultTransport = &mockTransport{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					gotURL = req.URL.String()
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
						Header:     make(http.Header),
					}, nil
				},
			}

			Flush(LogRequest{LogSourceName: CONCORD})
			if gotURL != tc.wantURL {
				t.Errorf("Flush request URL = %q, want %q", gotURL, tc.wantURL)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		envURL string
	}{
		{name: "malformed URL fails closed", envURL: "not-a-valid-url"},
		{name: "external domain fails closed (blocks exfiltration)", envURL: "https://evil.example.com/log"},
		{name: "non-loopback private IP fails closed", envURL: "http://10.0.0.1:8080/log"},
		{name: "non-http scheme fails closed", envURL: "ftp://127.0.0.1:8080/log"},
		{name: "userinfo in URL fails closed", envURL: "http://user:pass@localhost:8080/log"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(telemetryEndpointEnvVar, tc.envURL)
			called := false
			http.DefaultTransport = &mockTransport{
				roundTripFunc: func(req *http.Request) (*http.Response, error) {
					called = true
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBuffer(nil))}, nil
				},
			}

			Flush(LogRequest{LogSourceName: CONCORD})
			if called {
				t.Errorf("Expected %q to abort HTTP call without hitting external host or prod", tc.envURL)
			}
		})
	}
}
