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
	"bytes"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func resolveTelemetryEndpoint() (*url.URL, error) {
	custom := strings.TrimSpace(os.Getenv(telemetryEndpointEnvVar))
	if custom == "" {
		return url.Parse(clearcutProdURL)
	}
	u, err := url.Parse(custom)
	if err != nil {
		return nil, err
	}
	if !isAllowedCustomTelemetryURL(u) {
		return nil, fmt.Errorf("custom telemetry endpoint %q must be an http(s) loopback URL (localhost, 127.0.0.1, or ::1)", custom)
	}
	return u, nil
}

func isAllowedCustomTelemetryURL(u *url.URL) bool {
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" || u.User != nil {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func Flush(payload LogRequest) {

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logging.Error("Error marshalling Telemetry request JSON: %v", err)
		return
	}

	client := &http.Client{
		Timeout: uploadTimeout,
	}
	u, err := resolveTelemetryEndpoint()
	if err != nil {
		logging.Error("Error resolving Telemetry endpoint URL: %v", err)
		return
	}
	params := u.Query()
	params.Set("format", "json_proto")
	u.RawQuery = params.Encode()

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		logging.Error("Error creating Telemetry request to Clearcut: %v", err)
		return
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%v/%v", CLUSTER_TOOLKIT, config.GetToolkitVersion()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		logging.Error("Error sending Telemetry request to Clearcut: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		logging.Error("Telemetry request failed with status %d: %s", resp.StatusCode, string(body))
	}
}
