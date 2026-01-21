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
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |
| <a name="requirement_google"></a> [google](#requirement\_google) | > 5.0 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | > 5.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_install_gpu_operator"></a> [install\_gpu\_operator](#module\_install\_gpu\_operator) | ./helm_install | n/a |
| <a name="module_install_jobset"></a> [install\_jobset](#module\_install\_jobset) | ./helm_install | n/a |
| <a name="module_install_kueue"></a> [install\_kueue](#module\_install\_kueue) | ./helm_install | n/a |
| <a name="module_install_nvidia_dra_driver"></a> [install\_nvidia\_dra\_driver](#module\_install\_nvidia\_dra\_driver) | ./helm_install | n/a |

## Resources

| Name | Type |
|------|------|
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the gke cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_gke_cluster_exists"></a> [gke\_cluster\_exists](#input\_gke\_cluster\_exists) | A static flag that signals to downstream modules that a cluster has been created. Needed by community/modules/scripts/kubernetes-operations. | `bool` | `false` | no |
| <a name="input_gpu_operator"></a> [gpu\_operator](#input\_gpu\_operator) | Install [GPU Operator](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html) which uses the [Kubernetes operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) to automate the management of all NVIDIA software components needed to provision GPU. | <pre>object({<br/>    install = optional(bool, false)<br/>    version = optional(string, "v25.3.0")<br/>  })</pre> | `{}` | no |
| <a name="input_jobset"></a> [jobset](#input\_jobset) | Install [Jobset](https://github.com/kubernetes-sigs/jobset) which manages a group of K8s [jobs](https://kubernetes.io/docs/concepts/workloads/controllers/job/) as a unit. | <pre>object({<br/>    install = optional(bool, false)<br/>    version = optional(string, "v0.7.2")<br/>  })</pre> | `{}` | no |
| <a name="input_kueue"></a> [kueue](#input\_kueue) | Install and configure [Kueue](https://kueue.sigs.k8s.io/docs/overview/) workload scheduler. A configuration yaml/template file can be provided with config\_path to be applied right after kueue installation. If a template file provided, its variables can be set to config\_template\_vars. | <pre>object({<br/>    install              = optional(bool, false)<br/>    version              = optional(string, "v0.11.4")<br/>    config_path          = optional(string, null)<br/>    config_template_vars = optional(map(any), null)<br/>  })</pre> | `{}` | no |
| <a name="input_nvidia_dra_driver"></a> [nvidia\_dra\_driver](#input\_nvidia\_dra\_driver) | Installs [Nvidia DRA driver](https://github.com/NVIDIA/k8s-dra-driver-gpu) which supports Dynamic Resource Allocation for NVIDIA GPUs in Kubernetes | <pre>object({<br/>    install = optional(bool, false)<br/>    version = optional(string, "v25.3.0")<br/>  })</pre> | `{}` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the gke cluster. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
