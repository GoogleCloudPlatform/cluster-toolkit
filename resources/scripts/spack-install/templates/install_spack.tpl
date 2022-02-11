#!/bin/bash
set -e

PREFIX="spack:"

echo "$PREFIX Beginning setup..."
if [[ $EUID -ne 0 ]]; then
  echo "$PREFIX This script must be run as root"
  exit 1
fi

# Only install and configure spack if ${INSTALL_DIR} doesn't exist
if [ ! -d ${INSTALL_DIR} ]; then

  DEPS=""
  if [ ! "$(which pip3)" ]; then
      DEPS="$DEPS pip3"
  fi

  if [ ! "$(which git)" ]; then
     DEPS="$DEPS git"
  fi

  if [ -n "$DEPS" ]; then
    echo "$PREFIX Installing dependencies"
    if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] || [ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
      yum -y install $DEPS
    elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release || grep -qi ubuntu /etc/os-release; then
      echo "$PREFIX WARNING: unsupported installation in debian / ubuntu"
      apt install -y $DEPS
    else
      echo "$PREFIX Unsupported distribution"
      exit 1
    fi
  fi

  # Install google-cloud-storage
  echo "$PREFIX Installing Google Cloud Storage via pip3..."
  pip3 install google-cloud-storage &> /dev/null

  # Install spack
  echo "$PREFIX Installing spack from ${SPACK_URL}..."
  mkdir -p ${INSTALL_DIR} &> /dev/null
  chmod a+rwx ${INSTALL_DIR} &> /dev/null
  chmod a+s ${INSTALL_DIR} &> /dev/null
  cd ${INSTALL_DIR} &> /dev/null
  git clone ${SPACK_URL} . &> /dev/null
  echo "$PREFIX Checking out ${SPACK_REF}..."
  git checkout ${SPACK_REF} &> /dev/null

  # Configure module names
  cat <<EOF >> ${INSTALL_DIR}/etc/spack/modules.yaml
modules:
  tcl:
    hash_length: 0
    whitelist:
      -  gcc
    blacklist:
      -  '%gcc@7.5.0'
    all:
      conflict:
        - '{name}'
      filter:
        environment_blacklist:
          - "C_INCLUDE_PATH"
          - "CPLUS_INCLUDE_PATH"
          - "LIBRARY_PATH"
    projections:
      all:               '{name}/{version}-{compiler.name}-{compiler.version}'
EOF

  chmod a+r ${INSTALL_DIR}/etc/spack/modules.yaml &> /dev/null

  source ${INSTALL_DIR}/share/spack/setup-env.sh &> /dev/null
  spack compiler find --scope site &> /dev/null

  echo "$PREFIX Setting up spack mirrors..."
  %{for m in MIRRORS ~}
  spack mirror add --scope site ${m.mirror_name} ${m.mirror_url} &> /dev/null
  %{endfor ~}

  spack buildcache keys --install --trust &> /dev/null
else
  source ${INSTALL_DIR}/share/spack/setup-env.sh &> /dev/null
fi

echo "$PREFIX Installing licenses..."
%{for lic in LICENSES ~}
gsutil cp ${lic.source} ${lic.dest} &> /dev/null
%{endfor ~}


echo "$PREFIX Installing compilers..."
%{for c in COMPILERS ~}
spack install ${c} &> /dev/null
spack load ${c} &> /dev/null
%{endfor ~}

spack compiler find --scope site &> /dev/null

echo "$PREFIX Installing software stack..."
%{for p in PACKAGES ~}
spack install ${p} &> /dev/null
%{endfor ~}

echo "$PREFIX Setup complete..."
exit 0
