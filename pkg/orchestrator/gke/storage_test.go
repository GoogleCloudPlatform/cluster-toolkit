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

package gke

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"cloud.google.com/go/filestore/apiv1/filestorepb"
	crm "google.golang.org/api/cloudresourcemanager/v1"
	iamapi "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	gcs "google.golang.org/api/storage/v1"
	k8syaml "sigs.k8s.io/yaml"
)

func TestParseSingleVolume(t *testing.T) {
	sm := &StorageManager{}

	tests := []struct {
		name        string
		input       string
		wantSrc     string
		wantDest    string
		wantRO      bool
		wantOpts    string
		wantProfile string
		wantSubPath string
		wantAttrs   map[string]string
		wantErr     bool
		wantErrSub  string
	}{
		{
			name:     "valid hostPath",
			input:    "/host/path;/container/path",
			wantSrc:  "/host/path",
			wantDest: "/container/path",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid hostPath rw",
			input:    "/host/path;/container/path;rw",
			wantSrc:  "/host/path",
			wantDest: "/container/path",
			wantRO:   false,
			wantErr:  false,
		},
		{
			name:     "valid pvc",
			input:    "my-pvc;/data",
			wantSrc:  "my-pvc",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid gcsfuse",
			input:    "gs://my-bucket;/data",
			wantSrc:  "gs://my-bucket",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore",
			input:    "filestore://my-instance/share;/data",
			wantSrc:  "filestore://my-instance/share",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore with port",
			input:    "filestore://10.0.0.2:2049/share;/data",
			wantSrc:  "filestore://10.0.0.2:2049/share",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore with port and mode",
			input:    "filestore://10.0.0.2:2049/share;/data;rw",
			wantSrc:  "filestore://10.0.0.2:2049/share",
			wantDest: "/data",
			wantRO:   false,
			wantErr:  false,
		},
		{
			name:       "invalid scheme",
			input:      "parallelstore://my-instance/share;/data",
			wantErr:    true,
			wantErrSub: "Unsupported scheme",
		},
		{
			name:       "invalid scheme with mode",
			input:      "parallelstore://my-instance/share;/data;ro",
			wantErr:    true,
			wantErrSub: "Unsupported scheme",
		},
		{
			name:       "missing destination for gcsfuse",
			input:      "gs://my-bucket",
			wantErr:    true,
			wantErrSub: "Missing destination",
		},
		{
			name:       "missing destination for filestore",
			input:      "filestore://my-instance/share",
			wantErr:    true,
			wantErrSub: "Missing destination",
		},
		{
			name:       "missing destination for gcsfuse with mode",
			input:      "gs://my-bucket;rw",
			wantErr:    true,
			wantErrSub: "Missing destination",
		},
		{
			name:       "missing destination for filestore with mode",
			input:      "filestore://my-instance/share;ro",
			wantErr:    true,
			wantErrSub: "Missing destination",
		},
		{
			name:       "invalid format missing dest",
			input:      "host/path",
			wantErr:    true,
			wantErrSub: "invalid volume format",
		},
		{
			name:     "valid filestore IPv6",
			input:    "filestore://[2001:db8::1]/share;/data",
			wantSrc:  "filestore://[2001:db8::1]/share",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore IPv6 rw",
			input:    "filestore://[2001:db8::1]/share;/data;rw",
			wantSrc:  "filestore://[2001:db8::1]/share",
			wantDest: "/data",
			wantRO:   false,
			wantErr:  false,
		},
		{
			name:       "missing destination for filestore IPv6",
			input:      "filestore://[2001:db8::1]/share",
			wantErr:    true,
			wantErrSub: "Missing destination",
		},
		{
			name:     "valid gcsfuse with options",
			input:    "gs://my-bucket;/data;options=logging:severity:info",
			wantSrc:  "gs://my-bucket",
			wantDest: "/data",
			wantRO:   true,
			wantOpts: "logging:severity:info",
		},
		{
			name:     "valid gcsfuse with options and rw",
			input:    "gs://my-bucket;/data;rw;options=logging:severity:info",
			wantSrc:  "gs://my-bucket",
			wantDest: "/data",
			wantRO:   false,
			wantOpts: "logging:severity:info",
		},
		{
			name:     "valid gcsfuse with options and rw flipped",
			input:    "gs://my-bucket;/data;options=logging:severity:info;rw",
			wantSrc:  "gs://my-bucket",
			wantDest: "/data",
			wantRO:   false,
			wantOpts: "logging:severity:info",
		},
		{
			name:       "options not supported for non-gcs",
			input:      "my-pvc;/data;options=abc",
			wantErr:    true,
			wantErrSub: "options= can only be used with GCS",
		},
		{
			name:       "every unsupported segment is reported in one error",
			input:      "filestore://my-instance/share;/data;options=abc;profile=training;attributes=x=1",
			wantErr:    true,
			wantErrSub: "options=, profile=, attributes= can only be used with GCS",
		},
		{
			name:     "valid gcsfuse with multiple comma-separated options",
			input:    "gs://my-bucket;/data;options=logging:severity:info,enable-atomic-rename-object:true",
			wantSrc:  "gs://my-bucket",
			wantDest: "/data",
			wantRO:   true,
			wantOpts: "logging:severity:info,enable-atomic-rename-object:true",
		},
		{
			name:        "profile alias training",
			input:       "gs://my-bucket;/data;profile=training",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
		},
		{
			name:        "profile alias checkpointing rw",
			input:       "gs://my-bucket/run1;/checkpoints;rw;profile=checkpointing",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/checkpoints",
			wantRO:      false,
			wantProfile: "gcsfusecsi-checkpointing",
			wantSubPath: "run1",
		},
		{
			name:        "inline gcs subpath is split from bucket",
			input:       "gs://my-bucket/datasets/imagenet;/data",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantSubPath: "datasets/imagenet",
		},
		{
			name:        "profile canonical storage class name",
			input:       "gs://my-bucket;/data;profile=gcsfusecsi-serving",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-serving",
		},
		{
			name:        "profile is case insensitive",
			input:       "gs://my-bucket;/data;profile=Training",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
		},
		{
			name:        "profile with options and attributes",
			input:       "gs://my-bucket;/models;ro;profile=serving;options=implicit-dirs;attributes=anywhereCacheZones=us-central1-a",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/models",
			wantRO:      true,
			wantOpts:    "implicit-dirs",
			wantProfile: "gcsfusecsi-serving",
			wantAttrs:   map[string]string{"anywhereCacheZones": "us-central1-a"},
		},
		{
			name:        "multiple attributes",
			input:       "gs://my-bucket;/models;profile=serving;attributes=a=1,b=2",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/models",
			wantRO:      true,
			wantProfile: "gcsfusecsi-serving",
			wantAttrs:   map[string]string{"a": "1", "b": "2"},
		},
		{
			name:       "unknown profile is rejected",
			input:      "gs://my-bucket;/data;profile=bogus",
			wantErr:    true,
			wantErrSub: "unsupported storage profile",
		},
		{
			name:       "empty profile is rejected",
			input:      "gs://my-bucket;/data;profile=",
			wantErr:    true,
			wantErrSub: "unsupported storage profile",
		},
		{
			name:       "profile not supported for filestore",
			input:      "filestore://my-instance/share;/data;profile=training",
			wantErr:    true,
			wantErrSub: "profile= can only be used with GCS fuse volumes",
		},
		{
			name:       "profile not supported for pvc",
			input:      "my-pvc;/data;profile=training",
			wantErr:    true,
			wantErrSub: "profile= can only be used with GCS fuse volumes",
		},
		{
			// volumeAttributes is one CSI field that exists on an inline
			// ephemeral volume as well as on a PersistentVolume, so attributes=
			// must not be gated behind profile=.
			name:      "attributes are accepted without a profile",
			input:     "gs://my-bucket;/data;attributes=fileCacheCapacity=100Gi",
			wantSrc:   "gs://my-bucket",
			wantDest:  "/data",
			wantRO:    true,
			wantAttrs: map[string]string{"fileCacheCapacity": "100Gi"},
		},
		{
			// Profile-only keys are warned about at build time, not rejected:
			// a hard allowlist would also block attributes added by a newer
			// driver release.
			name:      "storage-profile-only attributes are not rejected without a profile",
			input:     "gs://my-bucket;/data;attributes=anywhereCacheZones=us-central1-a",
			wantSrc:   "gs://my-bucket",
			wantDest:  "/data",
			wantRO:    true,
			wantAttrs: map[string]string{"anywhereCacheZones": "us-central1-a"},
		},
		{
			// options= already feeds this field on both paths.
			name:       "mountOptions is rejected as an attribute",
			input:      "gs://my-bucket;/data;attributes=mountOptions=implicit-dirs",
			wantErr:    true,
			wantErrSub: "use options=",
		},
		{
			// Whitespace is trimmed on both sides of the '=', so a naturally
			// typed ", " separator does not smuggle a leading space into the
			// value that the CSI driver would then see.
			name:        "whitespace around attribute keys and values is trimmed",
			input:       "gs://my-bucket;/data;profile=training;attributes= a =1, b = 2 ",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs:   map[string]string{"a": "1", "b": "2"},
		},
		{
			// SplitN(kv, "=", 2): only the first '=' separates, so a value may
			// contain as many more as it likes.
			name:        "an attribute value may contain further '=' characters",
			input:       "gs://my-bucket;/data;profile=training;attributes=url=https://example.com/a=b",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs:   map[string]string{"url": "https://example.com/a=b"},
		},
		{
			name:       "attributes without value are rejected",
			input:      "gs://my-bucket;/data;profile=training;attributes=novalue",
			wantErr:    true,
			wantErrSub: "invalid volume attribute",
		},
		{
			name:       "empty attributes are rejected",
			input:      "gs://my-bucket;/data;profile=training;attributes=",
			wantErr:    true,
			wantErrSub: "no key=value pairs",
		},
		{
			name:       "attribute key with illegal characters is rejected",
			input:      "gs://my-bucket;/data;profile=training;attributes=bad key=1",
			wantErr:    true,
			wantErrSub: "invalid volume attribute key",
		},
		{
			name:       "duplicate attribute key is rejected",
			input:      "gs://my-bucket;/data;profile=training;attributes=a=1,a=2",
			wantErr:    true,
			wantErrSub: "duplicate volume attribute key",
		},
		{
			// The Rapid Cache example in docs/gcluster_job_guide.md. The value
			// is only dangerous once it reaches YAML, where a bare '*' would be
			// an alias node; the template quotes it.
			name:        "anywhereCacheZones wildcard is accepted",
			input:       "gs://my-bucket;/data;profile=training;attributes=anywhereCacheZones=*",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs:   map[string]string{"anywhereCacheZones": "*"},
		},
		{
			// Previously rejected: attributes= split on every ',', so the
			// second zone was read as a key. A ',' now only separates when a
			// '<key>=' follows it, which "us-central1-b" does not.
			name:        "anywhereCacheZones multi-zone list is accepted",
			input:       "gs://my-bucket;/data;profile=training;attributes=anywhereCacheZones=us-central1-a,us-central1-b",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs:   map[string]string{"anywhereCacheZones": "us-central1-a,us-central1-b"},
		},
		{
			// The other documented GKE parameter whose value is a comma
			// separated list. ':' is not a legal key character, so none of the
			// internal commas look like the start of a new pair.
			name:        "fuseFileCacheMediumPriority list is accepted",
			input:       "gs://my-bucket;/data;profile=training;attributes=fuseFileCacheMediumPriority=gpu:ram|lssd,tpu:ram,general_purpose:ram|lssd",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs: map[string]string{
				"fuseFileCacheMediumPriority": "gpu:ram|lssd,tpu:ram,general_purpose:ram|lssd",
			},
		},
		{
			// A list value followed by a genuine second pair: the ',' before
			// 'bucketScanTimeout=' separates, the ones inside the zone list do
			// not.
			name:        "list value and a following pair are both parsed",
			input:       "gs://my-bucket;/data;profile=training;attributes=anywhereCacheZones=us-central1-a,us-central1-b,bucketScanTimeout=5m",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs: map[string]string{
				"anywhereCacheZones": "us-central1-a,us-central1-b",
				"bucketScanTimeout":  "5m",
			},
		},
		{
			// A trailing ',' terminated a (skipped) empty segment before the
			// change and must keep doing so, rather than becoming part of the
			// preceding value.
			name:        "trailing comma is still a separator",
			input:       "gs://my-bucket;/data;profile=training;attributes=bucketScanTimeout=5m,",
			wantSrc:     "gs://my-bucket",
			wantDest:    "/data",
			wantRO:      true,
			wantProfile: "gcsfusecsi-training",
			wantAttrs:   map[string]string{"bucketScanTimeout": "5m"},
		},
		{
			name:       "repeated attributes= is rejected",
			input:      "gs://my-bucket;/data;profile=training;attributes=a=1;attributes=b=2",
			wantErr:    true,
			wantErrSub: "attributes= may only be specified once per --mount",
		},
		{
			name:       "repeated options= is rejected",
			input:      "gs://my-bucket;/data;options=implicit-dirs;options=only-dir=x",
			wantErr:    true,
			wantErrSub: "options= may only be specified once per --mount",
		},
		{
			name:       "repeated profile= is rejected",
			input:      "gs://my-bucket;/data;profile=training;profile=serving",
			wantErr:    true,
			wantErrSub: "profile= may only be specified once per --mount",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pm, err := sm.parseSingleVolume(tc.input)

			if (err != nil) != tc.wantErr {
				t.Fatalf("parseSingleVolume() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if tc.wantErrSub != "" && !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error message = %q, want sub %q", err.Error(), tc.wantErrSub)
				}
				return
			}

			want := parsedMount{
				Src:        tc.wantSrc,
				Dest:       tc.wantDest,
				ReadOnly:   tc.wantRO,
				Options:    tc.wantOpts,
				Profile:    tc.wantProfile,
				SubPath:    tc.wantSubPath,
				Attributes: tc.wantAttrs,
			}
			if !reflect.DeepEqual(pm, want) {
				t.Errorf("parseSingleVolume() = %+v, want %+v", pm, want)
			}
		})
	}
}

