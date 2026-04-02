// Copyright 2026 Google LLC
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

package gcloud

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

var (
	gcloudCache sync.Map // map[string][]byte (cmd args string -> output)
	execCommand = exec.Command
)

// RunGcloudJsonCommand runs a gcloud command and automatically appends --format=json.
// It caches the result based on the arguments to prevent duplicate executions.
func RunGcloudJsonCommand(args ...string) ([]byte, error) {
	// Append --format=json if not already there
	formatSpecified := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--format=") {
			formatSpecified = true
			break
		}
	}
	if !formatSpecified {
		args = append(args, "--format=json")
	}

	cacheKey := strings.Join(args, " ")
	if v, ok := gcloudCache.Load(cacheKey); ok {
		return v.([]byte), nil
	}

	cmd := execCommand("gcloud", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute gcloud %s: %w. Output: %s", cacheKey, err, string(out))
	}

	gcloudCache.Store(cacheKey, out)
	return out, nil
}
