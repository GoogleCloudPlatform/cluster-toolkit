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
	"hpc-toolkit/pkg/config"

	"github.com/zclconf/go-cty/cty"
)

func getBlueprint(args []string) config.Blueprint {
	if len(args) == 0 {
		return config.Blueprint{}
	}
	bp, _, _ := config.NewBlueprint(args[0])
	return bp
}

func getEventMetadataKVPairs(sourceMetadata map[string]string) []map[string]string {
	eventMetadata := make([]map[string]string, 0)
	for k, v := range sourceMetadata {
		eventMetadata = append(eventMetadata, map[string]string{
			"key":   k,
			"value": v,
		})
	}
	return eventMetadata
}

func getKeyFromBlueprint(key string, bp config.Blueprint) string {
	val, err := bp.Eval(config.GlobalRef(key).AsValue())
	if err != nil {
		return ""
	}
	v, _ := val.Unmark()
	if !v.IsNull() && v.Type() == cty.String {
		return v.AsString()
	}
	return ""
}
