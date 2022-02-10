# Google HPC-Toolkit

## Description

HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

HPC Toolkit allows customers to deploy turnkey HPC environments (compute,
networking, storage, etc) following Google Cloud best-practices, in a repeatable
manner. The HPC Toolkit is designed to be highly customizable and extensible,
and intends to address the HPC deployment needs of a broad range of customers.

## Dependencies

* make
* git
* [golang](https://golang.org/doc/install): version 1.16 or greater, used to
  build ghpc.
  * To setup GOPATH and development environment: `export PATH=$PATH:$(go env GOPATH)/bin`
* [Terraform](https://www.terraform.io/downloads.html): version 1.0.0 or
  greater, used to deploy blueprints.
* [Packer](https://www.packer.io/downloads): version 1.6.0 or greater, used to
  build images.

## Build and Install

Simply run `make` in the root directory.

## Basic Usage

To create a blueprint, an input YAML file needs to be written or adapted from
the examples under [examples](examples/). A good starting point is
[examples/hpc-cluster-small.yaml](examples/hpc-cluster-small.yaml) which creates
a blueprint for a new network, a filestore instance and a slurm login node and
controller. More information on the example configs can be found in the
[README.md](examples/README.md) of the [examples](examples/) directory.

In order to create a blueprint using `ghpc`, first ensure you've updated your
config template to include your GCP project ID then run the following command:

```shell
./ghpc create examples/hpc-cluster-small.yaml
```

The blueprint directory, named as the `blueprint_name` field from the input
config will be created in the same directory as ghpc. The output directory can
be specified by -o flag.

```shell
./ghpc create examples/hpc-cluster-small.yaml -o blueprints/
```

To deploy the blueprint, use terraform in the resource group directory:

```shell
cd hpc-cluster-small/primary # From hpc-cluster-small.yaml example
terraform init
terraform apply
```

## MacOS details

* Install GNU `findutils` with Homebrew or Conda
  * `brew install findutils` (and follow instructions for modifying `PATH`)
  * `conda install findutils`
* If using `conda`, it's easier to use conda-forge Golang without CGO
  * `conda install go go-nocgo go-nocgo_osx-64`

## Development

Please use the `pre-commit` hooks [configured](./.pre-commit-config.yaml) in
this repository to ensure that all Terraform and golang modules are validated
and properly documented before pushing code changes.
[pre-commit](https://pre-commit.com/) can be installed using standard package
managers, more details can be found at [the pre-commit website](https://pre-commit.com/).

The pre-commits configured in the HPC Toolkit have a set of
dependencies that need to be installed before successfully passing all
pre-commits. TFLint and ShellCheck must be installed manually, the instructions
can be found
[here for tflint](https://github.com/terraform-linters/tflint#installation).
and [here for shellcheck](https://github.com/koalaman/shellcheck#installing)
The other dependencies can be installed by running the following command in the
root directory:

```shell
make install-deps-dev
```

pre-commit is enabled on a repo-by-repo basis by switching to the root
directory of the repo and running:

```shell
pre-commit install
```

During development, to re-build the ghpc binary run the following command:

```shell
make ghpc-dev
```

which in addition to building the binary will also run go fmt and vet against
the codebase.

### Packer

Auto-generated READMEs are created for Packer resources similar to Terraform
resources. These docs are generated as part of a pre-commit hook (packer-readme)
which searches for `*.pkr.hcl` files. If a packer config is written in another
file, for instance JSON, terraform docs should be run manually against the
resource directory before pushing changes. To generate the documentation, run
the following script against the packer config file:

```shell
tools/autodoc/terraform_docs.sh resources/packer/new_resource/image.json
```
