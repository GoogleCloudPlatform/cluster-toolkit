# Google HPC-Toolkit

## Description
HPC Toolkit is an open-source software offered by Google Cloud which makes it
easy for customers to deploy HPC environments on Google Cloud.

HPC Toolkit allows customers to deploy turnkey HPC environments (compute,
networking, storage, etc) following Google Cloud best-practices, in a repeatable
manner. The HPC Toolkit is designed to be highly customizable and extensible,
and intends to address the HPC deployment needs of a broad range of customers.

## Dependencies
* [Terraform](https://www.terraform.io/downloads.html)
* [Packer](https://www.packer.io/downloads)
* [golang](https://golang.org/doc/install)
  * To setup GOPATH and development environment: `export PATH=$PATH:$(go env GOPATH)/bin`
* [terraform-docs](https://github.com/terraform-docs/terraform-docs) (for development only)
    * `go install github.com/terraform-docs/terraform-docs@v0.16.0`

## Build and Install
Simply run `make` in the root directory.

## Basic Usage
To create a blueprint, an input YAML file needs to be written or adapted from
the examples under `examples`. A good starting point is
`examples/hpc-cluster-small.yaml` which creates a blueprint for a new network,
a filestore instance and a slurm login node and controller.
More information on the example configs can be found in the README.md of the
`examples` directory.

In order to create a blueprint using `ghpc`, first ensure you've updated your
config template to include your GCP project ID then run the following command:

```
./ghpc create --config examples/hpc-cluster-small.yaml
```

The blueprint directory, named as the `blueprint_name` field from the input
config will be created in the same directory as ghpc.

To deploy the blueprint, use terraform in the resource group directory:
```
cd hpc-slurm/primary # From hpc-cluster-small.yaml example
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
Please use the `pre-commit` hooks configured in this repository to ensure
that all Terraform modules are validated and properly documented before pushing
code changes. [pre-commit][https://pre-commit.com/] can be installed using
standard package managers. It is enabled on a repo-by-repo basis by switching to
the root directory of the repo and running:

```shell
pre-commit install
```
