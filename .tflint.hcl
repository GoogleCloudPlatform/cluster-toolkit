// Copyright 2022 Google LLC
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

plugin "google" {
  enabled = true
  version = "0.26.0"
  source  = "github.com/terraform-linters/tflint-ruleset-google"
}
plugin "terraform" {
  enabled = true
  version = "0.5.0"
  source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}
rule "terraform_deprecated_index" {
  enabled = true
}
rule "terraform_unused_declarations" {
  enabled = true
}
rule "terraform_documented_variables" {
  enabled = true
}
rule "terraform_comment_syntax" {
  enabled = true
}
rule "terraform_documented_outputs" {
  enabled = true
}
rule "terraform_documented_variables" {
  enabled = true
}
rule "terraform_typed_variables" {
  enabled = true
}
rule "terraform_naming_convention" {
  enabled = true
}
rule "terraform_required_version" {
  enabled = true
}
rule "terraform_required_providers" {
  enabled = true
}
rule "terraform_unused_required_providers" {
  enabled = true
}
rule "terraform_deprecated_interpolation" {
  enabled = true
}
rule "terraform_module_pinned_source" {
  enabled = true
}
rule "terraform_module_version" {
  enabled = true
}
rule "terraform_workspace_remote" {
  enabled = true
}
// Disable because many of our HPC modules do not have nor need main.tf files
rule "terraform_standard_module_structure" {
  enabled = false
}
