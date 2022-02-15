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
      DEPS="$DEPS python3-pip"
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
  pip3 install google-cloud-storage > ${LOG_FILE} 2>&1

  # Install spack
  echo "$PREFIX Installing spack from ${SPACK_URL}..."
  {
  mkdir -p ${INSTALL_DIR};
  chmod a+rwx ${INSTALL_DIR};
  chmod a+s ${INSTALL_DIR};
  cd ${INSTALL_DIR};
  git clone ${SPACK_URL} .
  } &>> ${LOG_FILE}
  echo "$PREFIX Checking out ${SPACK_REF}..."
  git checkout ${SPACK_REF} >> ${LOG_FILE} 2>&1

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

  {
  chmod a+r ${INSTALL_DIR}/etc/spack/modules.yaml;
  source ${INSTALL_DIR}/share/spack/setup-env.sh;
  spack compiler find --scope site
  } &>> ${LOG_FILE} 2>&1

  echo "$PREFIX Setting up spack mirrors..."
  %{for m in MIRRORS ~}
  spack mirror add --scope site ${m.mirror_name} ${m.mirror_url} >> ${LOG_FILE} 2>&1
  %{endfor ~}

  spack buildcache keys --install --trust >> ${LOG_FILE} 2>&1
else
  source ${INSTALL_DIR}/share/spack/setup-env.sh >> ${LOG_FILE} 2>&1
fi

echo "$PREFIX Installing licenses..."
%{for lic in LICENSES ~}
gsutil cp ${lic.source} ${lic.dest} >> ${LOG_FILE} 2>&1
%{endfor ~}

echo "$PREFIX Installing compilers..."
%{for c in COMPILERS ~}
{
spack install ${c};
spack load ${c};
} &>> ${LOG_FILE}
%{endfor ~}

spack compiler find --scope site >> ${LOG_FILE} 2>&1

echo "$PREFIX Installing root spack specs..."
%{for p in PACKAGES ~}
spack install ${p} >> ${LOG_FILE} 2>&1
%{endfor ~}

echo "$PREFIX Configuring spack environments"
%{for e in ENVIRONMENTS ~}

{
spack env create ${e.name};
spack env activate ${e.name};
} &>> ${LOG_FILE}

echo "$PREFIX    Configuring spack environment ${e.name}"
%{for p in e.packages ~}
spack add ${p} >> ${LOG_FILE} 2>&1
%{endfor ~}

echo "$PREFIX    Concretizing spack environment ${e.name}"
spack concretize >> ${LOG_FILE} 2>&1
echo "$PREFIX    Installing packages for spack environment ${e.name}"
spack install >> ${LOG_FILE} 2>&1

spack env deactivate >> ${LOG_FILE} 2>&1

%{endfor ~}

echo "$PREFIX Setup complete..."
exit 0
