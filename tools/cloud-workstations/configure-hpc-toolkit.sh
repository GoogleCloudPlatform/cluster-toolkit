#!/bin/bash
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

set -e

# Check if the hpc-toolkit folder exists in the user home directory
if [ ! -d "$HOME/hpc-toolkit" ]; then
	# If not, clone the repository
	git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git "$HOME/hpc-toolkit"
	cd "$HOME/hpc-toolkit" || exit
	make install-dev-deps
	pre-commit install
fi

# Run only on initial bootup
FLAG="$HOME/.firstboot"
if [[ ! -f $FLAG ]]; then
	# Set path for go binaries
	echo "export PATH=$PATH:$HOME/go/bin:/usr/local/go/bin" >>"$HOME/.bashrc"
	export PATH=$PATH:$HOME/go/bin:/usr/local/go/bin
	# Set up Code OSS for golang
	grep -v -e '^#' -e '^$' /etc/hpc-toolkit-config/code_oss_requirements.txt | xargs -L1 /opt/code-oss/bin/codeoss-cloudworkstations --install-extension
	go install golang.org/x/tools/gopls@latest
	go install github.com/ramya-rao-a/go-outline@v0.0.0-20210608161538-9736a4bde949
	go install github.com/rogpeppe/godef@v1.1.2
	go install golang.org/x/tools/cmd/guru@latest
	touch "$FLAG"
fi
