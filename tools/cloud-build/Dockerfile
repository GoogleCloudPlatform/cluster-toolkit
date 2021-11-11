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

# Getting Terraform and Packer
FROM golang:bullseye
RUN curl -fsSL https://apt.releases.hashicorp.com/gpg | apt-key add -
RUN apt-get -y update && apt-get -y install software-properties-common
RUN apt-add-repository "deb [arch=$(dpkg --print-architecture)] https://apt.releases.hashicorp.com bullseye main"
RUN apt-get -y update && apt-get install -y unzip python3-pip terraform packer

RUN pip install pre-commit
RUN curl -s https://raw.githubusercontent.com/terraform-linters/tflint/master/install_linux.sh | bash

RUN go install github.com/terraform-docs/terraform-docs@v0.16.0
RUN go install golang.org/x/lint/golint@latest
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
RUN go install github.com/go-critic/go-critic/cmd/gocritic@latest
RUN go install github.com/google/addlicense@latest

# Setting GHPC dependencies
WORKDIR /ghpc-tmp
COPY ./ ./

RUN make ghpc vet

WORKDIR /ghpc




