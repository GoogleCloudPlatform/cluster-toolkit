# Build

## Table of Contents

<!-- mdformat-toc start --slug=github --no-anchors --maxlevel=6 --minlevel=1 -->

- [Build](#build)
  - [Table of Contents](#table-of-contents)
  - [Overview](#overview)
    - [Compatibility](#compatibility)
  - [Slurm](#slurm)
    - [With Custom Registry](#with-custom-registry)
  - [Development](#development)
  - [Multiple Architectures](#multiple-architectures)
    - [Emulation (QEMU)](#emulation-qemu)
    - [Multiple Native Nodes](#multiple-native-nodes)

<!-- mdformat-toc end -->

## Overview

Instructions for building images via [docker bake].

### Compatibility

| Software      |                      Minimum Version                       |
| ------------- | :--------------------------------------------------------: |
| Docker Engine | [28.1.0](https://docs.docker.com/engine/release-notes/28/) |

## Slurm

Build Slurm from the selected Slurm version and Linux flavor.

```sh
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./$VERSION/$FLAVOR/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS --print
docker bake $BAKE_IMPORTS
```

For example, the following will build Slurm 25.05 on Rocky Linux 9.

```sh
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./25.05/rockylinux9/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS --print
docker bake $BAKE_IMPORTS
```

### With Custom Registry

Build Slurm from the selected Slurm version and Linux flavor.

```sh
export REGISTRY="my/registry"
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./$VERSION/$FLAVOR/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS --print
docker bake $BAKE_IMPORTS
```

## Development

Build Slurm from the selected repository and branch for a Slurm version and
Linux flavor.

> [!NOTE]
> The docker SSH agent is used to avoid credentials leaking into the image
> layers. You will need to add a default private key if the target repository is
> private.

```sh
ssh-add ~/.ssh/id_ed25519 # if private repo
```

Build Slurm from the selected Slurm version and Linux flavor.

```sh
export GIT_REPO=git@github.com:SchedMD/slurm.git
export GIT_BRANCH=master
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./$VERSION/$FLAVOR/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS dev --print
docker bake $BAKE_IMPORTS dev
```

## Multiple Architectures

Build Slurm images with the `multiarch` target:

```sh
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./$VERSION/$FLAVOR/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS multiarch --print
docker bake $BAKE_IMPORTS multiarch
```

There are multiple ways to configure builders for multiple
[architectures/platforms][multi-platform].

### Emulation (QEMU)

A single machine can be configured to use QEMU to emulate different
architectures.

> [!NOTE]
> Emulation with QEMU can be much slower than native builds, especially for
> compute-heavy tasks like compilation and compression or decompression.

Install host dependencies.

```sh
sudo dnf install -y qemu-user-binfmt qemu-user-static
sudo apt-get install -y binfmt-support qemu-user-static
```

Configure QEMU with docker:

```sh
docker run --rm --privileged tonistiigi/binfmt --install all
docker run --rm --privileged multiarch/qemu-user-static --reset -p yes
```

Create a docker builder for QEMU to use:

```sh
docker buildx create --name multiarch --bootstrap
docker buildx inspect multiarch
```

Build Slurm images:

```sh
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./$VERSION/$FLAVOR/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS --builder multiarch multiarch --print
docker bake $BAKE_IMPORTS --builder multiarch multiarch
```

> [!WARNING]
> Compiling Slurm with QEMU can take more than 1 hour instead of a few minutes
> on a native architecture.

### Multiple Native Nodes

Create a docker builder for multiple architectures:

```sh
docker buildx create --name multiarch --bootstrap
docker buildx inspect multiarch
```

The following command creates a multi-node builder from Docker contexts named
node-amd64 and node-arm64. This example assumes that you've already added those
contexts.

```sh
docker buildx ls
docker buildx create --name multiarch node-amd64
docker buildx create --name multiarch --append node-arm64
```

Build Slurm images:

```sh
export BAKE_IMPORTS="--file ./docker-bake.hcl --file ./$VERSION/$FLAVOR/slurm.hcl"
cd ./schedmd/slurm/
docker bake $BAKE_IMPORTS --builder multiarch multiarch --print
docker bake $BAKE_IMPORTS --builder multiarch multiarch
```

<!-- Links -->

[docker bake]: https://docs.docker.com/build/bake/introduction/
[multi-platform]: https://docs.docker.com/build/building/multi-platform/
