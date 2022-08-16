#!/bin/sh
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

apt_wait() {
	while fuser /var/lib/dpkg/lock 2>/dev/null 2>&1; do
		echo "Sleeping for dpkg lock"
		sleep 3
	done
	while fuser /var/lib/apt/lists/lock 2>/dev/null 2>&1; do
		echo "Sleeping for apt lists lock"
		sleep 3
	done
	if [ -f /var/log/unattended-upgrades/unattended-upgrades.log ]; then
		echo "Sleeping until unattended-upgrades finishes"
		while fuser /var/log/unattended-upgrades/unattended-upgrades.log 2>/dev/null 2>&1; do
			sleep 3
		done
	fi
}

# Installs any dependencies needed for python based on the OS
install_python_deps() {
	if [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release 2>/dev/null ||
		grep -qi ubuntu /etc/os-release 2>/dev/null; then
		apt install -y python3-distutils
	fi
}

# Gets the name of the python executable for python starting with python3, then
# checking python. Sets the variable to an empty string if neither are found.
get_python_path() {
	python_path=""
	if [ -f /bin/python3 ]; then
		python_path="/bin/python3"
	elif [ -f /bin/python ]; then
		python_path="/bin/python"
	fi
}

# Returns the python major version. If provided, it will use the first argument
# as the python executable, otherwise it will default to simply "python".
get_python_major_version() {
	python_path=${1:-python}
	python_major_version=$(${python_path} -c "import sys; print(sys.version_info.major)")
}

# Returns the python minor version. If provided, it will use the first argument
# as the python executable, otherwise it will default to simply "python".
get_python_minor_version() {
	python_path=${1:-python}
	python_minor_version=$(${python_path} -c "import sys; print(sys.version_info.minor)")
}

# Install python3 with the yum package manager. Updates python_path to the
# newly installed packaged.
install_python3_yum() {
	## TODO restrict repos to search through in centos to decrease overhead
	yum install -y python3
	python_path=$(rpm -ql python3 | grep 'python3$')
}

# Install python3 with the apt package manager. Updates python_path to the
# newly installed packaged.
install_python3_apt() {
	apt_wait
	apt install -y python3 python3-distutils
	python_path=$(dpkg -L python3 | grep 'python3$')
}

install_python3() {
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] ||
		[ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
		install_python3_yum
	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release 2>/dev/null ||
		grep -qi ubuntu /etc/os-release 2>/dev/null; then
		install_python3_apt
	else
		echo "Error: Unsupported Distribution"
		return 1
	fi
}

main() {
	# Get the python3 executable, or install it if not found
	get_python_path
	if [ "${python_path}" = "" ]; then
		if ! install_python3; then
			return 1
		fi
	fi
	get_python_major_version "${python_path}"
	if [ "${python_major_version}" = "2" ]; then
		if ! install_python3; then
			return 1
		fi
	fi
	install_python_deps

	# Install and/or upgrade pip
	get_python_minor_version "${python_path}"
	if [ "${python_minor_version}" -lt 7 ]; then
		get_pip_url="https://bootstrap.pypa.io/pip/${python_major_version}.${python_minor_version}/get-pip.py"
	else
		get_pip_url="https://bootstrap.pypa.io/pip/get-pip.py"
	fi
	curl -Os ${get_pip_url}
	${python_path} get-pip.py

	# Create pip virtual environment for HPC Toolkit
	${python_path} -m pip install virtualenv
	${python_path} -m virtualenv /usr/local/ghpc-venv
	python_path=/usr/local/ghpc-venv/bin/python3

	# Install ansible
	${python_path} -m pip install ansible==4.10.0
}

main
