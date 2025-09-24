# syntax=docker/dockerfile:1
# SPDX-FileCopyrightText: Copyright (C) SchedMD LLC.
# SPDX-FileCopyrightText: Copyright (C) 2025 Google LLC.
# SPDX-License-Identifier: Apache-2.0

################################################################################

ARG SLURM_VERSION=24.11-latest
ARG SLURM_DIR="slurm-${SLURM_VERSION}"
ARG PARENT_IMAGE=rockylinux:9

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

ARG SLURM_VERSION
ENV SLURM_VERSION=${SLURM_VERSION}
ARG SLURM_DIR

USER root
WORKDIR /tmp/

COPY --from=slurm-src /workspace/${SLURM_DIR} ${SLURM_DIR}

COPY patches/ patches/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#rpmbuild
RUN <<EOR
# Patch and Build Slurm
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  '@Development' rpm-build 'dnf-command(builddep)' 'dnf-command(config-manager)' epel-release
dnf -q config-manager --enable crb
dnf -q -y upgrade --refresh
## Patch
find $(pwd)/patches/ -type f -name "*.patch" -print0 | sort -z | xargs -t0r -n1 patch -p1 -d ${SLURM_DIR} -i
## Build
SLURM_RPM_DIR="$(rpmspec -q ${SLURM_DIR}/slurm.spec 2> /dev/null | grep -Eo "slurm-[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+(-[[:alnum:]]+)?" | head -n1 | sed -E 's/-[[:digit:]]+$//g')"
if [ "${SLURM_DIR}" != "$SLURM_RPM_DIR" ]; then
  mv ${SLURM_DIR} $SLURM_RPM_DIR
fi
rm -f ${SLURM_RPM_DIR}.tar.bz2
tar -caf ${SLURM_RPM_DIR}.tar.bz2 $SLURM_RPM_DIR
dnf -q -y builddep -D '_with_slurmrestd 1' -D '_with_jwt 1' -D '_with_yaml 1' -D '_with_hwloc --with-hwloc' -D '_with_numa 1' ${SLURM_RPM_DIR}/slurm.spec
## FIXME: remove package installations after RPM spec makes them dependencies
dnf -q -y install --setopt='install_weak_deps=False' \
  kernel-headers dbus-devel
rpmbuild -ta --with slurmrestd --with jwt ${SLURM_RPM_DIR}.tar.bz2 >/dev/null
## Cleanup
dnf clean all
EOR

################################################################################
FROM ${PARENT_IMAGE} AS base

SHELL ["bash", "-c"]

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

COPY --from=build /root/rpmbuild/RPMS/**/*.rpm /tmp/

RUN <<EOR
# Install Dependencies
set -xeuo pipefail
# For: Slurm RPM Dependencies
dnf -q -y install --setopt='install_weak_deps=False' \
  'dnf-command(config-manager)' epel-release
dnf config-manager --enable crb
dnf -q -y upgrade --refresh
# Miscellaneous
dnf -q -y install --setopt='install_weak_deps=False' \
  ncurses
# Init System
dnf -q -y install --setopt='install_weak_deps=False' \
  supervisor tini
dnf clean all
EOR

################################################################################
# SLURM: slurmd
# BUILD: `docker build --target=slurmd -t [<registry>/]slurmd:<tag> .`
################################################################################
FROM base AS slurmd

SHELL ["bash", "-c"]

USER root
WORKDIR /tmp/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Slurm Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  gawk \
  ./slurm-devel-[0-9]*.rpm \
  ./slurm-libpmi-[0-9]*.rpm \
  ./slurm-pam_slurm-[0-9]*.rpm \
  ./slurm-slurmd-[0-9]*.rpm \
  ./slurm-[0-9]*.rpm
rm *.rpm && dnf clean all

apt-get update
apt-get  -qq -y install libpmix-dev

# Configure
mkdir -p /var/spool/slurmd/
cp -v /etc/nsswitch.conf{,.bak}
sed -i -E "s/^passwd:[[:space:]]+/&slurm /g" /etc/nsswitch.conf
sed -i -E "s/^group:[[:space:]]+/&slurm /g" /etc/nsswitch.conf

add-apt-repository ppa:git-core/ppa
apt update; apt install git

EOR

COPY files/slurmd/etc/supervisor/supervisord.conf /etc/supervisor/supervisord.conf
COPY files/slurmd/ /

RUN chmod +r+x /usr/local/bin/entrypoint.sh
RUN chmod +r+x / && chmod +r+x /usr/ && chmod +r+x /usr/bin/ &&  chmod +r+x /etc/ && chmod +r+x /usr/local/

EXPOSE 6818/tcp
ENTRYPOINT ["entrypoint.sh"]