func TestValidateMounts(t *testing.T) {
	sm := &StorageManager{}

	type testCase struct {
		name      string
		mounts    []string
		wantErr   bool
		errSubstr string
	}

	tests := []testCase{
		{
			name: "duplicate dest",
			mounts: []string{
				"gs://my-bucket;/data",
				"my-pvc;/data",
			},
			wantErr:   true,
			errSubstr: "duplicate volume destination",
		},
		{
			name: "duplicate src",
			mounts: []string{
				"gs://my-bucket;/data1",
				"gs://my-bucket;/data2",
			},
			wantErr:   true,
			errSubstr: "duplicate volume source",
		},
		{
			name: "unsupported scheme",
			mounts: []string{
				"parallelstore://foo;/data",
			},
			wantErr:   true,
			errSubstr: "Unsupported scheme",
		},
		{
			name: "root directory",
			mounts: []string{
				"gs://my-bucket;/",
			},
			wantErr:   true,
			errSubstr: "cannot be the root directory",
		},
		{
			name: "valid mounts",
			mounts: []string{
				"gs://my-bucket;/data1",
				"my-pvc;/data2",
			},
			wantErr: false,
		},
	}

	for _, reserved := range []string{"/dev", "/proc", "/sys", "/etc", "/bin", "/sbin", "/usr", "/lib", "/lib64"} {
		tests = append(tests, testCase{
			name:      "reserved dir " + reserved,
			mounts:    []string{"gs://my-bucket;" + reserved},
			wantErr:   true,
			errSubstr: "cannot be a reserved system directory",
		})
	}

	for _, reserved := range []string{"/proc/sys", "/etc/kubernetes", "/usr/local/bin"} {
		tests = append(tests, testCase{
			name:      "nested reserved dir " + reserved,
			mounts:    []string{"gs://my-bucket;" + reserved},
			wantErr:   true,
			errSubstr: "cannot be within a reserved system directory",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.ValidateMounts(tt.mounts)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got %v", tt.errSubstr, err)
				}
			} else if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestValidateRamdiskDir(t *testing.T) {
	sm := &StorageManager{}

	type testCase struct {
		name       string
		ramdiskDir string
		rawMounts  []string
		wantErr    bool
		errSubstr  string
	}

	tests := []testCase{
		{
			name:       "Empty Ramdisk - Pass",
			ramdiskDir: "",
		},
		{
			name:       "Relative Path - Fail",
			ramdiskDir: "relative/path",
			wantErr:    true,
			errSubstr:  "--gke-mtc-ramdisk-dir must be an absolute path",
		},
		{
			name:       "Root Directory - Fail",
			ramdiskDir: "/",
			wantErr:    true,
			errSubstr:  "cannot be the root directory",
		},
		{
			name:       "Mount Conflict - Fail",
			ramdiskDir: "/tmp/ramdisk",
			rawMounts:  []string{"gs://bucket;/tmp/ramdisk"},
			wantErr:    true,
			errSubstr:  "conflicts with duplicate mount destination",
		},
		{
			name:       "Valid Path without Conflict - Pass",
			ramdiskDir: "/tmp/ramdisk",
			rawMounts:  []string{"gs://bucket;/data"},
		},
	}

	for _, reserved := range []string{"/dev", "/proc", "/sys", "/etc", "/bin", "/sbin", "/usr", "/lib", "/lib64", "/proc/"} {
		tests = append(tests, testCase{
			name:       "Reserved Dir " + reserved + " - Fail",
			ramdiskDir: reserved,
			wantErr:    true,
			errSubstr:  "cannot be a reserved system directory",
		})
	}

	for _, reserved := range []string{"/proc/sys", "/etc/kubernetes", "/usr/local/bin"} {
		tests = append(tests, testCase{
			name:       "Nested Reserved Dir " + reserved + " - Fail",
			ramdiskDir: reserved,
			wantErr:    true,
			errSubstr:  "cannot be within a reserved system directory",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.ValidateRamdiskDir(tt.ramdiskDir, tt.rawMounts)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got %v", tt.errSubstr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestProcessMounts_Basic(t *testing.T) {
	sm := &StorageManager{}
	job := orchestrator.JobDefinition{}

	mounts := []string{
		"gs://my-bucket;/data",
		"/host/path;/host",
		"my-pvc;/pvc",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}

	if len(infos) != 3 {
		t.Fatalf("expected 3 mount infos, got %d", len(infos))
	}

	expectedTypes := []string{"gcsfuse", "hostPath", "pvc"}
	for i, info := range infos {
		if info.Type != expectedTypes[i] {
			t.Errorf("expected type %s, got %s", expectedTypes[i], info.Type)
		}
	}
}

func TestProcessMounts_Filestore_IP(t *testing.T) {
	sm := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			if isIP && nameOrIP == "10.0.0.2" {
				return "10.0.0.2", "10-0-0-2", 1024, nil
			}
			return "", "", 0, fmt.Errorf("unexpected query")
		},
	}
	job := orchestrator.JobDefinition{}

	// Test valid IP-based Filestore mount
	mounts := []string{
		"filestore://10.0.0.2/share;/data",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	info := infos[0]
	if info.Type != "pvc" || info.Source != "gcluster-filestore-10-0-0-2-share" {
		t.Errorf("unexpected mount info: %+v", info)
	}

	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	verifyFilestoreManifest(t, manifests[0], "gcluster-filestore-10-0-0-2-share", "10.0.0.2", "/share", "1024Gi")
}

func TestProcessMounts_Filestore_Sanitize(t *testing.T) {
	sm := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			if isIP && nameOrIP == "10.0.0.2" {
				return "10.0.0.2", "10-0-0-2", 1024, nil
			}
			return "", "", 0, fmt.Errorf("unexpected query")
		},
	}
	job := orchestrator.JobDefinition{}

	// Test sanitization of PVC Name (lowercase, underscores and slashes to hyphens)
	mounts := []string{
		"filestore://10.0.0.2/MY_complex_SHARE/sub_folder/sub_subfolder;/data",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	info := infos[0]
	expectedPVCName := "gcluster-filestore-10-0-0-2-my-complex-share-sub-folder-sub-subfolder"
	if info.Source != expectedPVCName {
		t.Errorf("expected source %s, got %s", expectedPVCName, info.Source)
	}
	verifyFilestoreManifest(t, manifests[0], expectedPVCName, "10.0.0.2", "/MY_complex_SHARE/sub_folder/sub_subfolder", "1024Gi")

	// Test double/leading slashes in share name
	mounts = []string{
		"filestore://10.0.0.2//my_share_name;/data",
	}
	infos, manifests, err = sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	if infos[0].Source != "gcluster-filestore-10-0-0-2-my-share-name" {
		t.Errorf("expected source gcluster-filestore-10-0-0-2-my-share-name, got %s", infos[0].Source)
	}
	verifyFilestoreManifest(t, manifests[0], "gcluster-filestore-10-0-0-2-my-share-name", "10.0.0.2", "/my_share_name", "1024Gi")

	// Test special characters in share name (collapse multiple hyphens, trim trailing hyphens)
	mounts = []string{
		"filestore://10.0.0.2/share@name.with-dots_and_stuff!;/data",
	}
	infos, manifests, err = sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	expectedPVCName = "gcluster-filestore-10-0-0-2-share-name-with-dots-and-stuff"
	if infos[0].Source != expectedPVCName {
		t.Errorf("expected source %s, got %s", expectedPVCName, infos[0].Source)
	}
	verifyFilestoreManifest(t, manifests[0], expectedPVCName, "10.0.0.2", "/share@name.with-dots_and_stuff!", "1024Gi")
}

func TestProcessMounts_Filestore_Mock(t *testing.T) {
	// Test valid non-IP Filestore mount using a mocked client resolver hook
	smMock := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			if !isIP && projectID == "my-project" && location == "us-central1-a" && nameOrIP == "my-filestore-instance" {
				return "10.11.12.13", "my-filestore-instance", 1024, nil
			}
			return "", "", 0, fmt.Errorf("unexpected query params: %s, %s, %s", projectID, location, nameOrIP)
		},
	}
	jobMock := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-central1-a",
	}
	mountsMock := []string{
		"filestore://my-filestore-instance/my_share;/data",
	}

	infosMock, manifestsMock, errMock := smMock.ProcessMounts(mountsMock, jobMock)
	if errMock != nil {
		t.Fatalf("unexpected error: %v", errMock)
	}
	if len(infosMock) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infosMock))
	}
	if infosMock[0].Source != "gcluster-filestore-my-filestore-instance-my-share" {
		t.Errorf("expected source gcluster-filestore-my-filestore-instance-my-share, got %s", infosMock[0].Source)
	}
	verifyFilestoreManifest(t, manifestsMock[0], "gcluster-filestore-my-filestore-instance-my-share", "10.11.12.13", "/my_share", "1024Gi")
}

func TestProcessMounts_Filestore_TrailingSlash(t *testing.T) {
	sm := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			if isIP && nameOrIP == "10.0.0.2" {
				return "10.0.0.2", "10-0-0-2", 1024, nil
			}
			return "", "", 0, fmt.Errorf("unexpected query")
		},
	}
	job := orchestrator.JobDefinition{}

	// Test trailing slash handling in Filestore mount URI (single and multiple).
	// Both spellings resolve to the same instance and share, so they collapse
	// onto one PVC/PV: the manifest is emitted once and the Pod declares one
	// volume, while each mount keeps its own mount path.
	mountsSlash := []string{
		"filestore://10.0.0.2/share/;/data",
		"filestore://10.0.0.2/share///;/data2",
	}
	infosSlash, manifestsSlash, errSlash := sm.ProcessMounts(mountsSlash, job)
	if errSlash != nil {
		t.Fatalf("unexpected error: %v", errSlash)
	}
	if len(infosSlash) != 2 {
		t.Fatalf("expected 2 mount infos, got %d", len(infosSlash))
	}
	if infosSlash[0].Source != "gcluster-filestore-10-0-0-2-share" {
		t.Errorf("expected source gcluster-filestore-10-0-0-2-share, got %s", infosSlash[0].Source)
	}
	if infosSlash[1].Source != "gcluster-filestore-10-0-0-2-share" {
		t.Errorf("expected source gcluster-filestore-10-0-0-2-share, got %s", infosSlash[1].Source)
	}
	if infosSlash[0].MountPath != "/data" || infosSlash[1].MountPath != "/data2" {
		t.Errorf("expected mount paths /data and /data2, got %q and %q", infosSlash[0].MountPath, infosSlash[1].MountPath)
	}
	if infosSlash[0].Name != infosSlash[1].Name {
		t.Errorf("expected both mounts to reuse one Pod volume, got %q and %q", infosSlash[0].Name, infosSlash[1].Name)
	}
	if len(manifestsSlash) != 1 {
		t.Fatalf("expected the shared PVC/PV manifest to be emitted once, got %d manifests", len(manifestsSlash))
	}
	verifyFilestoreManifest(t, manifestsSlash[0], "gcluster-filestore-10-0-0-2-share", "10.0.0.2", "/share", "1024Gi")
}

func verifyFilestoreManifest(t *testing.T, manifest, name, server, path, capacity string) {
	// Verify that namespace: default is NOT in the PVC manifest
	if strings.Contains(manifest, "namespace:") {
		t.Errorf("manifest contains namespace, which should be omitted: %s", manifest)
	}
	// Verify PVC Name
	if !strings.Contains(manifest, "name: "+name) {
		t.Errorf("manifest missing expected PVC name: %s", manifest)
	}
	// Verify PV Name (with -default suffix in tests)
	expectedPVName := name + "-default"
	if !strings.Contains(manifest, "name: "+expectedPVName) {
		t.Errorf("manifest missing expected PV name %s: %s", expectedPVName, manifest)
	}
	// Verify PVC binds to PV
	if !strings.Contains(manifest, "volumeName: "+expectedPVName) {
		t.Errorf("manifest missing expected volumeName binding %s: %s", expectedPVName, manifest)
	}
	if !strings.Contains(manifest, "server: "+server) {
		t.Errorf("manifest missing expected server IP: %s", manifest)
	}
	if !strings.Contains(manifest, "path: "+path) {
		t.Errorf("manifest missing expected path: %s", manifest)
	}
	if !strings.Contains(manifest, "storage: "+capacity) {
		t.Errorf("manifest missing expected capacity %s, got:\n%s", capacity, manifest)
	}
}

func TestProcessMounts_Filestore_SingleMountGolden(t *testing.T) {
	sm := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			return "10.0.0.2", "myinstance", 2048, nil
		},
	}

	infos, manifests, err := sm.ProcessMounts([]string{"filestore://myinstance/share;/data"}, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantInfo := MountInfo{
		Name:      "vol-0",
		Source:    "gcluster-filestore-myinstance-share",
		MountPath: "/data",
		Type:      "pvc",
		ReadOnly:  true,
	}
	if len(infos) != 1 || !reflect.DeepEqual(infos[0], wantInfo) {
		t.Fatalf("MountInfo = %#v, want exactly one %#v", infos, wantInfo)
	}

	if len(manifests) != 1 {
		t.Fatalf("expected exactly 1 manifest, got %d", len(manifests))
	}
	const wantManifest = `apiVersion: v1
kind: PersistentVolume
metadata:
  name: gcluster-filestore-myinstance-share-default
  labels:
    gcluster.google.com/managed-by: cluster-toolkit
    gcluster.google.com/storage-type: filestore
spec:
  capacity:
    storage: 2048Gi
  accessModes:
  - ReadWriteMany
  nfs:
    path: /share
    server: 10.0.0.2
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: gcluster-filestore-myinstance-share
  labels:
    gcluster.google.com/managed-by: cluster-toolkit
    gcluster.google.com/storage-type: filestore
spec:
  accessModes:
  - ReadWriteMany
  storageClassName: ""
  volumeName: gcluster-filestore-myinstance-share-default
  resources:
    requests:
      storage: 2048Gi
`
	if manifests[0] != wantManifest {
		t.Errorf("rendered Filestore manifest changed.\n--- got ---\n%s\n--- want ---\n%s", manifests[0], wantManifest)
	}

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)

	const wantVolumes = `              - name: vol-0
                persistentVolumeClaim:
                  claimName: gcluster-filestore-myinstance-share`
	if opts.VolumesYAML != wantVolumes {
		t.Errorf("Pod volumes changed.\n--- got ---\n%s\n--- want ---\n%s", opts.VolumesYAML, wantVolumes)
	}

	const wantMounts = `                - mountPath: /data
                  name: vol-0
                  readOnly: true`
	if opts.VolumeMountsYAML != wantMounts {
		t.Errorf("Pod volumeMounts changed.\n--- got ---\n%s\n--- want ---\n%s", opts.VolumeMountsYAML, wantMounts)
	}

	if opts.GCSFuseEnabled {
		t.Error("GCSFuseEnabled must stay false for a Filestore-only job")
	}
}

func TestProcessMounts_Filestore_Invalid(t *testing.T) {
	sm := &StorageManager{}
	job := orchestrator.JobDefinition{}

	// Test invalid empty share name
	_, _, err := sm.ProcessMounts([]string{"filestore://10.0.0.2/;/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "Expected format: filestore://") {
		t.Errorf("expected error for empty share name, got %v", err)
	}

	// Test invalid empty share name with multiple slashes
	_, _, err = sm.ProcessMounts([]string{"filestore://10.0.0.2///;/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "Expected format: filestore://") {
		t.Errorf("expected error for empty share name with multiple slashes, got %v", err)
	}

	// Test invalid empty instance name
	_, _, err = sm.ProcessMounts([]string{"filestore:///share;/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "Expected format: filestore://") {
		t.Errorf("expected error for empty instance name, got %v", err)
	}

	// Test mock Filestore client returning lookup error
	smMockError := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			return "", "", 0, fmt.Errorf("filestore API lookup failed")
		},
	}
	_, _, err = smMockError.ProcessMounts([]string{"filestore://my-instance/share;/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "filestore API lookup failed") {
		t.Errorf("expected API lookup error, got %v", err)
	}
}

func TestAddVolumeOptions(t *testing.T) {
	sm := &StorageManager{}
	opts := &ManifestOptions{}

	vols := []MountInfo{
		{
			Name:      "vol-0",
			Source:    "gs://my-bucket",
			MountPath: "/data1",
			Type:      "gcsfuse",
			ReadOnly:  true,
		},
		{
			Name:      "vol-1",
			Source:    "/host/path",
			MountPath: "/data2",
			Type:      "hostPath",
			ReadOnly:  false,
		},
		{
			Name:      "vol-2",
			Source:    "my-pvc",
			MountPath: "/data3",
			Type:      "pvc",
			ReadOnly:  true,
		},
	}

	sm.AddVolumeOptions(opts, vols)

	// Verify volumeMounts YAML contains readOnly for read-only mounts
	actualVolumeMounts := opts.VolumeMountsYAML
	if !strings.Contains(actualVolumeMounts, "readOnly: true") {
		t.Errorf("expected readOnly: true in volume mounts, got:\n%s", actualVolumeMounts)
	}

	// Verify vol-1 (readOnly = false) does NOT contain readOnly: true
	lines := strings.Split(actualVolumeMounts, "\n")
	var vol1LineIdx = -1
	for idx, line := range lines {
		if strings.Contains(line, "name: vol-1") {
			vol1LineIdx = idx
			break
		}
	}
	if vol1LineIdx == -1 {
		t.Fatalf("could not find vol-1 in volume mounts, got:\n%s", actualVolumeMounts)
	}
	// Verify next few lines don't contain readOnly: true
	for i := vol1LineIdx + 1; i < len(lines) && i < vol1LineIdx+3; i++ {
		if strings.Contains(lines[i], "readOnly:") {
			t.Errorf("expected vol-1 to not have readOnly field, but found: %s", lines[i])
		}
	}

	// Verify volumes YAML
	if !strings.Contains(opts.VolumesYAML, "persistentVolumeClaim") {
		t.Errorf("expected persistentVolumeClaim in volumes, got:\n%s", opts.VolumesYAML)
	}
	if !strings.Contains(opts.VolumesYAML, "gcsfuse.csi.storage.gke.io") {
		t.Errorf("expected gcsfuse CSI driver in volumes, got:\n%s", opts.VolumesYAML)
	}
}

func TestSanitizePVCName_TruncationHyphen(t *testing.T) {
	// 252 characters of 'a', followed by '-' and 'b'
	longName := strings.Repeat("a", 252) + "-b"
	// Truncated at 253, it becomes strings.Repeat("a", 252) + "-"
	sanitized := sanitizePVCName(longName)
	if strings.HasSuffix(sanitized, "-") {
		t.Errorf("sanitized name has a trailing hyphen: %s", sanitized)
	}
	expected := strings.Repeat("a", 252)
	if sanitized != expected {
		t.Errorf("expected %s, got %s", expected, sanitized)
	}
}

func TestProcessMounts_Filestore_Ambiguous(t *testing.T) {
	// Mock instances: same name "my-filestore", different locations
	inst1 := &filestorepb.Instance{
		Name:  "projects/my-project/locations/us-central1-a/instances/my-filestore",
		State: filestorepb.Instance_READY,
		Networks: []*filestorepb.NetworkConfig{
			{
				IpAddresses: []string{"10.0.0.1"},
			},
		},
		FileShares: []*filestorepb.FileShareConfig{
			{
				CapacityGb: 1024,
			},
		},
	}
	inst2 := &filestorepb.Instance{
		Name:  "projects/my-project/locations/us-east1-b/instances/my-filestore",
		State: filestorepb.Instance_READY,
		Networks: []*filestorepb.NetworkConfig{
			{
				IpAddresses: []string{"10.0.0.2"},
			},
		},
		FileShares: []*filestorepb.FileShareConfig{
			{
				CapacityGb: 2048,
			},
		},
	}

	sm := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				if projectID == "my-project" {
					return []*filestorepb.Instance{inst1, inst2}, nil
				}
				return nil, fmt.Errorf("unexpected project: %s", projectID)
			},
		},
	}

	// Case 1: Cluster location is us-central1-a. Should resolve to inst1.
	job1 := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-central1-a",
	}
	mounts1 := []string{
		"filestore://my-filestore/share;/data",
	}

	infos1, manifests1, err := sm.ProcessMounts(mounts1, job1)
	if err != nil {
		t.Fatalf("unexpected error for case 1: %v", err)
	}
	if len(infos1) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos1))
	}
	if infos1[0].Source != "gcluster-filestore-my-filestore-share" {
		t.Errorf("expected source gcluster-filestore-my-filestore-share, got %s", infos1[0].Source)
	}
	verifyFilestoreManifest(t, manifests1[0], "gcluster-filestore-my-filestore-share", "10.0.0.1", "/share", "1024Gi")

	// Case 2: Cluster location is us-east1-b. Should resolve to inst2.
	job2 := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-east1-b",
	}
	infos2, manifests2, err := sm.ProcessMounts(mounts1, job2) // reuse mounts1 as it only contains name
	if err != nil {
		t.Fatalf("unexpected error for case 2: %v", err)
	}
	if len(infos2) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos2))
	}
	verifyFilestoreManifest(t, manifests2[0], "gcluster-filestore-my-filestore-share", "10.0.0.2", "/share", "2048Gi")

	// Case 3: Cluster location is us-west1-a (no match). Should fail due to ambiguity.
	job3 := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-west1-a",
	}
	_, _, err = sm.ProcessMounts(mounts1, job3)
	if err == nil {
		t.Errorf("expected error for case 3 (no matching location), got nil")
	} else if !strings.Contains(err.Error(), "multiple Filestore instances named") {
		t.Errorf("expected ambiguity error, got: %v", err)
	}

	// Case 4: Cluster location is empty. Should fail due to ambiguity.
	job4 := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "",
	}
	_, _, err = sm.ProcessMounts(mounts1, job4)
	if err == nil {
		t.Errorf("expected error for case 4 (empty location), got nil")
	} else if !strings.Contains(err.Error(), "multiple Filestore instances named") {
		t.Errorf("expected ambiguity error, got: %v", err)
	}
}

