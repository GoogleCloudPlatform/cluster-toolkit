# Module authoring guidelines

Modules should adhere to following guidelines.

## Terraform Requirements

The module source field must point to a single terraform module. We recommend
the following structure:

* `main.tf` file composing the terraform resources using provided variables.
* `variables.tf` file defining the variables used.
* (Optional) `outputs.tf` file defining any exported outputs used (if any).
* (Optional) `modules/` sub-directory pointing to submodules needed to create the
  top level module.

## General Best Practices

* Variables for environment-specific values (like `project_id`) should not be
  given defaults. This forces the calling module to provide meaningful values.
* Variables should only have zero-value defaults, such as null (preferred) or empty string,
  where leaving the variable empty is a valid preference which will not be
  rejected by the underlying API(s).
* Set good defaults wherever possible. Be opinionated about HPC use cases.
* Follow common variable [naming conventions](#use-common-names-and-types-for-common-variables).
* If there are common hpc-toolkit variables already defined, then do not set defaults (`region`, `zone`, `project_id`, `deployment_name`, etc.)
* All files should contain a license header. Headers can be added automatically using [addlicense](https://github.com/google/addlicense), or `pre-commit` hook if adding a Google License.
* No `provider` blocks should be defined in reusable modules. It is OK to impose a range of acceptable provider versions.
  In the case on conflicts, the root module will configure all providers and pass alternatives as an alias. See:
https://developer.hashicorp.com/terraform/language/modules/develop/providers#implicit-provider-inheritance

## Terraform Coding Standards

Any Terraform based modules in the HPC Toolkit should implement the following
standards:

* `terraform-docs` is used to generate `README` files for each module.
* The order for parameters in inputs should be:
  * `description`
  * `type`
  * `default`
* The order for parameters in outputs should be:
  * `description`
  * `value`

## Do not create resources that can be passed externally

Do not create resources that can be passed externally unless:
* resource has to be owned uniquely by the module;
* resource has to conform to module specific constraints (e.g. vm-instance with particular image, or firewall rule to serve needs of this module);
* the resource cannot possibly be (re)used outside of this module.

Examples resources already provided by core toolkit modules:

* [vm-instance](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/compute/vm-instance)
* [vpc & subnetworks](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/network)
* [filestore](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/file-system/filestore)

File systems, networks, service accounts, GCS buckets can all be passed into the module and should not be created within the module.

## Prefer FQN over ambiguous formats

For instance, in networks `network_self_link` over `network_name`, `subnetwork_self_link` over `subnetwork_name`, as these immediately work with shared VPCs, and already specify `region/zone/project_ids`.

## All resources should be labeled

The module, if it creates any resource, should take a variable called `labels` and apply it to every resource.

```hcl
variable "labels" {
  description = "Labels to add to the resources. Key-value pairs."
  type        = map(string)
}
```

If the module creates its own labels, then we recommend merging user-provided labels into the module’s labels:

```hcl
locals {
  labels = merge(var.labels, { ghpc_module = "my-module" })
}
```

## Use common names and types for common variables

Matching names allow implicitly inject variables into the module.

* `project_id {type=string}` - the GCP project ID in which to create the GCP resources;
* `labels {type=map(string)}` - [labels](https://cloud.google.com/resource-manager/docs/creating-managing-labels) added to the module. In order to include any module in advanced
  monitoring, labels must be exposed. We strongly recommend that all modules
  expose this variable. It also makes it easy for customers to filter resources on the cloud console and billing;
* `region {type=string}` - the GCP;
  [region](https://cloud.google.com/compute/docs/regions-zones) the module will be created in;
* `zone {type=string}` - the GCP [zone](https://cloud.google.com/compute/docs/regions-zones)
  the module will be created in;
* `deployment_name {type=string}`  - the name of the current deployment of a blueprint. This
  can help to avoid naming conflicts of modules when multiple deployments are
  created from the same blueprint. `deployment_name` is often used to determine default resource names, or a prefix to the resource names e.g. [`modules/filestore.deploy_name`](../modules/file-system/filestore/README.md#inputs);

### `instance_image {type=object({family=string,project=string})}`

To take/return information about instance image use variable `instance_image`. If it's critical for the module to include the `name` use `type=map(string)` instead.

### `enable_oslogin {type=string}`

When relevant, Enable or Disable OS Login with `"ENABLE"` or `"DISABLE"`. Set to `"INHERIT"` to inherit the project OS Login setting. . Note this ongoing development is not yet fully homogenized in the Cloud HPC Toolkit.

### Network

Properties of networks are represented by scattered variables:
* `network_name` - name of the network (avoid using this);
* `network_id` - ID of the network;
* `network_self_link` - URI of the VPC (preferred);
* `subnetwork_name` - the name of the primary subnetwork;
* `subnetwork_self_link` - self-link to the primary subnetwork (preferred);
* `subnetwork_address` - address range of the primary subnetwork.

### Network Storage

If your module provides a mountable network storage it should output `network_storage` of type:

```hcl
object({
 server_ip = string
 remote_mount = string
 local_mount = string
 fs_type = string
 mount_options         = string      
 client_install_runner = map(string) # Runner for installing client
 mount_runner          = map(string) # Runner to mount the file-system
})
```

If a module returns multiple "storages" it should output `network_storage` of type `list(object(... same as above...))`.

If a module consumes network storage it should have a variable `network_storage` of type `list(object(... any subset of fields from above ...))`.

## Use startup-script module

If there is a need to execute shell script, ansible playbook or just upload file to the vm-instance, consider using `startup-script` [module](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/scripts/startup-script) as a first option. `startup-script` module takes care of uploading local files to the GCS and downloading files to the vm-instance, and installing ansible if needed, configuring ssh, and executing requested scripts.

To represent a script to execute HPC Toolkit modules use "runners". Runner is a `map(string)` with following expected fields:

* `destination`: (Required) The name of the file at the destination VM;
* `type`: (Required) One of the following: shell, ansible-local, and data;
* `content`: (Optional) Content to be uploaded and executed;
* `source`: (Optional) A path to the file or data you want to upload;
* `args`: (Optional) Arguments to be passed to the executable.

If your module consumes/produces scripts to run vm-instances, please adhere to this format.

`startup-script` module example of  usage:

```hcl
variable "extra_runner" {
  description = "Custom script provided by user to run on vm-instance"
  type = map(string)
}
...
locals {
  common_runner = {  # some setup required by your module
    "type"        = "shell"
    "content"     = "echo Hello"
    "destination" = "say_hi.sh"
  }
  runners = [local.common_runner, var.extra_runner]
}
...
module "startup_script" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/scripts/startup-script"
  deployment_name = var.deployment_name
  project_id      = var.project_id
  region          = var.region
  labels          = local.labels
  runners = local.runners
}
...
module "vm" {
  source = "github.com/GoogleCloudPlatform/hpc-toolkit//modules/compute/vm-instance"
  ...
  startup_script = module.startup_script.startup_script
}
```

For more information see [startup-script/README](https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/modules/scripts/startup-script#readme).

## Module `metadata.yaml`

**All** modules should have a `metadata.yaml` file in the root directory. The metadata format follows Cloud Foundation Toolkit metadata [schema](https://github.com/GoogleCloudPlatform/cloud-foundation-toolkit/blob/640a8858ffce99a4512904563a2b00e6768f5a31/cli/bpmetadata/schema/gcp-blueprint-metadata.json#L299) with addition of toolkit-specific section `ghpc`.

See example below:

```yaml
---
spec:
  requirements:
    # `services` has to be defined, 
    # if no services are required, use empty list: []
    services: 
    - serviceA.googleapis.com
    - serviceB.googleapis.com
ghpc:  # [optional]
  # [optional] `inject_module_id`, if set, will inject blueprint 
  # module id as a value for the module variable `var_name`.
  inject_module_id: var_name
  # [optional] `has_to_be_used` is a boolean flag, if set to true,
  # the creation will fail if the module is not used.
  has_to_be_used: true 
```
