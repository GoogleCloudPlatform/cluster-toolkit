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

  echo "$PREFIX Configuring spack..."
  %{for c in CONFIGS ~}
    %{if c.type == "single-config" ~}
      spack config --scope=${c.scope} add "${c.content}" >> ${LOG_FILE} 2>&1
    %{endif ~}

    %{if c.type == "file" ~}
      {
      cat << 'EOF' > ${INSTALL_DIR}/spack_conf.yaml
${c.content}
EOF

      spack config --scope=${c.scope} add -f ${INSTALL_DIR}/spack_conf.yaml
      rm -f ${INSTALL_DIR}/spack_conf.yaml
      } &>> ${LOG_FILE} 2>&1
    %{endif ~}
  %{endfor ~}

  echo "$PREFIX Setting up spack mirrors..."
  %{for m in MIRRORS ~}
  spack mirror add --scope site ${m.mirror_name} ${m.mirror_url} >> ${LOG_FILE} 2>&1
  %{endfor ~}

  echo "$PREFIX Installing GPG keys"
  spack gpg init >> ${LOG_FILE} 2>&1
  %{for k in GPG_KEYS ~}
    %{if k.type == "file" ~}
      spack gpg trust ${k.path}
    %{endif ~}

    %{if k.type == "new" ~}
      spack gpg create "${k.name}" ${k.email}
    %{endif ~}
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
    spack install ${INSTALL_FLAGS} ${c};
    spack load ${c};
    spack clean -s
  } &>> ${LOG_FILE}
%{endfor ~}

spack compiler find --scope site >> ${LOG_FILE} 2>&1

echo "$PREFIX Installing root spack specs..."
%{for p in PACKAGES ~}
  spack install ${INSTALL_FLAGS} ${p} >> ${LOG_FILE} 2>&1
  spack clean -s
%{endfor ~}

echo "$PREFIX Setup complete..."
