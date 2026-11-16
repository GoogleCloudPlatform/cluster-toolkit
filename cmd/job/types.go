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

package job

import "time"

const (
	contextFileName = "context.json"
	stateDirName    = ".gcluster"
	stateFileName   = "job_prereq_state.json"
	stateFreshness  = 24 * time.Hour // State is considered fresh for 24 hours
)

type missingPrereq struct {
	name     string
	commands []string
}

// Context holds the active CLI context.
type Context struct {
	ProjectID   string `json:"project"`
	ClusterName string `json:"cluster"`
	Location    string `json:"location"`
}

// PrereqState holds the current state of prerequisite checks.
type PrereqState struct {
	GCloudSDKInstalled           bool      `json:"gcloud_sdk_installed"`
	GCloudProjectConfigured      bool      `json:"gcloud_project_configured"`
	GCloudAuthenticated          bool      `json:"gcloud_authenticated"`
	ADCConfigured                bool      `json:"adc_configured"`
	KubectlInstalled             bool      `json:"kubectl_installed"`
	GKEGCloudAuthPluginInstalled bool      `json:"gke_gcloud_auth_plugin_installed"`
	ArtifactRegistryAPIEnabled   bool      `json:"artifact_registry_api_enabled"`
	LastCheckedProjectID         string    `json:"last_checked_project_id"`
	LastCheckedTimestamp         time.Time `json:"last_checked_timestamp"`
}

type dockerAuthEntry struct {
	Auth          string `json:"auth"`
	IdentityToken string `json:"identitytoken"`
	RegistryToken string `json:"registrytoken"`
	Password      string `json:"password"`
}

type dockerConfig struct {
	Auths       map[string]dockerAuthEntry `json:"auths"`
	CredHelpers map[string]string          `json:"credHelpers"`
	CredsStore  string                     `json:"credsStore"`
}
