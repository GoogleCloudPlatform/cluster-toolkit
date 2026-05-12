## Description

This module simplifies the following functionality:

* Applying Kubernetes manifests to GKE clusters: It provides flexible options for specifying manifests, allowing you to either directly embed them as strings content or reference them from URLs, files, templates, or entire .yaml and .tftpl files in directories.
* Deploying commonly used infrastructure like [Kueue](https://kueue.sigs.k8s.io/docs/),[Jobset](https://jobset.sigs.k8s.io/docs/), [NCCL gIB plugin](https://docs.cloud.google.com/ai-hypercomputer/docs/nccl/overview) or [`asapd-lite`](https://docs.cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute-custom-a4x-max#configure-mrdma-nics) daemonset.

> Note: Kueue can work with a variety of frameworks out of the box, find them [here](https://kueue.sigs.k8s.io/docs/tasks/run/)

### Explanation

* **Manifest:**
  * **Raw String:** Specify manifests directly within the module configuration using the `content: manifest_body` format.
  * **File/Template/Directory Reference:** Set `source` to the path to:
    * A single URL to a manifest file. Ex.: `https://github.com/.../myrepo/manifest.yaml`.

    > **Note:** Applying from a URL has important limitations. Please review the [Considerations & Callouts for Applying from URLs](#applying-manifests-from-urls-considerations--callouts) section below.
    * A single local YAML manifest file (`.yaml` or `.yml`). Ex.: `./manifest.yaml`.
    * A template file (`.tftpl`) to generate a manifest. Ex.: `./template.yaml.tftpl`. You can pass the variables to format the template file in `template_vars`.
    * A directory containing multiple YAML or template files. Ex: `./manifests/` or `./manifests`. The module correctly identifies directories even if the trailing slash is omitted. For security and stability, the module only processes files with `.yaml`, `.yml`, or `.tftpl` extensions. Other files in the directory (like `README.md` etc. ) are automatically ignored.

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

You can specify a particular kueue version that you would like to use using the `version` flag. By default, we recommend customers to [use v0.17.1](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/modules/management/kubectl-apply/variables.tf#L126). You can find the list of supported kueue versions [here](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/modules/management/kubectl-apply/variables.tf#L24).

```yaml
  - id: workload_component_install
    source: modules/management/kubectl-apply
    use: [gke_cluster]
    settings:
      kueue:
        install: true
        version: 0.17.1
        config_path: $(ghpc_stage("manifests/user-provided-kueue-config.yaml.tftpl"))
        config_template_vars: {name: "dev-config", public: "false"}
      jobset:
        install: true
```

You can also install the `gib` plugin by setting the `gib` input variable.
The `path` field accepts a template file. You will need to provide variables for the template using `template_vars` field and can also specify a particular gib version that you would like to use using the `version` flag. You can find the list of supported machine types for the `gib` plugin [here](https://docs.cloud.google.com/ai-hypercomputer/docs/nccl/overview).

```yaml
  - id: workload_component_install
    source: modules/management/kubectl-apply
    use: [gke_cluster]
    settings:
      gib:
        install: true
        path: $(ghpc_stage("manifests/daemonset-gib.yaml.tftpl"))
        template_vars:
          version: v1.1.0
          accelerator_count: 2
```

You can install the `asapd-lite` daemonset for **A4X-Max Bare Metal** ([gke-a4x-max-bm](https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/examples/gke-a4x-max-bm)) by setting the `asapd_lite` input variable and providing the path to the installer manifest using the `config_path` field.

```yaml
  - id: workload_component_install
    source: modules/management/kubectl-apply
    use: [gke_cluster]
    settings:
      asapd_lite:
        install: true
        config_path: $(ghpc_stage("manifests/asapd-lite-installer.yaml"))
```

> **_NOTE:_**
>
> The `project_id` and `region` settings would be inferred from the deployment variables of the same name, but they are included here for clarity.
>
> Terraform may apply resources in parallel, leading to potential dependency issues. If a resource's dependencies aren't ready, it will be applied again up to 15 times.

## Callouts

### Helm-based Manifest Application

#### 1. Large Manifests and CRDs
Helm stores the entire release state (including the generated manifests) as a standard Kubernetes Secret in the release namespace. Before storing the state, Helm runs the YAML through [GZIP compression and base64 encoding](https://helm.sh/docs/topics/kubernetes_apis/#:~:text=The%20manifest%20is,of%20the%20release.). This effectively raises the limit to ~1MB or more, allowing for the deployment of very large manifests and complex CRDs without requiring Server-Side Apply (SSA). This behaviour is guaranteed because the [Terraform Helm Provider](https://github.com/hashicorp/terraform-provider-helm) directly imports the official [Helm Go SDK](https://github.com/helm/helm/tree/main/pkg/action).

#### 2. Helm-Release Suffixes
To make releases more identifiable, the module generates deterministic Helm release names based on the following precedence hierarchy:
* If you provide a `name` field in the `apply_manifests` list object, it will be used directly. Explicit names must be unique across the list.
* If applying from a local file or URL, it extracts the file basename and removes common extensions like `.yaml`, `.yml`, and `.tftpl` (including combined extensions like `.yaml.tftpl`).
* For raw content without a source or name, it falls back to using the module ID and a short hash: `${module_id}-raw-${hash}`.The result is truncated to 30 characters, and a short 7-character hash of the manifest configuration is appended to ensure uniqueness. This ensures the total length does not exceed Helm's 53-character limit.

#### 3. Re-deployment Conflicts
If a deployment fails, the `atomic = true` setting ensures that Helm automatically rolls back the release, preventing the cluster from being left in a "half-applied" state. If you encounter persistent conflicts during re-deployment due to immutable fields, you may need to manually delete the resource or the Helm release before re-applying.

### Applying Manifests from URLs: Considerations & Callouts

While this module supports applying manifests directly from remote `http://` or `https://` URLs, this method introduces complexities not present when using local files. For production environments, we recommend sourcing manifests from local paths or a version-controlled Git repository. Moreover, this method will be deprecated soon. Hence we recommend to use other methods to source manifests.

If you choose to use the URL method, be aware of the following potential issues and their solutions.

#### **1. Apply Order and Race Conditions**

The module applies manifests from the `apply_manifests` list in parallel. This can create a **race condition** if one manifest depends on another. The most common example is applying a manifest with custom resources (like a `ClusterQueue`) at the same time as the manifest that defines it (the `CustomResourceDefinition` or CRD).

There is **no guarantee** that the CRD will be applied before the resource that uses it. This can lead to non-deterministic deployment failures with errors like:

```Error: resource [kueue.x-k8s.io/v1beta2/ClusterQueue] isn't valid for cluster```

##### **Recommended Workaround: Two-Stage Apply**

To ensure a reliable deployment, you must manually enforce the correct order of operations.

1. **Initial Deployment:** In your blueprint, include **only** the manifest(s) containing the `CustomResourceDefinition` (CRD) resources in the `apply_manifests` list.

    *Example `settings` for the first run:*

    ```yaml
    settings:
      apply_manifests:
      # This manifest contains the CRDs for Kueue
      - source: "https://raw.githubusercontent.com/GoogleCloudPlatform/cluster-toolkit/refs/heads/develop/modules/management/kubectl-apply/manifests/kueue-v0.11.4.yaml"
    ```

2. **Run the deployment** (`gcluster deploy` or `terraform apply`).

3. **Second Deployment:** Once the first apply is successful, **add** the manifests containing your custom resources (like `ClusterQueue`, `LocalQueue`) to the list.

    *Example `settings` for the second run:*

    ```yaml
    settings:
      apply_manifests:
      # The CRD manifest is still present
      - source: "https://raw.githubusercontent.com/GoogleCloudPlatform/cluster-toolkit/refs/heads/develop/modules/management/kubectl-apply/manifests/kueue-v0.11.4.yaml"

      # Now, add your configuration manifest
      - source: "https://gist.githubusercontent.com/YourUser/..." # Your configuration URL
    ```

4. **Run the deployment command again.** Since the CRDs are now guaranteed to exist in the cluster, this second apply will succeed reliably.

#### **2. Terraform Template Files (`.tftpl`)**

* **Limitation:** This module **cannot** render a template file (`.tftpl`) when sourced from a remote URL.
* **Workaround:** You must render the template into a pure YAML file locally, host that rendered file at a URL, and provide the URL of the rendered file in your blueprint.

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.2 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |
| <a name="requirement_http"></a> [http](#requirement\_http) | ~> 3.0 |
| <a name="requirement_time"></a> [time](#requirement\_time) | ~> 0.13 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.2 |
| <a name="provider_http"></a> [http](#provider\_http) | ~> 3.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |
| <a name="provider_time"></a> [time](#provider\_time) | ~> 0.13 |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_configure_kueue"></a> [configure\_kueue](#module\_configure\_kueue) | ./helm_install | n/a |
| <a name="module_install_asapd_lite"></a> [install\_asapd\_lite](#module\_install\_asapd\_lite) | ./helm_install | n/a |
| <a name="module_install_cert_manager"></a> [install\_cert\_manager](#module\_install\_cert\_manager) | ./helm_install | n/a |
| <a name="module_install_gib"></a> [install\_gib](#module\_install\_gib) | ./helm_install | n/a |
| <a name="module_install_gpu_operator"></a> [install\_gpu\_operator](#module\_install\_gpu\_operator) | ./helm_install | n/a |
| <a name="module_install_jobset"></a> [install\_jobset](#module\_install\_jobset) | ./helm_install | n/a |
| <a name="module_install_kueue"></a> [install\_kueue](#module\_install\_kueue) | ./helm_install | n/a |
| <a name="module_install_nvidia_dra_driver"></a> [install\_nvidia\_dra\_driver](#module\_install\_nvidia\_dra\_driver) | ./helm_install | n/a |
| <a name="module_kubectl_apply_manifests"></a> [kubectl\_apply\_manifests](#module\_kubectl\_apply\_manifests) | ./helm_install | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [terraform_data.gib_validations](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.initial_gib_version](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.jobset_validations](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.kueue_validations](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [time_sleep.wait_for_webhook](https://registry.terraform.io/providers/hashicorp/time/latest/docs/resources/sleep) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |
| [http_http.manifest_from_url](https://registry.terraform.io/providers/hashicorp/http/latest/docs/data-sources/http) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_apply_manifests"></a> [apply\_manifests](#input\_apply\_manifests) | A list of manifests to apply to the GKE cluster using helm\_install. For more details on the underlying deployment mechanism, see the [helm\_install module](helm\_install/README.md). The `enable` input acts as a FF to apply a manifest or not. By default it is always set to `true`. | <pre>list(object({<br/>    name             = optional(string, null)<br/>    enable           = optional(bool, true)<br/>    content          = optional(string, null)<br/>    source           = optional(string, null)<br/>    template_vars    = optional(map(any), null)<br/>    wait_for_rollout = optional(bool, true)<br/>    namespace        = optional(string, null)<br/>  }))</pre> | `[]` | no |
| <a name="input_asapd_lite"></a> [asapd\_lite](#input\_asapd\_lite) | Install the asapd-lite daemonset for A4X-Max Bare Metal. | <pre>object({<br/>    install              = optional(bool, false)<br/>    config_path          = optional(string, null)<br/>    config_template_vars = optional(map(any), {})<br/>  })</pre> | `{}` | no |
| <a name="input_cert_manager"></a> [cert\_manager](#input\_cert\_manager) | Install [cert-manager](https://cert-manager.io/docs/) which manages TLS certificates for Kubernetes. | <pre>object({<br/>    install = optional(bool, false)<br/>    version = optional(string, "v1.17.2")<br/>  })</pre> | `{}` | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the gke cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_enable_pathways_for_tpus"></a> [enable\_pathways\_for\_tpus](#input\_enable\_pathways\_for\_tpus) | Enable Pathways for TPUs. This is automatically wired from gke-cluster module if used. | `bool` | `false` | no |
| <a name="input_gib"></a> [gib](#input\_gib) | Install the NCCL gIB plugin | <pre>object({<br/>    install = bool<br/>    path    = string<br/>    template_vars = object({<br/>      image   = optional(string, "us-docker.pkg.dev/gce-ai-infra/gpudirect-gib/nccl-plugin-gib")<br/>      version = string<br/>      node_affinity = optional(any, {<br/>        requiredDuringSchedulingIgnoredDuringExecution = {<br/>          nodeSelectorTerms = [{<br/>            matchExpressions = [{<br/>              key      = "cloud.google.com/gke-gpu",<br/>              operator = "In",<br/>              values   = ["true"]<br/>            }]<br/>          }]<br/>        }<br/>      })<br/>      accelerator_count = number<br/>      max_unavailable   = optional(string, "50%")<br/>    })<br/>  })</pre> | <pre>{<br/>  "install": false,<br/>  "path": "",<br/>  "template_vars": {<br/>    "accelerator_count": 0,<br/>    "version": ""<br/>  }<br/>}</pre> | no |
| <a name="input_gke_cluster_exists"></a> [gke\_cluster\_exists](#input\_gke\_cluster\_exists) | A static flag that signals to downstream modules that a cluster has been created. | `bool` | `false` | no |
| <a name="input_gpu_operator"></a> [gpu\_operator](#input\_gpu\_operator) | Install [GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html) which uses the [Kubernetes operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) to automate the management of all NVIDIA software components needed to provision GPU. | <pre>object({<br/>    install = optional(bool, false)<br/>    version = optional(string, "v25.3.0")<br/>  })</pre> | `{}` | no |
| <a name="input_jobset"></a> [jobset](#input\_jobset) | Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit. | <pre>object({<br/>    install           = optional(bool, false)<br/>    version           = optional(string, "0.10.1")<br/>    controller_cpu    = optional(string, null)<br/>    controller_memory = optional(string, null)<br/>  })</pre> | `{}` | no |
| <a name="input_kueue"></a> [kueue](#input\_kueue) | Install and configure [Kueue](https://kueue.sigs.k8s.io/docs/overview/) workload scheduler. A configuration yaml/template file can be provided with config\_path to be applied right after kueue installation. If a template file provided, its variables can be set to config\_template\_vars. | <pre>object({<br/>    # ATTENTION: If you update the KUEUE's default version below, please also update the corresponding<br/>    # defaultKueueVersion constant in pkg/orchestrator/gke/infra_manager.go. (note the 'v' prefix there)<br/>    version                  = optional(string, "0.17.1")<br/>    install                  = optional(bool, false)<br/>    config_path              = optional(string, null)<br/>    config_template_vars     = optional(map(any), null)<br/>    enable_pathways_for_tpus = optional(bool, false)<br/>    controller_cpu           = optional(string, null)<br/>    controller_memory        = optional(string, null)<br/>    controller_replicas      = optional(number, null)<br/>  })</pre> | `{}` | no |
| <a name="input_module_id"></a> [module\_id](#input\_module\_id) | The ID of the module as defined in the blueprint. Injected by ghpc. | `string` | `"kubectl-apply"` | no |
| <a name="input_nvidia_dra_driver"></a> [nvidia\_dra\_driver](#input\_nvidia\_dra\_driver) | Installs [Nvidia DRA driver](https://github.com/NVIDIA/k8s-dra-driver-gpu) which supports Dynamic Resource Allocation for NVIDIA GPUs in Kubernetes | <pre>object({<br/>    install          = optional(bool, false)<br/>    version          = optional(string, "v25.3.0")<br/>    accelerator_type = optional(string, "nvidia-gb200")<br/>  })</pre> | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the gke cluster. | `string` | n/a | yes |
| <a name="input_system_node_pool_id"></a> [system\_node\_pool\_id](#input\_system\_node\_pool\_id) | The ID of the system node pool. Used to ensure the node pool remains active during Kueue uninstallation. | `string` | `null` | no |
| <a name="input_target_architecture"></a> [target\_architecture](#input\_target\_architecture) | The target architecture for the GKE nodes and gIB plugin (e.g., 'x86\_64' or 'arm64'). | `string` | `"x86_64"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_k8s_prerequisites_ready"></a> [k8s\_prerequisites\_ready](#output\_k8s\_prerequisites\_ready) | Ensures sequential ordering with other Helm chart modules to avoid race conditions or deployment conflicts. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
