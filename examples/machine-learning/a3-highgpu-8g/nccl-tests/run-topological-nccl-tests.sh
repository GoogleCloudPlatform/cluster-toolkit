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

# shellcheck disable=SC2001
# shellcheck disable=SC2015
# shellcheck disable=SC2016

#SBATCH --partition=a3
#SBATCH --exclusive
#SBATCH --gpus-per-node=8
#SBATCH --ntasks-per-node=8
#SBATCH --nodes 2

# Usage: sbatch run-topological-nccl-tests.sh
echo "Running NCCL Tests on ${SLURM_JOB_NUM_NODES} nodes."

# Echo all the commands, for future reference
set -x

# This should be set to the squashfs file that you created for your application
CONTAINER_IMAGE=./nvidia+pytorch+23.10-py3.sqsh

# Only use TCPXO for multi-node jobs.
[[ "${SLURM_JOB_NUM_NODES}" -gt 1 ]] && export USE_TCPXO=yes || export USE_TCPXO=no

# Only use TCPXO for multi-node jobs.
if [[ ${USE_TCPXO} = "yes" ]]; then
	# Source the TCPXO environment profile
	# shellcheck source=/dev/null
	NCCL_LIB_DIR="/var/lib/tcpxo/lib64" source /var/lib/tcpxo/lib64/nccl-env-profile.sh
	export NCCL_FASTRAK_CTRL_DEV=enp0s12
	export NCCL_SOCKET_IFNAME=enp0s12
	export NCCL_CROSS_NIC=0
	export NCCL_FASTRAK_USE_SNAP=1
	export NCCL_FASTRAK_USE_LLCM=1
	export NCCL_FASTRAK_LLCM_DEVICE_DIRECTORY=/dev/aperture_devices

	# Dynamically detect network interfaces associated with GPUs
	gpu_bdfs=("0000:06:00.0" "0000:0c:00.0" "0000:86:00.0" "0000:8c:00.0")
	gpu_interfaces=()
	for bdf in "${gpu_bdfs[@]}"; do
		if [ -d "/sys/bus/pci/devices/$bdf/net" ]; then
			# shellcheck disable=SC2012
			ifname=$(ls "/sys/bus/pci/devices/$bdf/net" | head -n 1)
			if [ -n "$ifname" ]; then
				gpu_interfaces+=("$ifname")
			fi
		fi
	done
	if [ ${#gpu_interfaces[@]} -eq 4 ]; then
		NCCL_FASTRAK_IFNAME=$(
			IFS=,
			echo "${gpu_interfaces[*]}"
		)
		export NCCL_FASTRAK_IFNAME
		echo "Detected GPU interfaces: $NCCL_FASTRAK_IFNAME"
	else
		echo "WARNING: Could not detect all 4 GPU interfaces. Found: ${gpu_interfaces[*]}"
		# Fallback to hardcoded names if detection fails
		export NCCL_FASTRAK_IFNAME=enp6s0f0,enp12s0f0,enp134s0f0,enp140s0f0
	fi
else
	unset NCCL_NET
fi

# Here we grab all the environment variables that need to be passed down into the container.
HOST_VARS=$(sed 's/ \{1,\}/,/g' <<<"${!NCCL*}")

# Mount /var/tmp to allow the rest of the enroot container to be read-only, and
# mount current $PWD to /nccl to for accessing nccl-tests binary
CONTAINER_MOUNTS="/var/tmp:/var/tmp"

# Mount PWD to /nccl in the enroot container
CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"$PWD:/nccl"

# Mount required directories for TCPXO functionality
if [[ ${USE_TCPXO} = "yes" ]]; then
	CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"/var/lib/tcpxo/lib64:/var/lib/tcpxo/lib64"
	CONTAINER_MOUNTS=${CONTAINER_MOUNTS},"/dev/aperture_devices:/dev/aperture_devices"
fi

# Construct topology ordered hostfile
# The -n, -N, --ntasks-per-node, etc, must match the way the workload is
# launched in order to ensure proper placement.
srun --mpi=pmi2 \
	-n $((SLURM_JOB_NUM_NODES * 8)) \
	--ntasks-per-node=8 \
	bash -c 'curl -s -f "http://metadata.google.internal/computeMetadata/v1/instance/attributes/physical_host" -H "Metadata-Flavor: Google"; echo /$SLURMD_NODENAME' |
	sort -t / -s -k 1,4 |
	awk -F "/" '{print $NF}' >"/var/tmp/topo_sorted_hostfile_${SLURM_JOB_ID}"
export SLURM_HOSTFILE="/var/tmp/topo_sorted_hostfile_${SLURM_JOB_ID}"

# Create a custom config checker file to ignore strict CPU affinity and env var checks
cat <<'EOF' >"${PWD}/custom_guest_config.textproto"
env_var_check_level: CHECK_DISABLED
attention_keyword_check_level: CHECK_DISABLED
cpu_affinity_check_level: CHECK_DISABLED
EOF

# Run the workload
srun -l \
	--mpi=pmi2 \
	--cpu-bind=verbose \
	-n $((SLURM_JOB_NUM_NODES * 8)) \
	--ntasks-per-node=8 \
	--gpus-per-node=8 \
	--export=ALL \
	--container-image="${CONTAINER_IMAGE}" \
	--container-name=nccl \
	--container-env="${HOST_VARS}" \
	--container-mounts="${CONTAINER_MOUNTS}" \
	sh -c '
	  export NCCL_SHIMNET_GUEST_CONFIG_CHECKER_CONFIG_FILE=/nccl/custom_guest_config.textproto;
	  export LD_LIBRARY_PATH=/var/lib/tcpxo/lib64:/usr/lib/x86_64-linux-gnu:$LD_LIBRARY_PATH;
	  /nccl/nccl-tests/build/all_reduce_perf -b 1G -e 8G -f 2 -g 1 -w 5 --iters 40
	'
