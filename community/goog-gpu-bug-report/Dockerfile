# Copyright 2026 "Google LLC"
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

# Using Debian 12 as the base image
FROM marketplace.gcr.io/google/debian12

# Set the working directory in the container
WORKDIR /app

# Copy the local directory to the container
COPY app/* /app/

# The host's NVIDIA driver directories come after the container's own entries.
# Depending on the environment, the right directory is mounted into the
# container.
ENV PATH=$PATH:/var/lib/nvidia/bin:/home/kubernetes/bin/nvidia/bin:/host_usr/bin
ENV LD_LIBRARY_PATH=/usr/lib/aarch64-linux-gnu:/usr/lib/x86_64-linux-gnu:/var/lib/nvidia/lib64:/home/kubernetes/bin/nvidia/lib64:/host_usr/lib64:/host_usr/lib/aarch64-linux-gnu:/host_usr/lib/x86_64-linux-gnu:/nvidia_alias

# Install system dependencies
RUN apt-get update && apt-get upgrade -y && \
    apt-get install -y --no-install-recommends util-linux pciutils \
    build-essential zlib1g-dev libncurses5-dev libgdbm-dev libnss3-dev \
    libssl-dev libreadline-dev libffi-dev libsqlite3-dev wget ca-certificates libbz2-dev \
    acpica-tools kmod usbutils procps nano \
    mesa-utils dmidecode vulkan-tools ibutils x11-xserver-utils

# Install packages for diagnostic tools (iostat, ibstat, ibv_devinfo,
# journalctl). ibverbs-providers has the RDMA device drivers that ibv_devinfo
# needs, and systemd has journalctl, which reads the host's journal mounted at
# /var/log. libibverbs1 and sysstat only recommend them, so
# --no-install-recommends leaves them out unless they are listed here.
RUN apt-get install -y --no-install-recommends sysstat infiniband-diags \
    ibverbs-utils ibverbs-providers systemd

# MFT's installer needs dkms to build mst_pci/mst_pciconf on non-COS hosts.
RUN apt-get install -y --no-install-recommends dkms linux-kbuild-6.1

# libheif has no Debian 12 fix for CVE-2026-84384/84444/84446/84447 (fixed only
# upstream in 1.23.2) and nothing in this image needs it. It only gets in through
# the libc-dev-bin -> libc-devtools -> libgd3 -> libheif1 Recommends chain, so
# purge that chain (and the codecs it drags in) and fail the build if any of it
# is still present.
RUN apt-get purge -y libc-devtools libgd3 libheif1 && \
    apt-get autoremove --purge -y && \
    apt-get clean && \
    for pkg in libheif1 libgd3 libc-devtools; do \
      if [ "$(dpkg-query -W -f='${db:Status-Status}' "$pkg" 2>/dev/null)" = installed ]; then \
        echo "ERROR: $pkg is still installed" >&2; exit 1; \
      fi; \
    done

RUN wget https://www.python.org/ftp/python/3.13.12/Python-3.13.12.tar.xz && \
    tar -xvf Python-3.13.12.tar.xz && \
    cd Python-3.13.12 && \
    ./configure --enable-optimizations && \
    make -j $(nproc) && make altinstall && \
    cd .. && \
    rm -rf Python-3.13.12 Python-3.13.12.tar.xz && \
    rm -rf /usr/local/lib/python3.13/test

RUN ln -s /usr/local/bin/pip3.13 /usr/local/bin/pip3 && \
    ln -s /usr/local/bin/python3.13 /usr/local/bin/python3

# pip < 26.2 is affected by CVE-2026-3219, CVE-2026-6357, CVE-2026-8643 and
# CVE-2026-13346; pin the floor so a rebuild cannot silently reintroduce them.
RUN python3 -m pip install --upgrade 'pip>=26.2' setuptools wheel

# Create a virtual environment.
RUN python3 -m venv .venv

RUN .venv/bin/pip install --upgrade 'pip>=26.2' setuptools wheel

# Install Python dependencies
RUN .venv/bin/pip install --no-cache-dir --require-hashes -r requirements.txt

# Remove dormant ensurepip bundled wheels to remediate CVEs
RUN rm -rf /usr/local/lib/python3.13/ensurepip

ENTRYPOINT [".venv/bin/python", "goog-gpu-bug-report.py"]
