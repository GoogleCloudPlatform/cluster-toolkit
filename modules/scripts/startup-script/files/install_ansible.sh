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

REQ_ANSIBLE_VERSION=2.11
REQ_ANSIBLE_PIP_VERSION=4.10.0
REQ_PIP_MAJOR_VERSION=21
REQ_PYTHON3_VERSION=6

apt_wait() {
	while fuser /var/lib/dpkg/lock >/dev/null 2>&1; do
		echo "Sleeping for dpkg lock"
		sleep 3
	done
	while fuser /var/lib/apt/lists/lock >/dev/null 2>&1; do
		echo "Sleeping for apt lists lock"
		sleep 3
	done
	if [ -f /var/log/unattended-upgrades/unattended-upgrades.log ]; then
		echo "Sleeping until unattended-upgrades finishes"
		while fuser /var/log/unattended-upgrades/unattended-upgrades.log >/dev/null 2>&1; do
			sleep 3
		done
	fi
}

# Installs any dependencies needed for python based on the OS
install_python_deps() {
	if [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release 2>/dev/null ||
		grep -qi ubuntu /etc/os-release 2>/dev/null; then
		apt-get install -y python3-distutils
	fi
}

# Gets the name of the python executable for python starting with python3, then
# checking python. Sets the variable to an empty string if neither are found.
get_python_path() {
	python_path=""
	if which python3 2>/dev/null; then
		python_path=$(which python3 2>/dev/null)
	elif which python 2>/dev/null; then
		python_path=$(which python 2>/dev/null)
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
	major_version=$(rpm -E "%{rhel}")
	enable_repo=""
	if [ "${major_version}" -eq "7" ]; then
		enable_repo="base,epel"
	elif [ "${major_version}" -eq "8" ]; then
		enable_repo="baseos"
	else
		echo "Unsupported version of centos/RHEL/Rocky"
		return 1
	fi
	yum install --disablerepo="*" --enablerepo=${enable_repo} -y python3 python3-pip
	python_path=$(rpm -ql python3 | grep 'bin/python3$')
}

# Install python3 with the apt package manager. Updates python_path to the
# newly installed packaged.
install_python3_apt() {
	apt_wait
	apt-get install -y python3 python3-distutils python3-pip
	python_path=$(which python3)
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

# Install python3 with the yum package manager. Updates python_path to the
# newly installed packaged.
install_pip3_yum() {
	major_version=$(rpm -E "%{rhel}")
	enable_repo=""
	if [ "${major_version}" -eq "7" ]; then
		enable_repo="base,epel"
	elif [ "${major_version}" -eq "8" ]; then
		enable_repo="baseos"
	else
		echo "Unsupported version of centos/RHEL/Rocky"
		return 1
	fi
	yum install --disablerepo="*" --enablerepo=${enable_repo} -y python3-pip
}

# Install python3 with the apt package manager. Updates python_path to the
# newly installed packaged.
install_pip3_apt() {
	apt-get update
	apt-get install -y python3-pip
}

install_pip3() {
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] ||
		[ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
		install_pip3_yum
	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release 2>/dev/null ||
		grep -qi ubuntu /etc/os-release 2>/dev/null; then
		install_pip3_apt
	else
		echo "Error: Unsupported Distribution"
		return 1
	fi
}

main() {
	# Get the python3 executable, or install it if not found
	get_python_path
	get_python_major_version "${python_path}"
	get_python_minor_version "${python_path}"
	if [ "${python_path}" = "" ] || [ "${python_major_version}" = "2" ] || [ "${python_minor_version}" -lt "${REQ_PYTHON3_VERSION}" ]; then
		if ! install_python3; then
			return 1
		fi
		get_python_major_version "${python_path}"
		get_python_minor_version "${python_path}"
	else
		install_python_deps
	fi

	# Install and/or upgrade pip
	if ! ${python_path} -m pip --version 2>/dev/null; then
		if ! install_pip3; then
			return 1
		fi
	fi
	pip_version=$(${python_path} -m pip --version | sed -nr 's/^pip ([0-9]+\.[0-9]+).*$/\1/p')
	pip_major_version=$(echo "${pip_version}" | cut -d '.' -f 1)
	if [ "${pip_major_version}" -lt "${REQ_PIP_MAJOR_VERSION}" ]; then
		${python_path} -m pip install --upgrade pip
	fi

	# Create pip virtual environment for HPC Toolkit
	${python_path} -m pip install virtualenv
	${python_path} -m virtualenv /usr/local/ghpc-venv
	venv_python_path=/usr/local/ghpc-venv/bin/python3

	# when Ansible creates virtual environments, it defaults to virtualenv
	# these steps install virtualenv in the Ansible virtual environment and
	# ensures that it will be found
	${venv_python_path} -m pip install virtualenv
	if [ ! -f /etc/ansible/ansible.cfg ]; then
		mkdir /etc/ansible
		cat <<-EOF >/etc/ansible/ansible.cfg
			[defaults]
			interpreter_python=${venv_python_path}
			stdout_callback=debug
			stderr_callback=debug
		EOF
	fi

	# Install ansible
	ansible_version=""
	if which ansible-playbook 2>/dev/null; then
		ansible_version=$(ansible-playbook --version 2>/dev/null | sed -nr 's/^ansible-playbook.*([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')
		ansible_major_vers=$(echo "${ansible_version}" | cut -d '.' -f 1)
		ansible_minor_vers=$(echo "${ansible_version}" | cut -d '.' -f 2)
		ansible_req_major_vers=$(echo "${REQ_ANSIBLE_VERSION}" | cut -d '.' -f 1)
		ansible_req_minor_vers=$(echo "${REQ_ANSIBLE_VERSION}" | cut -d '.' -f 2)
	fi
	if [ -z "${ansible_version}" ] || [ "${ansible_major_vers}" -ne "${ansible_req_major_vers}" ] ||
		[ "${ansible_minor_vers}" -lt "${ansible_req_minor_vers}" ]; then
		${venv_python_path} -m pip install ansible==${REQ_ANSIBLE_PIP_VERSION}
	fi
}

main
