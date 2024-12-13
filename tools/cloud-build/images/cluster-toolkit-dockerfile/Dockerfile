# Copyright 2024 Google LLC
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


# Use a google-cloud-cli image as the base
ARG BASE_IMAGE=gcr.io/google.com/cloudsdktool/google-cloud-cli:stable
FROM ${BASE_IMAGE}

# Install necessary tools and libraries
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        apt-transport-https \
        ca-certificates \
        curl \
        gnupg \
        lsb-release \
        git \
        make \
        unzip \
        wget \
        python3-pip

# Install Terraform
ARG TERRAFORM_VERSION=1.5.2
RUN wget -q "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip" -O terraform.zip && \
    unzip terraform.zip && \
    mv terraform /usr/local/bin/ && \
    rm terraform.zip

# Install Packer
ARG PACKER_VERSION=1.8.6
RUN wget -q "https://releases.hashicorp.com/packer/${PACKER_VERSION}/packer_${PACKER_VERSION}_linux_amd64.zip" -O packer.zip && \
    unzip packer.zip && \
    mv packer /usr/local/bin/ && \
    rm packer.zip

# Install Go
ARG GO_VERSION=1.23.0
RUN wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O go.tar.gz && \
    tar -C /usr/local -xzf go.tar.gz && \
    rm go.tar.gz

# Set GOPATH and add to PATH
ENV GOPATH /go
ENV PATH $PATH:/usr/local/go/bin:$GOPATH/bin

# Clone the Cluster Toolkit repository
ARG CLUSTER_TOOLKIT_REF=main
RUN git clone --branch ${CLUSTER_TOOLKIT_REF} https://github.com/GoogleCloudPlatform/cluster-toolkit.git /cluster-toolkit

# Build the gcluster binary
WORKDIR /cluster-toolkit
RUN make

# Make gcluster available
RUN cp /cluster-toolkit/gcluster /usr/local/bin/gcluster
RUN echo $PATH

# Create /out directory
RUN mkdir /out
WORKDIR /out

RUN echo "[global]" >> /etc/pip.conf
RUN echo "break-system-packages = true" >> /etc/pip.conf

# Command to execute when running the container (placeholder)
ENTRYPOINT ["gcluster"]
CMD ["--help"]
