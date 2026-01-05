# gcloud Module

This module allows you to run a series of gcloud commands as part of a Cluster Toolkit deployment.

## Usage

```yaml
- id: my_gcloud_steps
  source: community/modules/gcloud
  settings:
    commands:
      - gcloud compute networks create my-network --subnet-mode=custom
      - gcloud compute networks subnets create my-subnet --network=my-network --range=10.0.0.0/24 --region=us-central1
      - gcloud compute instances create my-vm --zone=us-central1-a --network=my-network --subnet=my-subnet --machine-type=e2-medium
## Dependency Management

This module uses `local-exec` provisioners to run `gcloud` commands. As such, it does not expose any outputs that other Terraform modules can consume to establish dependencies.

To ensure that resources managed by this module are fully provisioned before other modules in subsequent deployment groups run, you **must** place this `gcloud` module in a deployment group that is ordered *before* any groups that depend on the resources it creates.

For example:

```yaml
deployment_groups:
  - group: gcloud_setup
    modules:
      - id: my_gcloud_commands
        source: community/modules/scripts/gcloud
        settings:
          # ... gcloud commands to create a network and subnet

  - group: vm_deployment
    modules:
      - id: my_vms
        source: ./modules/compute/vm
        settings:
          # This module can assume the network and subnet from the gcloud_setup group exist
          network: my-network
          subnet: my-subnet
          # ... other vm settings
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.13 |
| <a name="requirement_null"></a> [null](#requirement\_null) | >= 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_null"></a> [null](#provider\_null) | >= 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [null_resource.gcloud_commands](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_commands"></a> [commands](#input\_commands) | A list of gcloud command pairs for creation and destruction. | <pre>list(object({<br/>    create = string<br/>    delete = string<br/>  }))</pre> | `[]` | no |
| <a name="input_module_instance_id"></a> [module\_instance\_id](#input\_module\_instance\_id) | The unique ID of this module instance in the blueprint. This is automatically populated by gcluster. | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
