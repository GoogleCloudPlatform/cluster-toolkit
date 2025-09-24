# syntax=docker/dockerfile:1
# SPDX-FileCopyrightText: Copyright (C) SchedMD LLC.
# SPDX-License-Identifier: Apache-2.0

################################################################################

ARG SLURM_VERSION=master
ARG SLURM_DIR=slurm
ARG PARENT_IMAGE=rockylinux:9

################################################################################
FROM alpine:latest AS slurm-src

ARG SLURM_VERSION
ARG SLURM_DIR
ARG SLURM_TAR="${SLURM_VERSION}"

WORKDIR /workspace/

ADD https://github.com/SchedMD/slurm/archive/${SLURM_TAR}.tar.gz ${SLURM_VERSION}.tar.gz

RUN <<EOR
# Unpack Slurm Source
set -xeuo pipefail
apk add --no-cache tar
mkdir -p ${SLURM_DIR}
tar --strip-components=1 -zxvf ${SLURM_TAR}.tar.gz -C ${SLURM_DIR}
rm ${SLURM_TAR}.tar.gz
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
dnf -q -y builddep \
  -D '_with_slurmrestd 1' \
  -D '_with_jwt 1' \
  -D '_with_yaml 1' \
  -D '_with_hwloc --with-hwloc' \
  -D '_with_numa 1' \
  -D '_with_pmix --with-pmix' \
  -D '_with_ucx --with-ucx' \
  -D '_with_lua yes' \
  -D '_with_freeipmi 1' \
  -D '_with_hdf5 yes' \
  -D '_with_libcurl yes' \
  ${SLURM_RPM_DIR}/slurm.spec
## FIXME: remove package installations after RPM spec makes them dependencies: cgroups/v2
dnf -q -y install --setopt='install_weak_deps=False' \
  kernel-headers dbus-devel
## FIXME: remove package installations after RPM spec makes them dependencies: --with-hdf5
dnf -q -y install --setopt='install_weak_deps=False' \
  hdf5-devel
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
ARG SLURM_USER_UID=401
ARG SLURM_USER_GID=401

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
FROM base AS base-extra

SHELL ["bash", "-c"]

RUN <<EOR
# Install Extra Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  openmpi
dnf clean all
EOR

################################################################################
# SLURM: slurmctld
# BUILD: `docker build --target=slurmctld -t [<registry>/]slurmctld:<tag> .`
################################################################################
FROM base AS slurmctld

SHELL ["bash", "-c"]

USER root
WORKDIR /tmp/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Slurm Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  ./slurm-slurmctld-[0-9]*.rpm \
  ./slurm-[0-9]*.rpm
rm *.rpm && dnf clean all
EOR

COPY files/slurmctld/etc/supervisor/* /etc/supervisor/
COPY files/slurmctld/usr/local/bin/* /usr/local/bin/

EXPOSE 6817/tcp
ENTRYPOINT ["entrypoint.sh"]

################################################################################
# SLURM: slurmd
# BUILD: `docker build --target=slurmd -t [<registry>/]slurmd:<tag> .`
################################################################################
FROM base-extra AS slurmd

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
# Configure
mkdir -p /var/spool/slurmd/
cp -v /etc/nsswitch.conf{,.bak}
sed -i -E "s/^passwd:[[:space:]]+/&slurm /g" /etc/nsswitch.conf
sed -i -E "s/^group:[[:space:]]+/&slurm /g" /etc/nsswitch.conf
EOR

COPY files/slurmd/etc/supervisor/* /etc/supervisor/
COPY files/slurmd/usr/local/bin/* /usr/local/bin/

EXPOSE 6818/tcp
ENTRYPOINT ["entrypoint.sh"]

################################################################################
# SLURM: slurmdbd
# BUILD: `docker build --target=slurmdbd -t [<registry>/]slurmdbd:<tag> .`
################################################################################
FROM base AS slurmdbd

SHELL ["bash", "-c"]

USER root
WORKDIR /tmp/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Slurm Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  ./slurm-slurmdbd-[0-9]*.rpm \
  ./slurm-[0-9]*.rpm
rm *.rpm && dnf clean all
EOR

EXPOSE 6819/tcp
ENTRYPOINT ["tini", "-g", "--", "slurmdbd", "-D"]

################################################################################
# SLURM: slurmrestd
# BUILD: `docker build --target=slurmrestd -t [<registry>/]slurmrestd:<tag> .`
################################################################################
FROM base AS slurmrestd

SHELL ["bash", "-c"]

USER root
WORKDIR /tmp/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Slurm Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  ./slurm-slurmrestd-[0-9]*.rpm \
  ./slurm-[0-9]*.rpm
rm *.rpm && dnf clean all
EOR

EXPOSE 6820/tcp
ENTRYPOINT ["tini", "-g", "--", "slurmrestd"]
CMD ["0.0.0.0:6820"]

################################################################################
# SLURM: sackd
# BUILD: `docker build --target=sackd -t [<registry>/]sackd:<tag> .`
################################################################################
FROM base-extra AS sackd

SHELL ["bash", "-c"]

USER root
WORKDIR /tmp/

ADD https://dl.k8s.io/release/stable.txt kubernetes_stable.txt

# Ref: https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/#install-using-native-package-management
RUN <<EOR
# Install kubectl
set -xeuo pipefail
VERSION="$(cat kubernetes_stable.txt | grep -Eo '^v[0-9]+\.[0-9]+')"
cat <<EOF | tee /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/${VERSION}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/${VERSION}/rpm/repodata/repomd.xml.key
EOF
dnf -q -y install --setopt='install_weak_deps=False' kubectl
rm *.txt && dnf clean all
EOR

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Slurm Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  ./slurm-sackd-[0-9]*.rpm \
  ./slurm-[0-9]*.rpm
# For: Helm Chart
dnf -q -y install --setopt='install_weak_deps=False' \
  rsync gettext iputils
rm *.rpm && dnf clean all
# Prepare Directories
mkdir -p /run/slurm/
EOR

COPY files/sackd/etc/supervisor/* /etc/supervisor/
COPY files/sackd/usr/local/bin/* /usr/local/bin/

ENTRYPOINT ["entrypoint.sh"]

################################################################################
# NAME : login
# BUILD: `docker build --target=login -t [<registry>/]login:<tag> .`
################################################################################
FROM sackd AS login

SHELL ["bash", "-c"]

USER root
WORKDIR /tmp/

# Ref: https://slurm.schedmd.com/quickstart_admin.html#pkg_install
RUN <<EOR
# Install Packages
set -xeuo pipefail
dnf -q -y install --setopt='install_weak_deps=False' \
  openssh-server \
  dbus-daemon \
  oddjob-mkhomedir \
  authselect sssd sssd-ad sssd-ldap sssd-dbus
rm *.rpm && dnf clean all
# Configure
mkdir -p /etc/authselect
authselect select sssd with-mkhomedir --force
rm -f /etc/ssh/ssh_host_*
EOR

COPY files/login/etc/supervisor/* /etc/supervisor/
COPY files/login/usr/local/bin/* /usr/local/bin/

EXPOSE 22/tcp
ENTRYPOINT ["entrypoint.sh"]
