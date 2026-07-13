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
	"context"
	"fmt"
	"strings"
	"testing"

	"hpc-toolkit/pkg/orchestrator"

	"cloud.google.com/go/filestore/apiv1/filestorepb"
)

func TestParseSingleVolume(t *testing.T) {
	sm := &StorageManager{}

	tests := []struct {
		name       string
		input      string
		wantSrc    string
		wantDest   string
		wantRO     bool
		wantErr    bool
		wantErrSub string
	}{
		{
			name:     "valid hostPath",
			input:    "/host/path:/container/path",
			wantSrc:  "/host/path",
			wantDest: "/container/path",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid hostPath rw",
			input:    "/host/path:/container/path:rw",
			wantSrc:  "/host/path",
			wantDest: "/container/path",
			wantRO:   false,
			wantErr:  false,
		},
		{
			name:     "valid pvc",
			input:    "my-pvc:/data",
			wantSrc:  "my-pvc",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid gcsfuse",
			input:    "gs://my-bucket:/data",
			wantSrc:  "gs://my-bucket",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore",
			input:    "filestore://my-instance/share:/data",
			wantSrc:  "filestore://my-instance/share",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore with port",
			input:    "filestore://10.0.0.2:2049/share:/data",
			wantSrc:  "filestore://10.0.0.2:2049/share",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore with port and mode",
			input:    "filestore://10.0.0.2:2049/share:/data:rw",
			wantSrc:  "filestore://10.0.0.2:2049/share",
			wantDest: "/data",
			wantRO:   false,
			wantErr:  false,
		},
		{
			name:       "invalid scheme",
			input:      "parallelstore://my-instance/share:/data",
			wantErr:    true,
			wantErrSub: "Unsupported scheme",
		},
		{
			name:       "invalid scheme with mode",
			input:      "parallelstore://my-instance/share:/data:ro",
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
			input:      "gs://my-bucket:rw",
			wantErr:    true,
			wantErrSub: "Missing destination",
		},
		{
			name:       "missing destination for filestore with mode",
			input:      "filestore://my-instance/share:ro",
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
			input:    "filestore://[2001:db8::1]/share:/data",
			wantSrc:  "filestore://[2001:db8::1]/share",
			wantDest: "/data",
			wantRO:   true,
			wantErr:  false,
		},
		{
			name:     "valid filestore IPv6 rw",
			input:    "filestore://[2001:db8::1]/share:/data:rw",
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, dest, readOnly, err := sm.parseSingleVolume(tc.input)

			if (err != nil) != tc.wantErr {
				t.Fatalf("parseSingleVolume() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if tc.wantErrSub != "" && err != nil && !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error message = %q, want sub %q", err.Error(), tc.wantErrSub)
				}
				return
			}

			if src != tc.wantSrc {
				t.Errorf("parseSingleVolume() src = %v, want %v", src, tc.wantSrc)
			}
			if dest != tc.wantDest {
				t.Errorf("parseSingleVolume() dest = %v, want %v", dest, tc.wantDest)
			}
			if readOnly != tc.wantRO {
				t.Errorf("parseSingleVolume() readOnly = %v, want %v", readOnly, tc.wantRO)
			}
		})
	}
}

func TestValidateMounts(t *testing.T) {
	sm := &StorageManager{}

	mounts := []string{
		"gs://my-bucket:/data",
		"my-pvc:/data", // duplicate dest
	}
	err := sm.ValidateMounts(mounts)
	if err == nil || !strings.Contains(err.Error(), "duplicate volume destination") {
		t.Errorf("expected duplicate destination error, got %v", err)
	}

	mounts = []string{
		"gs://my-bucket:/data1",
		"gs://my-bucket:/data2", // duplicate src
	}
	err = sm.ValidateMounts(mounts)
	if err == nil || !strings.Contains(err.Error(), "duplicate volume source") {
		t.Errorf("expected duplicate source error, got %v", err)
	}

	mounts = []string{
		"parallelstore://foo:/data", // unsupported scheme
	}
	err = sm.ValidateMounts(mounts)
	if err == nil || !strings.Contains(err.Error(), "Unsupported scheme") {
		t.Errorf("expected unsupported scheme error, got %v", err)
	}

	mounts = []string{
		"gs://my-bucket:/data1",
		"my-pvc:/data2",
	}
	err = sm.ValidateMounts(mounts)
	if err != nil {
		t.Errorf("expected no error for valid mounts, got %v", err)
	}
}

