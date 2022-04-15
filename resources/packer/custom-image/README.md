## Description

This resource is an example of creating an image with Packer using the HPC
Toolkit. Packer operates by provisioning a short-lived VM in Google Cloud and
executing scripts to customize the VM for repeated usage. This Packer "template"
installs Ansible and supports the execution of user-specified Ansible playbooks
to customize the VM.

### Example

The following example assumes operation in Cloud Region us-central1 and
in zone us-central1-c. You may substitute your own preferred region and zone.
You will need a Cloud VPC Network that allows

* either public IP addresses or Identity-Aware Proxy (IAP) tunneling of SSH
  connections
* outbound connections to the public internet

If you already have such a network, identify its subnetwork in us-central1 or
your region of choice. If not, you can create one with this simple blueprint:

```yaml
---
blueprint_name: image-builder

vars:
  project_id: ## Set Project ID here ##
  deployment_name: image-builder-001
  region: us-central1
  zone: us-central1-c

resource_groups:
- group: network
  resources:
  - source: resources/network/vpc
    kind: terraform
    id: network1
    outputs:
    - subnetwork_name
```

The subnetwork name will be printed to the terminal after running `terraform
apply`. The following parameters will create a 100GB image without exposing the
build VM on the public internet. Create a file `input.auto.pkvars.hcl`:

```hcl
project_id = "## Set Project ID here ##"
zone       = "us-central1-c"
subnetwork = "## Set Subnetwork here ##"
use_iap          = true
omit_external_ip = true
disk_size        = 100

ansible_playbooks = [
  {
    playbook_file   = "./example-playbook.yml"
    galaxy_file     = "./requirements.yml"
    extra_arguments = ["-vv"]
  }
]
```

Substitute appropriate values for `project_id`, `zone`, and `subnetwork`.
Then execute

```shell
packer build .
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

No requirements.

## Providers

No providers.

## Modules

No modules.

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_ansible_playbooks"></a> [ansible\_playbooks](#input\_ansible\_playbooks) | n/a | <pre>list(object({<br>    playbook_file   = string<br>    galaxy_file     = string<br>    extra_arguments = list(string)<br>  }))</pre> | `[]` | no |
| <a name="input_disk_size"></a> [disk\_size](#input\_disk\_size) | Size of disk image in GB | `number` | `null` | no |
| <a name="input_machine_type"></a> [machine\_type](#input\_machine\_type) | VM machine type on which to build new image | `string` | `"n2-standard-4"` | no |
| <a name="input_omit_external_ip"></a> [omit\_external\_ip](#input\_omit\_external\_ip) | Provision the image building VM without a public IP address | `bool` | `false` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | n/a | `string` | n/a | yes |
| <a name="input_service_account_email"></a> [service\_account\_email](#input\_service\_account\_email) | The service account email to use. If null or 'default', then the default Compute Engine service account will be used. | `string` | `null` | no |
| <a name="input_service_account_scopes"></a> [service\_account\_scopes](#input\_service\_account\_scopes) | Service account scopes to attach to the instance. See<br>https://cloud.google.com/compute/docs/access/service-accounts. | `list(string)` | `null` | no |
| <a name="input_source_image"></a> [source\_image](#input\_source\_image) | Source OS image to build from | `string` | `null` | no |
| <a name="input_source_image_family"></a> [source\_image\_family](#input\_source\_image\_family) | Alternative to source\_image. Specify image family to build from latest image in family | `string` | `"hpc-centos-7"` | no |
| <a name="input_source_image_project_id"></a> [source\_image\_project\_id](#input\_source\_image\_project\_id) | A list of project IDs to search for the source image. Packer will search the<br>first project ID in the list first, and fall back to the next in the list,<br>until it finds the source image. | `list(string)` | <pre>[<br>  "cloud-hpc-image-public"<br>]</pre> | no |
| <a name="input_ssh_username"></a> [ssh\_username](#input\_ssh\_username) | Username to use for SSH access to VM | `string` | `"packer"` | no |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Name of subnetwork in which to provision image building VM | `string` | n/a | yes |
| <a name="input_tags"></a> [tags](#input\_tags) | Assign network tags to apply firewall rules to VM instance | `list(string)` | `null` | no |
| <a name="input_use_iap"></a> [use\_iap](#input\_use\_iap) | Use IAP proxy when connecting by SSH | `bool` | `false` | no |
| <a name="input_use_os_login"></a> [use\_os\_login](#input\_use\_os\_login) | Use OS Login when connecting by SSH | `bool` | `false` | no |
| <a name="input_zone"></a> [zone](#input\_zone) | Cloud zone in which to provision image building VM | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
