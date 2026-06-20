#!/bin/sh
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

set -ex
REQ_PYTHON3_VERSION=9

# Path to hashed requirements file
BUILD_TOOLS_REQ_FILE="$(dirname "$0")/build-tools.txt"
ANSIBLE_REQ_FILE="$(dirname "$0")/install_ansible_requirements.txt"

apt_wait() {
	while fuser /var/lib/apt/lists/lock >/dev/null 2>&1; do
		echo "Sleeping for apt lists lock"
		sleep 3
	done
}

# Installs any dependencies needed for python based on the OS
install_python_deps() {
	# this file is present on both Debian and Ubuntu OSes
	if [ -f /etc/debian_version ]; then
		apt_wait
		apt-get update --allow-releaseinfo-change-origin --allow-releaseinfo-change-label
		apt-get install -o DPkg::Lock::Timeout=600 -y python3-setuptools python3-venv
	fi
}

# Gets the name of the python executable for python starting with python3, then
# checking python. Sets the variable to an empty string if neither are found.
get_python_path() {
	python_path=""
	if command -v python3 1>/dev/null; then
		python_path=$(command -v python3)
	elif command -v python 1>/dev/null; then
		python_path=$(command -v python)
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
install_python3_dnf() {
	major_version=$(rpm -E "%{rhel}")
	set -- "--disablerepo=*" "--enablerepo=baseos,appstream"
	if grep -qi 'ID="rhel"' /etc/os-release; then
		# Do not set --disablerepo / --enablerepo on RedHat, due to
		# complex repo names; clear array
		set --
	fi
	# On Rocky Linux 9, Python 3.9 is installed by default but this
	# has already been dropped by ansible-core for control nodes.
	# https://docs.ansible.com/ansible/latest/reference_appendices/release_and_maintenance.html#ansible-core-support-matrix
	# Python 3.12 aligns with RHEL 10 default (GA: 13 May 2025) where
	# it is available as "python3*" but must be named explicitly on
	# older releases. It also ensures longer support for Ansible.
	if [ "${major_version}" -lt "10" ]; then
		dnf install "$@" -y python3.12 python3.12-pip
		python_path=$(command -v python3.12)
	else
		dnf install "$@" -y python3 python3-pip
		python_path=$(command -v python3)
	fi
}

# Install python3 with the apt package manager. Updates python_path to the
# newly installed packaged.
install_python3_apt() {
	apt_wait
	apt-get update --allow-releaseinfo-change-origin --allow-releaseinfo-change-label
	apt-get install -o DPkg::Lock::Timeout=600 -y python3 python3-setuptools python3-pip python3-venv
	python_path=$(command -v python3)
}

install_python3() {
	if [ -f /etc/redhat-release ] || [ -f /etc/oracle-release ] ||
		[ -f /etc/system-release ]; then
		install_python3_dnf
	elif [ -f /etc/debian_version ]; then
		install_python3_apt
	else
		echo "Error: Unsupported Distribution"
		return 1
	fi
}

# Install pip3 with the dnf package manager. Updates python_path to the
# newly installed packaged.
install_pip3_dnf() {
	major_version=$(rpm -E "%{rhel}")
	set -- "--disablerepo=*" "--enablerepo=baseos,appstream"
	if grep -qi 'ID="rhel"' /etc/os-release; then
		# Do not set --disablerepo / --enablerepo on RedHat, due to complex repo names
		# clear array
		set --
	fi
	# Python 3.12 aligns with RHEL 10 default (GA: 13 May 2025) where
	# it is available as "python3*" but must be named explicitly on
	# older releases. It also ensures longer support for Ansible.
	if [ "${major_version}" -lt "10" ]; then
		dnf install "$@" -y python3.12-pip
	else
		dnf install "$@" -y python3-pip
	fi
}

# Install pip3 with the apt package manager. Updates python_path to the
# newly installed packaged.
install_pip3_apt() {
	apt_wait
	apt-get update --allow-releaseinfo-change-origin --allow-releaseinfo-change-label
	apt-get install -o DPkg::Lock::Timeout=600 -y python3-pip
}

install_pip3() {
	if [ -f /etc/redhat-release ] || [ -f /etc/oracle-release ] ||
		[ -f /etc/system-release ]; then
		install_pip3_dnf
	elif [ -f /etc/debian_version ]; then
		install_pip3_apt
	else
		echo "Error: Unsupported Distribution"
		return 1
	fi
}

main() {
	if [ $# -gt 1 ]; then
		echo "Error: provide only 1 optional argument identifying virtual environment path for Ansible"
		return 1
	fi

	venv_path="${1:-/usr/local/ghpc-venv}"

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

	# Install OS-packaged pip
	if ! ${python_path} -m pip --version 2>/dev/null; then
		if ! install_pip3; then
			return 1
		fi
	fi

	# Create pip virtual environment for Cluster Toolkit
	${python_path} -m venv "${venv_path}" --copies
	venv_python_path="${venv_path}/bin/$(basename "${python_path}")"

	# Install build tools first
	${venv_python_path} -m pip install \
		--require-hashes \
		-r "${BUILD_TOOLS_REQ_FILE}"

	# Install Ansible and remaining dependencies
	${venv_python_path} -m pip install \
		--require-hashes \
		-r "${ANSIBLE_REQ_FILE}"

	# configure ansible to always use correct Python binary
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
	if [ -f "${venv_path}/bin/ansible-playbook" ]; then
		ansible_version=$("${venv_path}/bin/ansible-playbook" --version 2>/dev/null | sed -nr 's/^ansible-playbook.*([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')
		echo "Installed ansible version: ${ansible_version}"
	fi
	while read -r cmd; do
		if ! [ -L "/usr/bin/${cmd}" ]; then
			ln -s "${venv_path}/bin/${cmd}" "/usr/bin/${cmd}"
		fi
	done <<-EOF
		ansible
		ansible-config
		ansible-connection
		ansible-console
		ansible-doc
		ansible-galaxy
		ansible-inventory
		ansible-playbook
		ansible-pull
		ansible-test
		ansible-vault
	EOF
}

main "$@"
