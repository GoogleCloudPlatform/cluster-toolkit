#!/bin/bash
# Copyright 2024 Google LLC
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

set -e -o pipefail

SLURM_EXTERNAL_ROOT="/opt/apps/adm/slurm"
SLURM_MUX_FILE="slurm_mux"

mkdir -p "${SLURM_EXTERNAL_ROOT}"
mkdir -p "${SLURM_EXTERNAL_ROOT}/logs"
mkdir -p "${SLURM_EXTERNAL_ROOT}/etc"

# create common prolog / epilog "multiplex" script
if [ ! -f "${SLURM_EXTERNAL_ROOT}/${SLURM_MUX_FILE}" ]; then
	# indentation matters in EOT below; do not blindly edit!
	cat <<'EOT' >"${SLURM_EXTERNAL_ROOT}/${SLURM_MUX_FILE}"
#!/bin/bash
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

set -e

CMD="${0##*/}"
# Locate script
BASE=$(readlink -f $0)
BASE=${BASE%/*}

export CLUSTER_ADM_BASE=${BASE}

# Source config file if it exists for extra DEBUG settings
# used below
SLURM_MUX_CONF=${CLUSTER_ADM_BASE}/etc/slurm_mux.conf
if [[ -r ${SLURM_MUX_CONF} ]]; then
	source ${SLURM_MUX_CONF}
fi

# Setup logging if configured and directory exists
LOGFILE="/dev/null"
if [[ -d ${DEBUG_SLURM_MUX_LOG_DIR} && ${DEBUG_SLURM_MUX_ENABLE_LOG} == "yes" ]]; then
	LOGFILE="${DEBUG_SLURM_MUX_LOG_DIR}/${CMD}-${SLURM_SCRIPT_CONTEXT}-job-${SLURMD_NODENAME}.log"
	exec >>${LOGFILE} 2>&1
fi

# Global scriptlets
for SCRIPTLET in ${BASE}/${SLURM_SCRIPT_CONTEXT}.d/*.${SLURM_SCRIPT_CONTEXT}; do
	if [[ -x ${SCRIPTLET} ]]; then
		echo "Running ${SCRIPTLET}"
		${SCRIPTLET} $@ >>${LOGFILE} 2>&1
	fi
done

# Per partition scriptlets
for SCRIPTLET in ${BASE}/partition-${SLURM_JOB_PARTITION}-${SLURM_SCRIPT_CONTEXT}.d/*.${SLURM_SCRIPT_CONTEXT}; do
	if [[ -x ${SCRIPTLET} ]]; then
		echo "Running ${SCRIPTLET}"
		${SCRIPTLET} $@ >>${LOGFILE} 2>&1
	fi
done
EOT
fi

# ensure proper permissions on slurm_mux script
chmod 0755 "${SLURM_EXTERNAL_ROOT}/${SLURM_MUX_FILE}"

# create default slurm_mux configuration file
if [ ! -f "${SLURM_EXTERNAL_ROOT}/etc/slurm_mux.conf" ]; then
	cat <<'EOT' >"${SLURM_EXTERNAL_ROOT}/etc/slurm_mux.conf"
# these settings are intended for temporary debugging purposes only; leaving
# them enabled will write files for each job to a shared NFS directory without
# any automated cleanup
DEBUG_SLURM_MUX_LOG_DIR=/opt/apps/adm/slurm/logs
DEBUG_SLURM_MUX_ENABLE_LOG=no
EOT
fi

# create epilog symbolic link
if [ ! -L "${SLURM_EXTERNAL_ROOT}/slurm_epilog" ]; then
	cd ${SLURM_EXTERNAL_ROOT}
	# delete existing file if necessary
	rm -f slurm_epilog
	ln -s ${SLURM_MUX_FILE} slurm_epilog
	cd - >/dev/null
fi

# create prolog symbolic link
if [ ! -L "${SLURM_EXTERNAL_ROOT}/slurm_prolog" ]; then
	cd ${SLURM_EXTERNAL_ROOT}
	# delete existing file if necessary
	rm -f slurm_prolog
	ln -s ${SLURM_MUX_FILE} slurm_prolog
	cd - >/dev/null
fi
