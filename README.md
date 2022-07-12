# Google HPC-Toolkit

## Description

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

HPC Toolkit allows customers to deploy turnkey HPC environments (compute,
networking, storage, etc.) following Google Cloud best-practices, in a repeatable
manner. The HPC Toolkit is designed to be highly customizable and extensible,
and intends to address the HPC deployment needs of a broad range of customers.

More information can be found on the
[Google Cloud Docs](https://cloud.google.com/hpc-toolkit/docs/overview).

## Quickstart

Running through the
[quickstart tutorial](https://cloud.google.com/hpc-toolkit/docs/quickstarts/slurm-cluster)
is the recommended path to get started with the HPC Toolkit.

Find a full list of tutorials [here](docs/tutorials/README.md).

---

If a self directed path is preferred, you can use the following commands to
build the `ghpc` binary:

```shell
git clone git@github.com:GoogleCloudPlatform/hpc-toolkit.git
cd hpc-toolkit
make
./ghpc --version
./ghpc --help
```

> **_NOTE:_** You may need to [install dependencies](#dependencies) first.

## HPC Toolkit Components

Learn about the components that make up the HPC Toolkit and more on how it works
on the
[Google Cloud Docs Product Overview](https://cloud.google.com/hpc-toolkit/docs/overview#components).

## GCP Credentials

### Supplying cloud credentials to Terraform

Terraform can discover credentials for authenticating to Google Cloud Platform
in several ways. We will summarize Terraform's documentation for using
[gcloud][terraform-auth-gcloud] from your workstation and for automatically
finding credentials in cloud environments. We do **not** recommend following
Hashicorp's instructions for downloading
[service account keys][terraform-auth-sa-key].

[terraform-auth-gcloud]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started#configuring-the-provider
[terraform-auth-sa-key]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started#adding-credentials

### Cloud credentials on your workstation

You can generate cloud credentials associated with your Google Cloud account
using the following command:

```shell
gcloud auth application-default login
```

You will be prompted to open your web browser and authenticate to Google Cloud
and make your account accessible from the command-line. Once this command
completes, Terraform will automatically use your "Application Default
Credentials."

If you receive failure messages containing "quota project" you should change the
quota project associated with your Application Default Credentials with the
following command and provide your current project ID as the argument:

```shell
gcloud auth application-default set-quota-project ${PROJECT-ID}
```

### Cloud credentials in virtualized cloud environments

In virtualized settings, the cloud credentials of accounts can be attached
directly to the execution environment. For example: a VM or a container can
have [service accounts](https://cloud.google.com/iam/docs/service-accounts)
attached to them. The Google [Cloud Shell][cloud-shell] is an interactive
command line environment which inherits the credentials of the user logged in
to the Google Cloud Console.

[cloud-shell]: https://console.cloud.google.com/home/dashboard?cloudshell=true
[cloud-shell-limitations]: https://cloud.google.com/shell/docs/quotas-limits#limitations_and_restrictions

Many of the above examples are easily executed within a Cloud Shell environment.
Be aware that Cloud Shell has [several limitations][cloud-shell-limitations],
in particular an inactivity timeout that will close running shells after 20
minutes. Please consider it only for blueprints that are quickly deployed.

## Blueprint Warnings and Errors

By default, each blueprint is configured with a number of "validator" functions
which perform basic tests of your deployment variables. If `project_id`,
`region`, and `zone` are defined as deployment variables, then the following
validators are enabled:

```yaml
validators:
- validator: test_project_exists
  inputs:
    project_id: $(vars.project_id)
- validator: test_region_exists
  inputs:
    project_id: $(vars.project_id)
    region: $(vars.region)
- validator: test_zone_exists
  inputs:
    project_id: $(vars.project_id)
    zone: $(vars.zone)
- validator: test_zone_in_region
  inputs:
    project_id: $(vars.project_id)
    zone: $(vars.zone)
    region: $(vars.region)
```

This configures validators that check the validity of the project ID, region,
and zone. Additionally, it checks that the zone is in the region. Validators can
be overwritten, however they are limited to the set of functions defined above.

Validators can be explicitly set to the empty list:

```yaml
validators: []
```

They can also be set to 3 differing levels of behavior using the command-line
`--validation-level` flag` for the `create` and `expand` commands:

* `"ERROR"`: If any validator fails, the deployment directory will not be
  written. Error messages will be printed to the screen that indicate which
  validator(s) failed and how.
* `"WARNING"` (default): The deployment directory will be written even if any
  validators fail. Warning messages will be printed to the screen that indicate
  which validator(s) failed and how.
* `"IGNORE"`: Do not execute any validators, even if they are explicitly defined
  in a `validators` block or the default set is implicitly added.

For example, this command will set all validators to `WARNING` behavior:

```shell
./ghpc create --validation-level WARNING examples/hpc-cluster-small.yaml
```

The flag can be shortened to `-l` as shown below using `IGNORE` to disable all
validators.

```shell
./ghpc create -l IGNORE examples/hpc-cluster-small.yaml
```

## Enable GCP APIs

In a new GCP project there are several apis that must be enabled to deploy your
HPC cluster. These will be caught when you perform `terraform apply` but you can
save time by enabling them upfront.

See
[Google Cloud Docs](https://cloud.google.com/hpc-toolkit/docs/setup/configure-environment#enable-apis)
for instructions.

## GCP Quotas

You may need to request additional quota to be able to deploy and use your HPC
cluster.

See
[Google Cloud Docs](https://cloud.google.com/hpc-toolkit/docs/setup/hpc-blueprint#request-quota)
for more information.

## Billing Reports

You can view your billing reports for your HPC cluster on the
[Cloud Billing Reports](https://cloud.google.com/billing/docs/how-to/reports)
page. ​​To view the Cloud Billing reports for your Cloud Billing account,
including viewing the cost information for all of the Cloud projects that are
linked to the account, you need a role that includes the
`billing.accounts.getSpendingInformation` permission on your Cloud Billing
account.

To view the Cloud Billing reports for your Cloud Billing account:

1. In the Google Cloud Console, go to `Navigation Menu` >
   [`Billing`](https://console.cloud.google.com/billing/overview).
2. At the prompt, choose the Cloud Billing account for which you'd like to view
   reports. The Billing Overview page opens for the selected billing account.
3. In the Billing navigation menu, select `Reports`.

In the right side, expand the Filters view and then filter by label, specifying the key `ghpc_deployment` (or `ghpc_blueprint`) and the desired value.

## Troubleshooting

### Failure to Create Auto Scale Nodes (Slurm)

If your deployment succeeds but your jobs fail with the following error:

```shell
$ srun -N 6 -p compute hostname
srun: PrologSlurmctld failed, job killed
srun: Force Terminated job 2
srun: error: Job allocation 2 has been revoked
```

Possible causes could be [insufficient quota](#insufficient-quota) or
[placement groups](#placement-groups). Also see the
[Slurm user guide](https://docs.google.com/document/u/1/d/e/2PACX-1vS0I0IcgVvby98Rdo91nUjd7E9u83oIMCM4arne-9_IdBg6BdV1lBpUcSje_PyHcbAaErC1rY7p4u1g/pub).

#### Insufficient Quota

It may be that you have sufficient quota to deploy your cluster but insufficient
quota to bring up the compute nodes.

You can confirm this by SSHing into the `controller` VM and checking the
`resume.log` file:

```shell
$ cat /var/log/slurm/resume.log
...
resume.py ERROR: ... "Quota 'C2_CPUS' exceeded. Limit: 300.0 in region europe-west4.". Details: "[{'message': "Quota 'C2_CPUS' exceeded. Limit: 300.0 in region europe-west4.", 'domain': 'usageLimits', 'reason': 'quotaExceeded'}]">
```

The solution here is to [request more of the specified quota](#gcp-quotas),
`C2 CPUs` in the example above. Alternatively, you could switch the partition's
[machine type][partition-machine-type], to one which has sufficient quota.

[partition-machine-type]: community/modules/compute/SchedMD-slurm-on-gcp-partition/README.md#input_machine_type

#### Placement Groups

By default, placement groups (also called affinity groups) are enabled on the
compute partition. This places VMs close to each other to achieve lower network
latency. If it is not possible to provide the requested number of VMs in the
same placement group, the job may fail to run.

Again, you can confirm this by SSHing into the `controller` VM and checking the
`resume.log` file:

```shell
$ cat /var/log/slurm/resume.log
...
resume.py ERROR: group operation failed: Requested minimum count of 6 VMs could not be created.
```

One way to resolve this is to set [enable_placement][partition-enable-placement]
to `false` on the partition in question.

[partition-enable-placement]: https://github.com/GoogleCloudPlatform/hpc-toolkit/tree/main/community/modules/compute/SchedMD-slurm-on-gcp-partition#input_enable_placement

#### Insufficient Service Account Permissions

By default, the slurm controller, login and compute nodes use the
[Google Compute Engine Service Account (GCE SA)][def-compute-sa]. If this
service account or a custom SA used by the Slurm modules does not have
sufficient permissions, configuring the controller or running a job in Slurm may
fail.

If configuration of the Slurm controller fails, the error can be
seen by viewing the startup script on the controller:

```shell
sudo journalctl -u google-startup-scripts.service | less
```

An error similar to the following indicates missing permissions for the serivce
account:

```shell
Required 'compute.machineTypes.get' permission for ...
```

To solve this error, ensure your service account has the
`compute.instanceAdmin.v1` IAM role:

```shell
SA_ADDRESS=<SET SERVICE ACCOUNT ADDRESS HERE>

gcloud projects add-iam-policy-binding ${PROJECT_ID} \
    --member=serviceAccount:${SA_ADDRESS} --role=roles/compute.instanceAdmin.v1
```

If Slurm failed to run a job, view the resume log on the controller instance
with the following command:

```shell
sudo cat /var/log/slurm/resume.log
```

An error in `resume.log` simlar to the following indicates a permissions issue
as well:

```shell
The user does not have access to service account 'PROJECT_NUMBER-compute@developer.gserviceaccount.com'.  User: ''.  Ask a project owner to grant you the iam.serviceAccountUser role on the service account": ['slurm-hpc-small-compute-0-0']
```

As indicated, the service account must have the compute.serviceAccountUser IAM
role. This can be set with the following command:

```shell
SA_ADDRESS=<SET SERVICE ACCOUNT ADDRESS HERE>

gcloud projects add-iam-policy-binding ${PROJECT_ID} \
    --member=serviceAccount:${SA_ADDRESS} --role=roles/iam.serviceAccountUser
```

If the GCE SA is being used and cannot be updated, a new service account can be
created and used with the correct permissions. Instructions for how to do this
can be found in the [Slurm on Google Cloud User Guide][slurm-on-gcp-ug],
specifically the section titled "Create Service Accounts".

After creating the service account, it can be set via the
`compute_node_service_account` and `controller_service_account` settings on the
[slurm-on-gcp controller module][slurm-on-gcp-con] and the
"login_service_account" setting on the
[slurm-on-gcp login module][slurm-on-gcp-login].

[def-compute-sa]: https://cloud.google.com/compute/docs/access/service-accounts#default_service_account
[slurm-on-gcp-ug]: https://goo.gle/slurm-gcp-user-guide
[slurm-on-gcp-con]: community/modules/scheduler/SchedMD-slurm-on-gcp-controller/README.md
[slurm-on-gcp-login]: community/modules/scheduler/SchedMD-slurm-on-gcp-login-node/README.md

### Terraform Deployment

When `terraform apply` fails, Terraform generally provides a useful error
message. Here are some common reasons for the deployment to fail:

* **GCP Access:** The credentials being used to call `terraform apply` do not
  have access to the GCP project. This can be fixed by granting access in
  `IAM & Admin`.
* **Disabled APIs:** The GCP project must have the proper APIs enabled. See
  [Enable GCP APIs](#enable-gcp-apis).
* **Insufficient Quota:** The GCP project does not have enough quota to
  provision the requested resources. See [GCP Quotas](#gcp-quotas).
* **Filestore resource limit:** When regularly deploying filestore instances
  with a new vpc you may see an error during deployment such as:
  `System limit for internal resources has been reached`. See
  [this doc](https://cloud.google.com/filestore/docs/troubleshooting#system_limit_for_internal_resources_has_been_reached_error_when_creating_an_instance)
  for the solution.
* **Required permission not found:**
  * Example: `Required 'compute.projects.get' permission for 'projects/... forbidden`
  * Credentials may not be set, or are not set correctly. Please follow
    instructions at [Cloud credentials on your workstation](#cloud-credentials-on-your-workstation).
  * Ensure proper permissions are set in the cloud console
    [IAM section](https://console.cloud.google.com/iam-admin/iam).

### Failure to Destroy VPC Network

If `terraform destroy` fails with an error such as the following:

```text
│ Error: Error when reading or editing Subnetwork: googleapi: Error 400: The subnetwork resource 'projects/<project_name>/regions/<region>/subnetworks/<subnetwork_name>' is already being used by 'projects/<project_name>/zones/<zone>/instances/<instance_name>', resourceInUseByAnotherResource
```

or

```text
│ Error: Error waiting for Deleting Network: The network resource 'projects/<project_name>/global/networks/<vpc_network_name>' is already being used by 'projects/<project_name>/global/firewalls/<firewall_rule_name>'
```

These errors indicate that the VPC network cannot be destroyed because resources
were added outside of Terraform and that those resources depend upon the
network. These resources should be deleted manually. The first message indicates
that a new VM has been added to a subnetwork within the VPC network. The second
message indicates that a new firewall rule has been added to the VPC network.
If your error message does not look like these, examine it carefully to identify
the type of resouce to delete and its unique name. In the two messages above,
the resource names appear toward the end of the error message. The following
links will take you directly to the areas within the Cloud Console for managing
VMs and Firewall rules. Make certain that your project ID is selected in the
drop-down menu at the top-left.

* [Cloud Console: Manage VM instances][cc-vms]
* [Cloud Console: Manage Firewall Rules][cc-firewall]

[cc-vms]: https://console.cloud.google.com/compute/instances
[cc-firewall]:  https://console.cloud.google.com/networking/firewalls/list

## Inspecting the Deployment

The deployment will be created with the following directory structure:

```text
<<OUTPUT_PATH>>/<<DEPLOYMENT_NAME>>/{<<DEPLOYMENT_GROUPS>>}/
```

If an output directory is provided with the `--output/-o` flag, the deployment
directory will be created in the output directory, represented as
`<<OUTPUT_PATH>>` here. If not provided, `<<OUTPUT_PATH>>` will default to the
current working directory.

The deployment directory is created in `<<OUTPUT_PATH>>` as a directory matching
the provided `deployment_name` deployment variable (`vars`) in the blueprint.

Within the deployment directory are directories representing each deployment
group in the blueprint named the same as the `group` field for each element
in `deployment_groups`.

In each deployment group directory, are all of the configuration scripts and
modules needed to deploy. The modules are in a directory named `modules` named
the same as the source module, for example the
[vpc module](./modules/network/vpc/README.md) is in a directory named `vpc`.

A hidden directory containing meta information and backups is also created and
named `.ghpc`.

From the [hpc-cluster-small.yaml example](./examples/hpc-cluster-small.yaml), we
get the following deployment directory:

```text
hpc-small/
  primary/
    main.tf
    variables.tf
    terraform.tfvars
    modules/
      filestore/
      SchedMD-slurm-on-gcp-controller/
      SchedMD-slurm-on-gcp-login-node/
      SchedMD-slurm-on-gcp-partition/
      vpc/
    .ghpc/
```

## Dependencies

See
[Cloud Docs on Installing Dependencies](https://cloud.google.com/hpc-toolkit/docs/setup/install-dependencies).

### Notes on Packer

The Toolkit supports Packer templates in the contemporary [HCL2 file
format][pkrhcl2] and not in the legacy JSON file format. We require the use of
Packer 1.7.9 or above, and recommend using the latest release.

The Toolkit's [Packer template module documentation][pkrmodreadme] describes
input variables and their behavior. An [image-building example][pkrexample]
and [usage instructions][pkrexamplereadme] are provided. The example integrates
Packer, Terraform and
[startup-script](./modules/scripts/startup-script/README.md) runners to
demonstrate the power of customizing images using the same scripts that can be
applied at boot-time.

[pkrhcl2]: https://www.packer.io/guides/hcl
[pkrmodreadme]: modules/packer/custom-image/README.md
[pkrexamplereadme]: examples/README.md#image-builderyaml
[pkrexample]: examples/image-builder.yaml

## Development

The following setup is in addition to the [dependencies](#dependencies) needed
to build and run HPC-Toolkit.

Please use the `pre-commit` hooks [configured](./.pre-commit-config.yaml) in
this repository to ensure that all changes are validated, tested and properly
documented before pushing code changes. The pre-commits configured
in the HPC Toolkit have a set of dependencies that need to be installed before
successfully passing.

Follow these steps to install and setup pre-commit in your cloned repository:

1. Install pre-commit using the instructions from [the pre-commit website](https://pre-commit.com/).
1. Install TFLint using the instructions from
   [the TFLint documentation](https://github.com/terraform-linters/tflint#installation).

   > **_NOTE:_** The version of TFLint must be compatible with the Google plugin
   > version identified in [tflint.hcl](.tflint.hcl). Versions of the plugin
   > `>=0.16.0` should use `tflint>=0.35.0` and versions of the plugin
   > `<=0.15.0` should preferably use `tflint==0.34.1`. These versions are
   > readily available via GitHub or package managers.

1. Install ShellCheck using the instructions from
   [the ShellCheck documentation](https://github.com/koalaman/shellcheck#installing)
1. The other dev dependencies can be installed by running the following command
   in the project root directory:

    ```shell
    make install-dev-deps
    ```

1. Pre-commit is enabled on a repo-by-repo basis by running the following command
   in the project root directory:

    ```shell
    pre-commit install
    ```

Now pre-commit is configured to automatically run before you commit.

### Development on macOS

While macOS is a supported environment for building and executing the Toolkit,
it is not supported for Toolkit development due to GNU specific shell scripts.

If developing on a mac, a workaround is to install GNU tooling by installing
`coreutils` and `findutils` from a package manager such as homebrew or conda.

### Contributing

Please refer to the [contributing file](CONTRIBUTING.md) in our github repo, or
to
[Google’s Open Source documentation](https://opensource.google/docs/releasing/template/CONTRIBUTING/#).
