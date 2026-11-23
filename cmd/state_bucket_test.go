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

package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"hpc-toolkit/pkg/config"

	"cloud.google.com/go/storage"
	"github.com/zclconf/go-cty/cty"
)

// fakeGcs is a minimal stand-in for the GCS JSON API. Bucket lookups (GET)
// answer with lookupStatus; anything else is treated as a bucket creation.
type fakeGcs struct {
	mu           sync.Mutex
	lookupStatus int
	lookups      []string
	creates      []string // request URI of each creation
	createBodies []string
}

func (f *fakeGcs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		f.lookups = append(f.lookups, r.URL.Path)
		w.WriteHeader(f.lookupStatus)
		if f.lookupStatus == http.StatusOK {
			_, _ = w.Write([]byte(`{"name":"bucket"}`))
		} else {
			_, _ = w.Write([]byte(`{"error":{"code":0,"message":"` + http.StatusText(f.lookupStatus) + `"}}`))
		}
		return
	}
	f.creates = append(f.creates, r.URL.RequestURI())
	f.createBodies = append(f.createBodies, string(body))
	_, _ = w.Write([]byte(`{"name":"bucket"}`))
}

// useFakeGcs points the storage client at a fake GCS server.
func useFakeGcs(t *testing.T, lookupStatus int) *fakeGcs {
	t.Helper()
	f := &fakeGcs{lookupStatus: lookupStatus}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("STORAGE_EMULATOR_HOST", strings.TrimPrefix(srv.URL, "http://"))
	return f
}

// stubConfirm sets --auto-approve and the answer to any prompt, and returns
// the prompts that were shown.
func stubConfirm(t *testing.T, autoApprove, answer bool) *[]string {
	t.Helper()
	prompts := []string{}
	origAuto, origConfirm := flagAutoApprove, confirmActionFunc
	flagAutoApprove = autoApprove
	confirmActionFunc = func(prompt string) bool {
		prompts = append(prompts, prompt)
		return answer
	}
	t.Cleanup(func() { flagAutoApprove, confirmActionFunc = origAuto, origConfirm })
	return &prompts
}

func backendGroup(name, backendType string, cfg map[string]cty.Value) config.Group {
	return config.Group{
		Name: config.GroupName(name),
		TerraformBackend: config.TerraformBackend{
			Type:          backendType,
			Configuration: config.NewDict(cfg),
		},
	}
}

func stateBucketBlueprint(vars map[string]cty.Value, groups ...config.Group) config.Blueprint {
	if vars == nil {
		vars = map[string]cty.Value{
			"project_id": cty.StringVal("my-project"),
			"region":     cty.StringVal("us-central1"),
		}
	}
	return config.Blueprint{Vars: config.NewDict(vars), Groups: groups}
}

func plainGcsBlueprint() config.Blueprint {
	return stateBucketBlueprint(nil, backendGroup("primary", "gcs", map[string]cty.Value{
		"bucket": cty.StringVal("my-state"),
	}))
}

func TestStateBucketExists(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusOK)
	prompts := stubConfirm(t, false, false)
	bp := plainGcsBlueprint()

	if err := verifyGcsBuckets(context.Background(), bp); err != nil {
		t.Errorf("create: unexpected error: %v", err)
	}
	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("deploy: unexpected error: %v", err)
	}
	if len(gcs.lookups) != 2 || len(gcs.creates) != 0 || len(*prompts) != 0 {
		t.Errorf("got lookups=%v creates=%v prompts=%v; want two lookups only", gcs.lookups, gcs.creates, *prompts)
	}
}

func TestStateBucketMissingCreateOnlyWarns(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusNotFound)
	prompts := stubConfirm(t, false, true)

	if err := verifyGcsBuckets(context.Background(), plainGcsBlueprint()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(gcs.creates) != 0 || len(*prompts) != 0 {
		t.Errorf("create must not prompt or create; got creates=%v prompts=%v", gcs.creates, *prompts)
	}
}

func TestStateBucketMissingDeployDeclined(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusNotFound)
	prompts := stubConfirm(t, false, false)

	err := createGcsBucketsIfMissing(context.Background(), plainGcsBlueprint())
	if err == nil || err.Error() != "user aborted" {
		t.Errorf("got error %v; want \"user aborted\"", err)
	}
	if len(gcs.creates) != 0 {
		t.Errorf("bucket must not be created when declined; got %v", gcs.creates)
	}
	if len(*prompts) != 1 {
		t.Fatalf("got %d prompts; want 1", len(*prompts))
	}
	prompt := (*prompts)[0]
	for _, want := range []string{"'my-state'", "project 'my-project'", "location 'us-central1'",
		"gs://my-state --project=my-project --location=us-central1 --public-access-prevention"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt does not contain %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "local credentials") {
		t.Errorf("prompt for a plain backend must not mention local credentials:\n%s", prompt)
	}
}

