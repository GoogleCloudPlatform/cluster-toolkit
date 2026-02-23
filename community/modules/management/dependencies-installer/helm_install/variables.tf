# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Description: Input variables for the generic Helm release module.

# --- Required ---
variable "release_name" {
  description = "Name of the Helm release."
  type        = string
}

variable "chart_name" {
  description = "Name of the Helm chart (can be a chart reference, path to a packaged chart, path to an unpacked chart directory, or a URL)."
  type        = string
}

# --- Chart Location & Version ---
variable "chart_repository" {
  description = "URL of the Helm chart repository. Set to null or omit if 'chart_name' is a path or URL."
  type        = string
  default     = null
}

variable "chart_version" {
  description = "Version of the Helm chart to install. If omitted, the latest version will be selected (unless 'devel' is true)."
  type        = string
  default     = null
}

variable "devel" {
  description = "Use development versions, too ('helm install --devel'). Equivalent to version '>0.0.0-0'. If 'chart_version' is set, this is ignored."
  type        = bool
  default     = false
}

# --- Namespace ---
variable "namespace" {
  description = "Kubernetes namespace to install the Helm release into."
  type        = string
  default     = "default"
}

variable "create_namespace" {
  description = "Set to true to create the namespace if it does not exist ('helm install --create-namespace')."
  type        = bool
  default     = true # Common convenience setting
}

# --- Values Customization ---
variable "values_yaml" {
  description = "List of YAML strings or paths to YAML files containing chart values ('helm install -f'). Can use file() or templatefile()."
  type        = list(string)
  default     = []
}

variable "set_values" {
  description = "List of objects defining values to set ('helm install --set')."
  type = list(object({
    name  = string                     # Path to the value (e.g., 'service.type', 'replicaCount')
    value = string                     # The value to set
    type  = optional(string, "string") # Type of value ('string', 'json', 'yaml', 'file')
  }))
  default = []
}

# --- Installation/Upgrade Behavior ---
variable "description" {
  description = "Set an optional description for the Helm release."
  type        = string
  default     = null
}

variable "atomic" {
  description = "If set, the installation process purges chart on failure ('helm install --atomic'). The --wait flag will be set automatically if atomic is used."
  type        = bool
  default     = false
}

variable "wait" {
  description = "Will wait until all resources are in a ready state before marking the release as successful ('helm install --wait')."
  type        = bool
  default     = true # Often a good default for dependencies
}

variable "wait_for_jobs" {
  description = "If 'wait' is enabled, will wait until all Jobs have been completed before marking the release as successful ('helm install --wait-for-jobs')."
  type        = bool
  default     = false # Helm CLI default is false
}

variable "timeout" {
  description = "Time in seconds to wait for any individual Kubernetes operation (like Jobs for hooks) ('helm install --timeout')."
  type        = number
  default     = 300 # 5 minutes (Helm CLI default)
}

variable "cleanup_on_fail" {
  description = "Allow deletion of new resources created in this upgrade when the upgrade fails ('helm upgrade --cleanup-on-fail')."
  type        = bool
  default     = false
}

variable "dependency_update" {
  description = "Run 'helm dependency update' before installing the chart (useful if chart_name is a local path to an unpacked chart with dependencies)."
  type        = bool
  default     = false
}

variable "disable_crd_hooks" {
  description = "Prevent CRD hooks from running, but run other hooks ('helm install --no-crd-hook')."
  type        = bool
  default     = false
}

variable "disable_openapi_validation" {
  description = "If set, the installation process will not validate rendered templates against the Kubernetes OpenAPI Schema ('helm install --disable-openapi-validation')."
  type        = bool
  default     = false
}

variable "disable_webhooks" {
  description = "Prevent hooks from running ('helm install --no-hooks')."
  type        = bool
  default     = false
}

variable "force_update" {
  description = "Force resource update through delete/recreate if needed ('helm upgrade --force'). Use with caution."
  type        = bool
  default     = false
}

variable "lint" {
  description = "Run the helm chart linter during the plan ('helm lint')."
  type        = bool
  default     = false
}

variable "max_history" {
  description = "Limit the maximum number of revisions saved per release ('helm upgrade --history-max'). 0 for no limit."
  type        = number
  default     = null # Terraform provider defaults to Helm's default (usually 10)
}

variable "recreate_pods" {
  description = "Perform pods restart for the resource if applicable ('helm upgrade --recreate-pods'). Note: This flag is deprecated in Helm CLI v3 itself."
  type        = bool
  default     = false
}

variable "render_subchart_notes" {
  description = "If set, render subchart notes along with the parent chart's notes ('helm install --render-subchart-notes')."
  type        = bool
  default     = false
}

variable "reset_values" {
  description = "When upgrading, reset the values to the ones built into the chart ('helm upgrade --reset-values')."
  type        = bool
  default     = false
}

variable "reuse_values" {
  description = "When upgrading, reuse the last release's values and merge in any overrides ('helm upgrade --reuse-values'). If 'reset_values' is specified, this is ignored."
  type        = bool
  default     = false # Helm CLI default is false
}

variable "skip_crds" {
  description = "If set, no CRDs will be installed ('helm install --skip-crds'). By default, CRDs are installed if not present."
  type        = bool
  default     = false
}

# --- Verification & Credentials ---
variable "keyring" {
  description = "Location of public keys used for verification ('helm install --keyring'). Used if 'verify' is true."
  type        = string
  default     = null # Defaults to Helm's default keyring location
}

variable "pass_credentials" {
  description = "Pass credentials to all domains ('helm install --pass-credentials'). Use with caution."
  type        = bool
  default     = false
}

variable "verify" {
  description = "Verify the package before installing it ('helm install --verify')."
  type        = bool
  default     = false
}

# --- Advanced Rendering ---
variable "postrender" {
  description = "Configuration for a post-rendering executable ('helm install --post-renderer'). Should be an object with 'binary_path' attribute."
  type = object({
    binary_path = string # Path to the post-renderer executable
  })
  default = null # Disabled by default
}
