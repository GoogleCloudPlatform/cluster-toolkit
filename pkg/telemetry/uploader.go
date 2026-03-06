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
	"net/http"
	"net/url"
)

func Flush(payload LogRequest) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshalling JSON: %v\n", err)
		return
	}

	client := &http.Client{
		Timeout: HttpServerTimeout,
	}

	u, err := url.Parse(ClearcutProdURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		return
	}
	params := url.Values{}
	params.Add("format", "json_proto")
	u.RawQuery = params.Encode()

	req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}
	req.Header.Set("User-Agent", fmt.Sprintf("CLUSTER_TOOLKIT/%v", config.GetToolkitVersion()))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		logging.Error("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()
}