func TestStateBucketMissingDeployAccepted(t *testing.T) {
	for _, autoApprove := range []bool{false, true} {
		gcs := useFakeGcs(t, http.StatusNotFound)
		prompts := stubConfirm(t, autoApprove, true)

		if err := createGcsBucketsIfMissing(context.Background(), plainGcsBlueprint()); err != nil {
			t.Errorf("autoApprove=%v: unexpected error: %v", autoApprove, err)
		}
		if wantPrompts := map[bool]int{false: 1, true: 0}[autoApprove]; len(*prompts) != wantPrompts {
			t.Errorf("autoApprove=%v: got %d prompts; want %d", autoApprove, len(*prompts), wantPrompts)
		}
		if len(gcs.creates) != 1 {
			t.Fatalf("autoApprove=%v: got creates=%v; want one", autoApprove, gcs.creates)
		}
		if !strings.Contains(gcs.creates[0], "project=my-project") {
			t.Errorf("bucket created in the wrong project: %s", gcs.creates[0])
		}
		for _, want := range []string{`"name":"my-state"`, `"location":"us-central1"`, `"publicAccessPrevention":"enforced"`} {
			if !strings.Contains(gcs.createBodies[0], want) {
				t.Errorf("creation request does not contain %s: %s", want, gcs.createBodies[0])
			}
		}
	}
}

func TestStateBucketMissingNoRegion(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusNotFound)
	prompts := stubConfirm(t, false, true)
	bp := stateBucketBlueprint(map[string]cty.Value{"project_id": cty.StringVal("my-project")},
		backendGroup("primary", "gcs", map[string]cty.Value{"bucket": cty.StringVal("my-state")}))

	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains((*prompts)[0], "default location") || strings.Contains((*prompts)[0], "--location") {
		t.Errorf("prompt should mention the default location and no --location flag:\n%s", (*prompts)[0])
	}
	if len(gcs.creates) != 1 {
		t.Errorf("got creates=%v; want one", gcs.creates)
	}
}

func TestStateBucketMissingNoProject(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusNotFound)
	prompts := stubConfirm(t, true, true)
	bp := stateBucketBlueprint(map[string]cty.Value{"region": cty.StringVal("us-central1")},
		backendGroup("primary", "gcs", map[string]cty.Value{"bucket": cty.StringVal("my-state")}))

	err := createGcsBucketsIfMissing(context.Background(), bp)
	if err == nil || !strings.Contains(err.Error(), "project_id is missing") {
		t.Errorf("got error %v; want a missing project_id error", err)
	}
	if len(gcs.creates) != 0 || len(*prompts) != 0 {
		t.Errorf("got creates=%v prompts=%v; want none", gcs.creates, *prompts)
	}
}

// An inconclusive lookup is not a missing bucket: warn and carry on.
func TestStateBucketLookupInconclusive(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusForbidden)
	prompts := stubConfirm(t, true, true)
	bp := plainGcsBlueprint()

	if err := verifyGcsBuckets(context.Background(), bp); err != nil {
		t.Errorf("create: unexpected error: %v", err)
	}
	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("deploy: unexpected error: %v", err)
	}
	if len(gcs.creates) != 0 || len(*prompts) != 0 {
		t.Errorf("got creates=%v prompts=%v; want none", gcs.creates, *prompts)
	}
}

func TestStateBucketCustomEndpointSkipped(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusNotFound)
	prompts := stubConfirm(t, true, true)
	bp := stateBucketBlueprint(nil, backendGroup("primary", "gcs", map[string]cty.Value{
		"bucket":                 cty.StringVal("my-state"),
		backendCustomEndpointKey: cty.StringVal("https://storage.example.com/storage/v1/"),
	}))

	if err := verifyGcsBuckets(context.Background(), bp); err != nil {
		t.Errorf("create: unexpected error: %v", err)
	}
	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("deploy: unexpected error: %v", err)
	}
	if len(gcs.lookups) != 0 || len(gcs.creates) != 0 || len(*prompts) != 0 {
		t.Errorf("got lookups=%v creates=%v prompts=%v; want no GCS calls and no prompt", gcs.lookups, gcs.creates, *prompts)
	}
}

