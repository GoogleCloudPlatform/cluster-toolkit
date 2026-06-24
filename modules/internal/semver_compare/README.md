<!--
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Semver Compare Module

This internal module securely performs a semantic version comparison (major.minor.patch) between a target version and a required minimum version.

It safely parses inputs using native Terraform `regex()`. It evaluates up to four hierarchical components: `major`, `minor`, `patch`, and an optional GKE build number (`-gke.X`). It explicitly ignores any other suffixes (e.g., `-beta`, `+build123`) that follow the parsed components.

Critically, this module implements **fail-open validation**: if the provided `current_version` cannot be resolved to a standard 3-integer format (for example, if a user specifies a Github branch name like `my-custom-feature` or a commit SHA `sha256-4c4892`), the output `is_greater_than_or_equal` evaluates to `true`. This protects the Cluster Toolkit from inadvertently blocking advanced users running fully custom artifacts.

**Note on GKE Versions:** This module evaluates the `-gke.X` suffix as a post-release build number (where `1.35.0-gke.100` is strictly *greater* than `1.35.0`). This correctly maps to GKE's versioning scheme, but diverges from strict SemVer which treats hyphenated suffixes as pre-releases.

## Usage

You must invoke this inside a module, and usually consume it via a `lifecycle { precondition {} }` block since `module` outputs cannot be natively read from `variable { validation {} }` blocks.

```hcl
module "version_check" {
  source          = "../../internal/semver_compare"
  current_version = "1.35.2-gke.1269001"
  minimum_version = "1.35.0"
}

resource "terraform_data" "feature_guard" {
  lifecycle {
    precondition {
      condition     = module.version_check.is_greater_than_or_equal
      error_message = "Your environment requires version >= 1.35.0."
    }
  }
}
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_current_version"></a> [current\_version](#input\_current\_version) | The version string to evaluate (e.g. 1.35.2-gke, v0.15.2, sha256-123). | `string` | n/a | yes |
| <a name="input_minimum_version"></a> [minimum\_version](#input\_minimum\_version) | The minimum required version (e.g. 1.35.0). | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_is_greater_than_or_equal"></a> [is\_greater\_than\_or\_equal](#output\_is\_greater\_than\_or\_equal) | True if the version meets the minimum requirement, or if the version is a non-standard custom string (fail-open). |
| <a name="output_is_valid_semver"></a> [is\_valid\_semver](#output\_is\_valid\_semver) | True if both versions could be parsed into major.minor semantic logic. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
