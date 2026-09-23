# Project Metadata Module

Sets one or more Google Cloud Compute Engine project metadata items using `google_compute_project_metadata_item`.

## Important Considerations

### 1. Project-Wide Scope
Compute project metadata applies to **every virtual machine instance in the project**, including instances not created or managed by this blueprint. Special metadata keys such as `enable-oslogin`, `ssh-keys`, and `startup-script` alter behavior project-wide.

### 2. Pre-Existing Keys and Collisions
The underlying `google_compute_project_metadata_item` resource requires Terraform to create and manage the metadata key from its inception. If a key is already present in project metadata outside Terraform state (e.g., set via `gcloud compute project-info add-metadata` or another deployment), Terraform will fail during apply:

```text
Error: key "<key>" already present in metadata for project "<project>". Use `terraform import` to manage it with Terraform
```

* **Recommended Scoping**: Always namespace metadata keys with deployment- or experiment-specific prefixes (e.g., `"$(vars.user_experiment_name)_USER_EXPERIMENT_NAME"` or `"$(vars.deployment_name)_MY_KEY"`). This avoids collisions when multiple deployments share the same GCP project.
* **Recovery / Migration Step**: If a key was created outside of Terraform and needs to be managed by this module, remove it from the project before applying:

  ```bash
  gcloud compute project-info remove-metadata --keys <key> --project <project_id>
  ```

  Then re-run `gcluster deploy`.

### 3. Deletion Policy
By default (`deletion_policy = "DELETE"`), destroying the deployment removes the managed metadata keys from the project. If other project workloads or downstream tools depend on the metadata persisting after the deployment is torn down, set `deletion_policy: "ABANDON"`.

---

## Example Usage

```yaml
- group: metadata
  modules:
  - id: set-project-metadata
    source: community/modules/project/project-metadata
    settings:
      project_id: $(vars.project_id)
      deletion_policy: "DELETE" # or "ABANDON"
      metadata:
        - key: "$(vars.user_experiment_name)_USER_EXPERIMENT_NAME"
          value: $(vars.user_experiment_name)
        - key: "$(vars.deployment_name)_STATUS"
          value: "deployed"
```

---

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
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

## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |

## Modules

No modules.

## Resources

| Name | Type |
| ---- | ---- |
| [google_compute_project_metadata_item.items](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_project_metadata_item) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_deletion_policy"></a> [deletion\_policy](#input\_deletion\_policy) | The deletion policy for the metadata items. Can be 'DELETE' (removes the key from project metadata on destroy) or 'ABANDON' (leaves the metadata key in place on destroy). | `string` | `"DELETE"` | no |
| <a name="input_metadata"></a> [metadata](#input\_metadata) | Project compute metadata items to set as a list of key-value objects. | <pre>list(object({<br/>    key   = string<br/>    value = string<br/>  }))</pre> | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID where compute metadata will be set. | `string` | n/a | yes |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_metadata_items"></a> [metadata\_items](#output\_metadata\_items) | A map of created or updated project metadata keys and values. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
