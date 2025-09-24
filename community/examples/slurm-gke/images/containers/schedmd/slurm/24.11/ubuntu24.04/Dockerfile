# syntax=docker/dockerfile:1
# SPDX-FileCopyrightText: Copyright (C) SchedMD LLC.
# SPDX-FileCopyrightText: Copyright (C) 2025 Google LLC.
# SPDX-License-Identifier: Apache-2.0

################################################################################

ARG SLURM_VERSION=24.11-latest
ARG SLURM_DIR="slurm-${SLURM_VERSION}"
ARG PARENT_IMAGE=ubuntu:24.04

################################################################################
FROM alpine:latest AS slurm-src

ARG SLURM_VERSION
ARG SLURM_DIR

WORKDIR /workspace/

ADD https://download.schedmd.com/slurm/${SLURM_DIR}.tar.bz2 ${SLURM_DIR}.tar.bz2

RUN <<EOR
# Unpack Slurm Source
set -xeuo pipefail
apk add --no-cache tar
mkdir -p ${SLURM_DIR}
tar --strip-components=1 -jxvf ${SLURM_DIR}.tar.bz2 -C ${SLURM_DIR}
rm ${SLURM_DIR}.tar.bz2
EOR

################################################################################
FROM ${PARENT_IMAGE} AS build

SHELL ["bash", "-c"]

ARG DEBIAN_FRONTEND=noninteractive

ARG SLURM_VERSION
ENV SLURM_VERSION=${SLURM_VERSION}
ARG SLURM_DIR

USER root
WORKDIR /tmp/

COPY --from=slurm-src /workspace/${SLURM_DIR} ${SLURM_DIR}

COPY patches/ patches/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#debuild
RUN <<EOR
# Patch and Build Slurm
set -xeuo pipefail
apt-get -qq update
apt-get -qq -y upgrade
apt-get -qq -y install --no-install-recommends \
  build-essential fakeroot devscripts equivs
## Patch
find $(pwd)/patches/ -type f -name "*.patch" -print0 | sort -z | xargs -t0r -n1 patch -p1 -d ${SLURM_DIR} -i
## Build
mk-build-deps -ir --tool='apt-get -qq -y -o Debug::pkgProblemResolver=yes --no-install-recommends' ${SLURM_DIR}/debian/control
( cd ${SLURM_DIR} && debuild -b -uc -us >/dev/null )
## Cleanup
apt-get clean && rm -rf /var/lib/apt/lists/*
EOR

################################################################################
FROM ${PARENT_IMAGE} AS base

SHELL ["bash", "-c"]

ARG DEBIAN_FRONTEND=noninteractive

ARG SLURM_VERSION
ENV SLURM_VERSION=${SLURM_VERSION}

USER root
WORKDIR /tmp/

ARG SLURM_USER=slurm
ARG SLURM_USER_UID=981
ARG SLURM_USER_GID=981

RUN <<EOR
# Create SlurmUser
set -xeuo pipefail
groupadd --system --gid=${SLURM_USER_GID} ${SLURM_USER}
useradd --system --no-log-init --uid=${SLURM_USER_UID} --gid=${SLURM_USER_GID} --shell=/usr/sbin/nologin ${SLURM_USER}
EOR

RUN <<EOR
# Remove Users
set -xeuo pipefail
for user in $(ls /home/); do
  userdel --remove $user
done
EOR

COPY --from=build /tmp/*.deb /tmp/

RUN <<EOR
# Install Dependencies
set -xeuo pipefail
apt-get -qq update
apt-get -qq -y upgrade
# Init System
apt-get -qq -y install --no-install-recommends \
  supervisor tini
apt-get clean && rm -rf /var/lib/apt/lists/*
EOR

################################################################################
# SLURM: slurmd
# BUILD: `docker build --target=slurmd -t [<registry>/]slurmd:<tag> .`
################################################################################
FROM base AS slurmd

SHELL ["bash", "-c"]

ARG DEBIAN_FRONTEND=noninteractive

USER root
WORKDIR /tmp/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Slurm Packages
set -xeuo pipefail
apt-get -qq update

apt-get -qq -y install git
apt-get -qq -y install build-essential

apt-get -qq -y install --no-install-recommends --fix-broken \
  gawk \
  ./slurm-smd-client_[0-9]*.deb \
  ./slurm-smd-dev_[0-9]*.deb \
  ./slurm-smd-doc_[0-9]*.deb \
  ./slurm-smd-libnss-slurm_[0-9]*.deb \
  ./slurm-smd-libpam-slurm-adopt_[0-9]*.deb \
  ./slurm-smd-libpmi2-0_[0-9]*.deb \
  ./slurm-smd-slurmd_[0-9]*.deb \
  ./slurm-smd_[0-9]*.deb
rm *.deb && apt-get clean && rm -rf /var/lib/apt/lists/*

apt-get update
apt-get  -qq -y install libpmix-dev

# Configure
mkdir -p /var/spool/slurmd/
cp -v /etc/nsswitch.conf{,.bak}
sed -i -E "s/^passwd:[[:space:]]+/&slurm /g" /etc/nsswitch.conf
sed -i -E "s/^group:[[:space:]]+/&slurm /g" /etc/nsswitch.conf

EOR

COPY files/slurmd/etc/supervisor/supervisord.conf /etc/supervisor/supervisord.conf
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

RUN chmod +r+x /usr/local/bin/entrypoint.sh
RUN chmod +r+x / && chmod +r+x /usr/ && chmod +r+x /usr/bin/ &&  chmod +r+x /etc/ && chmod +r+x /usr/local/

RUN ls -l /usr/local/bin/entrypoint.sh
EXPOSE 6818/tcp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
