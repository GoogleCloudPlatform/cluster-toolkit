# Google Cluster Toolkit (formerly HPC Toolkit)

## Description

Cluster Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy AI/ML and HPC environments on Google Cloud.

Cluster Toolkit allows customers to deploy turnkey AI/ML and HPC environments (compute,
networking, storage, etc.) following Google Cloud best-practices, in a repeatable
manner. The Cluster Toolkit is designed to be highly customizable and extensible,
and intends to address the AI/ML and HPC deployment needs of a broad range of customers.

## Detailed documentation and examples

The Toolkit comes with a suite of [tutorials], [examples], and full
documentation for a suite of [modules] that have been designed for AI/ML and HPC use cases.
More information can be found on the
[Google Cloud Docs](https://cloud.google.com/cluster-toolkit/docs/overview).

[tutorials]: docs/tutorials/README.md
[examples]: examples/README.md
[modules]: modules/README.md

## Quickstart

Running through the
[quickstart tutorial](https://cloud.google.com/cluster-toolkit/docs/quickstarts/slurm-cluster)
is the recommended path to get started with the Cluster Toolkit.

---

If a self directed path is preferred, you can use the following commands to
build the `gcluster` binary:

```shell
git clone https://github.com/GoogleCloudPlatform/cluster-toolkit
cd cluster-toolkit
make
./gcluster --version
./gcluster --help
```

> **_NOTE:_** You may need to [install dependencies](#dependencies) first.

## Cluster Toolkit Components

Learn about the components that make up the Cluster Toolkit and more on how it works
on the
[Google Cloud Docs Product Overview](https://cloud.google.com/cluster-toolkit/docs/overview#components).

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

## VM Image Support

### Standard Images

The Cluster Toolkit officially supports the following VM images:

* HPC Rocky Linux 8
* Debian 11
* Ubuntu 20.04 LTS

For more information on these and other images, see
[docs/vm-images.md](docs/vm-images.md).

### Slurm Images

> **_Warning:_** Slurm Terraform modules cannot be directly used on the standard OS images. They must be used in combination with images built for the versioned release of the Terraform module.

The Cluster Toolkit provides modules and examples for implementing pre-built and custom Slurm VM images, see [Slurm on GCP](docs/vm-images.md#slurm-on-gcp)

## Blueprint Validation

The Toolkit contains "validator" functions that perform basic tests of the
blueprint to ensure that deployment variables are valid and that the AI/ML and HPC
environment can be provisioned in your Google Cloud project. Further information
can be found in [dedicated documentation](docs/blueprint-validation.md).

## Enable GCP APIs

In a new GCP project there are several APIs that must be enabled to deploy your
cluster. These will be caught when you perform `terraform apply` but you can
save time by enabling them upfront.

See
[Google Cloud Docs](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment#enable-apis)
for instructions.

## GCP Quotas

You may need to request additional quota to be able to deploy and use your
cluster.

See
[Google Cloud Docs](https://cloud.google.com/cluster-toolkit/docs/setup/hpc-blueprint#request-quota)
for more information.

## Billing Reports

You can view your billing reports for your cluster on the
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

### Authentication

Confirm that you have [properly setup Google Cloud credentials](#gcp-credentials)

### Slurm Clusters

Please see the dedicated [troubleshooting guide for Slurm](docs/slurm-troubleshooting.md).

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
* **Filestore resource limit:** When regularly deploying Filestore instances
  with a new VPC you may see an error during deployment such as:
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
the type of resource to delete and its unique name. In the two messages above,
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

From the [hpc-slurm.yaml example](./examples/hpc-slurm.yaml), we
get the following deployment directory:

```text
hpc-slurm/
  primary/
    main.tf
    modules/
    providers.tf
    terraform.tfvars
    variables.tf
    versions.tf
  .ghpc/
```

## Dependencies

See
[Cloud Docs on Installing Dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).

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
to build and run Cluster-Toolkit.

Please use the `pre-commit` hooks [configured](./.pre-commit-config.yaml) in
this repository to ensure that all changes are validated, tested and properly
documented before pushing code changes. The pre-commits configured
in the Cluster Toolkit have a set of dependencies that need to be installed before
successfully passing.

Follow these steps to install and setup pre-commit in your cloned repository:

1. Install pre-commit using the instructions from [the pre-commit website](https://pre-commit.com/).
1. Install TFLint using the instructions from
   [the TFLint documentation](https://github.com/terraform-linters/tflint#installation).

   > **_NOTE:_** The version of TFLint must be compatible with the Google plugin
   > version identified in [tflint.hcl](.tflint.hcl). Versions of the plugin
   > `>=0.20.0` should use `tflint>=0.40.0`. These versions are readily
   > available via GitHub or package managers. Please review the [TFLint Ruleset
   > for Google Release Notes][tflint-google] for up-to-date requirements.

[tflint-google]: https://github.com/terraform-linters/tflint-ruleset-google/releases

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

Please refer to the [contributing file](CONTRIBUTING.md) in our GitHub
repository, or to
[Google’s Open Source documentation](https://opensource.google/docs/releasing/template/CONTRIBUTING/#).
