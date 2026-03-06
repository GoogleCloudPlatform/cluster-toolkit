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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hpc-toolkit/pkg/config"
)

func TestFlush(t *testing.T) {
	// Save the original global variables to restore them after testing
	origURL := ClearcutProdURL
	origTimeout := HttpServerTimeout

	defer func() {
		ClearcutProdURL = origURL
		HttpServerTimeout = origTimeout

	}()

	tests := []struct {
		name          string
		payload       LogRequest
		serverHandler http.HandlerFunc
		setupGlobals  func(serverURL string)
	}{
		{
			name:    "successful request sets correct headers and payload",
			payload: LogRequest{}, // Use a dummy/empty payload
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				// 1. Verify Method
				if r.Method != http.MethodPost {
					t.Errorf("Method = %s, want %s", r.Method, http.MethodPost)
				}

				// 2. Verify Query Params
				if gotFormat := r.URL.Query().Get("format"); gotFormat != "json_proto" {
					t.Errorf("Query param 'format' = %q, want %q", gotFormat, "json_proto")
				}

				// 3. Verify Headers
				if gotCT := r.Header.Get("Content-Type"); gotCT != "application/json" {
					t.Errorf("Header Content-Type = %q, want %q", gotCT, "application/json")
				}

				wantUA := "CLUSTER_TOOLKIT/" + config.GetToolkitVersion()
				if gotUA := r.Header.Get("User-Agent"); gotUA != wantUA {
					t.Errorf("Header User-Agent = %q, want %q", gotUA, wantUA)
				}

				// 4. Verify Body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Failed to read request body: %v", err)
				}

				expectedBody, _ := json.Marshal(LogRequest{})
				if string(body) != string(expectedBody) {
					t.Errorf("Body = %s, want %s", string(body), string(expectedBody))
				}

				// Return 200 OK
				w.WriteHeader(http.StatusOK)
			},
			setupGlobals: func(serverURL string) {
				ClearcutProdURL = serverURL
				HttpServerTimeout = 5 * time.Second
			},
		},
		{
			name:    "client handles network timeout error",
			payload: LogRequest{},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				// Artificially delay the response to trigger the client timeout
				time.Sleep(10 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
			setupGlobals: func(serverURL string) {
				ClearcutProdURL = serverURL
				// Set an aggressive timeout to ensure client.Do(req) fails
				HttpServerTimeout = 1 * time.Millisecond
			},
		},
		{
			name:          "invalid URL format prevents request creation",
			payload:       LogRequest{},
			serverHandler: nil, // Server won't be hit
			setupGlobals: func(_ string) {
				// A control character in the URL scheme forces http.NewRequest to return an error
				ClearcutProdURL = "http://192.168.0.%31/"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var serverURL string

			// Initialize the mock HTTP server if the test case provides a handler
			if tc.serverHandler != nil {
				ts := httptest.NewServer(tc.serverHandler)
				defer ts.Close()
				serverURL = ts.URL
			}

			// Apply test-specific globals (e.g., overriding the URL to the mock server's URL)
			if tc.setupGlobals != nil {
				tc.setupGlobals(serverURL)
			}

			// Execute the function. Because Flush() returns no error, test failures
			// for the "happy path" are primarily caught by the `serverHandler` assertions.
			// Network/parsing error paths are verified by ensuring no panics occur.
			Flush(tc.payload)
		})
	}
}
