# Copyright 2026 Google LLC
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

FROM golang:1.26.1-trixie

ENV GOCACHE=/ghpc_go_cache

# copy the source code, to build the binary
# use `/workspace` to match path in cache for future builds
COPY ./ /workspace

RUN apt-get -y update && apt-get -y install \
    gnupg \
    keychain \
    # `dnsutils` provides `dig` used by integration tests
    dnsutils && \
    # install terraform and packer
    curl -fsSL https://apt.releases.hashicorp.com/gpg | gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://apt.releases.hashicorp.com trixie main" | tee /etc/apt/sources.list.d/hashicorp.list && \
    apt-get -y update && \
    # Pin Terraform version to 1.12.2
    apt-get install -y unzip python3-pip python3-venv terraform=1.12.2-1 packer jq && \
    # install gcloud
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" \
      | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list && \
    apt-get -y update && apt-get -y install google-cloud-sdk && \
    apt-get -y install kubectl && \
    # following is required to execute kubectl commands
    apt-get -y install google-cloud-cli-gke-gcloud-auth-plugin && \
    apt-get clean && rm -rf /var/lib/apt/lists/* && \
    # create virtual environment
    python3 -m venv /opt/venv && \
    # install ansible and python dependencies
    /opt/venv/bin/pip install --no-cache-dir --upgrade pip && \
    /opt/venv/bin/pip install --no-cache-dir -r https://raw.githubusercontent.com/GoogleCloudPlatform/slurm-gcp/master/scripts/requirements.txt && \
    /opt/venv/bin/pip install --no-cache-dir ansible && \
    /opt/venv/bin/pip install --no-cache-dir paramiko && \
    /opt/venv/bin/pip install --no-cache-dir netaddr && \
    rm -rf ~/.cache/pip/* && \
    # compile the binary to warm up `/ghpc_go_cache`
    cd /workspace && make gcluster && \
    # remove /workspace to reduce image size
    rm -rf /workspace

ENV PATH="/opt/venv/bin:$PATH"
