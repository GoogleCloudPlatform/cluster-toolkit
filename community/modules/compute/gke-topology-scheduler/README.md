## Description

This module enables topology on a Google Kubernetes Engine cluster.
This is implemented based on sources and instructions explained [here](https://github.com/GoogleCloudPlatform/container-engine-accelerators/tree/master/gpudirect-tcpxo/topology-scheduler).

## Prerequisites

For topology awareness to be enabled, a GKE node pool has to be created with
compact placement. Specifically, the `physical_host` attribute
[ref](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies#verify-vm-location)
should be present for each GPU node in the cluster.

### Example

The following example installs topology scheduler on a GKE cluster.

```yaml
- id: topology_aware_scheduler_install
    source: community/modules/compute/gke-topology-scheduler
    use: [gke_cluster]
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.3 |

## Providers

No providers.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_kubectl_apply"></a> [kubectl\_apply](#module\_kubectl\_apply) | ../../../../modules/management/kubectl-apply | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_gke_cluster_exists"></a> [gke\_cluster\_exists](#input\_gke\_cluster\_exists) | A static flag that signals to modules that a cluster has been created. | `bool` | `false` | no |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
