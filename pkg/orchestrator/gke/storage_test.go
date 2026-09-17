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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"

	"cloud.google.com/go/filestore/apiv1/filestorepb"
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
			wantErrSub: "options= is currently only supported for GCS",
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
			wantSrc:     "gs://my-bucket/run1",
			wantDest:    "/checkpoints",
			wantRO:      false,
			wantProfile: "gcsfusecsi-checkpointing",
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
			wantErrSub: "profile= is only supported for GCS fuse volumes",
		},
		{
			name:       "profile not supported for pvc",
			input:      "my-pvc;/data;profile=training",
			wantErr:    true,
			wantErrSub: "profile= is only supported for GCS fuse volumes",
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

	wantPVC := "gcluster-gcsfuse-imagenet-dataset-training"
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
		if got := nestedString(t, pv, "spec", "claimRef", "name"); got != "gcluster-gcsfuse-imagenet-dataset-training" {
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
	if infos[0].Source != "gcluster-gcsfuse-model-checkpoints-checkpointing" {
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

func TestGCSFuseGatewayNaming_Determinism(t *testing.T) {
	base := gcsFuseGatewayPVCName("b", "training", "", nil)
	if base != "gcluster-gcsfuse-b-training" {
		t.Fatalf("canonical name = %q", base)
	}
	if again := gcsFuseGatewayPVCName("b", "training", "", nil); again != base {
		t.Errorf("name is not deterministic: %q vs %q", again, base)
	}

	withOpts := gcsFuseGatewayPVCName("b", "training", "implicit-dirs", nil)
	if withOpts == base {
		t.Error("custom options must change the gateway name")
	}
	if again := gcsFuseGatewayPVCName("b", "training", "implicit-dirs", nil); again != withOpts {
		t.Errorf("hashed name is not deterministic: %q vs %q", again, withOpts)
	}

	a := gcsFuseGatewayPVCName("b", "training", "", map[string]string{"x": "1", "y": "2"})
	bName := gcsFuseGatewayPVCName("b", "training", "", map[string]string{"y": "2", "x": "1"})
	if a != bName {
		t.Errorf("attribute ordering changed the name: %q vs %q", a, bName)
	}
	if diff := gcsFuseGatewayPVCName("b", "training", "", map[string]string{"x": "2"}); diff == a {
		t.Error("divergent attributes must produce different gateway names")
	}

	long := gcsFuseGatewayPVCName(strings.Repeat("a", 300), "training", "", nil)
	if len(long) > maxGeneratedPVCNameLength {
		t.Errorf("name length = %d, want <= %d", len(long), maxGeneratedPVCNameLength)
	}
	if strings.HasSuffix(long, "-") {
		t.Errorf("truncated name %q must not end with '-'", long)
	}
	longHashed := gcsFuseGatewayPVCName(strings.Repeat("a", 300), "training", "implicit-dirs", nil)
	if len(longHashed) > maxGeneratedPVCNameLength {
		t.Errorf("hashed name length = %d, want <= %d", len(longHashed), maxGeneratedPVCNameLength)
	}
}

func TestGCSFuseGatewayNaming_TruncationIsCollisionFree(t *testing.T) {
	longBucket := strings.Repeat("a", 230)

	train := gcsFuseGatewayPVCName(longBucket, "training", "", nil)
	ckpt := gcsFuseGatewayPVCName(longBucket, "checkpointing", "", nil)
	if train == ckpt {
		t.Errorf("different profiles on a long bucket collapsed onto %q", train)
	}
	if !strings.HasSuffix(train, "-training") || !strings.HasSuffix(ckpt, "-checkpointing") {
		t.Errorf("profile suffix was truncated away: %q / %q", train, ckpt)
	}

	one := gcsFuseGatewayPVCName(strings.Repeat("a", 220)+"-one", "training", "", nil)
	two := gcsFuseGatewayPVCName(strings.Repeat("a", 220)+"-two", "training", "", nil)
	if one == two {
		t.Errorf("different long buckets collapsed onto %q", one)
	}

	withOpts := gcsFuseGatewayPVCName(longBucket, "training", "implicit-dirs", nil)
	if withOpts == train {
		t.Error("custom options must still change the name after truncation")
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
	dotted := gcsFuseGatewayPVCName("my.bucket", "training", "", nil)
	hyphen := gcsFuseGatewayPVCName("my-bucket", "training", "", nil)
	under := gcsFuseGatewayPVCName("my_bucket", "training", "", nil)

	if dotted == hyphen {
		t.Errorf("buckets my.bucket and my-bucket collapsed onto %q", dotted)
	}
	if under == hyphen {
		t.Errorf("buckets my_bucket and my-bucket collapsed onto %q", under)
	}
	if dotted == under {
		t.Errorf("buckets my.bucket and my_bucket collapsed onto %q", dotted)
	}

	if hyphen != "gcluster-gcsfuse-my-bucket-training" {
		t.Errorf("a losslessly sanitized bucket must not gain a digest, got %q", hyphen)
	}

	if again := gcsFuseGatewayPVCName("my.bucket", "training", "", nil); again != dotted {
		t.Errorf("lossy name is not deterministic: %q vs %q", again, dotted)
	}
	if !strings.HasPrefix(dotted, "gcluster-gcsfuse-my-bucket-") {
		t.Errorf("lossy name %q lost its readable prefix", dotted)
	}

	domain := gcsFuseGatewayPVCName("example.com.datasets", "training", "", nil)
	flat := gcsFuseGatewayPVCName("example-com-datasets", "training", "", nil)
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
		"gs://;/data;profile=training",
		"gs:///;/data;profile=training",
	} {
		if err := sm.ValidateMounts([]string{mount}); err == nil {
			t.Errorf("ValidateMounts(%q) = nil, want an error before the image build", mount)
		}
	}

	if err := sm.ValidateMounts([]string{"gs://MyBucket;/data"}); err != nil {
		t.Errorf("profile-less gs:// validation changed behaviour: %v", err)
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
		Source:    "gs://logs/run1",
		MountPath: "/logs",
		Type:      "gcsfuse",
		ReadOnly:  false,
		Options:   "implicit-dirs",
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
	if infos[0].Source != "gcluster-gcsfuse-training-data-training" {
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
		"gs://shared/datasets;/data;ro;profile=training",
		"gs://shared/eval;/eval;ro;profile=training",
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

func TestVolumeAttributes_MountOptionsKeyRejected(t *testing.T) {
	sm := &StorageManager{}

	for _, mount := range []string{
		"gs://my-bucket;/data;attributes=mountOptions=implicit-dirs",
		"gs://my-bucket;/data;profile=training;attributes=mountOptions=implicit-dirs",
	} {
		_, err := sm.parseSingleVolume(mount)
		if err == nil {
			t.Errorf("%s: expected mountOptions to be rejected as an attribute", mount)
			continue
		}
		if !strings.Contains(err.Error(), "use options=") {
			t.Errorf("%s: error should point at options=, got: %v", mount, err)
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
