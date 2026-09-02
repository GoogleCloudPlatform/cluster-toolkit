#!/bin/bash

# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
set -o pipefail
set -x

export sym_installer=$1
export sym_fixpack=$2
export sym_entitlement=$3
export symphony_install_dir=$4
export sym_source_bucket=$5

echo "=== Starting IBM Spectrum Symphony Installation ==="
echo "Installer:        ${sym_installer}"
echo "Fixpack:          ${sym_fixpack}"
echo "Entitlement:      ${sym_entitlement}"
echo "Install Dir:      ${symphony_install_dir}"
echo "Source Bucket:    ${sym_source_bucket}"

echo "=== 1/7: Rebuilding RPM database and cleaning cache ==="
# 1. Remove the stale Berkeley DB (BDB) environment files
rm -f /var/lib/rpm/__db*

# 2. Rebuild the RPM database index
rpm --rebuilddb

# 3. Clean package manager caches to ensure a fresh state
yum clean all

echo "=== 2/7: Installing required dependencies and packages ==="
yum install -y \
	bc \
	gettext \
	bind-utils \
	net-tools \
	libnsl \
	openssl \
	ed \
	dejavu-serif-fonts \
	findutils \
	sudo \
	vim \
	zip \
	diffutils \
	iproute \
	procps \
	jq \
	glibc-locale-source \
	glibc-langpack-en &&
	(alternatives --set python /usr/bin/python3 2>/dev/null || ln -sf /usr/bin/python3 /usr/bin/python || true) &&
	yum clean all && rm -rf /var/cache/yum

echo "=== 3/7: Adding and configuring egoadmin user and limits ==="
useradd -G wheel -m egoadmin && echo egoadmin:Admin | chpasswd && echo "egoadmin ALL=(ALL) NOPASSWD: ALL" >>/etc/sudoers.d/symphony-cluster-admins
touch /var/run/utmp && chmod 664 /var/run/utmp && chown root:utmp /var/run/utmp
echo LC_ALL=en_US.UTF-8 >/etc/locale.conf &&
	LC_ALL=en_US.UTF-8 localedef -v -c -i en_US -f UTF-8 en_US.UTF-8 | true
echo "egoadmin soft nproc  65536" >>/etc/security/limits.conf &&
	echo "egoadmin hard nproc  65536" >>/etc/security/limits.conf &&
	echo "egoadmin soft nofile 65536" >>/etc/security/limits.conf &&
	echo "egoadmin hard nofile 65536" >>/etc/security/limits.conf

echo "=== 4/7: Downloading installation packages from gs://${sym_source_bucket} ==="
gcloud storage cp gs://${sym_source_bucket}/${sym_installer} /tmp
gcloud storage cp gs://${sym_source_bucket}/${sym_entitlement} /tmp
gcloud storage cp gs://${sym_source_bucket}/${sym_fixpack} /tmp

echo "=== 5/7: Running IBM Spectrum Symphony installer ==="
cd /tmp
mkdir -p ${symphony_install_dir}
export ENV EGO_TOP=${symphony_install_dir} CLUSTERADMIN=egoadmin IBM_SPECTRUM_SYMPHONY_LICENSE_ACCEPT=Y SIMPLIFIEDWEM=N DISABLESSL=Y
mv /tmp/${sym_entitlement} ${EGO_TOP}
chmod a+x ${sym_installer}
./${sym_installer} --prefix ${EGO_TOP} --quiet

echo "=== 6/7: Installing Symphony fixpack ==="
export SYM_FIXPACK_TAR=${sym_fixpack}
export SYM_FIXPACK_PATH=/tmp/${SYM_FIXPACK_TAR}
export SYM_FIXPACK_NAME=$(basename $SYM_FIXPACK_TAR .tar.gz)
export SYM_FIXPACK_DIR=/opt/ibm/${SYM_FIXPACK_NAME}

mkdir -p ${SYM_FIXPACK_DIR}
chmod 770 ${SYM_FIXPACK_DIR}
tar -xf ${SYM_FIXPACK_PATH} -C ${SYM_FIXPACK_DIR}
chmod u+r ${SYM_FIXPACK_DIR}/*
chmod a+x ${SYM_FIXPACK_DIR}/*.sh

cd ${SYM_FIXPACK_DIR}

echo "Applying fixpack for egoadmin..."
su -s /bin/bash $CLUSTERADMIN -c "source $EGO_TOP/profile.platform && ${SYM_FIXPACK_DIR}/sym-7.3.2.sh -c -i"

echo "Applying fixpack RPM updates..."
source $EGO_TOP/profile.platform
${SYM_FIXPACK_DIR}/symrpm-7.3.2.sh -c -i

cd

echo "Cleaning up temporary fixpack files..."
rm -rf $EGO_TOP/patch/backup/*
rm -rf ${SYM_FIXPACK_DIR}
rm -f ${SYM_FIXPACK_PATH}

echo "=== 7/7: Updating Symphony configuration ==="
cd ${EGO_TOP}

sed -i "s|BINARY_TYPE=\"fail\"|BINARY_TYPE=\"linux-x86_64\"|g" kernel/conf/profile.ego &&
	sed -i "s|BINARY_TYPE=\"fail\"|BINARY_TYPE=\"linux-x86_64\"|g" jre/profile.jre &&
	sed -i "s|BINARY_TYPE=\"fail\"|BINARY_TYPE=\"linux-x86_64\"|g" soam/conf/profile.soam &&
	sed -i -e "s|AUTOMATIC|MANUAL|g" eservice/esc/conf/services/plc_service.xml &&
	sed -i -e "s|AUTOMATIC|MANUAL|g" eservice/esc/conf/services/purger_service.xml &&
	sed -i -e "s|AUTOMATIC|MANUAL|g" eservice/esc/conf/services/wsg.xml &&
	sed -i -e "s|AUTOMATIC|MANUAL|g" eservice/esc/conf/services/rsa.xml &&
	sed -i -e "s|AUTOMATIC|MANUAL|g" eservice/esc/conf/services/execproxy.xml &&
	sed -i -e "s|MANUAL|AUTOMATIC|g" eservice/esc/conf/services/symrest.xml

cat <<'EOF' >>kernel/conf/ego.conf
EGO_DYNAMIC_HOST_TIMEOUT=10m
EGO_DYNAMIC_HOST_WAIT_TIME=1
EGO_DISABLE_ROOT_REX=Y
EGO_ELIM_RUNAS_CLUSTER_ADMIN=Y
EGO_LIM_IS_IN_CONTAINER=Y
EGO_GET_CONF=LIM
EOF

echo "=== IBM Spectrum Symphony Installation Completed Successfully ==="