func TestStateBucketEmptyCustomEndpointIsChecked(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusOK)
	stubConfirm(t, false, false)
	bp := stateBucketBlueprint(nil, backendGroup("primary", "gcs", map[string]cty.Value{
		"bucket":                 cty.StringVal("my-state"),
		backendCustomEndpointKey: cty.StringVal(""),
	}))

	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(gcs.lookups) != 1 {
		t.Errorf("got lookups=%v; want one", gcs.lookups)
	}
}

// A bucket shared by a custom-endpoint backend and a plain one is still checked.
func TestStateBucketSharedWithCustomEndpoint(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusOK)
	stubConfirm(t, false, false)
	bp := stateBucketBlueprint(nil,
		backendGroup("one", "gcs", map[string]cty.Value{
			"bucket":                 cty.StringVal("my-state"),
			backendCustomEndpointKey: cty.StringVal("https://storage.example.com/storage/v1/"),
		}),
		backendGroup("two", "gcs", map[string]cty.Value{"bucket": cty.StringVal("my-state")}))

	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(gcs.lookups) != 1 {
		t.Errorf("got lookups=%v; want one", gcs.lookups)
	}
}

func TestStateBucketBackendCredentialsStillCreated(t *testing.T) {
	for _, key := range backendCredentialKeys {
		gcs := useFakeGcs(t, http.StatusNotFound)
		prompts := stubConfirm(t, false, true)
		bp := stateBucketBlueprint(nil, backendGroup("primary", "gcs", map[string]cty.Value{
			"bucket": cty.StringVal("my-state"),
			key:      cty.StringVal("some-value"),
		}))

		if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
			t.Errorf("%s: unexpected error: %v", key, err)
		}
		if len(gcs.creates) != 1 {
			t.Errorf("%s: got creates=%v; want one", key, gcs.creates)
		}
		if len(*prompts) != 1 || !strings.Contains((*prompts)[0], "local credentials") {
			t.Errorf("%s: prompt should say local credentials are used; got %v", key, *prompts)
		}
	}
}

func TestStateBucketNonGcsBackendIgnored(t *testing.T) {
	gcs := useFakeGcs(t, http.StatusNotFound)
	prompts := stubConfirm(t, true, true)
	bp := stateBucketBlueprint(nil, backendGroup("primary", "s3", map[string]cty.Value{
		"bucket": cty.StringVal("my-state"),
	}))

	if err := verifyGcsBuckets(context.Background(), bp); err != nil {
		t.Errorf("create: unexpected error: %v", err)
	}
	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("deploy: unexpected error: %v", err)
	}
	if len(gcs.lookups) != 0 || len(gcs.creates) != 0 || len(*prompts) != 0 {
		t.Errorf("got lookups=%v creates=%v prompts=%v; want none", gcs.lookups, gcs.creates, *prompts)
	}
}

// Not being able to create a client must not stop create or deploy.
func TestStateBucketClientFailureDoesNotBlock(t *testing.T) {
	prompts := stubConfirm(t, true, true)
	orig := newGcsClient
	newGcsClient = func(context.Context) (*storage.Client, error) {
		return nil, errors.New("could not find default credentials")
	}
	t.Cleanup(func() { newGcsClient = orig })
	bp := plainGcsBlueprint()

	if err := verifyGcsBuckets(context.Background(), bp); err != nil {
		t.Errorf("create: unexpected error: %v", err)
	}
	if err := createGcsBucketsIfMissing(context.Background(), bp); err != nil {
		t.Errorf("deploy: unexpected error: %v", err)
	}
	if len(*prompts) != 0 {
		t.Errorf("got prompts=%v; want none", *prompts)
	}
}

func TestCheckBackendConfigType(t *testing.T) {
	tests := []struct {
		name        string
		currentType string
		flags       []string
		wantErr     bool
	}{
		{"no flag", "s3", nil, false},
		{"no backend set", "", []string{"bucket=b"}, false},
		{"gcs backend set", "gcs", []string{"bucket=b"}, false},
		{"other backend, no type", "s3", []string{"bucket=b"}, true},
		{"other backend, same type given", "s3", []string{"bucket=b", "type=s3"}, false},
		{"other backend, gcs type given", "s3", []string{"type=gcs", "bucket=b"}, false},
		{"type only as a value", "s3", []string{"bucket=type"}, true},
		{"type only as a key prefix", "s3", []string{"typed=x"}, true},
	}
	for _, tc := range tests {
		err := checkBackendConfigType(tc.currentType, tc.flags)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: got error %v; wantErr=%v", tc.name, err, tc.wantErr)
		}
		if err == nil {
			continue
		}
		for _, want := range []string{"type=s3", "type=gcs"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error should offer %s: %v", tc.name, want, err)
			}
		}
	}
}