func TestProcessMounts_Basic(t *testing.T) {
	sm := &StorageManager{}
	job := orchestrator.JobDefinition{}

	mounts := []string{
		"gs://my-bucket:/data",
		"/host/path:/host",
		"my-pvc:/pvc",
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
		"filestore://10.0.0.2/share:/data",
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
		"filestore://10.0.0.2/MY_complex_SHARE/sub_folder/sub_subfolder:/data",
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
		"filestore://10.0.0.2//my_share_name:/data",
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
		"filestore://10.0.0.2/share@name.with-dots_and_stuff!:/data",
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
		"filestore://my-filestore-instance/my_share:/data",
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

	// Test trailing slash handling in Filestore mount URI (single and multiple)
	mountsSlash := []string{
		"filestore://10.0.0.2/share/:/data",
		"filestore://10.0.0.2/share///:/data2",
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
	verifyFilestoreManifest(t, manifestsSlash[0], "gcluster-filestore-10-0-0-2-share", "10.0.0.2", "/share", "1024Gi")

	if infosSlash[1].Source != "gcluster-filestore-10-0-0-2-share" {
		t.Errorf("expected source gcluster-filestore-10-0-0-2-share, got %s", infosSlash[1].Source)
	}
	verifyFilestoreManifest(t, manifestsSlash[1], "gcluster-filestore-10-0-0-2-share", "10.0.0.2", "/share", "1024Gi")
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

func TestProcessMounts_Filestore_Invalid(t *testing.T) {
	sm := &StorageManager{}
	job := orchestrator.JobDefinition{}

	// Test invalid empty share name
	_, _, err := sm.ProcessMounts([]string{"filestore://10.0.0.2/:/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "Expected format: filestore://") {
		t.Errorf("expected error for empty share name, got %v", err)
	}

	// Test invalid empty share name with multiple slashes
	_, _, err = sm.ProcessMounts([]string{"filestore://10.0.0.2///:/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "Expected format: filestore://") {
		t.Errorf("expected error for empty share name with multiple slashes, got %v", err)
	}

	// Test invalid empty instance name
	_, _, err = sm.ProcessMounts([]string{"filestore:///share:/data"}, job)
	if err == nil || !strings.Contains(err.Error(), "Expected format: filestore://") {
		t.Errorf("expected error for empty instance name, got %v", err)
	}

	// Test mock Filestore client returning lookup error
	smMockError := &StorageManager{
		getFilestoreIP: func(ctx context.Context, projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
			return "", "", 0, fmt.Errorf("filestore API lookup failed")
		},
	}
	_, _, err = smMockError.ProcessMounts([]string{"filestore://my-instance/share:/data"}, job)
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
		"filestore://my-filestore/share:/data",
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
		"filestore://10.0.0.3/share:/data",
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
		"filestore://my-filestore-instance/share:/data",
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
		"filestore://my-filestore/share:/data",
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
		"filestore://a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-name/a-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-very-long-share:/data",
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
		"filestore://inst1/share1:/data1",
		"filestore://inst2/share2:/data2",
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
		"filestore://[2001:db8::1]/share:/data",
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
		"filestore://[2001:db8::1:invalid]/share:/data",
	}
	_, _, err = smResolved.ProcessMounts(invalidMounts, job)
	if err == nil {
		t.Errorf("expected error for invalid IPv6 format, got nil")
	}
}
