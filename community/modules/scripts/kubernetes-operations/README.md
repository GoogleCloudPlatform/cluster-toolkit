## Description

This module performs pre-defined operations on Kubernetes resources that would
otherwise be executed using `kubectl`.

The `kubernetes-operations` module is owned and maintained by the
[ai-infra-cluster-provisioning] Github project. Full documentation of the module
interface can be found in that project on the [`kubernetes-operations`] page.

### Examples

The following example will use the [`kubernetes-operations`] module to create a
DaemonSet that will install Nvidia drivers on GPU nodes.

```yaml
  - id: gke_cluster
    source: community/modules/scheduler/gke-cluster
    use: [network1]
    settings:
      enable_private_endpoint: false  # Allows for access from authorized public IPs
      master_authorized_networks:
      - display_name: deployment-machine
        cidr_block: <your-ip-address>/32
    outputs: [instructions]

  - id: install-nvidia-drivers
    source: github.com/GoogleCloudPlatform/ai-infra-cluster-provisioning//aiinfra-cluster/modules/kubernetes-operations?ref=v0.6.0
    use: [gke_cluster]
    settings:
      install_nvidia_driver: true
```

> **Note**: The IP address of the machine calling Terraform must be listed as a
> `master_authorized_network` otherwise the [`kubernetes-operations`] module
> will not be able to communicate with the cluster.

### Version Compatibility

Only version [v0.6.0] of this module has been tested for compatibility with the HPC Toolkit. Older versions will not work and newer versions are untested.

[v0.6.0]: https://github.com/GoogleCloudPlatform/ai-infra-cluster-provisioning/releases/tag/v0.6.0
[`kubernetes-operations`]: https://github.com/GoogleCloudPlatform/ai-infra-cluster-provisioning/tree/v0.6.0/aiinfra-cluster/modules/kubernetes-operations
[ai-infra-cluster-provisioning]: https://github.com/GoogleCloudPlatform/ai-infra-cluster-provisioning/tree/v0.6.0
