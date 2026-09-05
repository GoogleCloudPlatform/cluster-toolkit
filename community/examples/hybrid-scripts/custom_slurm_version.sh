#!/bin/bash
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

set -e

SLURM_VERSION="$1" # set your version here
if [ -z "$SLURM_VERSION" ]; then
	echo "slurm_version not set; skipping custom Slurm build"
	exit 0
fi

SLURM_SRC_DIR="/usr/local/src"
cd "$SLURM_SRC_DIR"

curl -o slurm-"${SLURM_VERSION}".tar.bz2 https://download.schedmd.com/slurm/slurm-"${SLURM_VERSION}".tar.bz2
tar -xjf slurm-"${SLURM_VERSION}".tar.bz2
ln -sfn slurm-"${SLURM_VERSION}" slurm

BUILD_DIR=$(mktemp -d)
cd "$BUILD_DIR"

/usr/local/src/slurm/configure \
	--prefix=/usr/local \
	--sysconfdir=/usr/local/etc/slurm \
	--with-pmix \
	--with-jwt=/usr/local \
	--with-systemdsystemunitdir=/usr/lib/systemd/system/ >/dev/null

make -j install >/dev/null
cd contribs
make -j install >/dev/null

cd "$SLURM_SRC_DIR"
rm -rf "$BUILD_DIR"

ln -sfn slurm/contribs/slurm_completion_help/slurm_completion.sh /etc/bash_completion.d/slurm
