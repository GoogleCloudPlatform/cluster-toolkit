# Semver Compare Module

This internal module securely performs a semantic version comparison (major.minor.patch) between a target version and a required minimum version.

It safely parses inputs using native Terraform `regex()`, explicitly ignoring anything after the `patch` string (e.g., `-gke.126`, `-beta`, `+build123`). 

Critically, this module implements **fail-open validation**: if the provided `current_version` cannot be resolved to a standard 3-integer format (for example, if a user specifies a Github branch name like `my-custom-feature` or a commit SHA `sha256-4c4892`), the output `is_greater_than_or_equal` evaluates to `true`. This protects the Cluster Toolkit from inadvertently blocking advanced users running fully custom artifacts.

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