#!/bin/bash
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

# An artifact registry repo is needed before running this:
# https://cloud.google.com/artifact-registry/docs
# gcloud artifacts repositories create hpc-toolkit-repo --repository-format=docker --location=us-central1 --description="Docker repository for HPC Toolkit"
# gcloud artifacts repositories list

tmpdir="$(mktemp -d)"
echo "created temporary build directory at ${tmpdir}"
cp -R tools/test_examples/* ${tmpdir}
cp -R examples ${tmpdir}
cp -R resources ${tmpdir}
cp -R ghpc ${tmpdir}
cd ${tmpdir}
gcloud builds submit --config hpc-toolkit.yaml
cd -
if [ -d ${tmpdir} ]; then
  echo "removing ${tmpdir}"
  rm -rf ${tmpdir}
fi