type mockFilestoreClient struct {
	listInstancesFunc func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error)
}

func (m *mockFilestoreClient) listInstances(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
	return m.listInstancesFunc(ctx, projectID)
}

func TestProcessMounts_Filestore_Fallback(t *testing.T) {
	// Case 1: API Failure Fallback (IP)
	smAPIFail := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				return nil, fmt.Errorf("API error")
			},
		},
	}
	job := orchestrator.JobDefinition{
		ProjectID: "my-project",
	}
	mountsIP := []string{
		"filestore://10.0.0.3/share;/data",
	}

	infos1, manifests1, err := smAPIFail.ProcessMounts(mountsIP, job)
	if err != nil {
		t.Fatalf("unexpected error for API failure fallback: %v", err)
	}
	if len(infos1) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos1))
	}
	if infos1[0].Source != "gcluster-filestore-10-0-0-3-share" {
		t.Errorf("expected source gcluster-filestore-10-0-0-3-share, got %s", infos1[0].Source)
	}
	verifyFilestoreManifest(t, manifests1[0], "gcluster-filestore-10-0-0-3-share", "10.0.0.3", "/share", "1024Gi")

	// Case 2: Not Found Fallback (IP)
	smNotFound := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				return []*filestorepb.Instance{}, nil
			},
		},
	}
	infos2, manifests2, err := smNotFound.ProcessMounts(mountsIP, job)
	if err != nil {
		t.Fatalf("unexpected error for Not Found fallback: %v", err)
	}
	if len(infos2) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos2))
	}
	verifyFilestoreManifest(t, manifests2[0], "gcluster-filestore-10-0-0-3-share", "10.0.0.3", "/share", "1024Gi")

	// Case 3: Name Resolution Failure (Name should fail, not fallback)
	mountsName := []string{
		"filestore://my-filestore-instance/share;/data",
	}
	_, _, err = smAPIFail.ProcessMounts(mountsName, job)
	if err == nil {
		t.Errorf("expected error for Name resolution failure, got nil")
	} else if !strings.Contains(err.Error(), "failed to list Filestore instances") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestProcessMounts_Filestore_LocationOverlap(t *testing.T) {
	sm := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				return []*filestorepb.Instance{
					{
						Name: "projects/my-project/locations/europe-west1/instances/my-filestore",
						Networks: []*filestorepb.NetworkConfig{
							{
								IpAddresses: []string{"10.0.0.1"},
							},
						},
						FileShares: []*filestorepb.FileShareConfig{
							{
								Name:       "share",
								CapacityGb: 1024,
							},
						},
						State: filestorepb.Instance_READY,
					},
					{
						Name: "projects/my-project/locations/europe-west10/instances/my-filestore",
						Networks: []*filestorepb.NetworkConfig{
							{
								IpAddresses: []string{"10.0.0.2"},
							},
						},
						FileShares: []*filestorepb.FileShareConfig{
							{
								Name:       "share",
								CapacityGb: 1024,
							},
						},
						State: filestorepb.Instance_READY,
					},
				}, nil
			},
		},
	}
	// Cluster in europe-west10
	job := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "europe-west10",
	}
	mounts := []string{
		"filestore://my-filestore/share;/data",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	// Should resolve to the one in europe-west10 (IP 10.0.0.2)
	verifyFilestoreManifest(t, manifests[0], "gcluster-filestore-my-filestore-share", "10.0.0.2", "/share", "1024Gi")
}

func TestProcessMounts_Filestore_PVNameLengthLimit(t *testing.T) {
	sm := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				return []*filestorepb.Instance{
					{
						Name: "projects/my-project/locations/us-central1/instances/a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-name",
						Networks: []*filestorepb.NetworkConfig{
							{
								IpAddresses: []string{"10.0.0.1"},
							},
						},
						FileShares: []*filestorepb.FileShareConfig{
							{
								Name:       "a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-share",
								CapacityGb: 1024,
							},
						},
						State: filestorepb.Instance_READY,
					},
				}, nil
			},
		},
	}
	job := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-central1",
	}
	mounts := []string{
		"filestore://a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-name/a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-share;/data",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	expectedPVCName := infos[0].Source
	if len(expectedPVCName) > 189 {
		t.Errorf("expected PVC name length <= 189, got %d", len(expectedPVCName))
	}
	// The PV name should be PVCName + "-default" which will be <= 188 chars (well within 253 limit)
	verifyFilestoreManifest(t, manifests[0], expectedPVCName, "10.0.0.1", "/a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-share", "1024Gi")
}

func TestProcessMounts_Filestore_Caching(t *testing.T) {
	callCount := 0
	sm := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				callCount++
				return []*filestorepb.Instance{
					{
						Name: "projects/my-project/locations/us-central1/instances/inst1",
						Networks: []*filestorepb.NetworkConfig{
							{
								IpAddresses: []string{"10.0.0.1"},
							},
						},
						FileShares: []*filestorepb.FileShareConfig{
							{
								Name:       "share1",
								CapacityGb: 1024,
							},
						},
						State: filestorepb.Instance_READY,
					},
					{
						Name: "projects/my-project/locations/us-central1/instances/inst2",
						Networks: []*filestorepb.NetworkConfig{
							{
								IpAddresses: []string{"10.0.0.2"},
							},
						},
						FileShares: []*filestorepb.FileShareConfig{
							{
								Name:       "share2",
								CapacityGb: 1024,
							},
						},
						State: filestorepb.Instance_READY,
					},
				}, nil
			},
		},
	}
	job := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-central1",
	}
	mounts := []string{
		"filestore://inst1/share1;/data1",
		"filestore://inst2/share2;/data2",
	}

	_, _, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected API to be called exactly once, called %d times", callCount)
	}
}

func TestProcessMounts_Filestore_IPv6(t *testing.T) {
	// Case 1: Resolved IPv6
	smResolved := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				return []*filestorepb.Instance{
					{
						Name: "projects/my-project/locations/us-central1/instances/my-ipv6-filestore",
						Networks: []*filestorepb.NetworkConfig{
							{
								IpAddresses: []string{"2001:db8::1"},
							},
						},
						FileShares: []*filestorepb.FileShareConfig{
							{
								Name:       "share",
								CapacityGb: 1024,
							},
						},
						State: filestorepb.Instance_READY,
					},
				}, nil
			},
		},
	}
	job := orchestrator.JobDefinition{
		ProjectID:       "my-project",
		ClusterLocation: "us-central1",
	}
	mounts := []string{
		"filestore://[2001:db8::1]/share;/data",
	}

	infos, manifests, err := smResolved.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infos))
	}
	if infos[0].Source != "gcluster-filestore-my-ipv6-filestore-share" {
		t.Errorf("expected source gcluster-filestore-my-ipv6-filestore-share, got %s", infos[0].Source)
	}
	verifyFilestoreManifest(t, manifests[0], "gcluster-filestore-my-ipv6-filestore-share", "2001:db8::1", "/share", "1024Gi")

	// Case 2: Fallback IPv6 (API fails)
	smFallback := &StorageManager{
		filestoreClient: &mockFilestoreClient{
			listInstancesFunc: func(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
				return nil, fmt.Errorf("API error")
			},
		},
	}
	infosFb, manifestsFb, err := smFallback.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error for fallback: %v", err)
	}
	if len(infosFb) != 1 {
		t.Fatalf("expected 1 mount info, got %d", len(infosFb))
	}
	// gcluster-filestore-2001-db8-1-share is sanitized version of gcluster-filestore-2001:db8::1-share
	expectedPVCName := "gcluster-filestore-2001-db8-1-share"
	if infosFb[0].Source != expectedPVCName {
		t.Errorf("expected source %s, got %s", expectedPVCName, infosFb[0].Source)
	}
	verifyFilestoreManifest(t, manifestsFb[0], expectedPVCName, "2001:db8::1", "/share", "1024Gi")

	// Case 3: Invalid IPv6 format in scheme
	invalidMounts := []string{
		"filestore://[2001:db8::1:invalid]/share;/data",
	}
	_, _, err = smResolved.ProcessMounts(invalidMounts, job)
	if err == nil {
		t.Errorf("expected error for invalid IPv6 format, got nil")
	}
}

func splitManifestDocs(t *testing.T, manifest string) []map[string]interface{} {
	t.Helper()
	var docs []map[string]interface{}
	for _, raw := range strings.Split(manifest, "\n---\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var doc map[string]interface{}
		if err := k8syaml.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("generated manifest is not valid YAML: %v\n---\n%s", err, raw)
		}
		docs = append(docs, doc)
	}
	return docs
}

func findDoc(t *testing.T, docs []map[string]interface{}, kind string) map[string]interface{} {
	t.Helper()
	for _, d := range docs {
		if k, _ := d["kind"].(string); k == kind {
			return d
		}
	}
	t.Fatalf("no %s document found in manifest", kind)
	return nil
}

func nested(t *testing.T, doc map[string]interface{}, path ...string) interface{} {
	t.Helper()
	var cur interface{} = doc
	for _, p := range path {
		m, ok := cur.(map[string]interface{})
		if !ok {
			t.Fatalf("path %v: %q is not a map", path, p)
		}
		cur, ok = m[p]
		if !ok {
			t.Fatalf("path %v: key %q not found", path, p)
		}
	}
	return cur
}

func nestedString(t *testing.T, doc map[string]interface{}, path ...string) string {
	t.Helper()
	v := nested(t, doc, path...)
	s, ok := v.(string)
	if !ok {
		t.Fatalf("path %v is %T, want string", path, v)
	}
	return s
}

func assertProfilePVSpec(t *testing.T, pv map[string]interface{}, wantPV, wantPVC string) {
	t.Helper()
	if got := nestedString(t, pv, "metadata", "name"); got != wantPV {
		t.Errorf("PV name = %q, want %q", got, wantPV)
	}
	if got := nestedString(t, pv, "spec", "storageClassName"); got != "gcsfusecsi-training" {
		t.Errorf("PV storageClassName = %q, want gcsfusecsi-training", got)
	}
	if got := nestedString(t, pv, "spec", "persistentVolumeReclaimPolicy"); got != "Retain" {
		t.Errorf("PV reclaim policy = %q, want Retain", got)
	}
	if got := nestedString(t, pv, "spec", "csi", "driver"); got != "gcsfuse.csi.storage.gke.io" {
		t.Errorf("PV csi driver = %q", got)
	}
	if got := nestedString(t, pv, "spec", "csi", "volumeHandle"); got != "imagenet-dataset" {
		t.Errorf("PV volumeHandle = %q, want imagenet-dataset", got)
	}
	if got := nestedString(t, pv, "spec", "claimRef", "name"); got != wantPVC {
		t.Errorf("PV claimRef.name = %q, want %q", got, wantPVC)
	}
	if got := nestedString(t, pv, "spec", "claimRef", "namespace"); got != "default" {
		t.Errorf("PV claimRef.namespace = %q, want default", got)
	}
	accessModes, ok := nested(t, pv, "spec", "accessModes").([]interface{})
	if !ok || len(accessModes) != 1 || accessModes[0] != "ReadWriteMany" {
		t.Errorf("PV accessModes = %v, want [ReadWriteMany]", accessModes)
	}
	if _, present := pv["spec"].(map[string]interface{})["mountOptions"]; present {
		t.Error("PV must not declare mountOptions when the user supplied none")
	}
}

func assertProfilePVCSpec(t *testing.T, pvc map[string]interface{}, wantPV, wantPVC string) {
	t.Helper()
	if got := nestedString(t, pvc, "metadata", "name"); got != wantPVC {
		t.Errorf("PVC name = %q, want %q", got, wantPVC)
	}
	if got := nestedString(t, pvc, "metadata", "namespace"); got != "default" {
		t.Errorf("PVC namespace = %q, want default", got)
	}
	if got := nestedString(t, pvc, "spec", "volumeName"); got != wantPV {
		t.Errorf("PVC volumeName = %q, want %q", got, wantPV)
	}
	if got := nestedString(t, pvc, "spec", "storageClassName"); got != "gcsfusecsi-training" {
		t.Errorf("PVC storageClassName = %q", got)
	}
}

func TestGCSFuseProfile_ManifestRendering(t *testing.T) {
	sm := &StorageManager{}
	job := orchestrator.JobDefinition{}

	infos, manifests, err := sm.ProcessMounts([]string{"gs://imagenet-dataset;/data;ro;profile=training"}, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 1 || len(infos) != 1 {
		t.Fatalf("expected 1 manifest and 1 mount info, got %d manifests and %d infos", len(manifests), len(infos))
	}

	wantPVC := "gcluster-gcsfuse-imagenet-dataset-training-c8a67a"
	wantPV := wantPVC + "-default"
	wantInfo := MountInfo{
		Name:                "vol-0",
		Type:                "pvc",
		Source:              wantPVC,
		MountPath:           "/data",
		ReadOnly:            true,
		NeedsGCSFuseSidecar: true,
	}
	if !reflect.DeepEqual(infos[0], wantInfo) {
		t.Errorf("MountInfo = %+v, want %+v", infos[0], wantInfo)
	}

	docs := splitManifestDocs(t, manifests[0])
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents (PV + PVC), got %d", len(docs))
	}

	assertProfilePVSpec(t, findDoc(t, docs, "PersistentVolume"), wantPV, wantPVC)
	assertProfilePVCSpec(t, findDoc(t, docs, "PersistentVolumeClaim"), wantPV, wantPVC)
}

func renderGCSFuseGateway(t *testing.T, params GCSFusePVPVCTemplateParams) string {
	t.Helper()
	var g *GKEOrchestrator
	tmpl, err := g.parseGKETextTemplate("gcs_fuse_pv_pvc.tmpl")
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}
	return buf.String()
}

func TestGCSFuseProfile_TemplateQuotesInjectedValues(t *testing.T) {
	// text/template applies no escaping, so every injection point must be %q-quoted.
	hostileNS := "evil\n    name: hijacked-pvc\n  # "
	manifest := renderGCSFuseGateway(t, GCSFusePVPVCTemplateParams{
		PVName:           "gcluster-gcsfuse-b-training-default",
		PVCName:          "gcluster-gcsfuse-b-training",
		Namespace:        hostileNS,
		StorageClassName: "gcsfusecsi-training",
		Capacity:         "1Gi",
		VolumeHandle:     "b",
		VolumeAttributes: map[string]string{"fileCacheCapacity": "100Gi"},
	})

	docs := splitManifestDocs(t, manifest)
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents (PV + PVC), got %d:\n%s", len(docs), manifest)
	}

	claimRef, ok := nested(t, findDoc(t, docs, "PersistentVolume"), "spec", "claimRef").(map[string]interface{})
	if !ok {
		t.Fatalf("claimRef is not a map:\n%s", manifest)
	}
	wantClaimRef := map[string]interface{}{"namespace": hostileNS, "name": "gcluster-gcsfuse-b-training"}
	if !reflect.DeepEqual(claimRef, wantClaimRef) {
		t.Errorf("claimRef = %#v, want %#v; an injected key means a value escaped its scalar", claimRef, wantClaimRef)
	}

	if got := nestedString(t, findDoc(t, docs, "PersistentVolumeClaim"), "metadata", "namespace"); got != hostileNS {
		t.Errorf("PVC namespace = %q, want the hostile value preserved verbatim as one scalar", got)
	}
}

