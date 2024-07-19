# Copyright 2022 Google LLC
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

FROM golang:bullseye

ENV GOCACHE=/ghpc_go_cache

# copy the source code, to build the binary
# use `/workspace` to match path in cache for future builds
COPY ./ /workspace

RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add -  && \
    apt-get -y update && apt-get -y install \
    # `software-properties-common` providers `add-apt-repository`
    software-properties-common \
    keychain \
    # `dnsutils` provides `dig` used by integration tests
    dnsutils && \
    # install terraform and packer
    apt-add-repository "deb [arch=$(dpkg --print-architecture)] https://apt.releases.hashicorp.com bullseye main" && \
    apt-get -y update && apt-get install -y unzip python3-pip python3-netaddr terraform packer jq && \
    # install gcloud
    echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" \
      | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list && \
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg \
      | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - && \
    apt-get -y update && apt-get -y install google-cloud-sdk && \
    apt-get -y install kubectl && \
    # following is required to execute kubectl commands
    apt-get -y install google-cloud-cli-gke-gcloud-auth-plugin && \ 
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    # install ansible and python dependencies
    pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/v5/scripts/requirements.txt && \
    pip install --no-cache-dir ansible && \
    rm -rf ~/.cache/pip/* && \
    # compile the binary to warm up `/ghpc_go_cache`
    cd /workspace && make ghpc && \
    # remove /workspace to reduce image size
    rm -rf /workspace
