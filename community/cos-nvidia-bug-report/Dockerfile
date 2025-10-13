# Copyright 2025 "Google LLC"
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

# Using Debian 12.10 as the base image
FROM debian:12.10

# Set the working directory in the container
WORKDIR /app

# Copy the local directory to the container
COPY app/* /app/

ENV PATH=$PATH:/var/lib/nvidia/bin:/var/lib/nvidia/lib64:/usr/local/nvidia/bin:/usr/local/nvidia/lib64/
ENV LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/var/lib/nvidia/bin:/var/lib/nvidia/lib64:/usr/local/nvidia/bin:/usr/local/nvidia/lib64


# Install system dependencies
RUN apt-get update && \
    apt-get install -y util-linux pciutils \
    build-essential zlib1g-dev libncurses5-dev libgdbm-dev libnss3-dev \
    libssl-dev libreadline-dev libffi-dev libsqlite3-dev wget libbz2-dev \
    acpica-tools kmod usbutils procps nano python3-pip python3.11-venv

RUN apt-get install -y mesa-utils dmidecode vulkan-tools ibutils x11-xserver-utils
RUN wget https://www.python.org/ftp/python/3.13.1/Python-3.13.1.tgz

RUN tar -xvf Python-3.13.1.tgz

RUN cd Python-3.13.1 && ./configure --enable-optimizations && \
    make -j $(nproc) && make altinstall


RUN ln -s /usr/local/bin/pip3.13 /usr/local/bin/pip3 && \
    ln -s /usr/local/bin/python3.13 /usr/local/bin/python3

# Create and activate a virtual environment.
RUN python3 -m venv .venv

RUN . .venv/bin/activate

# Install Python dependencies
RUN .venv/bin/pip install --no-cache-dir -r requirements.txt

ENTRYPOINT [".venv/bin/python", "cos-nvidia-bug-report.py"]
