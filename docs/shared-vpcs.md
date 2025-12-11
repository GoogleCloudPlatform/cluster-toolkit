# Shared VPCs with HPC

The HPC toolkit supports the use of shared-vpcs.

The module is located in  `modules/network/pre-existing-subnetwork`.

The extension is build to support subnet level permissions.

The subnet is referenced directly using self_link:

```yaml
- group: primary
  modules:
  - source: modules/network/pre-existing-subnetwork
    kind: terraform
    settings:
      subnetwork_self_link: https://compute.googleapis.com/compute/v1/projects/{project}/regions/{region}/subnetworks/{subnetwork}
      name = name-of-subnet (optional - not used when subnet_self_link is defined)
      region = name-of-region (optional - not used when subnet_self_link is defined)
      project = name-of-project (optional - not used when subnet_self_link is defined)
    id: hpc_network
```

As described in documentation:
[https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/compute_subnetwork]

If subnetwork_self_link is provided then name,region,project is ignored.

Since using the HPC toolkit creates a new service account for the cluster, the cluster service accounts needs roles/compute.networkUser on the subnet on shared VPC.

To accomplish this on an automated basis, it's possible to use a cloud-function that listens on new service account creations/deletions, and uses a dedicated service account, to manage the access the subnet.

An example function is provided in `community/other/cloud-function-for-shared-vpcs/`.
