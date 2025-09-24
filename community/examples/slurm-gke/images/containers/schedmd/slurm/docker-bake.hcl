// SPDX-FileCopyrightText: Copyright (C) SchedMD LLC.
// SPDX-License-Identifier: Apache-2.0

################################################################################

variable "REGISTRY" {
  default = "ghcr.io/slinkyproject"
}

variable "SUFFIX" {}

################################################################################

slurm_version = "master"
slurm_dir = slurm_version(slurm_version)
linux_flavor = "rockylinux9"
context = "${slurm_dir}/${linux_flavor}"

################################################################################

function "slurm_semantic_version" {
  params = [version]
  result = regex("^(?<major>[0-9]+)\\.(?<minor>[0-9]+)\\.(?<patch>[0-9]+)(?:-(?<rev>.+))?$", "${version}")
}

function "slurm_version" {
  params = [version]
  result = (
    length(regexall("^(?<major>[0-9]+)\\.(?<minor>[0-9]+)\\.(?<patch>[0-9]+)(?:-(?<rev>.+))?$", "${version}")) > 0
      ? format("%s.%s", "${slurm_semantic_version("${version}")["major"]}", "${slurm_semantic_version("${version}")["minor"]}")
      : version
  )
}

function "format_tag" {
  params = [registry, stage, version, flavor, suffix]
  result = format("%s:%s", join("/", compact([registry, stage])), join("-", compact([version, flavor, suffix])))
}

################################################################################

target "_slurm" {
  args = {
    SLURM_VERSION = slurm_version
  }
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.authors" = "slinky@schedmd.com"
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/documentation.html"
    "org.opencontainers.image.license" = "GPL-2.0-or-later WITH openssl-exception"
    "org.opencontainers.image.vendor" = "SchedMD LLC."
    "org.opencontainers.image.version" = slurm_version
    "org.opencontainers.image.source" = "https://github.com/SlinkyProject/containers"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "vendor" = "SchedMD LLC."
    "version" = slurm_version
    "release" = "https://github.com/SlinkyProject/containers"
  }
}

target "_slurmctld" {
  inherits = ["_slurm"]
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.title" = "Slurm Control Plane"
    "org.opencontainers.image.description" = "slurmctld - The central management daemon of Slurm"
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/slurmctld.html"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "name" = "Slurm Control Plane"
    "summary" = "slurmctld - The central management daemon of Slurm"
    "description" = "slurmctld - The central management daemon of Slurm"
  }
}

target "_slurmd" {
  inherits = ["_slurm"]
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.title" = "Slurm Worker Agent"
    "org.opencontainers.image.description" = "slurmd - The compute node daemon for Slurm"
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/slurmd.html"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "name" = "Slurm Worker Agent"
    "summary" = "slurmd - The compute node daemon for Slurm"
    "description" = "slurmd - The compute node daemon for Slurm"
  }
}

target "_slurmdbd" {
  inherits = ["_slurm"]
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.title" = "Slurm Database Agent"
    "org.opencontainers.image.description" = "slurmdbd - Slurm Database Daemon"
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/slurmdbd.html"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "name" = "Slurm Database Agent"
    "summary" = "slurmdbd - Slurm Database Daemon"
    "description" = "slurmdbd - Slurm Database Daemon"
  }
}

target "_slurmrestd" {
  inherits = ["_slurm"]
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.title" = "Slurm REST API Agent"
    "org.opencontainers.image.description" = "slurmrestd - Interface to Slurm via REST API"
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/slurmrestd.html"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "name" = "Slurm REST API Agent"
    "summary" = "slurmrestd - Interface to Slurm via REST API"
    "description" = "slurmrestd - Interface to Slurm via REST API"
  }
}

target "_sackd" {
  inherits = ["_slurm"]
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.title" = "Slurm Auth/Cred Server"
    "org.opencontainers.image.description" = "sackd - Slurm Auth and Cred Kiosk Daemon"
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/sackd.html"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "name" = "Slurm Auth/Cred Server"
    "summary" = "sackd - Slurm Auth and Cred Kiosk Daemon"
    "description" = "sackd - Slurm Auth and Cred Kiosk Daemon"
  }
}

