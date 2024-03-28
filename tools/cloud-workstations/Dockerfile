# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Getting Terraform and Packer
FROM us-central1-docker.pkg.dev/cloud-workstations-images/predefined/code-oss:latest

ARG TFLINT_VERSION
# Need to get shellcheck directly as the on the repo is outdated and causes failures
ARG SHELLCHECK_VER=v0.9.0

RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add -  && \
    apt-get -y update && apt-get -y install \
    software-properties-common \
    keychain \
    dnsutils && \
    apt-add-repository "deb [arch=$(dpkg --print-architecture)] https://apt.releases.hashicorp.com bullseye main" && \
    apt-get -y update && apt-get install -y unzip python3-pip python3-venv terraform packer jq && \
    echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" \
      | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list && \
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg \
      | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - && \
    apt-get -y update && apt-get -y install google-cloud-sdk && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

RUN wget https://github.com/koalaman/shellcheck/releases/download/v0.9.0/shellcheck-$SHELLCHECK_VER.linux.x86_64.tar.xz && \
    tar xvf shellcheck-$SHELLCHECK_VER.linux.x86_64.tar.xz && \
    mv shellcheck-$SHELLCHECK_VER/shellcheck /bin/shellcheck

RUN curl -s https://raw.githubusercontent.com/terraform-linters/tflint/master/install_linux.sh | bash

COPY tools/cloud-build/requirements.txt cloud_build_requirements.txt
COPY docs/hybrid-slurm-cluster/requirements.txt slurm_requirements.txt

RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r https://raw.githubusercontent.com/SchedMD/slurm-gcp/master/scripts/requirements.txt && \
    pip install --no-cache-dir -r cloud_build_requirements.txt && \
    pip install --no-cache-dir -r slurm_requirements.txt && \
    rm -rf ~/.cache/pip/*

# Get the HPC config files and store them in the correct locations for startup
ARG CW_DIR=tools/cloud-workstations
ARG HPC_WORK_FILE=200_configure-hpc-toolkit.sh
ARG HPC_CONF_FILE=configure-hpc-toolkit.sh
ARG CODE_REQS=code_oss_requirements.txt
ARG HPC_CONFIG_DIR=/etc/hpc-toolkit-config
ARG START_DIR=/etc/workstation-startup.d
ARG INSTALL_DIR=/usr/local

COPY $CW_DIR/$HPC_CONF_FILE $INSTALL_DIR/bin
RUN chmod a+rx $INSTALL_DIR/bin/$HPC_CONF_FILE

COPY $CW_DIR/$HPC_WORK_FILE $START_DIR
RUN chmod a+rx $START_DIR/$HPC_WORK_FILE

RUN mkdir $HPC_CONFIG_DIR
COPY $CW_DIR/$CODE_REQS $HPC_CONFIG_DIR
RUN chmod a+r $HPC_CONFIG_DIR/$CODE_REQS
