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

# Getting Terraform and Packer
FROM golang:bullseye
RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add -  && \
    apt-get -y update && apt-get -y install \
    software-properties-common \
    keychain \
    dnsutils \
    shellcheck && \
    apt-add-repository "deb [arch=$(dpkg --print-architecture)] https://apt.releases.hashicorp.com bullseye main" && \
    apt-get -y update && apt-get install -y unzip python3-pip terraform packer jq && \
    echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main" \
      | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list && \
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg \
      | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - && \
    apt-get -y update && apt-get -y install google-cloud-sdk && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

RUN pip install pre-commit ansible && rm -rf ~/.cache/pip/*

RUN curl -s https://raw.githubusercontent.com/terraform-linters/tflint/master/install_linux.sh | bash

RUN go install github.com/terraform-docs/terraform-docs@latest      && \
    go install golang.org/x/lint/golint@latest                      && \
    go install github.com/fzipp/gocyclo/cmd/gocyclo@latest          && \
    go install github.com/go-critic/go-critic/cmd/gocritic@latest   && \
    go install github.com/google/addlicense@latest                  && \
    go install mvdan.cc/sh/v3/cmd/shfmt@latest                      && \
    go install golang.org/x/tools/cmd/goimports@latest

# Setting GHPC dependencies
WORKDIR /ghpc-tmp
COPY ./ ./

RUN make ghpc

WORKDIR /ghpc
