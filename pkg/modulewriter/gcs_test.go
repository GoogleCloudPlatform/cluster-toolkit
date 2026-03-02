/*
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package modulewriter writes modules to a deployment directory

package modulewriter

import (
	"hpc-toolkit/pkg/config"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
)

func TestGetBackendConfig(t *testing.T) {
	tests := []struct {
		name       string
		bp         config.Blueprint
		wantBucket string
		wantPrefix string
	}{
		{
			name: "NoBackend",
			bp: config.Blueprint{
				TerraformBackendDefaults: config.TerraformBackend{Type: "local"},
			},
			wantBucket: "",
			wantPrefix: "",
		},
		{
			name: "GCS_BucketOnly",
			bp: config.Blueprint{
				TerraformBackendDefaults: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.StringVal("my-bucket"),
					}),
				},
			},
			wantBucket: "my-bucket",
			wantPrefix: "",
		},
		{
			name: "GCS_BucketAndPrefix",
			bp: config.Blueprint{
				TerraformBackendDefaults: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.StringVal("my-bucket"),
						"prefix": cty.StringVal("my-prefix"),
					}),
				},
			},
			wantBucket: "my-bucket",
			wantPrefix: "my-prefix",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotBucket, gotPrefix := getBackendConfig(tc.bp)
			if diff := cmp.Diff(tc.wantBucket, gotBucket); diff != "" {
				t.Errorf("bucket mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantPrefix, gotPrefix); diff != "" {
				t.Errorf("prefix mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUploadArtifactsToBackend_Skip(t *testing.T) {
	// Test that it returns nil (no error) when backend is not GCS
	bp := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{Type: "local"},
	}
	if err := UploadArtifactsToBackend(bp, "."); err != nil {
		t.Errorf("UploadArtifactsToBackend failed unexpectedly: %v", err)
	}

	// Test that it returns nil when bucket is missing
	bpGCS := config.Blueprint{
		TerraformBackendDefaults: config.TerraformBackend{Type: "gcs"},
	}
	if err := UploadArtifactsToBackend(bpGCS, "."); err != nil {
		t.Errorf("UploadArtifactsToBackend failed unexpectedly on missing bucket: %v", err)
	}
}
