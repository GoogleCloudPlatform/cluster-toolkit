#!/bin/sh
# Copyright 2021 Google LLC
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

run_test(){
  set -e
  example=$1
  echo "testing $example"
  BLUEPRINT=$(uuidgen)
  tmpdir="$(mktemp -d)"
  echo "testing ${example} in ${tmpdir}"
  exampleFile=$(basename $example)
  cp ${example} "${tmpdir}/"
  cd "${tmpdir}"
  sed -i "s/blueprint_name: .*/blueprint_name: ${BLUEPRINT}/" ${exampleFile} || \
    { echo "could not set blueprint_name"; exit 1; }

  PROJECT=${PROJECT:-$(gcloud config get-value project 2>/dev/null)}
  if [ -z "$PROJECT" ]; then echo "PROJECT is not set."; exit 1;
  else echo PROJECT=$PROJECT
  fi

  sed -i "s/project_id: .*/project_id: ${PROJECT}/" ${exampleFile} || \
    { echo "could not set project_id"; exit 1; }
  cd ${cwd}
  ./ghpc create -c ${tmpdir}/${exampleFile} || { echo "error creating blueprint"; exit 1; }
  mv ${BLUEPRINT} ${tmpdir}
  cd ${tmpdir}/${BLUEPRINT} || { echo "can't cd into the blueprint folder"; exit 1; }
  for folder in `ls`;
  do
    cd $folder
    echo "testing the group ${folder}..."
    if [ -f 'main.tf' ];
    then
      tfpw=$(pwd)
      terraform init -no-color -backend=false > "${exampleFile}.init"|| \
        { echo "terraform init failed for ${example}, logs in ${tfpw}"; exit 1; }
      terraform validate -no-color > "${exampleFile}.plan" || \
        { echo "terraform validate failed for ${example}, logs in ${tfpw}"; exit 1; }
    else
      echo "terraform not found in folder ${folder}. Skipping."
    fi
    cd .. # back to blueprint folder
  done
  cd ..
  rm -rf ${BLUEPRINT} || { echo "could not remove blueprint folder from $(pwd)"; exit 1; }
  cd ${cwd}
  rm -r ${tmpdir}
}

check_background(){
  if ! wait -n; then
    echo "a test failed. Exiting with status 1."
    wait
    exit 1
  fi
}

CONFIGS=$(find tools/test_examples/test_configs examples/ -name "*.yaml" -type f)
cwd=$(pwd)
NPROCS=${NPROCS:-$(nproc)}
echo "Running tests in $NPROCS processes"
for example in $CONFIGS
do
  JNUM=$(jobs | wc -l)
  echo "$JNUM jobs running"
  if [ $JNUM -lt $NPROCS ]
  then
    run_test $example &
  else
    echo "Reached max number of parallel tests. Waiting for one to finish."
    check_background
  fi
done
JNUM=$(jobs | wc -l)
while [ $JNUM -gt 0 ]; do
  check_background
  JNUM=$(jobs | wc -l)
done
