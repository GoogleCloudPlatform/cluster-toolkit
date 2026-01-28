# Copyright 2026 Google LLC
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
ARG SHELLCHECK_VER=v0.9.0

# --- Install Base Utilities and Tools ---
RUN apt-get -y update && apt-get -y install \
    software-properties-common \
    keychain \
    dnsutils \
    curl \
    gnupg \
    lsb-release \
    unzip \
    jq \
    tmux && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# --- Install ShellCheck ---
RUN wget https://github.com/koalaman/shellcheck/releases/download/${SHELLCHECK_VER}/shellcheck-${SHELLCHECK_VER}.linux.x86_64.tar.xz && \
    tar xvf shellcheck-${SHELLCHECK_VER}.linux.x86_64.tar.xz && \
    mv shellcheck-${SHELLCHECK_VER}/shellcheck /usr/local/bin/shellcheck && \
    rm -rf shellcheck-*

# --- Install TFLint ---
RUN curl -s https://raw.githubusercontent.com/terraform-linters/tflint/master/install_linux.sh | bash

# --- Add HashiCorp Repo and Install Terraform/Packer ---
RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | tee /etc/apt/sources.list.d/hashicorp.list && \
    apt-get -y update && apt-get install -y \
    terraform \
    packer && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# --- Install Python and Set as Default ---
RUN add-apt-repository ppa:deadsnakes/ppa && \
    apt-get -y update && \
    apt-get -y install \
        python3.12 \
        python3.12-venv \
        python3.12-dev \
        python3-pip && \
    update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.12 1 && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# --- Create Virtual Environment ---
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# --- Copy configuration files ---
ARG CW_DIR=tools/cloud-workstations
ARG HPC_WORK_FILE=200_configure-hpc-toolkit.sh
ARG HPC_CONF_FILE=configure-hpc-toolkit.sh
ARG CODE_REQS=code_oss_requirements.txt
ARG HPC_CONFIG_DIR=/etc/hpc-toolkit-config
ARG START_DIR=/etc/workstation-startup.d
ARG INSTALL_DIR=/usr/local

COPY $CW_DIR/$HPC_CONF_FILE $INSTALL_DIR/bin/$HPC_CONF_FILE
RUN chmod a+rx $INSTALL_DIR/bin/$HPC_CONF_FILE

COPY $CW_DIR/$HPC_WORK_FILE $START_DIR/$HPC_WORK_FILE
RUN chmod a+rx $START_DIR/$HPC_WORK_FILE

COPY $CW_DIR/$CODE_REQS $HPC_CONFIG_DIR/$CODE_REQS
RUN chmod a+r $HPC_CONFIG_DIR/$CODE_REQS

COPY tools/cloud-build/requirements.txt cloud_build_requirements.txt
COPY docs/hybrid-slurm-cluster/requirements.txt slurm_requirements.txt

# --- Install Python Dependencies ---
RUN pip install --no-cache-dir --upgrade pip setuptools wheel && \
    pip install --no-cache-dir --prefer-binary --no-build-isolation -r https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/refs/heads/master/scripts/requirements.txt && \
    pip install --no-cache-dir --prefer-binary --no-build-isolation -r cloud_build_requirements.txt && \
    pip install --no-cache-dir --prefer-binary --no-build-isolation -r slurm_requirements.txt && \

    rm -rf ~/.cache/pip/*
