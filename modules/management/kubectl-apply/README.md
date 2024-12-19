## Description

This module simplifies the following functionality:

* Applying Kubernetes manifests to GKE clusters: It provides flexible options for specifying manifests, allowing you to either directly embed them as strings content or reference them from URLs, files, templates, or entire .yaml and .tftpl files in directories.
* Deploying commonly used infrastructure like [Kueue](https://kueue.sigs.k8s.io/docs/) or [Jobset](https://jobset.sigs.k8s.io/docs/).

> Note: Kueue can work with a variety of frameworks out of the box, find them [here](https://kueue.sigs.k8s.io/docs/tasks/run/)

### Explanation

* **Manifest:**
  * **Raw String:** Specify manifests directly within the module configuration using the `content: manifest_body` format.
  * **File/Template/Directory Reference:** Set `source` to the path to:
    * A single URL to a manifest file. Ex.: `https://github.com/.../myrepo/manifest.yaml`.
    * A single local YAML manifest file (`.yaml`). Ex.: `./manifest.yaml`.
    * A template file (`.tftpl`) to generate a manifest. Ex.: `./template.yaml.tftpl`. You can pass the variables to format the template file in `template_vars`.
    * A directory containing multiple YAML or template files. Ex: `./manifests/`. You can pass the variables to format the template files in `template_vars`.

#### Manifest Example

```yaml
- id: existing-gke-cluster
  source: modules/scheduler/pre-existing-gke-cluster
  settings:
    project_id: $(vars.project_id)
    cluster_name: my-gke-cluster
    region: us-central1

- id: kubectl-apply
  source: modules/management/kubectl-apply
  use: [existing-gke-cluster]
  settings:
    - content: |
        apiVersion: v1
        kind: Namespace
        metadata:
          name: my-namespace
    - source: "https://github.com/kubernetes-sigs/jobset/releases/download/v0.6.0/manifests.yaml"
    - source: $(ghpc_stage("manifests/configmap1.yaml"))
    - source: $(ghpc_stage("manifests/configmap2.yaml.tftpl"))
      template_vars: {name: "dev-config", public: "false"}
    - source: $(ghpc_stage("manifests"))/
      template_vars: {name: "dev-config", public: "false"}
```

#### Pre-build infrastructure Example

```yaml
  - id: workload_component_install
    source: modules/management/kubectl-apply
    use: [gke_cluster]
    settings:
      kueue:
        install: true
        config_path: $(ghpc_stage("manifests/user-provided-kueue-config.yaml"))
      jobset:
        install: true
```

The `config_path` field in `kueue` installation accepts a template file, too. You will need to provide variables for the template using `config_template_vars` field.

```yaml
  - id: workload_component_install
    source: modules/management/kubectl-apply
    use: [gke_cluster]
    settings:
      kueue:
        install: true
        config_path: $(ghpc_stage("manifests/user-provided-kueue-config.yaml.tftpl"))
        config_template_vars: {name: "dev-config", public: "false"}
      jobset:
        install: true
```

> **_NOTE:_**
>
> The `project_id` and `region` settings would be inferred from the deployment variables of the same name, but they are included here for clarity.
>
> Terraform may apply resources in parallel, leading to potential dependency issues. If a resource's dependencies aren't ready, it will be applied again up to 15 times.

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2024 Google LLC

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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | > 5.0 |
| <a name="requirement_http"></a> [http](#requirement\_http) | ~> 3.0 |
| <a name="requirement_kubectl"></a> [kubectl](#requirement\_kubectl) | >= 1.7.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | > 5.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_configure_kueue"></a> [configure\_kueue](#module\_configure\_kueue) | ./kubectl | n/a |
| <a name="module_install_jobset"></a> [install\_jobset](#module\_install\_jobset) | ./kubectl | n/a |
| <a name="module_install_kueue"></a> [install\_kueue](#module\_install\_kueue) | ./kubectl | n/a |
| <a name="module_kubectl_apply_manifests"></a> [kubectl\_apply\_manifests](#module\_kubectl\_apply\_manifests) | ./kubectl | n/a |

## Resources

| Name | Type |
|------|------|
| [terraform_data.jobset_validations](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.kueue_validations](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_apply_manifests"></a> [apply\_manifests](#input\_apply\_manifests) | A list of manifests to apply to GKE cluster using kubectl. For more details see [kubectl module's inputs](kubectl/README.md). | <pre>list(object({<br/>    content           = optional(string, null)<br/>    source            = optional(string, null)<br/>    template_vars     = optional(map(any), null)<br/>    server_side_apply = optional(bool, false)<br/>    wait_for_rollout  = optional(bool, true)<br/>  }))</pre> | `[]` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the gke cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_jobset"></a> [jobset](#input\_jobset) | Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit. | <pre>object({<br/>    install = optional(bool, false)<br/>    version = optional(string, "v0.5.2")<br/>  })</pre> | `{}` | no |
| <a name="input_kueue"></a> [kueue](#input\_kueue) | Install and configure [Kueue](https://kueue.sigs.k8s.io/docs/overview/) workload scheduler. A configuration yaml/template file can be provided with config\_path to be applied right after kueue installation. If a template file provided, its variables can be set to config\_template\_vars. | <pre>object({<br/>    install              = optional(bool, false)<br/>    version              = optional(string, "v0.8.1")<br/>    config_path          = optional(string, null)<br/>    config_template_vars = optional(map(any), null)<br/>  })</pre> | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the gke cluster. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
