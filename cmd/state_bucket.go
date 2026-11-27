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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulewriter"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/zclconf/go-cty/cty"
)

// backendCustomEndpointKey is the "gcs" backend setting that points Terraform
// at a storage endpoint other than Google Cloud Storage.
const backendCustomEndpointKey = "storage_custom_endpoint"

// backendCredentialKeys are the "gcs" backend settings that make Terraform
// authenticate differently from the local Application Default Credentials.
var backendCredentialKeys = []string{"credentials", "access_token", "impersonate_service_account"}

// newGcsClient creates the client used to look up and create state buckets.
// It is a variable so that tests can replace it.
var newGcsClient = func(ctx context.Context) (*storage.Client, error) {
	return storage.NewClient(ctx)
}

// stateBucket is a GCS bucket holding Terraform state that gcluster can look
// up and, if it is missing, create.
type stateBucket struct {
	name string
	// ownCredentials is true if a backend using this bucket sets its own
	// credentials or impersonation. gcluster still uses local credentials.
	ownCredentials bool
}

// backendSets reports whether an evaluated backend configuration sets key to
// a non-empty value.
func backendSets(cfg config.Dict, key string) bool {
	if !cfg.Has(key) {
		return false
	}
	v, _ := cfg.Get(key).Unmark()
	if v.IsNull() {
		return false
	}
	if v.IsKnown() && v.Type() == cty.String && v.AsString() == "" {
		return false
	}
	return true
}

// gcsStateBuckets returns the Terraform state buckets that gcluster should
// look up. A bucket is left out, with a log line, if every "gcs" backend that
// uses it sets storage_custom_endpoint: such a bucket does not live in Google
// Cloud Storage, so a lookup there says nothing about it.
func gcsStateBuckets(bp config.Blueprint) ([]stateBucket, error) {
	// Also validates the bucket names of all "gcs" backends.
	names, err := modulewriter.GetUniqueGcsBuckets(bp)
	if err != nil {
		return nil, err
	}

	type usage struct{ inGcs, ownCredentials bool }
	uses := map[string]*usage{}
	for _, g := range bp.Groups {
		be := g.TerraformBackend
		if be.Type != "gcs" || !be.Configuration.Has("bucket") {
			continue
		}
		cfg, err := bp.EvalDict(be.Configuration)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate terraform backend configuration for group %q: %w", g.Name, err)
		}
		name := cfg.Get("bucket").AsString()
		if uses[name] == nil {
			uses[name] = &usage{}
		}
		if backendSets(cfg, backendCustomEndpointKey) {
			continue
		}
		uses[name].inGcs = true
		for _, key := range backendCredentialKeys {
			if backendSets(cfg, key) {
				uses[name].ownCredentials = true
			}
		}
	}

	buckets := []stateBucket{}
	for _, name := range names {
		u := uses[name]
		if u == nil || !u.inGcs {
			logging.Info("Not checking Terraform state bucket '%s': its backend sets %s, so it is not looked up or created in Google Cloud Storage.", name, backendCustomEndpointKey)
			continue
		}
		buckets = append(buckets, stateBucket{name: name, ownCredentials: u.ownCredentials})
	}
	return buckets, nil
}

// describeBucketCreation returns where and how a missing state bucket would be
// created, for the confirmation prompt, and the equivalent gcloud command.
func describeBucketCreation(bucket stateBucket, projectID, region string) (target, manualCmd string) {
	target = fmt.Sprintf("project '%s'", projectID)
	manualCmd = fmt.Sprintf("gcloud storage buckets create gs://%s --project=%s", bucket.name, projectID)
	if region != "" {
		target += fmt.Sprintf(", location '%s'", region)
		manualCmd += fmt.Sprintf(" --location=%s", region)
	} else {
		target += ", default location"
	}
	manualCmd += " --public-access-prevention"
	if bucket.ownCredentials {
		target += ", using your local credentials rather than the credentials or impersonation set in the backend configuration"
	}
	return target, manualCmd
}

// checkBackendConfigType rejects a --backend-config without "type=" when it
// would replace a backend of another type. The flag defaults to the "gcs"
// type, so it would otherwise silently turn, say, an "s3" backend set in the
// blueprint or deployment file into a "gcs" one.
func checkBackendConfigType(currentType string, s []string) error {
	if len(s) == 0 || currentType == "" || currentType == "gcs" {
		return nil
	}
	for _, cfg := range s {
		if key, _, found := strings.Cut(cfg, "="); found && key == "type" {
			return nil
		}
	}
	return fmt.Errorf("--backend-config has no 'type=' and would replace the %q backend set in the blueprint or deployment file with a \"gcs\" backend; "+
		"add 'type=%s' to keep that backend type, or 'type=gcs' to switch to a GCS backend", currentType, currentType)
}
