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
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var (
	metadata       = make(map[string]string)
	eventStartTime time.Time
)

func CollectPreMetrics(cmd *cobra.Command, args []string) {
	eventStartTime = time.Now()

	metadata[COMMAND_NAME] = getCommandName(cmd)
	metadata[IS_TEST_DATA] = getIsTestData()

}

func CollectPostMetrics(errorCode int) {
	metadata[RUNTIME_MS] = getRuntime()
	metadata[EXIT_CODE] = strconv.Itoa(errorCode)
}

func getCommandName(cmd *cobra.Command) string {
	return cmd.Name()
}

func getIsTestData() string {
	return "true"
}

func getRuntime() string {
	eventEndTime := time.Now()
	return strconv.FormatInt(eventEndTime.Sub(eventStartTime).Milliseconds(), 10)
}