// profileStorageManager returns a StorageManager whose namespace is preset so no cluster call is made.
func profileStorageManager(customTemplatesPath string) *StorageManager {
	return &StorageManager{orchestrator: &GKEOrchestrator{
		gkeCustomTemplatesPath: customTemplatesPath,
		namespace:              "default",
	}}
}

func writeCustomGatewayTemplate(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "gcs_fuse_pv_pvc.tmpl"), []byte(body), 0644); err != nil {
		t.Fatalf("failed to write custom template: %v", err)
	}
}

func TestGCSFuseProfile_HonorsCustomTemplateOverride(t *testing.T) {
	const mount = "gs://imagenet-dataset;/data;ro;profile=training"

	t.Run("override is used", func(t *testing.T) {
		dir := t.TempDir()
		writeCustomGatewayTemplate(t, dir, `apiVersion: v1
kind: PersistentVolume
metadata:
  name: {{ printf "%q" .PVName }}
  annotations:
    example.com/rendered-by: custom-template
spec:
  storageClassName: {{ printf "%q" .StorageClassName }}
  claimRef:
    namespace: {{ printf "%q" .Namespace }}
    name: {{ printf "%q" .PVCName }}
`)

		_, manifests, err := profileStorageManager(dir).ProcessMounts([]string{mount}, orchestrator.JobDefinition{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
		if got := nestedString(t, pv, "metadata", "annotations", "example.com/rendered-by"); got != "custom-template" {
			t.Errorf("annotation = %q, want the override to have been rendered", got)
		}
		if _, embedded := pv["metadata"].(map[string]interface{})["labels"]; embedded {
			t.Error("embedded default was rendered instead of the override")
		}
		// Params the override omits must still be supplied, not error.
		if got := nestedString(t, pv, "spec", "claimRef", "name"); got != "gcluster-gcsfuse-imagenet-dataset-training-61350c" {
			t.Errorf("claimRef.name = %q", got)
		}
	})

	t.Run("falls back to embedded when absent", func(t *testing.T) {
		_, manifests, err := profileStorageManager(t.TempDir()).ProcessMounts([]string{mount}, orchestrator.JobDefinition{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
		if got := nestedString(t, pv, "metadata", "labels", "gcluster.google.com/managed-by"); got != "cluster-toolkit" {
			t.Errorf("managed-by label = %q, want the embedded default to have been used", got)
		}
	})

	t.Run("unknown field in override is reported", func(t *testing.T) {
		dir := t.TempDir()
		writeCustomGatewayTemplate(t, dir, "name: {{ .NoSuchField }}\n")

		_, _, err := profileStorageManager(dir).ProcessMounts([]string{mount}, orchestrator.JobDefinition{})
		if err == nil {
			t.Fatal("expected an error when the override references an unknown field")
		}
		if !strings.Contains(err.Error(), "GCSFuse PV/PVC template") {
			t.Errorf("error = %v, want it to name the offending template", err)
		}
	})
}

func TestGCSFuseProfile_SubPathIsDelegatedToPod(t *testing.T) {
	sm := &StorageManager{}
	infos, manifests, err := sm.ProcessMounts(
		[]string{"gs://model-checkpoints/run1/shard2;/checkpoints;rw;profile=checkpointing"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if infos[0].SubPath != "run1/shard2" {
		t.Errorf("subPath = %q, want run1/shard2", infos[0].SubPath)
	}
	if infos[0].ReadOnly {
		t.Error("expected rw mount to not be read-only")
	}
	if infos[0].Source != "gcluster-gcsfuse-model-checkpoints-checkpointing-c75dd3" {
		t.Errorf("gateway PVC name = %q", infos[0].Source)
	}

	pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
	if got := nestedString(t, pv, "spec", "csi", "volumeHandle"); got != "model-checkpoints" {
		t.Errorf("PV volumeHandle = %q, want the bare bucket name", got)
	}
	if _, present := pv["spec"].(map[string]interface{})["mountOptions"]; present {
		t.Error("PV must not receive only-dir/read-only mountOptions derived from the sub path")
	}

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	if !strings.Contains(opts.VolumeMountsYAML, "subPath: run1/shard2") {
		t.Errorf("volumeMounts YAML missing subPath:\n%s", opts.VolumeMountsYAML)
	}
}

func TestGCSFuseProfile_OptionsAndAttributes(t *testing.T) {
	sm := &StorageManager{}
	_, manifests, err := sm.ProcessMounts(
		[]string{"gs://weights;/models;profile=serving;options=implicit-dirs,file-cache:max-size-mb:2000;attributes=anywhereCacheZones=us-central1-a"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
	mountOptions, ok := nested(t, pv, "spec", "mountOptions").([]interface{})
	if !ok || len(mountOptions) != 2 {
		t.Fatalf("PV mountOptions = %v, want 2 entries", mountOptions)
	}
	if mountOptions[0] != "implicit-dirs" || mountOptions[1] != "file-cache:max-size-mb:2000" {
		t.Errorf("PV mountOptions = %v", mountOptions)
	}
	if got := nestedString(t, pv, "spec", "csi", "volumeAttributes", "anywhereCacheZones"); got != "us-central1-a" {
		t.Errorf("volumeAttributes.anywhereCacheZones = %q", got)
	}

	name := nestedString(t, pv, "metadata", "name")
	if !strings.HasPrefix(name, "gcluster-gcsfuse-weights-serving-") {
		t.Errorf("PV name = %q, want a hash-suffixed canonical prefix", name)
	}
	if name == "gcluster-gcsfuse-weights-serving-default" {
		t.Error("custom options must not collide with the canonical gateway name")
	}
}

func TestGCSFuseProfile_RapidCacheWildcardRendersValidYAML(t *testing.T) {
	sm := &StorageManager{}
	_, manifests, err := sm.ProcessMounts(
		[]string{"gs://dataset/imagenet;/data;ro;profile=training;attributes=anywhereCacheZones=*"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(manifests[0], `"anywhereCacheZones": "*"`) {
		t.Errorf("expected a quoted wildcard in the rendered PV, got:\n%s", manifests[0])
	}
	pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
	if got := nestedString(t, pv, "spec", "csi", "volumeAttributes", "anywhereCacheZones"); got != "*" {
		t.Errorf("volumeAttributes.anywhereCacheZones = %q, want \"*\"", got)
	}
}

func TestGCSFuseProfile_OnlyDirStacksWithSubPath(t *testing.T) {
	sm := &StorageManager{}

	infos, manifests, err := sm.ProcessMounts(
		[]string{"gs://dataset;/data;ro;profile=training;options=only-dir=imagenet"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
	mountOptions, ok := nested(t, pv, "spec", "mountOptions").([]interface{})
	if !ok || len(mountOptions) != 1 || mountOptions[0] != "only-dir=imagenet" {
		t.Fatalf("PV mountOptions = %v, want [only-dir=imagenet]", mountOptions)
	}
	if infos[0].SubPath != "" {
		t.Errorf("SubPath = %q, want empty: a bare bucket must not add a second scoping level", infos[0].SubPath)
	}

	infos, _, err = sm.ProcessMounts(
		[]string{"gs://dataset/imagenet;/data;ro;profile=training;options=only-dir=imagenet"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if infos[0].SubPath != "imagenet" {
		t.Fatalf("SubPath = %q, want %q", infos[0].SubPath, "imagenet")
	}
	mount := buildVolumeMountSpec(infos[0])
	if mount["subPath"] != "imagenet" {
		t.Errorf("volumeMount subPath = %v, want imagenet (stacked on top of only-dir)", mount["subPath"])
	}
}

func gatewayNameFor(t *testing.T, sm *StorageManager, mount string) string {
	t.Helper()
	infos, _, err := sm.ProcessMounts([]string{mount}, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("ProcessMounts(%q): %v", mount, err)
	}
	return infos[0].Source
}

func TestGCSFuseGatewayNaming_FollowsPVSpec(t *testing.T) {
	sm := profileStorageManager(t.TempDir())
	base := gatewayNameFor(t, sm, "gs://b;/data;profile=training")
	if again := gatewayNameFor(t, sm, "gs://b;/data;profile=training"); again != base {
		t.Errorf("name is not deterministic: %q vs %q", again, base)
	}
	if opts := gatewayNameFor(t, sm, "gs://b;/data;profile=training;options=implicit-dirs"); opts == base {
		t.Error("custom options must change the gateway name")
	}
	a := gatewayNameFor(t, sm, "gs://b;/data;profile=training;attributes=x=1,y=2")
	if b := gatewayNameFor(t, sm, "gs://b;/data;profile=training;attributes=y=2,x=1"); a != b {
		t.Errorf("attribute ordering changed the name: %q vs %q", a, b)
	}
	if diff := gatewayNameFor(t, sm, "gs://b;/data;profile=training;attributes=x=2"); diff == a {
		t.Error("divergent attributes must produce different gateway names")
	}

	// A template or default change (e.g. a release raising capacity) must yield a new gateway, not a spec clash.
	const tmpl = "kind: PersistentVolume\nspec:\n  storageClassName: {{ printf \"%%q\" .StorageClassName }}\n  capacity:\n    storage: %s\n"
	small, large := t.TempDir(), t.TempDir()
	writeCustomGatewayTemplate(t, small, fmt.Sprintf(tmpl, "5Gi"))
	writeCustomGatewayTemplate(t, large, fmt.Sprintf(tmpl, "10Gi"))
	if gatewayNameFor(t, profileStorageManager(small), "gs://b;/data;profile=training") ==
		gatewayNameFor(t, profileStorageManager(large), "gs://b;/data;profile=training") {
		t.Error("a PV spec change must produce a different gateway name")
	}

	long := gcsFuseGatewayPVCName(strings.Repeat("a", 300), "training", "abc123")
	if len(long) > maxGeneratedPVCNameLength {
		t.Errorf("name length = %d, want <= %d", len(long), maxGeneratedPVCNameLength)
	}
	if strings.HasSuffix(long, "-") {
		t.Errorf("truncated name %q must not end with '-'", long)
	}
}

func TestGCSFuseGatewayNaming_TruncationIsCollisionFree(t *testing.T) {
	longBucket := strings.Repeat("a", 230)

	train := gcsFuseGatewayPVCName(longBucket, "training", "abc123")
	ckpt := gcsFuseGatewayPVCName(longBucket, "checkpointing", "abc123")
	if train == ckpt {
		t.Errorf("different profiles on a long bucket collapsed onto %q", train)
	}
	if !strings.HasSuffix(train, "-training-abc123") || !strings.HasSuffix(ckpt, "-checkpointing-abc123") {
		t.Errorf("profile suffix was truncated away: %q / %q", train, ckpt)
	}

	one := gcsFuseGatewayPVCName(strings.Repeat("a", 220)+"-one", "training", "abc123")
	two := gcsFuseGatewayPVCName(strings.Repeat("a", 220)+"-two", "training", "abc123")
	if one == two {
		t.Errorf("different long buckets collapsed onto %q", one)
	}

	withOpts := gcsFuseGatewayPVCName(longBucket, "training", "def456")
	if withOpts == train {
		t.Error("a different spec hash must still change the name after truncation")
	}

	for _, name := range []string{train, ckpt, one, two, withOpts} {
		if len(name) > maxGeneratedPVCNameLength {
			t.Errorf("name %q is %d chars, want <= %d", name, len(name), maxGeneratedPVCNameLength)
		}
		if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
			t.Errorf("name %q is not a valid DNS-1123 style object name", name)
		}
	}
}

func TestGCSFuseGatewayNaming_LossySanitizationIsCollisionFree(t *testing.T) {
	dotted := gcsFuseGatewayPVCName("my.bucket", "training", "abc123")
	hyphen := gcsFuseGatewayPVCName("my-bucket", "training", "abc123")
	under := gcsFuseGatewayPVCName("my_bucket", "training", "abc123")

	if dotted == hyphen {
		t.Errorf("buckets my.bucket and my-bucket collapsed onto %q", dotted)
	}
	if under == hyphen {
		t.Errorf("buckets my_bucket and my-bucket collapsed onto %q", under)
	}
	if dotted == under {
		t.Errorf("buckets my.bucket and my_bucket collapsed onto %q", dotted)
	}

	if hyphen != "gcluster-gcsfuse-my-bucket-training-abc123" {
		t.Errorf("a losslessly sanitized bucket must not gain a digest, got %q", hyphen)
	}

	if again := gcsFuseGatewayPVCName("my.bucket", "training", "abc123"); again != dotted {
		t.Errorf("lossy name is not deterministic: %q vs %q", again, dotted)
	}
	if !strings.HasPrefix(dotted, "gcluster-gcsfuse-my-bucket-") {
		t.Errorf("lossy name %q lost its readable prefix", dotted)
	}

	domain := gcsFuseGatewayPVCName("example.com.datasets", "training", "abc123")
	flat := gcsFuseGatewayPVCName("example-com-datasets", "training", "abc123")
	if domain == flat {
		t.Errorf("domain-scoped bucket collapsed onto the flattened name %q", domain)
	}

	for _, name := range []string{dotted, hyphen, under, domain, flat} {
		if len(name) > maxGeneratedPVCNameLength {
			t.Errorf("name %q is %d chars, want <= %d", name, len(name), maxGeneratedPVCNameLength)
		}
		if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
			t.Errorf("name %q is not a valid DNS-1123 style object name", name)
		}
	}
}

func TestGCSFuseProfile_SameBucketTwoProfilesPassesValidation(t *testing.T) {
	sm := &StorageManager{}
	if err := sm.ValidateMounts([]string{
		"gs://shared;/train;ro;profile=training",
		"gs://shared;/ckpt;rw;profile=checkpointing",
	}); err != nil {
		t.Errorf("same bucket under two profiles must be allowed, got: %v", err)
	}

	if err := sm.ValidateMounts([]string{
		"gs://shared/train;/train;ro",
		"gs://shared/eval;/eval;ro",
	}); err != nil {
		t.Errorf("distinct subpaths on the same inline bucket must be allowed, got: %v", err)
	}

	if err := sm.ValidateMounts([]string{
		"gs://shared/train;/train;ro;profile=training",
		"gs://shared/eval;/eval;ro;profile=training",
	}); err != nil {
		t.Errorf("distinct subpaths on the same profile bucket must be allowed, got: %v", err)
	}

	if err := sm.ValidateMounts([]string{
		"gs://bkt;/a;profile=training;options=implicit-dirs",
		"gs://bkt;/c;profile=training;options=file-cache:max-size-mb:2000",
	}); err != nil {
		t.Errorf("same bucket+profile with distinct options= must be allowed, got: %v", err)
	}

	if err := sm.ValidateMounts([]string{
		"gs://bkt;/a;profile=training;options=implicit-dirs",
		"gs://bkt;/c;profile=training;options=implicit-dirs",
	}); err == nil {
		t.Error("expected a duplicate-source error for identical bucket+profile+options")
	}

	if err := sm.ValidateMounts([]string{
		"gs://shared;/a;ro;profile=training",
		"gs://shared;/b;ro;profile=training",
	}); err == nil {
		t.Error("expected a duplicate-source error for identical source+profile")
	}

	if err := sm.ValidateMounts([]string{"gs://shared;/a", "gs://shared;/b"}); err == nil {
		t.Error("expected a duplicate-source error for repeated profile-less sources")
	}
}

func TestGCSFuseProfile_InvalidSourceFailsPreflight(t *testing.T) {
	sm := &StorageManager{}
	for _, mount := range []string{
		"gs://MyBucket;/data;profile=training",
		"gs://MyBucket;/data",
		"gs://;/data;profile=training",
		"gs://;/data",
		"gs:///;/data;profile=training",
		"gs:///;/data",
	} {
		if err := sm.ValidateMounts([]string{mount}); err == nil {
			t.Errorf("ValidateMounts(%q) = nil, want an error before the image build", mount)
		}
	}
}

func TestGCSFuseProfile_BackwardCompatibility(t *testing.T) {
	sm := &StorageManager{}
	infos, manifests, err := sm.ProcessMounts(
		[]string{"gs://logs/run1;/logs;rw;options=implicit-dirs"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("profile-less gs:// mount must not generate manifests, got %d", len(manifests))
	}
	got := infos[0]
	want := MountInfo{
		Name:      "vol-0",
		Source:    "gs://logs",
		MountPath: "/logs",
		Type:      "gcsfuse",
		ReadOnly:  false,
		Options:   "implicit-dirs",
		SubPath:   "run1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MountInfo = %+v, want %+v", got, want)
	}

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	if !opts.GCSFuseEnabled {
		t.Error("expected GCSFuseEnabled for an inline gs:// mount")
	}
	if !strings.Contains(opts.VolumesYAML, "gcsfuse.csi.storage.gke.io") {
		t.Errorf("expected inline CSI volume spec, got:\n%s", opts.VolumesYAML)
	}
	if !strings.Contains(opts.VolumesYAML, "bucketName: logs") {
		t.Errorf("expected bare bucketName in inline CSI volumeAttributes, got:\n%s", opts.VolumesYAML)
	}
	if !strings.Contains(opts.VolumeMountsYAML, "subPath: run1") {
		t.Errorf("expected subPath delegated to volumeMount, got:\n%s", opts.VolumeMountsYAML)
	}
	if strings.Contains(opts.VolumesYAML, "persistentVolumeClaim") {
		t.Errorf("profile-less mount must not become a PVC:\n%s", opts.VolumesYAML)
	}
}

func TestGCSFuseProfile_MixedMounts(t *testing.T) {
	sm := &StorageManager{}
	mounts := []string{
		"gs://training-data/train;/data;ro;profile=training",
		"gs://experiment-logs/run1;/logs;rw",
		"/host/scratch;/scratch;rw",
		"my-existing-pvc;/shared",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 4 {
		t.Fatalf("expected 4 mount infos, got %d", len(infos))
	}
	if len(manifests) != 1 {
		t.Fatalf("expected exactly 1 generated manifest (the profile gateway), got %d", len(manifests))
	}

	wantTypes := []string{"pvc", "gcsfuse", "hostPath", "pvc"}
	for i, want := range wantTypes {
		if infos[i].Type != want {
			t.Errorf("mount %d type = %q, want %q", i, infos[i].Type, want)
		}
	}
	if infos[0].Source != "gcluster-gcsfuse-training-data-training-490464" {
		t.Errorf("profile gateway PVC = %q", infos[0].Source)
	}
	if infos[3].Source != "my-existing-pvc" {
		t.Errorf("user PVC must pass through untouched, got %q", infos[3].Source)
	}
	if infos[3].NeedsGCSFuseSidecar {
		t.Error("a user-provided PVC must not request the GCSFuse sidecar")
	}

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	if !opts.GCSFuseEnabled {
		t.Error("expected GCSFuseEnabled when the job has GCSFuse volumes")
	}
	if n := strings.Count(opts.VolumesYAML, "name: vol-"); n != 4 {
		t.Errorf("expected 4 distinct pod volumes, got %d:\n%s", n, opts.VolumesYAML)
	}
}

func TestGCSFuseProfile_SidecarAnnotationForProfileOnlyJob(t *testing.T) {
	sm := &StorageManager{}
	infos, _, err := sm.ProcessMounts([]string{"gs://b;/data;profile=training"}, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	if !opts.GCSFuseEnabled {
		t.Error("GCSFuseEnabled must be true for profile-backed PVC mounts")
	}
}

func TestGCSFuseProfile_SharedGatewayDeduplication(t *testing.T) {
	sm := &StorageManager{}
	mounts := []string{
		"gs://shared/datasets;/data;ro;profile=training;attributes=a=1,b=2",
		"gs://shared/eval;/eval;rw;profile=training;attributes=b=2,a=1",
	}

	infos, manifests, err := sm.ProcessMounts(mounts, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected the shared gateway manifest to be emitted once, got %d", len(manifests))
	}
	if infos[0].Name != infos[1].Name {
		t.Errorf("expected both mounts to share pod volume %q, got %q", infos[0].Name, infos[1].Name)
	}
	if infos[0].Source != infos[1].Source {
		t.Errorf("expected both mounts to share PVC %q, got %q", infos[0].Source, infos[1].Source)
	}
	if !infos[0].ReadOnly || infos[1].ReadOnly {
		t.Errorf("ReadOnly = %v / %v, want true / false on shared gateway", infos[0].ReadOnly, infos[1].ReadOnly)
	}
	if infos[0].SubPath != "datasets" || infos[1].SubPath != "eval" {
		t.Errorf("subPaths = %q / %q, want datasets / eval", infos[0].SubPath, infos[1].SubPath)
	}

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	if n := strings.Count(opts.VolumesYAML, "name: vol-"); n != 1 {
		t.Errorf("expected 1 pod volume for the shared gateway, got %d:\n%s", n, opts.VolumesYAML)
	}
	if n := strings.Count(opts.VolumeMountsYAML, "mountPath:"); n != 2 {
		t.Errorf("expected 2 volumeMounts, got %d:\n%s", n, opts.VolumeMountsYAML)
	}
	if n := strings.Count(opts.VolumeMountsYAML, "readOnly: true"); n != 1 {
		t.Errorf("expected readOnly: true only on the ro volumeMount, got %d:\n%s", n, opts.VolumeMountsYAML)
	}
}

func TestGCSFuseProfile_DivergentOptionsCreateSeparateGateways(t *testing.T) {
	sm := &StorageManager{}
	infos, manifests, err := sm.ProcessMounts([]string{
		"gs://shared/datasets;/data;ro;profile=training;options=implicit-dirs",
		"gs://shared/eval;/eval;ro;profile=training;options=only-dir=eval",
	}, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 2 || infos[0].Name == infos[1].Name || infos[0].Source == infos[1].Source {
		t.Errorf("divergent options= must not deduplicate: got %d manifests, volumes %q / %q", len(manifests), infos[0].Name, infos[1].Name)
	}
}

func TestGCSFuseProfile_ResolveNamespaceErrors(t *testing.T) {
	errSM := &StorageManager{
		orchestrator: &GKEOrchestrator{kubeClient: &MockKubeClient{Err: fmt.Errorf("kubeconfig broken")}},
	}
	if _, _, err := errSM.ProcessMounts([]string{"gs://bkt;/data;profile=training"}, orchestrator.JobDefinition{}); err == nil || !strings.Contains(err.Error(), "failed to resolve namespace for storage gateway") {
		t.Errorf("expected failed to resolve namespace error, got: %v", err)
	}

	emptySM := &StorageManager{
		orchestrator: &GKEOrchestrator{kubeClient: &MockKubeClient{ExplicitEmpty: true}},
	}
	if _, _, err := emptySM.ProcessMounts([]string{"gs://bkt;/data;profile=training"}, orchestrator.JobDefinition{}); err == nil || !strings.Contains(err.Error(), "Specify one explicitly with --gke-namespace") {
		t.Errorf("expected empty-namespace --gke-namespace error, got: %v", err)
	}
}

func TestGCSFuseProfile_DifferentProfilesSameBucket(t *testing.T) {
	sm := &StorageManager{}
	mounts := []string{
		"gs://shared/a;/train;ro;profile=training",
		"gs://shared/b;/ckpt;rw;profile=checkpointing",
	}
	infos, manifests, err := sm.ProcessMounts(mounts, orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 gateway manifests, got %d", len(manifests))
	}
	if infos[0].Source == infos[1].Source {
		t.Errorf("different profiles must not share a gateway (%q)", infos[0].Source)
	}
}

func TestGCSFuseProfile_InvalidBucket(t *testing.T) {
	sm := &StorageManager{}
	cases := []struct {
		name   string
		mount  string
		errSub string
	}{
		{"empty bucket", "gs://;/data;profile=training", "bucket name is missing"},
		{"uppercase bucket", "gs://MyBucket;/data;profile=training", "invalid GCS bucket name"},
		{"bucket with slash only", "gs:///;/data;profile=training", "bucket name is missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := sm.ProcessMounts([]string{tc.mount}, orchestrator.JobDefinition{})
			if err == nil || !strings.Contains(err.Error(), tc.errSub) {
				t.Errorf("error = %v, want substring %q", err, tc.errSub)
			}
		})
	}
}

func TestSplitGCSSource(t *testing.T) {
	cases := []struct {
		src         string
		wantBucket  string
		wantSubPath string
		wantErr     bool
	}{
		{src: "gs://bucket", wantBucket: "bucket"},
		{src: "gs://bucket/", wantBucket: "bucket"},
		{src: "gs://bucket/a", wantBucket: "bucket", wantSubPath: "a"},
		{src: "gs://bucket/a/b/", wantBucket: "bucket", wantSubPath: "a/b"},
		{src: "gs://my-bucket_1.x/a", wantBucket: "my-bucket_1.x", wantSubPath: "a"},
		{src: "gs://", wantErr: true},
		{src: "gs://-bad", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			bucket, subPath, err := splitGCSSource(tc.src)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if bucket != tc.wantBucket || subPath != tc.wantSubPath {
				t.Errorf("= (%q, %q), want (%q, %q)", bucket, subPath, tc.wantBucket, tc.wantSubPath)
			}
		})
	}
}

func TestNormalizeProfileName(t *testing.T) {
	for alias, want := range map[string]string{
		"training":            "gcsfusecsi-training",
		"checkpointing":       "gcsfusecsi-checkpointing",
		"serving":             "gcsfusecsi-serving",
		"gcsfusecsi-training": "gcsfusecsi-training",
		" Serving ":           "gcsfusecsi-serving",
	} {
		got, err := normalizeProfileName(alias)
		if err != nil {
			t.Errorf("normalizeProfileName(%q) unexpected error: %v", alias, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeProfileName(%q) = %q, want %q", alias, got, want)
		}
	}

	if _, err := normalizeProfileName("gcsfusecsi-unknown"); err == nil {
		t.Error("expected an error for an unknown profile")
	}
}

func assertFilestoreSharedVolumes(t *testing.T, opts *ManifestOptions, wantClaim string) {
	t.Helper()
	if got := strings.Count(opts.VolumesYAML, "- name:"); got != 1 {
		t.Errorf("expected 1 Pod volume, got %d:\n%s", got, opts.VolumesYAML)
	}
	if got := strings.Count(opts.VolumeMountsYAML, "mountPath:"); got != 3 {
		t.Errorf("expected 3 Pod volumeMounts, got %d:\n%s", got, opts.VolumeMountsYAML)
	}
	if got := strings.Count(opts.VolumesYAML, "claimName: "+wantClaim); got != 1 {
		t.Errorf("expected the claim to be referenced by exactly 1 Pod volume, got %d:\n%s", got, opts.VolumesYAML)
	}
	if strings.Contains(opts.VolumesYAML, "readOnly") {
		t.Errorf("a PVC-backed Pod volume must not carry readOnly; it would apply to every mount of it:\n%s", opts.VolumesYAML)
	}
	const wantSharedMounts = `                - mountPath: /data
                  name: vol-0
                  readOnly: true
                - mountPath: /data2
                  name: vol-0
                - mountPath: /data3
                  name: vol-0
                  readOnly: true`
	if opts.VolumeMountsYAML != wantSharedMounts {
		t.Errorf("shared-volume volumeMounts changed.\n--- got ---\n%s\n--- want ---\n%s", opts.VolumeMountsYAML, wantSharedMounts)
	}
}

func TestProcessMounts_Filestore_SameInstanceTwoSpellings(t *testing.T) {
	sm := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			if nameOrIP == "10.0.0.2" || nameOrIP == "myinstance" {
				return "10.0.0.2", "myinstance", 2048, nil
			}
			return "", "", 0, fmt.Errorf("unexpected query %q", nameOrIP)
		},
	}
	job := orchestrator.JobDefinition{}

	mounts := []string{
		"filestore://10.0.0.2/share;/data",
		"filestore://myinstance/share;/data2;rw",
		"filestore://myinstance/share//;/data3",
	}

	if err := sm.ValidateMounts(mounts); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	infos, manifests, err := sm.ProcessMounts(mounts, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const wantClaim = "gcluster-filestore-myinstance-share"
	wantInfos := []MountInfo{
		{Name: "vol-0", Type: "pvc", Source: wantClaim, MountPath: "/data", ReadOnly: true},
		{Name: "vol-0", Type: "pvc", Source: wantClaim, MountPath: "/data2", ReadOnly: false},
		{Name: "vol-0", Type: "pvc", Source: wantClaim, MountPath: "/data3", ReadOnly: true},
	}
	if !reflect.DeepEqual(infos, wantInfos) {
		t.Fatalf("infos = %+v, want %+v", infos, wantInfos)
	}

	if len(manifests) != 1 {
		t.Fatalf("expected the PVC/PV manifest to be emitted exactly once, got %d manifests:\n%s", len(manifests), strings.Join(manifests, "\n---\n"))
	}
	verifyFilestoreManifest(t, manifests[0], wantClaim, "10.0.0.2", "/share", "2048Gi")

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	assertFilestoreSharedVolumes(t, opts, wantClaim)
}

func TestInlineAttributes_AcceptedWithoutProfile(t *testing.T) {
	sm := &StorageManager{}

	pm, err := sm.parseSingleVolume("gs://my-bucket;/data;attributes=fileCacheCapacity=100Gi,gcsfuseLoggingSeverity=debug")
	if err != nil {
		t.Fatalf("attributes= without profile= must be accepted, got error: %v", err)
	}
	if pm.Profile != "" {
		t.Errorf("Profile = %q, want empty", pm.Profile)
	}
	want := map[string]string{"fileCacheCapacity": "100Gi", "gcsfuseLoggingSeverity": "debug"}
	if !reflect.DeepEqual(pm.Attributes, want) {
		t.Errorf("Attributes = %#v, want %#v", pm.Attributes, want)
	}
}

func TestInlineAttributes_RenderedIntoInlineCSIVolume(t *testing.T) {
	sm := &StorageManager{}

	infos, manifests, err := sm.ProcessMounts(
		[]string{"gs://my-bucket;/data;attributes=fileCacheCapacity=100Gi"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("expected no additional manifests for an inline mount, got %d:\n%s", len(manifests), strings.Join(manifests, "\n"))
	}
	if got := infos[0].Attributes["fileCacheCapacity"]; got != "100Gi" {
		t.Fatalf("MountInfo.Attributes[fileCacheCapacity] = %q, want 100Gi", got)
	}

	opts := &ManifestOptions{}
	sm.AddVolumeOptions(opts, infos)
	if !strings.Contains(opts.VolumesYAML, "fileCacheCapacity: 100Gi") {
		t.Errorf("expected fileCacheCapacity in the inline volume, got:\n%s", opts.VolumesYAML)
	}
	if !strings.Contains(opts.VolumesYAML, "bucketName: my-bucket") {
		t.Errorf("expected bucketName to survive alongside user attributes, got:\n%s", opts.VolumesYAML)
	}
}

func TestInlineAttributes_ProfileOnlyKeyIsWarnedNotRejected(t *testing.T) {
	sm := &StorageManager{}

	infos, manifests, err := sm.ProcessMounts(
		[]string{"gs://my-bucket;/data;attributes=anywhereCacheZones=us-central1-a,us-central1-b"},
		orchestrator.JobDefinition{})
	if err != nil {
		t.Fatalf("a storage-profile-only attribute must not be rejected, got: %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("expected no additional manifests, got %d", len(manifests))
	}
	if got := infos[0].Attributes["anywhereCacheZones"]; got != "us-central1-a,us-central1-b" {
		t.Errorf("multi-zone value was mangled: %q", got)
	}
}

// Every reserved key must be refused on both the inline and the storage-profile
// path, since each renders volumeAttributes through a different code path.
func TestVolumeAttributes_ReservedKeysRejected(t *testing.T) {
	sm := &StorageManager{}

	for key, hint := range reservedVolumeAttributes {
		for _, mount := range []string{
			fmt.Sprintf("gs://my-bucket;/data;attributes=%s=some-value", key),
			fmt.Sprintf("gs://my-bucket;/data;profile=training;attributes=%s=some-value", key),
		} {
			_, err := sm.parseSingleVolume(mount)
			if err == nil {
				t.Errorf("%s: expected reserved attribute %q to be rejected", mount, key)
				continue
			}
			if !strings.Contains(err.Error(), hint) {
				t.Errorf("%s: error should carry the %q hint, got: %v", mount, key, err)
			}
		}
	}
}

func TestBuildVolumeSpec_AttributesOnlyApplyToInlineGCSFuse(t *testing.T) {
	attrs := map[string]string{"fileCacheCapacity": "100Gi"}

	spec := buildVolumeSpec(MountInfo{
		Name: "vol-0", Source: "gs://my-bucket", Type: "gcsfuse",
		ReadOnly: true, Options: "implicit-dirs", Attributes: attrs,
	})
	csi, ok := spec["csi"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected a csi volume, got %#v", spec)
	}
	got, ok := csi["volumeAttributes"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected volumeAttributes, got %#v", csi)
	}
	want := map[string]interface{}{
		"bucketName":        "my-bucket",
		"mountOptions":      "implicit-dirs",
		"fileCacheCapacity": "100Gi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("volumeAttributes = %#v, want %#v", got, want)
	}

	pvcSpec := buildVolumeSpec(MountInfo{
		Name: "vol-1", Source: "gcluster-gcsfuse-my-bucket-training", Type: "pvc", Attributes: attrs,
	})
	if _, leaked := pvcSpec["csi"]; leaked {
		t.Errorf("attributes leaked onto a PVC volume: %#v", pvcSpec)
	}
	if _, ok := pvcSpec["persistentVolumeClaim"]; !ok {
		t.Errorf("expected persistentVolumeClaim, got %#v", pvcSpec)
	}
}

func TestGCSFuseProfile_ExistingGatewayPVCheck(t *testing.T) {
	pvcName := gatewayNameFor(t, profileStorageManager(t.TempDir()), "gs://bkt;/data;profile=training")
	pvName := pvcName + "-default"

	matchingPVJSON := `{
		"spec": {
			"storageClassName": "gcsfusecsi-training",
			"capacity": {"storage": "5Gi"},
			"csi": {"volumeHandle": "bkt"}
		},
		"status": {"phase": "Bound"}
	}`
	mismatchedPVJSON := `{
		"spec": {
			"storageClassName": "gcsfusecsi-training",
			"capacity": {"storage": "10Gi"},
			"csi": {"volumeHandle": "bkt"}
		},
		"status": {"phase": "Bound"}
	}`
	releasedPVJSON := `{
		"spec": {
			"storageClassName": "gcsfusecsi-training",
			"capacity": {"storage": "5Gi"},
			"csi": {"volumeHandle": "bkt"}
		},
		"status": {"phase": "Released"}
	}`
	managedReleasedPVJSON := `{
		"metadata": {"labels": {"gcluster.google.com/managed-by": "cluster-toolkit"}},
		"spec": {
			"storageClassName": "gcsfusecsi-training",
			"capacity": {"storage": "10Gi"},
			"csi": {"volumeHandle": "bkt"}
		},
		"status": {"phase": "Released"}
	}`

	terminatingPVJSON := `{
		"metadata": {"deletionTimestamp": "2026-09-24T18:00:00Z", "labels": {"gcluster.google.com/managed-by": "cluster-toolkit"}},
		"spec": {
			"storageClassName": "gcsfusecsi-training",
			"capacity": {"storage": "10Gi"},
			"csi": {"volumeHandle": "bkt"}
		},
		"status": {"phase": "Bound"}
	}`

	// The mock fails any command it has no response for, so a missing del response makes a delete attempt an error.
	newSM := func(res shell.CommandResult, del []shell.CommandResult) *StorageManager {
		exec := NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get pv " + pvName: {res},
			"kubectl delete pv " + pvName + " --ignore-not-found --wait=true --timeout=60s": del,
		})
		return &StorageManager{orchestrator: &GKEOrchestrator{executor: exec, namespace: "default"}}
	}

	forbidden := shell.CommandResult{ExitCode: 1, Stderr: `persistentvolumes "x" is forbidden: User cannot get resource "persistentvolumes"`}
	tests := []struct {
		name    string
		res     shell.CommandResult
		del     []shell.CommandResult
		dryRun  bool
		wantErr []string // substrings; nil means success
	}{
		{name: "absent PV", res: shell.CommandResult{Stdout: ""}},
		{name: "matching Bound PV", res: shell.CommandResult{Stdout: matchingPVJSON}},
		{name: "mismatched PV", res: shell.CommandResult{Stdout: mismatchedPVJSON}, wantErr: []string{"already exists with different settings", `namespace "default"`, "kubectl describe pvc " + pvcName + " -n default", "kubectl delete pvc " + pvcName + " -n default && kubectl delete pv " + pvName, "Bucket data is not affected"}},
		{name: "Terminating PV explains pending deletion", res: shell.CommandResult{Stdout: terminatingPVJSON}, wantErr: []string{"is being deleted", "PVC default/" + pvcName, "kubectl delete pvc " + pvcName + " -n default"}},
		{name: "unmanaged Released PV is left to the user", res: shell.CommandResult{Stdout: releasedPVJSON}, wantErr: []string{"already exists in Released state", "will not delete it", "kubectl delete pv " + pvName}},
		{name: "managed Released PV is deleted and recreated", res: shell.CommandResult{Stdout: managedReleasedPVJSON}, del: []shell.CommandResult{{}}},
		{name: "managed Released PV delete failure", res: shell.CommandResult{Stdout: managedReleasedPVJSON}, del: []shell.CommandResult{{ExitCode: 1, Stderr: "forbidden"}}, wantErr: []string{"failed to delete stale gateway PV", "forbidden"}},
		{name: "dry-run skips cluster check", res: shell.CommandResult{Stdout: mismatchedPVJSON}, dryRun: true},
		{name: "kubectl failure fails fast", res: forbidden, wantErr: []string{"failed to inspect existing gateway PV", "is forbidden"}},
		{name: "unparsable PV fails fast", res: shell.CommandResult{Stdout: "{not json"}, wantErr: []string{"failed to parse existing gateway PV"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := orchestrator.JobDefinition{}
			if tc.dryRun {
				job.DryRunManifest = "out.yaml"
			}
			_, _, err := newSM(tc.res, tc.del).ProcessMounts([]string{"gs://bkt;/data;profile=training"}, job)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err, want)
				}
			}
		})
	}
}

func TestUnmanagedGatewayWarning(t *testing.T) {
	const pvName = "gcluster-gcsfuse-bkt-training-default"
	tests := []struct {
		name     string
		manifest string
		wantWarn bool
	}{
		{"labelled PV", `{"metadata":{"labels":{"gcluster.google.com/managed-by":"cluster-toolkit"}}}`, false},
		{"no labels", `{"metadata":{"name":"x"}}`, true},
		{"wrong value", `{"metadata":{"labels":{"gcluster.google.com/managed-by":"someone-else"}}}`, true},
		{"unparsable manifest", "{{not yaml", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := unmanagedGatewayWarning(pvName, tc.manifest)
			if gotWarn := msg != ""; gotWarn != tc.wantWarn {
				t.Fatalf("warning = %q, wantWarn %v", msg, tc.wantWarn)
			}
			if tc.wantWarn && (!strings.Contains(msg, pvName) || !strings.Contains(msg, managedByLabel+"="+managedByValue)) {
				t.Errorf("warning %q must name the PV and the missing label", msg)
			}
		})
	}
}

func TestGatewayTemplates_ReceiveLabelParams(t *testing.T) {
	t.Run("gcsfuse override can use label params", func(t *testing.T) {
		dir := t.TempDir()
		writeCustomGatewayTemplate(t, dir, `apiVersion: v1
kind: PersistentVolume
metadata:
  name: {{ printf "%q" .PVName }}
  labels:
    {{ printf "%q" .ManagedByLabel }}: {{ printf "%q" .ManagedByValue }}
    {{ printf "%q" .StorageTypeLabel }}: {{ printf "%q" .StorageType }}
`)
		_, manifests, err := profileStorageManager(dir).ProcessMounts([]string{"gs://bkt;/data;profile=training"}, orchestrator.JobDefinition{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pv := findDoc(t, splitManifestDocs(t, manifests[0]), "PersistentVolume")
		if got := nestedString(t, pv, "metadata", "labels", managedByLabel); got != managedByValue {
			t.Errorf("%s = %q, want %q", managedByLabel, got, managedByValue)
		}
		if got := nestedString(t, pv, "metadata", "labels", storageTypeLabel); got != storageTypeGCSFuse {
			t.Errorf("%s = %q, want %q", storageTypeLabel, got, storageTypeGCSFuse)
		}
	})

	t.Run("embedded templates render labels on PV and PVC", func(t *testing.T) {
		_, manifests, err := profileStorageManager(t.TempDir()).ProcessMounts([]string{"gs://bkt;/data;profile=training"}, orchestrator.JobDefinition{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		docs := splitManifestDocs(t, manifests[0])
		for _, kind := range []string{"PersistentVolume", "PersistentVolumeClaim"} {
			if got := nestedString(t, findDoc(t, docs, kind), "metadata", "labels", managedByLabel); got != managedByValue {
				t.Errorf("embedded %s %s = %q, want %q", kind, managedByLabel, got, managedByValue)
			}
		}
	})
}

// TestGCSFuseGatewayPVCName_PinnedNames pins exact gateway names for the embedded template: a change here renames
// every existing gateway on upgrade, so it must be deliberate.
func TestGCSFuseGatewayPVCName_PinnedNames(t *testing.T) {
	tests := []struct {
		name  string
		mount string
		want  string
	}{
		{"canonical", "gs://imagenet-dataset;/data;profile=training", "gcluster-gcsfuse-imagenet-dataset-training-c8a67a"},
		{"options", "gs://imagenet-dataset;/data;profile=training;options=implicit-dirs,file-cache:max-size-mb:-1", "gcluster-gcsfuse-imagenet-dataset-training-bf793e"},
		{"attributes", "gs://imagenet-dataset;/data;profile=training;attributes=fileCacheCapacity=50Gi", "gcluster-gcsfuse-imagenet-dataset-training-38b020"},
		{"dotted bucket", "gs://my.dotted.bucket;/data;profile=serving", "gcluster-gcsfuse-my-dotted-bucket-0561d95913-serving-d7aead"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := gatewayNameFor(t, profileStorageManager(t.TempDir()), tc.mount); got != tc.want {
				t.Errorf("gateway name for %q = %q, want %q", tc.mount, got, tc.want)
			}
		})
	}
}

type fakePreflightClient struct {
	number             int64
	numberErr          error
	projectBindings    []iamBinding
	projectBindingsErr error
	bindings           map[string][]iamBinding
	bindingsErr        map[string]error
	locations          map[string]bucketLocation
	locationErr        map[string]error
	roles              map[string][]string
	roleErr            map[string]error
	roleRequests       []string
	bindingRequests    []string
	locationRequests   []string
	projectIAMRequests []string
}

func (f *fakePreflightClient) projectNumber(_ context.Context, _ string) (int64, error) {
	if f.numberErr != nil {
		return 0, f.numberErr
	}
	return f.number, nil
}

func (f *fakePreflightClient) projectIAMBindings(_ context.Context, projectID string) ([]iamBinding, error) {
	f.projectIAMRequests = append(f.projectIAMRequests, projectID)
	if f.projectBindingsErr != nil {
		return nil, f.projectBindingsErr
	}
	return f.projectBindings, nil
}

func (f *fakePreflightClient) bucketIAMBindings(_ context.Context, bucket string) ([]iamBinding, error) {
	f.bindingRequests = append(f.bindingRequests, bucket)
	if err, ok := f.bindingsErr[bucket]; ok {
		return nil, err
	}
	return f.bindings[bucket], nil
}

func (f *fakePreflightClient) bucketLocation(_ context.Context, bucket string) (bucketLocation, error) {
	f.locationRequests = append(f.locationRequests, bucket)
	if err, ok := f.locationErr[bucket]; ok {
		return bucketLocation{}, err
	}
	return f.locations[bucket], nil
}

func (f *fakePreflightClient) rolePermissions(_ context.Context, role string) ([]string, error) {
	f.roleRequests = append(f.roleRequests, role)
	if err, ok := f.roleErr[role]; ok {
		return nil, err
	}
	return f.roles[role], nil
}

func newPreflightStorageManager(executor Executor, client storagePreflightClient) *StorageManager {
	return &StorageManager{orchestrator: newTestGKEOrchestrator(executor), preflightClient: client}
}

func scResponses(results ...shell.CommandResult) map[string][]shell.CommandResult {
	return map[string][]shell.CommandResult{"kubectl get storageclass": results}
}

func TestCollectProfileMounts(t *testing.T) {
	tests := []struct {
		name   string
		mounts []string
		want   []profileMount
	}{
		{
			name:   "no profile mounts",
			mounts: []string{"gs://bkt;/data", "/host;/local"},
		},
		{
			name:   "training profile",
			mounts: []string{"gs://bkt/sub;/data;ro;profile=training"},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-training"},
			},
		},
		{
			name:   "serving profile implies anywhere cache",
			mounts: []string{"gs://bkt;/data;ro;profile=serving"},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-serving", UsesAnywhereCache: true},
			},
		},
		{
			name:   "anywhere cache attribute on training profile",
			mounts: []string{"gs://bkt;/data;ro;profile=training;attributes=anywhereCacheZones=us-central1-a,us-central1-b"},
			want: []profileMount{
				{
					Bucket:             "bkt",
					Profile:            "gcsfusecsi-training",
					AnywhereCacheZones: []string{"us-central1-a", "us-central1-b"},
					UsesAnywhereCache:  true,
				},
			},
		},
		{
			name: "duplicate bucket and profile collapsed",
			mounts: []string{
				"gs://bkt/a;/data1;ro;profile=training",
				"gs://bkt/b;/data2;ro;profile=training",
			},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-training"},
			},
		},
		{
			name: "same bucket different profiles kept",
			mounts: []string{
				"gs://bkt;/data1;ro;profile=training",
				"gs://bkt;/data2;ro;profile=checkpointing",
			},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-training"},
				{Bucket: "bkt", Profile: "gcsfusecsi-checkpointing"},
			},
		},
		{
			name:   "all-zones sentinel names no zone",
			mounts: []string{"gs://bkt;/data;ro;profile=training;attributes=anywhereCacheZones=*"},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-training", UsesAnywhereCache: true},
			},
		},
		{
			name:   "none sentinel disables the cache on serving",
			mounts: []string{"gs://bkt;/data;ro;profile=serving;attributes=anywhereCacheZones=none"},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-serving"},
			},
		},
		{
			name: "plain mount does not mask a cache-enabled mount on the same bucket and profile",
			mounts: []string{
				"gs://bkt;/data1;ro;profile=training",
				"gs://bkt;/data2;ro;profile=training;attributes=anywhereCacheTTL=1h",
			},
			want: []profileMount{
				{Bucket: "bkt", Profile: "gcsfusecsi-training"},
				{Bucket: "bkt", Profile: "gcsfusecsi-training", UsesAnywhereCache: true},
			},
		},
	}

	sm := &StorageManager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sm.collectProfileMounts(tt.mounts)
			if err != nil {
				t.Fatalf("collectProfileMounts() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectProfileMounts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCollectProfileMountsInvalidMount(t *testing.T) {
	sm := &StorageManager{}
	if _, err := sm.collectProfileMounts([]string{"gs://bkt;/data;profile=bogus"}); err == nil {
		t.Fatal("collectProfileMounts() expected an error for an unsupported profile")
	}
}

func TestValidateStorageClassExists(t *testing.T) {
	tests := []struct {
		name    string
		result  shell.CommandResult
		dryRun  bool
		wantErr bool
	}{
		{
			name:   "storage class present",
			result: shell.CommandResult{ExitCode: 0, Stdout: "storageclass.storage.k8s.io/gcsfusecsi-training\n"},
		},
		{
			name:    "storage class absent",
			result:  shell.CommandResult{ExitCode: 0, Stdout: "\n"},
			wantErr: true,
		},
		{
			name:   "storage class absent on a dry run does not block",
			result: shell.CommandResult{ExitCode: 0, Stdout: "\n"},
			dryRun: true,
		},
		{
			name:   "lookup failed fails open",
			result: shell.CommandResult{ExitCode: 1, Stderr: "Error from server (Forbidden)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := newPreflightStorageManager(NewMockExecutor(scResponses(tt.result)), &fakePreflightClient{})
			err := sm.validateStorageClassExists("gcsfusecsi-training", tt.dryRun)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateStorageClassExists() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), minGCSFuseProfileGKEVersion) {
				t.Errorf("validateStorageClassExists() error %q should mention the minimum GKE version", err)
			}
		})
	}
}

func TestRunStorageProfilePreflightDryRunDoesNotBlock(t *testing.T) {
	executor := NewMockExecutor(scResponses(shell.CommandResult{ExitCode: 0, Stdout: ""}))
	sm := newPreflightStorageManager(executor, &fakePreflightClient{number: 1234})

	job := orchestrator.JobDefinition{
		ProjectID:       "proj",
		ClusterLocation: "us-central1",
		DryRunManifest:  "manifest.yaml",
		RawMounts:       []string{"gs://bkt;/data;ro;profile=training"},
	}

	if err := sm.RunStorageProfilePreflight(job); err != nil {
		t.Fatalf("RunStorageProfilePreflight() must not block a dry run, got: %v", err)
	}
}

func TestRunStorageProfilePreflightBlocksOnMissingStorageClass(t *testing.T) {
	executor := NewMockExecutor(scResponses(shell.CommandResult{ExitCode: 0, Stdout: ""}))
	sm := newPreflightStorageManager(executor, &fakePreflightClient{number: 1234})

	job := orchestrator.JobDefinition{
		ProjectID:       "proj",
		ClusterLocation: "us-central1",
		RawMounts:       []string{"gs://bkt;/data;ro;profile=training"},
	}

	err := sm.RunStorageProfilePreflight(job)
	if err == nil {
		t.Fatal("RunStorageProfilePreflight() expected an error when the StorageClass is missing")
	}
	if !strings.Contains(err.Error(), gcsFuseProfileSelector) {
		t.Errorf("error %q should point at the StorageClass discovery selector", err)
	}
}

func TestRunStorageProfilePreflightChecksEachBucketOnce(t *testing.T) {
	executor := NewMockExecutor(scResponses(
		shell.CommandResult{ExitCode: 0, Stdout: "storageclass.storage.k8s.io/gcsfusecsi-training\n"},
		shell.CommandResult{ExitCode: 0, Stdout: "storageclass.storage.k8s.io/gcsfusecsi-checkpointing\n"},
	))
	client := &fakePreflightClient{number: 42}
	sm := newPreflightStorageManager(executor, client)

	job := orchestrator.JobDefinition{
		ProjectID:       "proj",
		ClusterLocation: "us-central1",
		RawMounts: []string{
			"gs://bkt;/data1;ro;profile=training",
			"gs://bkt;/data2;ro;profile=checkpointing",
		},
	}

	if err := sm.RunStorageProfilePreflight(job); err != nil {
		t.Fatalf("RunStorageProfilePreflight() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(client.bindingRequests, []string{"bkt"}) {
		t.Errorf("bucketIAMBindings calls = %v, want exactly one call for \"bkt\"", client.bindingRequests)
	}
	if !reflect.DeepEqual(client.locationRequests, []string{"bkt"}) {
		t.Errorf("bucketLocation calls = %v, want exactly one call for \"bkt\"", client.locationRequests)
	}
}

func TestRunStorageProfilePreflightNoProfileMountsSkipsCluster(t *testing.T) {
	executor := NewMockExecutor(map[string][]shell.CommandResult{})
	sm := newPreflightStorageManager(executor, &fakePreflightClient{numberErr: fmt.Errorf("should not be called")})

	job := orchestrator.JobDefinition{
		ProjectID: "proj",
		RawMounts: []string{"gs://bkt;/data", "/host;/local"},
	}

	if err := sm.RunStorageProfilePreflight(job); err != nil {
		t.Fatalf("RunStorageProfilePreflight() unexpected error: %v", err)
	}
}

func TestRunStorageProfilePreflightFailsOpenOnIAMErrors(t *testing.T) {
	executor := NewMockExecutor(scResponses(shell.CommandResult{
		ExitCode: 0,
		Stdout:   "storageclass.storage.k8s.io/gcsfusecsi-serving\n",
	}))
	client := &fakePreflightClient{
		numberErr:   fmt.Errorf("permission denied"),
		locationErr: map[string]error{"bkt": fmt.Errorf("permission denied")},
	}
	sm := newPreflightStorageManager(executor, client)

	job := orchestrator.JobDefinition{
		ProjectID:       "proj",
		ClusterLocation: "us-central1-a",
		RawMounts:       []string{"gs://bkt;/data;ro;profile=serving"},
	}

	if err := sm.RunStorageProfilePreflight(job); err != nil {
		t.Fatalf("RunStorageProfilePreflight() must never block on IAM failures, got: %v", err)
	}
}

func TestRunStorageProfilePreflightBucketRegionMismatch(t *testing.T) {
	tests := []struct {
		name      string
		mount     string
		dryRun    bool
		wantBlock bool
	}{
		{name: "serving profile blocks", mount: "gs://bkt;/data;ro;profile=serving", wantBlock: true},
		{name: "serving profile with Rapid Cache off still blocks", mount: "gs://bkt;/data;ro;profile=serving;attributes=anywhereCacheZones=none", wantBlock: true},
		{name: "rapid cache on another profile blocks", mount: "gs://bkt;/data;ro;profile=training;attributes=anywhereCacheZones=us-central1-a", wantBlock: true},
		{name: "serving profile only warns on a dry run", mount: "gs://bkt;/data;ro;profile=serving", dryRun: true},
		{name: "training without Rapid Cache only warns", mount: "gs://bkt;/data;ro;profile=training"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			executor := NewMockExecutor(scResponses(shell.CommandResult{ExitCode: 0, Stdout: "storageclass.storage.k8s.io/sc\n"}))
			client := &fakePreflightClient{
				number:    42,
				locations: map[string]bucketLocation{"bkt": {Location: "US-EAST1", LocationType: "region"}},
			}
			job := orchestrator.JobDefinition{ProjectID: "proj", ClusterLocation: "us-central1-a", RawMounts: []string{tc.mount}}
			if tc.dryRun {
				job.DryRunManifest = "manifest.yaml"
			}
			err := newPreflightStorageManager(executor, client).RunStorageProfilePreflight(job)
			if tc.wantBlock && (err == nil || !strings.Contains(err.Error(), "same region")) {
				t.Fatalf("RunStorageProfilePreflight() = %v, want a blocking co-location error", err)
			}
			if !tc.wantBlock && err != nil {
				t.Fatalf("RunStorageProfilePreflight() = %v, want only a warning", err)
			}
		})
	}
}

func TestCollectProfileMountsSplitsSubPath(t *testing.T) {
	sm := profileStorageManager(t.TempDir())
	got, err := sm.collectProfileMounts([]string{"gs://bkt/sub/dir;/data;ro;profile=serving", "gs://bkt;/other;ro;profile=serving"})
	if err != nil {
		t.Fatalf("collectProfileMounts() unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Bucket != "bkt" {
		t.Errorf("collectProfileMounts() = %+v, want one mount on bucket %q (the subpath is not part of the bucket)", got, "bkt")
	}
}

func TestGrantedAgentPermissions(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	customRole := "projects/proj/roles/gke.gcsfuse.profileUser"

	tests := []struct {
		name         string
		bindings     []iamBinding
		roles        map[string][]string
		roleErr      map[string]error
		want         []string
		wantRequests []string
	}{
		{
			name:     "storage admin grants everything",
			bindings: []iamBinding{{Role: "roles/storage.admin", Members: []string{"serviceAccount:" + agent}}},
			want:     append(append([]string{}, gcsFuseProfileBasePermissions...), gcsFuseAnywhereCachePermissions...),
		},
		{
			name:     "binding for a different member ignored",
			bindings: []iamBinding{{Role: "roles/storage.admin", Members: []string{"user:someone@example.com"}}},
		},
		{
			name:         "custom role permissions are expanded",
			bindings:     []iamBinding{{Role: customRole, Members: []string{"serviceAccount:" + agent}}},
			roles:        map[string][]string{customRole: {"storage.buckets.get", "storage.objects.list"}},
			want:         []string{"storage.buckets.get", "storage.objects.list"},
			wantRequests: []string{customRole},
		},
		{
			name:         "unreadable custom role contributes nothing",
			bindings:     []iamBinding{{Role: customRole, Members: []string{"serviceAccount:" + agent}}},
			roleErr:      map[string]error{customRole: fmt.Errorf("permission denied")},
			wantRequests: []string{customRole},
		},
		{
			name:     "legacy bucket reader grants the base permissions without an API call",
			bindings: []iamBinding{{Role: "roles/storage.legacyBucketReader", Members: []string{"serviceAccount:" + agent}}},
			want:     gcsFuseProfileBasePermissions,
		},
		{
			name:         "object viewer is credited only with what it grants",
			bindings:     []iamBinding{{Role: "roles/storage.objectViewer", Members: []string{"serviceAccount:" + agent}}},
			roles:        map[string][]string{"roles/storage.objectViewer": {"storage.objects.get", "storage.objects.list"}},
			want:         []string{"storage.objects.get", "storage.objects.list"},
			wantRequests: []string{"roles/storage.objectViewer"},
		},
		{
			name: "permissions from several roles are unioned",
			bindings: []iamBinding{
				{Role: "roles/storage.objectViewer", Members: []string{"serviceAccount:" + agent}},
				{Role: "roles/storage.legacyBucketReader", Members: []string{"serviceAccount:" + agent}},
			},
			roles:        map[string][]string{"roles/storage.objectViewer": {"storage.objects.get", "storage.objects.list"}},
			want:         []string{"storage.objects.get", "storage.objects.list", "storage.buckets.get"},
			wantRequests: []string{"roles/storage.objectViewer"},
		},
		{
			name: "a role bound twice is resolved once",
			bindings: []iamBinding{
				{Role: customRole, Members: []string{"serviceAccount:" + agent}},
				{Role: customRole, Members: []string{"serviceAccount:" + agent}},
			},
			roles:        map[string][]string{customRole: {"storage.buckets.get"}},
			want:         []string{"storage.buckets.get"},
			wantRequests: []string{customRole},
		},
		{
			name:     "member match is case insensitive",
			bindings: []iamBinding{{Role: "roles/owner", Members: []string{"serviceaccount:SERVICE-42@container-engine-robot.iam.gserviceaccount.com"}}},
			want:     append(append([]string{}, gcsFuseProfileBasePermissions...), gcsFuseAnywhereCachePermissions...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakePreflightClient{roles: tt.roles, roleErr: tt.roleErr}
			got := grantedAgentPermissions(context.Background(), newRoleResolver(client), tt.bindings, agent)
			if len(got) != len(tt.want) {
				t.Fatalf("grantedAgentPermissions() = %v, want %v", got, tt.want)
			}
			for _, p := range tt.want {
				if !got[p] {
					t.Errorf("grantedAgentPermissions() missing %q, got %v", p, got)
				}
			}
			if !reflect.DeepEqual(client.roleRequests, tt.wantRequests) {
				t.Errorf("rolePermissions calls = %v, want %v", client.roleRequests, tt.wantRequests)
			}
		})
	}
}

func TestRequiredProfilePermissions(t *testing.T) {
	base := requiredProfilePermissions(false)
	if len(base) != len(gcsFuseProfileBasePermissions) {
		t.Errorf("requiredProfilePermissions(false) = %v, want only the base permissions", base)
	}
	withCache := requiredProfilePermissions(true)
	if len(withCache) != len(gcsFuseProfileBasePermissions)+len(gcsFuseAnywhereCachePermissions) {
		t.Errorf("requiredProfilePermissions(true) = %v, want base plus Anywhere Cache permissions", withCache)
	}
	for i := 1; i < len(withCache); i++ {
		if withCache[i-1] > withCache[i] {
			t.Fatalf("requiredProfilePermissions(true) = %v, want sorted output", withCache)
		}
	}
}

func TestMissingPermissions(t *testing.T) {
	granted := map[string]bool{"storage.buckets.get": true}
	got := missingPermissions([]string{"storage.buckets.get", "storage.objects.list"}, granted)
	if !reflect.DeepEqual(got, []string{"storage.objects.list"}) {
		t.Errorf("missingPermissions() = %v, want [storage.objects.list]", got)
	}
	if got := missingPermissions([]string{"storage.buckets.get"}, granted); got != nil {
		t.Errorf("missingPermissions() = %v, want nil", got)
	}
}

func TestNormalizeToRegion(t *testing.T) {
	tests := map[string]string{
		"us-central1-a":               "us-central1",
		"us-central1":                 "us-central1",
		"US-CENTRAL1-A":               "us-central1",
		"northamerica-northeast1-b":   "northamerica-northeast1",
		"northamerica-northeast1":     "northamerica-northeast1",
		"europe-west4-a":              "europe-west4",
		"":                            "",
		"not-a-zone-or-region-at-all": "not-a-zone-or-region-at-all",
	}
	for in, want := range tests {
		if got := normalizeToRegion(in); got != want {
			t.Errorf("normalizeToRegion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCacheRegions(t *testing.T) {
	tests := []struct {
		name            string
		clusterLocation string
		zones           []string
		want            []string
	}{
		{name: "falls back to cluster region", clusterLocation: "us-central1-a", want: []string{"us-central1"}},
		{name: "regional cluster location", clusterLocation: "us-central1", want: []string{"us-central1"}},
		{name: "empty cluster location", clusterLocation: ""},
		{
			name:            "zones override the cluster region and are de-duplicated",
			clusterLocation: "us-central1",
			zones:           []string{"europe-west4-a", "europe-west4-b", "asia-east1-a"},
			want:            []string{"europe-west4", "asia-east1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cacheRegions(tt.clusterLocation, tt.zones); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cacheRegions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegionWithinBucketLocation(t *testing.T) {
	tests := []struct {
		name   string
		region string
		loc    bucketLocation
		want   bool
	}{
		{name: "regional match", region: "us-central1", loc: bucketLocation{Location: "US-CENTRAL1", LocationType: "region"}, want: true},
		{name: "regional mismatch", region: "us-east1", loc: bucketLocation{Location: "US-CENTRAL1", LocationType: "region"}},
		{name: "US multi-region accepts us region", region: "us-central1", loc: bucketLocation{Location: "US", LocationType: "multi-region"}, want: true},
		{name: "US multi-region rejects northamerica region", region: "northamerica-northeast1", loc: bucketLocation{Location: "US", LocationType: "multi-region"}},
		{name: "US multi-region rejects europe region", region: "europe-west4", loc: bucketLocation{Location: "US", LocationType: "multi-region"}},
		{name: "EU multi-region accepts europe region", region: "europe-west4", loc: bucketLocation{Location: "EU", LocationType: "multi-region"}, want: true},
		{name: "EU multi-region rejects us region", region: "us-central1", loc: bucketLocation{Location: "EU", LocationType: "multi-region"}},
		{name: "ASIA multi-region accepts asia region", region: "asia-east1", loc: bucketLocation{Location: "ASIA", LocationType: "multi-region"}, want: true},
		{
			name:   "custom dual-region accepts member region",
			region: "us-east1",
			loc:    bucketLocation{Location: "US-CENTRAL1+US-EAST1", LocationType: "dual-region", DataLocations: []string{"US-CENTRAL1", "US-EAST1"}},
			want:   true,
		},
		{
			name:   "custom dual-region rejects outside region",
			region: "europe-west4",
			loc:    bucketLocation{Location: "US-CENTRAL1+US-EAST1", LocationType: "dual-region", DataLocations: []string{"US-CENTRAL1", "US-EAST1"}},
		},
		{name: "predefined dual-region is not evaluated", region: "europe-west4", loc: bucketLocation{Location: "NAM4", LocationType: "dual-region"}, want: true},
		{name: "unknown multi-region is not evaluated", region: "us-central1", loc: bucketLocation{Location: "EUR", LocationType: "multi-region"}, want: true},
		{name: "unknown location is not evaluated", region: "us-central1", loc: bucketLocation{Location: "SOMETHING-NEW"}, want: true},
		{name: "empty bucket location is not evaluated", region: "us-central1", want: true},
		{name: "empty region is not evaluated", loc: bucketLocation{Location: "US-CENTRAL1"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := regionWithinBucketLocation(tt.region, tt.loc); got != tt.want {
				t.Errorf("regionWithinBucketLocation(%q, %+v) = %v, want %v", tt.region, tt.loc, got, tt.want)
			}
		})
	}
}

func TestUsesAnywhereCache(t *testing.T) {
	tests := []struct {
		name string
		pm   parsedMount
		want bool
	}{
		{name: "training without attributes", pm: parsedMount{Profile: "gcsfusecsi-training"}},
		{name: "serving", pm: parsedMount{Profile: "gcsfusecsi-serving"}, want: true},
		{
			name: "training with anywhere cache ttl",
			pm:   parsedMount{Profile: "gcsfusecsi-training", Attributes: map[string]string{"anywhereCacheTTL": "86400s"}},
			want: true,
		},
		{
			name: "training with unrelated attribute",
			pm:   parsedMount{Profile: "gcsfusecsi-training", Attributes: map[string]string{"bucketScanTimeout": "10s"}},
		},
		{
			name: "serving with zones disabled",
			pm:   parsedMount{Profile: "gcsfusecsi-serving", Attributes: map[string]string{"anywhereCacheZones": "none"}},
		},
		{
			name: "serving with zones disabled case insensitively",
			pm:   parsedMount{Profile: "gcsfusecsi-serving", Attributes: map[string]string{"anywhereCacheZones": " None "}},
		},
		{
			name: "serving with all zones",
			pm:   parsedMount{Profile: "gcsfusecsi-serving", Attributes: map[string]string{"anywhereCacheZones": "*"}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usesAnywhereCache(tt.pm); got != tt.want {
				t.Errorf("usesAnywhereCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitAnywhereCacheZones(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: ""},
		{in: "us-central1-a", want: []string{"us-central1-a"}},
		{in: " us-central1-a , US-CENTRAL1-B ", want: []string{"us-central1-a", "us-central1-b"}},
		{in: ",,"},
		{in: "*"},
		{in: "none"},
		{in: "NONE"},
		{in: "*,us-central1-a", want: []string{"us-central1-a"}},
	}
	for _, tt := range tests {
		if got := splitAnywhereCacheZones(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitAnywhereCacheZones(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestGKEServiceAgentEmail(t *testing.T) {
	got, err := gkeServiceAgentEmail(context.Background(), &fakePreflightClient{number: 123456789}, "proj")
	if err != nil {
		t.Fatalf("gkeServiceAgentEmail() unexpected error: %v", err)
	}
	want := "service-123456789@container-engine-robot.iam.gserviceaccount.com"
	if got != want {
		t.Errorf("gkeServiceAgentEmail() = %q, want %q", got, want)
	}

	if _, err := gkeServiceAgentEmail(context.Background(), &fakePreflightClient{numberErr: fmt.Errorf("denied")}, "proj"); err == nil {
		t.Error("gkeServiceAgentEmail() expected an error when the project number cannot be read")
	}
}

func TestWarnOnMissingBucketIAMQueriesCustomRoleOnce(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	customRole := "projects/proj/roles/gke.gcsfuse.profileUser"

	client := &fakePreflightClient{
		bindings: map[string][]iamBinding{
			"bkt": {{Role: customRole, Members: []string{"serviceAccount:" + agent}}},
		},
		roles: map[string][]string{customRole: append(append([]string{}, gcsFuseProfileBasePermissions...), gcsFuseAnywhereCachePermissions...)},
	}

	resolve := newRoleResolver(client)
	if msg := bucketIAMWarning(context.Background(), client, resolve, agent, "bkt", requiredProfilePermissions(true), lazyProjectGrants(context.Background(), client, resolve, "proj", agent)); msg != "" {
		t.Errorf("unexpected warning: %s", msg)
	}

	if !reflect.DeepEqual(client.roleRequests, []string{customRole}) {
		t.Errorf("rolePermissions calls = %v, want exactly one call for %q", client.roleRequests, customRole)
	}
	if len(client.projectIAMRequests) != 0 {
		t.Errorf("projectIAMBindings calls = %v, want none when the bucket policy already grants everything", client.projectIAMRequests)
	}
}

func TestDocumentedIAMSetups(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	const customRole = "projects/proj/roles/gke.gcsfuse.profileUser"
	customRolePermissions := map[string][]string{customRole: gcsFuseProfileAllPermissions}

	tests := []struct {
		name            string
		bucketBindings  []iamBinding
		projectBindings []iamBinding
		roles           map[string][]string
		usesCache       bool
		wantMissing     []string
	}{
		{
			name:           "option A custom role bound on the bucket",
			bucketBindings: []iamBinding{{Role: customRole, Members: []string{"serviceAccount:" + agent}}},
			roles:          customRolePermissions,
			usesCache:      true,
		},
		{
			name:            "option A custom role bound on the project",
			projectBindings: []iamBinding{{Role: customRole, Members: []string{"serviceAccount:" + agent}}},
			roles:           customRolePermissions,
			usesCache:       true,
		},
		{
			name:           "option B legacy bucket reader on a training mount",
			bucketBindings: []iamBinding{{Role: "roles/storage.legacyBucketReader", Members: []string{"serviceAccount:" + agent}}},
		},
		{
			name:           "option B legacy bucket reader on a serving mount",
			bucketBindings: []iamBinding{{Role: "roles/storage.legacyBucketReader", Members: []string{"serviceAccount:" + agent}}},
			usesCache:      true,
			wantMissing:    gcsFuseAnywhereCachePermissions,
		},
		{
			name:           "object viewer alone leaves the bucket permission missing",
			bucketBindings: []iamBinding{{Role: "roles/storage.objectViewer", Members: []string{"serviceAccount:" + agent}}},
			roles:          map[string][]string{"roles/storage.objectViewer": {"storage.objects.get", "storage.objects.list"}},
			wantMissing:    []string{"storage.buckets.get"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakePreflightClient{
				number:          42,
				bindings:        map[string][]iamBinding{"bkt": tt.bucketBindings},
				projectBindings: tt.projectBindings,
				roles:           tt.roles,
			}
			required := requiredProfilePermissions(tt.usesCache)
			resolve := newRoleResolver(client)
			got, err := bucketIAMShortfall(context.Background(), client, resolve, agent, "bkt", required,
				lazyProjectGrants(context.Background(), client, resolve, "proj", agent))
			if err != nil {
				t.Fatalf("bucketIAMShortfall() unexpected error: %v", err)
			}
			assertSamePermissions(t, "bucketIAMShortfall()", got, tt.wantMissing)

			warnResolve := newRoleResolver(client)
			msg := bucketIAMWarning(context.Background(), client, warnResolve, agent, "bkt", required,
				lazyProjectGrants(context.Background(), client, warnResolve, "proj", agent))
			if len(tt.wantMissing) == 0 {
				if msg != "" {
					t.Fatalf("bucketIAMWarning() = %q, want the pre-flight to stay silent", msg)
				}
				return
			}
			if msg == "" {
				t.Fatalf("bucketIAMWarning() stayed silent, want a warning naming %v", tt.wantMissing)
			}
			for _, p := range tt.wantMissing {
				if !strings.Contains(msg, p) {
					t.Errorf("bucketIAMWarning() = %q, want it to name the missing permission %q", msg, p)
				}
			}
			for _, p := range required {
				if !slices.Contains(tt.wantMissing, p) && strings.Contains(msg, p) {
					t.Errorf("bucketIAMWarning() = %q, must not name %q, which the agent holds", msg, p)
				}
			}
		})
	}
}

func assertSamePermissions(t *testing.T, label string, got, want []string) {
	t.Helper()
	gotSorted := append([]string{}, got...)
	wantSorted := append([]string{}, want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	if !reflect.DeepEqual(gotSorted, wantSorted) {
		t.Fatalf("%s = %v, want %v", label, gotSorted, wantSorted)
	}
}

func TestUnreadableRoleStaysSilentOnASatisfiedBucket(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	client := &fakePreflightClient{
		number: 42,
		bindings: map[string][]iamBinding{"bkt": {
			{Role: "roles/storage.admin", Members: []string{"serviceAccount:" + agent}},
			{Role: "roles/pubsub.publisher", Members: []string{"serviceAccount:" + agent}},
		}},
		roleErr: map[string]error{"roles/pubsub.publisher": fmt.Errorf("permission denied")},
	}

	resolve := newRoleResolver(client)
	msg := bucketIAMWarning(context.Background(), client, resolve, agent, "bkt", requiredProfilePermissions(true),
		lazyProjectGrants(context.Background(), client, resolve, "proj", agent))
	if msg != "" {
		t.Fatalf("bucketIAMWarning() = %q, want silence on a bucket that grants everything", msg)
	}
	if got := resolve.unreadableRoles(); !reflect.DeepEqual(got, []string{"roles/pubsub.publisher"}) {
		t.Errorf("unreadableRoles() = %v, want the failure to be recorded for later reporting", got)
	}
}

func TestUnreadableRoleIsReportedWhenTheBucketFallsShort(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	customRole := "projects/proj/roles/gke.gcsfuse.profileUser"
	client := &fakePreflightClient{
		number:   42,
		bindings: map[string][]iamBinding{"bkt": {{Role: customRole, Members: []string{"serviceAccount:" + agent}}}},
		roleErr:  map[string]error{customRole: fmt.Errorf("permission denied")},
	}

	resolve := newRoleResolver(client)
	msg := bucketIAMWarning(context.Background(), client, resolve, agent, "bkt", requiredProfilePermissions(false),
		lazyProjectGrants(context.Background(), client, resolve, "proj", agent))
	if !strings.Contains(msg, customRole) {
		t.Errorf("bucketIAMWarning() = %q, want it to name the role it could not read", msg)
	}
}

func TestProjectLevelGrantSatisfiesBucketCheck(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	customRole := "projects/proj/roles/gke.gcsfuse.profileUser"

	client := &fakePreflightClient{
		number:          42,
		projectBindings: []iamBinding{{Role: customRole, Members: []string{"serviceAccount:" + agent}}},
		roles:           map[string][]string{customRole: append(append([]string{}, gcsFuseProfileBasePermissions...), gcsFuseAnywhereCachePermissions...)},
		bindings:        map[string][]iamBinding{"bkt": nil},
	}

	grants := lazyProjectGrants(context.Background(), client, newRoleResolver(client), "proj", agent)
	if missing := missingPermissions(requiredProfilePermissions(true), grants()); len(missing) != 0 {
		t.Errorf("lazyProjectGrants() left %v missing, want none", missing)
	}
	grants()
	if !reflect.DeepEqual(client.projectIAMRequests, []string{"proj"}) {
		t.Errorf("projectIAMBindings calls = %v, want exactly one memoized call for \"proj\"", client.projectIAMRequests)
	}
}

func TestProjectLevelGrantsFailOpen(t *testing.T) {
	client := &fakePreflightClient{projectBindingsErr: fmt.Errorf("permission denied")}
	grants := lazyProjectGrants(context.Background(), client, newRoleResolver(client), "proj", "agent@example.com")
	if got := grants(); len(got) != 0 {
		t.Errorf("lazyProjectGrants() = %v, want an empty set when the policy cannot be read", got)
	}
}

func TestWarnOnMissingIAMQueriesEachBucketOnce(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	client := &fakePreflightClient{
		number:   42,
		bindings: map[string][]iamBinding{"bkt": {{Role: "roles/storage.admin", Members: []string{"serviceAccount:" + agent}}}},
	}

	warnOnMissingIAM(context.Background(), client, "proj", agent, []profileMount{
		{Bucket: "bkt", Profile: "gcsfusecsi-training"},
		{Bucket: "bkt", Profile: "gcsfusecsi-serving", UsesAnywhereCache: true},
	})

	if !reflect.DeepEqual(client.bindingRequests, []string{"bkt"}) {
		t.Errorf("bucketIAMBindings calls = %v, want exactly one call for \"bkt\"", client.bindingRequests)
	}
}

func TestWarnOnMissingIAMFallsBackToProjectPolicy(t *testing.T) {
	const agent = "service-42@container-engine-robot.iam.gserviceaccount.com"
	client := &fakePreflightClient{
		number:          42,
		projectBindings: []iamBinding{{Role: "roles/storage.admin", Members: []string{"serviceAccount:" + agent}}},
		bindings:        map[string][]iamBinding{"b1": nil, "b2": nil},
	}

	warnOnMissingIAM(context.Background(), client, "proj", agent, []profileMount{
		{Bucket: "b1", Profile: "gcsfusecsi-training"},
		{Bucket: "b2", Profile: "gcsfusecsi-serving", UsesAnywhereCache: true},
	})

	if !reflect.DeepEqual(client.projectIAMRequests, []string{"proj"}) {
		t.Errorf("projectIAMBindings calls = %v, want exactly one memoized call for \"proj\"", client.projectIAMRequests)
	}
}

func TestBucketCacheUse(t *testing.T) {
	order, needsCache := bucketCacheUse([]profileMount{
		{Bucket: "b1", Profile: "gcsfusecsi-training"},
		{Bucket: "b2", Profile: "gcsfusecsi-serving", UsesAnywhereCache: true},
		{Bucket: "b1", Profile: "gcsfusecsi-serving", UsesAnywhereCache: true},
	})
	if !reflect.DeepEqual(order, []string{"b1", "b2"}) {
		t.Errorf("bucketCacheUse() order = %v, want [b1 b2]", order)
	}
	if !needsCache["b1"] || !needsCache["b2"] {
		t.Errorf("bucketCacheUse() needsCache = %v, want both buckets flagged", needsCache)
	}
}

func newFakeGCPPreflightClient(t *testing.T, handler http.HandlerFunc) (*gcpPreflightClient, func() []string) {
	t.Helper()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	clientOpts := []option.ClientOption{
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
	}
	storageSvc, err := gcs.NewService(ctx, append(clientOpts, option.WithEndpoint(srv.URL+"/storage/v1/"))...)
	if err != nil {
		t.Fatalf("failed to build Cloud Storage test client: %v", err)
	}
	iamSvc, err := iamapi.NewService(ctx, append(clientOpts, option.WithEndpoint(srv.URL+"/"))...)
	if err != nil {
		t.Fatalf("failed to build IAM test client: %v", err)
	}
	crmSvc, err := crm.NewService(ctx, append(clientOpts, option.WithEndpoint(srv.URL+"/"))...)
	if err != nil {
		t.Fatalf("failed to build Resource Manager test client: %v", err)
	}

	return &gcpPreflightClient{storage: storageSvc, iam: iamSvc, crm: crmSvc}, func() []string { return paths }
}

func preflightAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/b/bkt/iam"):
			_, _ = fmt.Fprint(w, `{"bindings":[{"role":"roles/storage.admin","members":["serviceAccount:service-42@container-engine-robot.iam.gserviceaccount.com"]}]}`)
		case strings.Contains(r.URL.Path, "/b/bkt"):
			_, _ = fmt.Fprint(w, `{"location":"US-CENTRAL1+US-EAST1","locationType":"dual-region","customPlacementConfig":{"dataLocations":["US-CENTRAL1","US-EAST1"]}}`)
		case strings.Contains(r.URL.Path, "/roles/"):
			_, _ = fmt.Fprint(w, `{"includedPermissions":["storage.buckets.get","storage.objects.list"]}`)
		case strings.HasSuffix(r.URL.Path, ":getIamPolicy"):
			_, _ = fmt.Fprint(w, `{"bindings":[{"role":"projects/proj/roles/gke.gcsfuse.profileUser","members":["serviceAccount:service-42@container-engine-robot.iam.gserviceaccount.com"]}]}`)
		case strings.Contains(r.URL.Path, "/projects/"):
			_, _ = fmt.Fprint(w, `{"projectNumber":"42"}`)
		default:
			http.Error(w, `{"error":{"code":404,"message":"unexpected path"}}`, http.StatusNotFound)
		}
	}
}

func TestGCPPreflightClientReadsBucketMetadata(t *testing.T) {
	client, requestedPaths := newFakeGCPPreflightClient(t, preflightAPIHandler())
	ctx := context.Background()

	bindings, err := client.bucketIAMBindings(ctx, "bkt")
	if err != nil {
		t.Fatalf("bucketIAMBindings() unexpected error: %v", err)
	}
	wantBindings := []iamBinding{{
		Role:    "roles/storage.admin",
		Members: []string{"serviceAccount:service-42@container-engine-robot.iam.gserviceaccount.com"},
	}}
	if !reflect.DeepEqual(bindings, wantBindings) {
		t.Errorf("bucketIAMBindings() = %+v, want %+v", bindings, wantBindings)
	}

	loc, err := client.bucketLocation(ctx, "bkt")
	if err != nil {
		t.Fatalf("bucketLocation() unexpected error: %v", err)
	}
	wantLoc := bucketLocation{
		Location:      "US-CENTRAL1+US-EAST1",
		LocationType:  "dual-region",
		DataLocations: []string{"US-CENTRAL1", "US-EAST1"},
	}
	if !reflect.DeepEqual(loc, wantLoc) {
		t.Errorf("bucketLocation() = %+v, want %+v", loc, wantLoc)
	}

	paths := requestedPaths()
	assertRequested(t, paths, "/storage/v1/b/bkt/iam", "optionsRequestedPolicyVersion=3")
	assertRequested(t, paths, "/storage/v1/b/bkt?")
}

func TestGCPPreflightClientReadsProjectAndRole(t *testing.T) {
	client, requestedPaths := newFakeGCPPreflightClient(t, preflightAPIHandler())
	ctx := context.Background()

	number, err := client.projectNumber(ctx, "proj")
	if err != nil || number != 42 {
		t.Fatalf("projectNumber() = %d, %v, want 42, nil", number, err)
	}

	perms, err := client.rolePermissions(ctx, "projects/proj/roles/gke.gcsfuse.profileUser")
	if err != nil {
		t.Fatalf("rolePermissions() unexpected error: %v", err)
	}
	if !reflect.DeepEqual(perms, []string{"storage.buckets.get", "storage.objects.list"}) {
		t.Errorf("rolePermissions() = %v, want the two included permissions", perms)
	}

	predefined, err := client.rolePermissions(ctx, "roles/storage.legacyBucketReader")
	if err != nil {
		t.Fatalf("rolePermissions() unexpected error for a predefined role: %v", err)
	}
	if !reflect.DeepEqual(predefined, []string{"storage.buckets.get", "storage.objects.list"}) {
		t.Errorf("rolePermissions() = %v, want the two included permissions", predefined)
	}

	if _, err := client.rolePermissions(ctx, "storage.admin"); err == nil {
		t.Error("rolePermissions() expected an error for a role name with no recognized scheme")
	}

	bindings, err := client.projectIAMBindings(ctx, "proj")
	if err != nil {
		t.Fatalf("projectIAMBindings() unexpected error: %v", err)
	}
	wantBindings := []iamBinding{{
		Role:    "projects/proj/roles/gke.gcsfuse.profileUser",
		Members: []string{"serviceAccount:service-42@container-engine-robot.iam.gserviceaccount.com"},
	}}
	if !reflect.DeepEqual(bindings, wantBindings) {
		t.Errorf("projectIAMBindings() = %+v, want %+v", bindings, wantBindings)
	}

	paths := requestedPaths()
	assertRequested(t, paths, "/v1/projects/proj?")
	assertRequested(t, paths, "/v1/projects/proj/roles/gke.gcsfuse.profileUser?")
	assertRequested(t, paths, "/v1/projects/proj:getIamPolicy")
	assertRequested(t, paths, "/v1/roles/storage.legacyBucketReader?")
}

func TestGCPPreflightClientSurfacesAPIErrors(t *testing.T) {
	client, _ := newFakeGCPPreflightClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":403,"message":"permission denied"}}`, http.StatusForbidden)
	})

	ctx := context.Background()
	if _, err := client.bucketIAMBindings(ctx, "bkt"); err == nil {
		t.Error("bucketIAMBindings() expected an error on a 403 response")
	}
	if _, err := client.bucketLocation(ctx, "bkt"); err == nil {
		t.Error("bucketLocation() expected an error on a 403 response")
	}
	if _, err := client.projectNumber(ctx, "proj"); err == nil {
		t.Error("projectNumber() expected an error on a 403 response")
	}
}

func assertRequested(t *testing.T, paths []string, wantParts ...string) {
	t.Helper()
	for _, p := range paths {
		matched := true
		for _, part := range wantParts {
			if !strings.Contains(p, part) {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Errorf("requested paths %v, want one containing %v", paths, wantParts)
}
