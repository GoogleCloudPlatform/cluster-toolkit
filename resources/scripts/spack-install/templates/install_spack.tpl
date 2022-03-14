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

  {
  source ${INSTALL_DIR}/share/spack/setup-env.sh;
  spack compiler find --scope site
  } &>> ${LOG_FILE} 2>&1

  echo "$PREFIX Configuring spack..."
  %{for c in CONFIGS ~}
    %{if c.type == "singleton" ~}
      spack config --scope=${c.scope} add "${c.value}" >> ${LOG_FILE} 2>&1
    %{endif ~}

    %{if c.type == "file" ~}
      {
      cat <<EOF > ${INSTALL_DIR}/spack_conf.yaml
${c.value}
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