target "_login" {
  inherits = ["_slurm"]
  labels = {
    # Ref: https://github.com/opencontainers/image-spec/blob/v1.0/annotations.md
    "org.opencontainers.image.title" = "Slurm Login Container"
    "org.opencontainers.image.description" = "An authenticated environment to submit Slurm workload from."
    "org.opencontainers.image.documentation" = "https://slurm.schedmd.com/quickstart_admin.html#login"
    # Ref: https://docs.redhat.com/en/documentation/red_hat_software_certification/2025/html/red_hat_openshift_software_certification_policy_guide/assembly-requirements-for-container-images_openshift-sw-cert-policy-introduction#con-image-metadata-requirements_openshift-sw-cert-policy-container-images
    "name" = "Slurm Login Container"
    "summary" = "An authenticated environment to submit Slurm workload from."
    "description" = "An authenticated environment to submit Slurm workload from."
  }
}

################################################################################

group "default" {
  targets = [
    "core",
  ]
}

group "all" {
  targets = [
    "core",
    "extras",
  ]
}

group "core" {
  targets = [
    "slurmctld",
    "slurmd",
    "slurmdbd",
    "slurmrestd",
    "sackd",
    "login",
  ]
}

target "slurmctld" {
  inherits = ["_slurmctld"]
  context = context
  target = "slurmctld"
  tags = [
    format_tag(REGISTRY, "slurmctld", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "slurmctld", slurm_version, linux_flavor, SUFFIX),
  ]
}

target "slurmd" {
  inherits = ["_slurmd"]
  context = context
  target = "slurmd"
  tags = [
    format_tag(REGISTRY, "slurmd", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "slurmd", slurm_version, linux_flavor, SUFFIX),
  ]
}

target "slurmdbd" {
  inherits = ["_slurmdbd"]
  context = context
  target = "slurmdbd"
  tags = [
    format_tag(REGISTRY, "slurmdbd", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "slurmdbd", slurm_version, linux_flavor, SUFFIX),
  ]
}

target "slurmrestd" {
  inherits = ["_slurmrestd"]
  context = context
  target = "slurmrestd"
  tags = [
    format_tag(REGISTRY, "slurmrestd", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "slurmrestd", slurm_version, linux_flavor, SUFFIX),
  ]
}

target "sackd" {
  inherits = ["_sackd"]
  context = context
  target = "sackd"
  tags = [
    format_tag(REGISTRY, "sackd", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "sackd", slurm_version, linux_flavor, SUFFIX),
  ]
}

target "login" {
  inherits = ["_login"]
  context = context
  target = "login"
  tags = [
    format_tag(REGISTRY, "login", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "login", slurm_version, linux_flavor, SUFFIX),
  ]
}

group "extras" {
  targets = [
    "slurmd_pyxis",
    "login_pyxis",
  ]
}

target "_pyxis" {
  context = context
  dockerfile = "Dockerfile.pyxis"
  args = {
    REGISTRY = REGISTRY
  }
}

target "slurmd_pyxis" {
  inherits = ["_slurmd", "_pyxis"]
  target = "slurmd-pyxis"
  tags = [
    format_tag(REGISTRY, "slurmd-pyxis", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "slurmd-pyxis", slurm_version, linux_flavor, SUFFIX),
  ]
  contexts = {
    format_tag(REGISTRY, "slurmd", slurm_version(slurm_version), linux_flavor, SUFFIX) = "target:slurmd"
    format_tag(REGISTRY, "slurmd", slurm_version, linux_flavor, SUFFIX) = "target:slurmd"
  }
}

target "login_pyxis" {
  inherits = ["_login", "_pyxis"]
  target = "login-pyxis"
  tags = [
    format_tag(REGISTRY, "login-pyxis", slurm_version(slurm_version), linux_flavor, SUFFIX),
    format_tag(REGISTRY, "login-pyxis", slurm_version, linux_flavor, SUFFIX),
  ]
  contexts = {
    format_tag(REGISTRY, "slurmd", slurm_version(slurm_version), linux_flavor, SUFFIX) = "target:slurmd"
    format_tag(REGISTRY, "slurmd", slurm_version, linux_flavor, SUFFIX) = "target:slurmd"
    format_tag(REGISTRY, "login", slurm_version(slurm_version), linux_flavor, SUFFIX) = "target:login"
    format_tag(REGISTRY, "login", slurm_version, linux_flavor, SUFFIX) = "target:login"
  }
}

