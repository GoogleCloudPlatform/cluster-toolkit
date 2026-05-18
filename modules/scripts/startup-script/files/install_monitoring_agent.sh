#!/bin/bash
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
set -e -o pipefail

LEGACY_MONITORING_PACKAGE='stackdriver-agent'
LEGACY_MONITORING_SCRIPT_URL='https://dl.google.com/cloudagents/add-monitoring-agent-repo.sh'
LEGACY_LOGGING_PACKAGE='google-fluentd'
LEGACY_LOGGING_SCRIPT_URL='https://dl.google.com/cloudagents/add-logging-agent-repo.sh'

OPSAGENT_PACKAGE='google-cloud-ops-agent'
OPSAGENT_SCRIPT_URL='https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh'

ops_or_legacy="${1:-legacy}"

fail() {
	echo >&2 "[$(date +'%Y-%m-%dT%H:%M:%S%z')] $*"
	exit 1
}

# Secure, generalized installer wrapper that eliminates curl | bash
install_agent_shared() {
	local script_url="$1"
	local install_cmd="$2"
	local script_name
	script_name=$(basename "${script_url}")
	
	local tmp_dir
	tmp_dir=$(mktemp -d /tmp/add-agent-repo.XXXXXXXX)
	chmod 700 "${tmp_dir}"
	local tmp_script="${tmp_dir}/${script_name}"

	local MAX_RETRY=50
	local RETRY=0
	until [ ${RETRY} -eq ${MAX_RETRY} ] || {
		curl -fsS -o "${tmp_script}" "${script_url}" &&
			bash "${tmp_script}" --also-install &&
			${install_cmd}
	}; do
		RETRY=$((RETRY + 1))
		echo >&2 "WARNING: Installation step for ${script_name} failed on try ${RETRY} of ${MAX_RETRY}"
		sleep 5
	done

	rm -rf "${tmp_dir}"

	if [ $RETRY -eq $MAX_RETRY ]; then
		fail "Installation step for ${script_name} was not successful after ${MAX_RETRY} attempts."
	fi
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
		run_package_install() {
			apt-get update && apt-get install -y "${OPSAGENT_PACKAGE}"
		}
		install_agent_shared "${OPSAGENT_SCRIPT_URL}" run_package_install
	}

	install_stackdriver_agent() {
		run_monitoring_install() {
			apt-get update && apt-get install -y "${LEGACY_MONITORING_PACKAGE}"
		}
		run_logging_install() {
			apt-get install -y "${LEGACY_LOGGING_PACKAGE}"
		}
		install_agent_shared "${LEGACY_MONITORING_SCRIPT_URL}" run_monitoring_install
		install_agent_shared "${LEGACY_LOGGING_SCRIPT_URL}" run_logging_install
		service stackdriver-agent start
		service google-fluentd start
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
		run_package_install() {
			yum install -y "${OPSAGENT_PACKAGE}"
		}
		install_agent_shared "${OPSAGENT_SCRIPT_URL}" run_package_install
	}

	install_stackdriver_agent() {
		run_monitoring_install() {
			yum install -y "${LEGACY_MONITORING_PACKAGE}"
		}
		run_logging_install() {
			yum install -y "${LEGACY_LOGGING_PACKAGE}"
		}
		install_agent_shared "${LEGACY_MONITORING_SCRIPT_URL}" run_monitoring_install
		install_agent_shared "${LEGACY_LOGGING_SCRIPT_URL}" run_logging_install
		service stackdriver-agent start
		service google-fluentd start
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

	# Handle cases that agent is already installed
	if [[ -z "$(is_legacy_monitoring_installed)" && -n $(is_legacy_logging_installed) ]] ||
		[[ -n "$(is_legacy_monitoring_installed)" && -z $(is_legacy_logging_installed) ]]; then
		fail "Bad state: legacy agent is partially installed"
	elif [[ "${ops_or_legacy}" == "legacy" ]] && is_legacy_installed; then
		echo "Legacy agent is already installed"
		exit 0
	elif [[ "${ops_or_legacy}" != "legacy" ]] && is_opsagent_installed; then
		echo "Ops agent is already installed"
		exit 0
	elif is_legacy_installed || is_opsagent_installed; then
		fail "Agent is already installed but does not match requested agent of ${ops_or_legacy}"
	fi

	# install agent
	if [[ "${ops_or_legacy}" == "legacy" ]]; then
		echo "Installing legacy monitoring agent (stackdriver)"
		install_stackdriver_agent
	else
		echo "Installing cloud ops agent"
		echo "WARNING: cloud ops agent may have a performance impact. Consider using legacy monitoring agent (stackdriver)."
		install_opsagent
	fi
}

main
