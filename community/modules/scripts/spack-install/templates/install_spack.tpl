#!/bin/bash

set -e -o pipefail

SPACK_PYTHON=${SPACK_PYTHON_VENV}/bin/python3

PREFIX="spack:"

echo "$PREFIX Beginning setup..."
if [[ $EUID -ne 0 ]]; then
  echo "$PREFIX This script must be run as root"
  exit 1
fi

# create an /etc/profile.d file that sources the Spack environment; it safely
# skips sourcing when Spack has not yet been installed
if [ ! -f /etc/profile.d/spack.sh ]; then
        cat <<EOF > /etc/profile.d/spack.sh
SPACK_PYTHON=${SPACK_PYTHON_VENV}/bin/python3
if [ -f ${INSTALL_DIR}/share/spack/setup-env.sh ]; then
        . ${INSTALL_DIR}/share/spack/setup-env.sh
fi
EOF
        chmod 0644 /etc/profile.d/spack.sh
fi

# Only install and configure spack if ${INSTALL_DIR} doesn't exist
if [ ! -d ${INSTALL_DIR} ]; then

  # Install spack
  echo "$PREFIX Installing spack from ${SPACK_URL}..."
  {
  mkdir -p ${INSTALL_DIR};
  chmod a+rwx ${INSTALL_DIR};
  chmod a+s ${INSTALL_DIR};
  cd ${INSTALL_DIR};
  git clone --no-checkout ${SPACK_URL} .
  } &>> ${LOG_FILE}
  echo "$PREFIX Checking out ${SPACK_REF}..."
  git checkout ${SPACK_REF} >> ${LOG_FILE} 2>&1

  {
  source ${INSTALL_DIR}/share/spack/setup-env.sh;
  spack compiler find --scope site
  } &>> ${LOG_FILE} 2>&1

  spack gpg init

else
  source ${INSTALL_DIR}/share/spack/setup-env.sh >> ${LOG_FILE} 2>&1
fi

echo "$PREFIX Setup complete..."