################################################################################

group "multiarch" {
  targets = [
    "core-multiarch",
  ]
}

group "core-multiarch" {
  targets = [
    "slurmctld_multiarch",
    "slurmd_multiarch",
    "slurmdbd_multiarch",
    "slurmrestd-multiarch",
    "sackd_multiarch",
    "login_multiarch",
  ]
}

group "all-multiarch" {
  targets = [
    "core-multiarch",
    "extras-multiarch",
  ]
}

target "_multiarch" {
  platforms = [
    "linux/amd64",
    "linux/arm64"
  ]
}

target "slurmctld_multiarch" {
  inherits = ["slurmctld", "_multiarch"]
}

target "slurmd_multiarch" {
  inherits = ["slurmd", "_multiarch"]
}

target "slurmdbd_multiarch" {
  inherits = ["slurmdbd", "_multiarch"]
}

target "slurmrestd-multiarch" {
  inherits = ["slurmrestd", "_multiarch"]
}

target "sackd_multiarch" {
  inherits = ["sackd", "_multiarch"]
}

target "login_multiarch" {
  inherits = ["login", "_multiarch"]
}

group "extras-multiarch" {
  targets = [
    "slurmd_pyxis_multiarch",
    "login_pyxis_multiarch",
  ]
}

target "slurmd_pyxis_multiarch" {
  inherits = ["slurmd_pyxis", "_multiarch"]
}

target "login_pyxis_multiarch" {
  inherits = ["login_pyxis", "_multiarch"]
}

################################################################################

variable "GIT_REPO" {
  default = "git@gitlab.com:SchedMD/dev/slurm.git"
}

variable "GIT_BRANCH" {
  default = git_branch(slurm_version)
}

function "git_branch" {
  params = [version]
  result = (
    length(regexall("^(?<major>[0-9]+)\\.(?<minor>[0-9]+)\\.(?<patch>[0-9]+)(?:-(?<rev>.+))?$", "${version}")) > 0
      ? format("slurm-%s", slurm_version(version))
      : version
  )
}

target "_dev" {
  contexts = {
    "slurm-src" = "target:slurm-src-dev"
  }
  ssh = [
    # ssh-add ~/.ssh/id_ed25519
    { id = "default" },
  ]
}

target "slurm-src-dev" {
  dockerfile = "Dockerfile.dev"
  args = {
    GIT_REPO = GIT_REPO
    GIT_BRANCH = GIT_BRANCH
  }
}

group "dev" {
  targets = [
    "core-dev",
  ]
}

group "all-dev" {
  targets = [
    "core-dev",
    "extras-dev",
  ]
}

group "core-dev" {
  targets = [
    "slurmctld_dev",
    "slurmd_dev",
    "slurmdbd_dev",
    "slurmrestd_dev",
    "sackd_dev",
    "login_dev",
  ]
}

target "slurmctld_dev" {
  inherits = ["slurmctld", "_dev"]
}

target "slurmd_dev" {
  inherits = ["slurmd", "_dev"]
}

target "slurmdbd_dev" {
  inherits = ["slurmdbd", "_dev"]
}

target "slurmrestd_dev" {
  inherits = ["slurmrestd", "_dev"]
}

target "sackd_dev" {
  inherits = ["sackd", "_dev"]
}

target "login_dev" {
  inherits = ["login", "_dev"]
}

group "extras-dev" {
  targets = [
    "slurmd_pyxis_dev",
    "login_pyxis_dev",
  ]
}

target "slurmd_pyxis_dev" {
  inherits = ["slurmd_pyxis", "_dev"]
}

target "login_pyxis_dev" {
  inherits = ["login_pyxis", "_dev"]
}
