# Google HPC-Toolkit

## Description

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

HPC Toolkit allows customers to deploy turnkey HPC environments (compute,
networking, storage, etc) following Google Cloud best-practices, in a repeatable
manner. The HPC Toolkit is designed to be highly customizable and extensible,
and intends to address the HPC deployment needs of a broad range of customers.

## Installation

These instructions assume you are using
[Cloud Shell](https://cloud.google.com/shell) which comes with the above
dependencies pre-installed (minus Packer which is not needed for this example).

To use the HPC-Toolkit, you must clone the project from GitHub and build the
`ghpc` binary.

You must first set up Cloud Shell to authenticate with GitHub. We will use an
SSH key.

> **_NOTE:_** You can skip this step if you have previously set up cloud shell
> with GitHub.  
> **_NOTE:_** You can find much more detailed instructions for this step in the
> [GitHub docs](https://docs.github.com/en/authentication/connecting-to-github-with-ssh).  
> **_NOTE:_** This step is only required during the private preview of the
> HPC-Toolkit.

```shell
# On Cloud Shell
ssh-keygen -t ed25519 -C "your_email@example.com"  # follow prompts
cat ~/.ssh/id_ed25519.pub                          # copy output
```

Use the output to add your Cloud Shell SSH key to GitHub by pasting your key [here](https://github.com/settings/ssh/new).

Next you will clone the HPC-Toolkit repo from GitHub.

```shell
git clone git@github.com:GoogleCloudPlatform/hpc-toolkit.git
```

Finally you build the toolkit.

```shell
cd hpc-toolkit && make
```

You should now have a binary named `ghpc` in the project root directory.
Optionally, you can run `./ghpc --version` to verify the build.

## Basic Usage

To create a blueprint, an input YAML file needs to be written or adapted from
one of the [examples](examples/).

These instructions will use
[examples/hpc-cluster-small.yaml](examples/hpc-cluster-small.yaml), which is a
good starting point and creates a blueprint containing:

* a new network
* a filestore instance
* a slurm login node
* a slurm controller

> **_NOTE:_** More information on the example configs can be found in
> [examples/README.md](examples/README.md).

These instructions assume you are using
[Cloud Shell](https://cloud.google.com/shell) in the context of the GCP project
you wish to deploy in, and that you are in the root directory of the hpc-toolkit
repo cloned during [installation](#installation).

The [examples/hpc-cluster-small.yaml](examples/hpc-cluster-small.yaml) file must
be updated to point to your GCP project ID. You can either edit the file
manually or run the following command.

```shell
sed -i \
  "s/## Set GCP Project ID Here ##/$GOOGLE_CLOUD_PROJECT/g" \
  examples/hpc-cluster-small.yaml
```

Now you can run `ghpc` with the following command:

```shell
./ghpc create examples/hpc-cluster-small.yaml
```

By default, the blueprint directory will be created in the same directory as the
`ghpc` binary and will have the name specified by the `blueprint_name` field
from the input config. Optionally, the output directory can be specified with
the `-o` flag as shown in the following example.

```shell
./ghpc create examples/hpc-cluster-small.yaml -o blueprints/
```

To deploy the blueprint, use terraform in the resource group directory:

> **_NOTE:_** Before you run this for the first time you may need to enable some
> APIs. See [Enable GCP APIs](#enable-gcp-apis).

```shell
cd hpc-cluster-small/primary # From hpc-cluster-small.yaml example
terraform init
terraform apply
```

Once the blueprint has successfully been deployed, take the following steps to run a job:

* First navigate to `Compute Engine` > `VM instances` in the Google Cloud Console.
* Next click on the `SSH` button associated with the `slurm-hpc-small-login0` instance.
* Finally run the `hostname` command on 3 nodes by running the following command in the shell popup:

```shell
$ srun -N 3 hostname
slurm-hpc-slurm-small-debug-0-0
slurm-hpc-slurm-small-debug-0-1
slurm-hpc-slurm-small-debug-0-2
```

By default, this runs the job on the `debug` partition. See details in
[examples/](examples/README.md#compute-partition) for how to run on the more
performant `compute` partition.  

> **_NOTE:_** Cloud Shell times out after 20 minutes of inactivity. This example
> deploys in about 5 minutes but for more complex deployments it may be
> necessary to deploy (`terraform apply`) from a cloud VM. The same process
> above can be used, although [dependencies](#dependencies) will need to be
> installed first.

This example does not contain any Packer-based resources but for completeness,
you can use the following command to deploy a Packer-based resource group:

```shell
cd <blueprint-directory>/<packer-group>/<custom-vm-image>
packer build .
```

## Enable GCP APIs

In a new GCP project there are several apis that must be enabled to deploy your
HPC cluster. These will be caught when you perform `terraform apply` but you can
save time by enabling them upfront.

List of APIs to enable ([instructions](https://cloud.google.com/apis/docs/getting-started#enabling_apis)):

* Compute Engine API
* Cloud Filestore API
* Cloud Runtime Configuration API - _needed for `high-io` example_

## Inspecting the Blueprint

The blueprint is created in the directory matching the provided blueprint_name
variable in the config. Within this directory are all the resources needed to
create a deployment. The blueprint directory will contain subdirectories
representing the resource groups defined in the config YAML. Most example
configurations contain a single resource group.

From the [example above](#basic-usage) we get the following blueprint:

```text
hpc-cluster-small/
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
```

## Dependencies

Much of the HPC Toolkit blueprint is built using Terraform and Packer, and
therefore they must be available in the same machine calling the toolkit. In
addition, building the HPC Toolkit from source requires git, make, and Go to be
installed.

List of dependencies:

* Terraform: version>=1.0.0 - [install instructions](https://www.terraform.io/downloads.html)
* Packer: version>=1.6.0 - [install instructions](https://www.packer.io/downloads)
* golang: version>=1.16 - [install instructions](https://golang.org/doc/install)
  * To setup GOPATH and development environment: `export PATH=$PATH:$(go env GOPATH)/bin`
* make
* git

## MacOS Details

* Install GNU `findutils` with Homebrew or Conda
  * `brew install findutils` (and follow instructions for modifying `PATH`)
  * `conda install findutils`
* If using `conda`, it's easier to use conda-forge Golang without CGO
  * `conda install go go-nocgo go-nocgo_osx-64`

## Development

The following setup is in addition to the [dependencies](#dependencies) needed
to build and run HPC-Toolkit.

Please use the `pre-commit` hooks [configured](./.pre-commit-config.yaml) in
this repository to ensure that all Terraform and golang modules are validated
and properly documented before pushing code changes. The pre-commits configured
in the HPC Toolkit have a set of dependencies that need to be installed before
successfully passing.

1. Install pre-commit using the instructions from [the pre-commit website](https://pre-commit.com/).
1. Install TFLint using the instructions from
   [the TFLint documentation](https://github.com/terraform-linters/tflint#installation).
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

### Packer Documentation

Auto-generated READMEs are created for Packer resources similar to Terraform
resources. These docs are generated as part of a pre-commit hook (packer-readme)
which searches for `*.pkr.hcl` files. If a packer config is written in another
file, for instance JSON, terraform docs should be run manually against the
resource directory before pushing changes. To generate the documentation, run
the following script against the packer config file:

```shell
tools/autodoc/terraform_docs.sh resources/packer/new_resource/image.json
```
