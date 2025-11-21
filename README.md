# Google Cluster Toolkit (formerly HPC Toolkit)

## Description

Cluster Toolkit, formerly known as Cloud HPC Toolkit, is an open-source software offered by Google Cloud which simplifies the process for you to deploy high performance computing (HPC), artificial intelligence (AI), and machine learning (ML) workloads on Google Cloud.

Cluster Toolkit lets you deploy ready-to-use AI, ML and HPC environments (including compute,
networking, or storage) following Google Cloud best-practices, in a repeatable
manner. Cluster Toolkit is designed to be highly customizable and extensible,
and to help address the AI, ML and HPC deployment needs of a broad range of use cases.

## AI Hypercomputer

Cluster Toolkit is part of [Google Cloud AI Hypercomputer][aihc], a supercomputing system that provides performance-optimized hardware, open software, leading ML frameworks, and flexible consumption models. To learn more about AI Hypercomputer solutions for GKE and Slurm, see the following:

* [GKE][aihc-gke]
* [Slurm][aihc-slurm]

[aihc]: https://cloud.google.com/ai-hypercomputer/docs
[aihc-gke]: https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute
[aihc-slurm]: https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster

## Detailed documentation and examples

Cluster Toolkit comes with a suite of [tutorials], [examples], and full documentation for a set of [modules] that have been designed for AI, ML and HPC use cases. To learn more, see the [Google Cloud documentation](https://cloud.google.com/cluster-toolkit/docs/overview).

[tutorials]: docs/tutorials/README.md
[examples]: examples/README.md
[modules]: modules/README.md

## Quickstart

To get started with the Cluster Toolkit, we recommend that you run through the quickstart tutorial in [Deploy an HPC cluster with Slurm](https://cloud.google.com/cluster-toolkit/docs/quickstarts/slurm-cluster).

---

If you don’t want to begin with the quickstart, you can use the following commands to
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

To learn more about Cluster Toolkit and its components, see [Components](https://docs.cloud.google.com/cluster-toolkit/docs/overview#components).

## Google Cloud Credentials

This section describes different approaches for gaining the credentials that you need for authentication on Google Cloud.

### Supplying cloud credentials to Terraform

You can use Terraform to discover credentials for authenticating to Google Cloud Platform in several ways. The following section summarizes Terraform's documentation for using
[gcloud][terraform-auth-gcloud] from your workstation and for automatically finding credentials in cloud environments. We do **not** recommend that you follow Hashicorp's instructions for downloading [service account keys][terraform-auth-sa-key].

[terraform-auth-gcloud]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started#configuring-the-provider
[terraform-auth-sa-key]: https://registry.terraform.io/providers/hashicorp/google/latest/docs/guides/getting_started#adding-credentials

### Generate Cloud credentials on your workstation

1. You can generate cloud credentials associated with your Google Cloud account using the following command on the cloud workstation terminal:

   ```shell
   gcloud auth application-default login
   ```

   Once this command completes, Terraform will automatically use your "Application Default Credentials (ADC)".

1. When you are prompted, open your web browser and authenticate to Google Cloud. Make your account accessible from the command-line by copy-pasting the token displayed on the screen.

1. If you see an error message that contains the phrase "quota project", you must change the quota project associated with your ADC. Run the following command, using your current project ID as the argument:

   ```shell
   gcloud auth application-default set-quota-project ${PROJECT-ID}
   ```

### Cloud credentials in virtualized cloud environments

In virtualized settings, cloud credentials of accounts can be attached directly to the execution environment. For example, a VM or a container can have [service accounts](https://cloud.google.com/iam/docs/service-accounts)
attached to them. [Cloud Shell][cloud-shell] is an interactive
command line environment which inherits your credentials when you are logged in to the Google Cloud Console.

Many of the example approaches described in this section can be executed within a Cloud Shell environment.
However, note that Cloud Shell has [several limitations][cloud-shell-limitations]. In particular, running shells are closed after 20 minutes of inactivity. For this reason, we recommend that you only use Cloud Shell for blueprints that are quickly deployed.

[cloud-shell]: https://console.cloud.google.com/home/dashboard?cloudshell=true
[cloud-shell-limitations]: https://cloud.google.com/shell/docs/quotas-limits#limitations_and_restrictions

## VM image support

This section describes the VM support available in Cluster Toolkit.

### Standard Images

Cluster Toolkit officially supports the following VM images:

* HPC Rocky Linux 8
* Debian 11
* Ubuntu 20.04 LTS

For more information on these and other images, see
[docs/vm-images.md](docs/vm-images.md).

### Slurm Images

> **_Warning:_** You can't use Slurm Terraform modules directly on the standard OS images. These modules must be used in combination with images built for the versioned release of the Terraform module.

The Cluster Toolkit provides modules and examples for implementing pre-built and custom Slurm VM images. To learn more, see [Slurm on GCP](docs/vm-images.md#slurm-on-gcp)

## Blueprint Validation

Cluster Toolkit contains "validator" functions that perform basic tests on the blueprint to ensure that deployment variables are valid and that the AI, ML and HPC environment can be provisioned in your Google Cloud project. To learn more, see the [Cluster Toolkit documentation](docs/blueprint-validation.md).

## Enable Google Cloud APIs

When you create a new Google Cloud project there are several APIs that you enable to deploy your cluster. These will be caught when you perform `terraform apply` but you can save time by enabling them upfront.

To learn more, see
[Set up Cluster Toolkit](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment#enable-apis).

## Google Cloud Quotas

You may need to request additional quota to be able to deploy and use your
cluster.

For more information, see
[Request additional quotas](https://cloud.google.com/cluster-toolkit/docs/setup/hpc-blueprint#request-quota).

## Billing Reports

You can view billing reports for your cluster on the
[Cloud Billing Reports](https://cloud.google.com/billing/docs/how-to/reports)
page. To view the Cloud Billing reports for your Cloud Billing account,
including the cost information for all of the Cloud projects that are
linked to the account, you need a role that includes the
`billing.accounts.getSpendingInformation` permission on your Cloud Billing
account.

To view the Cloud Billing reports for your Cloud Billing account, do the following:

1. In the Google Cloud Console, go to `Navigation Menu` >
   [`Billing`](https://console.cloud.google.com/billing/overview).
2. At the prompt, choose the Cloud Billing account for which you'd like to view
   reports. The Billing Overview page opens for the selected billing account.
3. In the Billing navigation menu, select `Reports`.

On the right side of the page, expand the `Filters` view and then filter by label, specifying the key `ghpc_deployment` (or `ghpc_blueprint`) and the desired value.

## Troubleshooting

### Authentication

Check your Google Cloud credential settings. To learn more, see [Google Cloud credentials](#google-cloud-credentials).

### Slurm Clusters

To troubleshoot Slurm, see the [Slurm Troubleshooting documentation](docs/slurm-troubleshooting.md).

### Terraform deployment

When a `terraform apply` command fails, Terraform generally provides an error
message. Some common reasons for the deployment to fail are as follows:

* **Google Cloud project Access:** The credentials being used to call `terraform apply` do not have access to the Google Cloud project. You can fix this issue by granting access in `IAM & Admin` [section](https://console.cloud.google.com/iam-admin/iam) on Google Cloud Console.
* **Disabled APIs:** Your Google Cloud project must have the proper APIs enabled. To learn more, see [Enable GCP APIs](#enable-gcp-apis).
* **Insufficient quota:** Your Google Cloud project does not have enough quota to
  provision the requested resources. To learn more, see [GCP Quotas](#gcp-quotas).
* **Filestore resource limit:** When regularly deploying Filestore instances
  with a new VPC, you may see an error during deployment such as:
  `System limit for internal resources has been reached`. See
  [this doc](https://cloud.google.com/filestore/docs/troubleshooting#system_limit_for_internal_resources_has_been_reached_error_when_creating_an_instance)
  for the solution.
* **Required permission not found:** If you see an error message such as the following, your credentials might not be set, or set incorrectly. `Required 'compute.projects.get' permission for 'projects/... forbidden`. To learn more, see [Cloud credentials on your workstation](#cloud-credentials-on-your-workstation). Ensure proper permissions are set in the Google Cloud Console [IAM section](https://console.cloud.google.com/iam-admin/iam).

### Failure to destroy VPC network

If the `terraform destroy` command fails, you might see an error message similar to one of the following examples. These errors indicate that the VPC network cannot be destroyed because resources that depend upon the network were added outside of Terraform and that those resources depend upon the network. You must manually delete these resources.
The following message indicates that a new VM has been added to a subnetwork within the VPC network:

```text
│ Error: Error when reading or editing Subnetwork: googleapi: Error 400: The subnetwork resource 'projects/<project_name>/regions/<region>/subnetworks/<subnetwork_name>' is already being used by 'projects/<project_name>/zones/<zone>/instances/<instance_name>', resourceInUseByAnotherResource
```

Or

The following message indicates that a new firewall rule has been added to the VPC network:

```text
│ Error: Error waiting for Deleting Network: The network resource 'projects/<project_name>/global/networks/<vpc_network_name>' is already being used by 'projects/<project_name>/global/firewalls/<firewall_rule_name>'
```

If you see an error message that does not look like either of the examples in this section, examine it carefully to identify the type of resource to delete and its unique name. As in the two preceding example messages, the resource name appears toward the end of the error message.

To manage VMs and firewall rules within the Cloud Console, see the following:

* [Cloud Console: Manage VM instances][cc-vms]
* [Cloud Console: Manage Firewall Rules][cc-firewall]

[cc-vms]: https://console.cloud.google.com/compute/instances
[cc-firewall]:  https://console.cloud.google.com/networking/firewalls/list

To ensure that you are looking at the correct page, check that your project ID is selected in the drop-down menu at the top-left of the screen.

## Inspect the deployment

The deployment that you create has the following directory structure:

```text
<<OUTPUT_PATH>>/<<DEPLOYMENT_NAME>>/{<<DEPLOYMENT_GROUPS>>}/
```

If an output directory is provided with the `--output/-o` flag, the deployment
directory will be created in the output directory, represented as
`<<OUTPUT_PATH>>`. If no output directory is provided, `<<OUTPUT_PATH>>` will default to the current working directory.

The deployment directory is created in `<<OUTPUT_PATH>>` as a directory matching
the provided `deployment_name` deployment variable (`vars`) in the blueprint.

There are directories representing each deployment group in the blueprint contained within the deployment directory. These directories are named the same as the `group` field for each element in `deployment_groups`.

Each deployment group directory contains all of the configuration scripts and
modules that you need to make a deployment. Each module in the `modules` directory is named as the resource it creates, for example the
[vpc module](./modules/network/vpc/README.md) is in a directory named `vpc`.

A hidden directory named .ghpc is also created. This directory contains meta information and backups.

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

For more information, see [Installing Dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies).

### Notes on Packer

Cluster Toolkit supports Packer templates in [HCL2 file
format][pkrhcl2] and not in the legacy JSON file format. You must use Packer 1.7.9 or above, although we recommend using the latest release.

The Toolkit's [Packer template module documentation][pkrmodreadme] describes
input variables and their behavior. An [image-building example][pkrexample]
and [usage instructions][pkrexamplereadme] are provided. The example integrates
Packer, Terraform and [startup-script](./modules/scripts/startup-script/README.md) runners to demonstrate the power of customizing images using the same scripts that can be applied at boot-time.

[pkrhcl2]: https://www.packer.io/guides/hcl
[pkrmodreadme]: modules/packer/custom-image/README.md
[pkrexamplereadme]: examples/README.md#image-builderyaml
[pkrexample]: examples/image-builder.yaml

## Development

The following setup is in addition to the [dependencies](#dependencies) needed
to build and run Cluster-Toolkit.

Before you push any code changes, use the `pre-commit` hooks [configured](./.pre-commit-config.yaml) in this repository to ensure that all changes are validated, tested and properly documented. The pre-commits configured
in Cluster Toolkit have a set of dependencies that need to be installed before they can be successfully passed.

To install and setup pre-commit in your cloned repository, do the following:

1. Install pre-commit using the instructions from [the pre-commit website](https://pre-commit.com/).
1. Install TFLint using the instructions from [the TFLint documentation](https://github.com/terraform-linters/tflint#installation).

   > **_NOTE:_** The version of TFLint that you install must be compatible with the Google plugin
   > version listed in [tflint.hcl](.tflint.hcl). Versions of the plugin
   > `>=0.20.0` should use `tflint>=0.40.0`. These versions are
   > available through GitHub or package managers. We recommend that you review the [TFLint Ruleset
   > for Google Release Notes][tflint-google] for up-to-date requirements.

[tflint-google]: https://github.com/terraform-linters/tflint-ruleset-google/releases

1. Install ShellCheck using the instructions from
   [the ShellCheck documentation](https://github.com/koalaman/shellcheck#installing)
1. The remaining dev dependencies can be installed by running the following command
   in the project root directory:

    ```shell
    make install-dev-deps
    ```

1. Enable pre-commit repo-by-repo basis by running the following command
   in the project root directory:

    ```shell
    pre-commit install
    ```

Pre-commit is configured to automatically run before you commit.

### Development on macOS

While macOS is a supported environment for building and executing with Cluster Toolkit,
it's not supported for Cluster Toolkit development due to GNU-specific shell scripts.

If you’re working on a Mac device, a workaround is to install GNU tooling by installing
`coreutils` and `findutils` from a package manager such as homebrew or conda.

### Contributing

Please refer to the [contributing file](CONTRIBUTING.md) in our GitHub
repository, or to
[Google’s Open Source documentation](https://opensource.google/docs/releasing/template/CONTRIBUTING/#).
