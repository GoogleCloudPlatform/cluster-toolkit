#!/bin/bash
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

LEGACY_MONITORING_PACKAGE='stackdriver-agent'
LEGACY_LOGGING_PACKAGE='google-fluentd'
OPSAGENT_PACKAGE='google-cloud-ops-agent'
OPSAGENT_SCRIPT_URL='https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh'

fail() {
	echo >&2 "[$(date +'%Y-%m-%dT%H:%M:%S%z')] $*"
	exit 1
}

handle_debian() {
	is_legacy_monitoring_installed() {
		dpkg-query --show --showformat 'dpkg-query: ${Package} is installed\n' ${LEGACY_MONITORING_PACKAGE} |
			grep "${LEGACY_MONITORING_PACKAGE} is installed"
	}

	is_legacy_logging_installed() {
		dpkg-query --show --showformat 'dpkg-query: ${Package} is installed\n' ${LEGACY_LOGGING_PACKAGE} |
			grep "${LEGACY_LOGGING_PACKAGE} is installed"
	}

	is_legacy_installed() {
		is_legacy_monitoring_installed || is_legacy_logging_installed
	}

	is_opsagent_installed() {
		dpkg-query --show --showformat 'dpkg-query: ${Package} is installed\n' ${OPSAGENT_PACKAGE} |
			grep "${OPSAGENT_PACKAGE} is installed"
	}

	install_opsagent() {
		MAX_RETRY=50
		RETRY=0
		until [ ${RETRY} -eq ${MAX_RETRY} ] || curl -s "${OPSAGENT_SCRIPT_URL}" | bash -s -- --also-install; do
			RETRY=$((RETRY + 1))
			echo "WARNING: Cloud ops installation failed on try ${RETRY} of ${MAX_RETRY}"
			sleep 5
		done
		if [ $RETRY -eq $MAX_RETRY ]; then
			echo "ERROR: Cloud ops installation was not successful after ${MAX_RETRY} attempts."
			exit 1
		fi
	}
}

handle_redhat() {
	is_legacy_monitoring_installed() {
		rpm --query --queryformat 'package %{NAME} is installed\n' ${LEGACY_MONITORING_PACKAGE} |
			grep "${LEGACY_MONITORING_PACKAGE} is installed"
	}

	is_legacy_logging_installed() {
		rpm --query --queryformat 'package %{NAME} is installed\n' ${LEGACY_LOGGING_PACKAGE} |
			grep "${LEGACY_LOGGING_PACKAGE} is installed"
	}

	is_legacy_installed() {
		is_legacy_monitoring_installed || is_legacy_logging_installed
	}

	is_opsagent_installed() {
		rpm --query --queryformat 'package %{NAME} is installed\n' ${OPSAGENT_PACKAGE} |
			grep "${OPSAGENT_PACKAGE} is installed"
	}

	install_opsagent() {
		curl -s https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh | bash -s -- --also-install
	}
}

main() {
	if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] || [ -f /etc/oracle-release ] || [ -f /etc/system-release ]; then
		handle_redhat
	elif [ -f /etc/debian_version ] || grep -qi ubuntu /etc/lsb-release || grep -qi ubuntu /etc/os-release; then
		handle_debian
	else
		fail "Unsupported platform."
	fi

	if is_legacy_installed || is_opsagent_installed; then
		fail "Legacy or Ops Agent is already installed."
	fi

	install_opsagent
}

main
