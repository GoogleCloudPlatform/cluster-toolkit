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
set -eu

trap "printf '\nCaught Ctrl+c. Exiting...\n'; exit" INT

# Use current unix timestamp as a unique tag
# for jobs submitted
TAG=$(date +%s)
TEST_DIR=${PWD}/nccl-tests-"${TAG}"
SOFTWARE_INSTALL=${1:-/opt/apps}

cat <<EOF
This script will install the following packages using on this VM:
  jq
  python3-venv

It will then set up enroot credentials to be able to use artifact registry
by adding the following to ${HOME}/.enroot/.credentials:

  machine us-docker.pkg.dev login oauth2accesstoken password \$(gcloud auth print-access-token)

And will clone ramble (https://github.com/GoogleCloudPlatform/ramble.git)
to "${SOFTWARE_INSTALL}"/, and it will create an enroot "sqsh" file located
in your ${HOME}/.enroot/ folder. Afterwards it will create a ramble workspace
to run a number of NCCL tests in $(readlink -f "${TEST_DIR}"/).

EOF

read -t 30 -rp "To continue, hit [enter]. To cancel, type [Ctrl-c]. Will auto-continue in 30s" || true

mkdir -p "${TEST_DIR}"

# Install prerequisites
sudo apt-get install -y python3-venv jq

# Create enroot credentials set up for artifact registry
mkdir -p "${HOME}"/.enroot/
export ENROOT_CONFIG_PATH=${HOME}/.enroot
ENROOT_CONFIG_CREDENTIALS="${HOME}"/.enroot/.credentials

if ! grep -q "us-docker.pkg.dev" "${ENROOT_CONFIG_CREDENTIALS}"; then
	cat <<EOF >>"${ENROOT_CONFIG_CREDENTIALS}"
machine us-docker.pkg.dev login oauth2accesstoken password \$(gcloud auth print-access-token)
EOF
fi

# Install ramble and make world read/writeable.
sudo git clone --depth 1 -c feature.manyFiles=true https://github.com/GoogleCloudPlatform/ramble.git "${SOFTWARE_INSTALL}"/ramble || true
sudo chmod -R a+w "${SOFTWARE_INSTALL}"/ramble

# Create python environment for ramble, and install requirements
python3 -m venv "${SOFTWARE_INSTALL}"/ramble/env || true
source "${SOFTWARE_INSTALL}"/ramble/env/bin/activate
pip install -q -r "${SOFTWARE_INSTALL}"/ramble/requirements.txt

# Activate ramble
. "${SOFTWARE_INSTALL}"/ramble/share/ramble/setup-env.sh

# Create a new workspace for this work
ramble workspace create -a -d "${TEST_DIR}"

# Populate ramble.yaml
cat <<EOF >"${TEST_DIR}"/configs/ramble.yaml
# Ramble Configuration for NCCL Tests
ramble:
  modifiers:
  - name: pyxis-enroot
  - name: nccl-gib
  variables:
    batch_submit: sbatch '{execute_experiment}'

    srun_args: >-
      --mpi=pmix
      --container-workdir /third_party/nccl-tests/build/
      --container-env LD_LIBRARY_PATH
      --container-image {container_path}
      --container-mounts "/usr/local/gib,/var/tmp"
      --container-writable
      --wait=60
      --kill-on-bad-exit=1

    mpi_command: srun {srun_args}

    hostlist: \${SLURM_JOB_NODELIST}

    container_dir: "${SOFTWARE_INSTALL}/ramble/sqsh"
    container_name: nccl-plugin-gib-diagnostic-arm64:v1.0.6
    container_uri: docker://us-docker.pkg.dev#gce-ai-infra/gpudirect-gib/nccl-plugin-gib-diagnostic-arm64:v1.0.6
    processes_per_node: 4
    processes_per_node: '{gpus_per_node}'
    gpus_per_node: '4'
    nccl-tests_path: null

  env_vars:
    set:
      OMPI_MCA_btl_tcp_if_include: enp0s1
      PMIX_MCA_gds: ^ds12
      UCX_NET_DEVICES: gpu0rdma0,gpu1rdma0,gpu2rdma0,gpu3rdma0
      PMIX_MCA_psec: native
      UCX_IB_FORK_INIT: n
      NCCL_NET: gIB
      NCCL_SOCKET_IFNAME: enp0s1,enp192s1
      LD_LIBRARY_PATH: /usr/local/gib/lib64:usr/local/nvidia/lib

  applications:
    nccl-tests:
      workloads:
        '{workload}':
          experiments:
            '{workload}-{n_nodes}':
              variables:
                workload: [all-gather, all-reduce, reduce-scatter]
                n_nodes: [2, 4, 8, 16, 32]
              matrix:
              - n_nodes
              - workload
EOF

# Populate slurm sbatch script
cat <<EOF >"${TEST_DIR}"/configs/execute_experiment.tpl
#!/bin/bash
#SBATCH -J {experiment_name}-"${TAG}"
#SBATCH --output={experiment_run_dir}/slurm-%j.out
#SBATCH -N {n_nodes}
#SBATCH --gpus-per-node=4
#SBATCH --exclusive
#SBATCH --ntasks-per-node={processes_per_node}

cd "{experiment_run_dir}"
{command}
EOF

cd "${RAMBLE_WORKSPACE}"

# Get number of nodes available
N_NODES=$(sinfo -h -o %D)

# Print available benchmarks
printf "\n--- Available Benchmarks on %s nodes --\n" "${N_NODES}"
ramble workspace info --where '{n_nodes} <= '"${N_NODES}"
printf "\n------- About to run the following: ------\n\n"
printf "source %s/ramble/env/bin/activate\n" "${SOFTWARE_INSTALL}"
printf ". %s/ramble/share/ramble/setup-env.sh\n" "${SOFTWARE_INSTALL}"
printf "ramble workspace activate %s\n" "${TEST_DIR}"
printf "ramble workspace setup --where '{n_nodes} <= %s'\n" "${N_NODES}"
printf "ramble on --where '{n_nodes} <= %s' \n" "${N_NODES}"

# Set up experiments
printf "\n--------- Setting up Benchmarks -------\n"
printf "         This may take ~5-10 minutes     \n"
ramble workspace setup --where '{n_nodes} <= '"${N_NODES}"

# Submit Experiments to Slurm
printf "\n----------- Running Benchmarks --------\n"
ramble on --where '{n_nodes} <= '"${N_NODES}"

# Wait for all to be done
# Use the TAG in the slurm jobs
until [[ $(squeue -h -o %j | grep -c "${TAG}") -eq 0 ]]; do
	clear
	echo "waiting for $(squeue -h -o %j | grep -c "${TAG}") jobs to finish"
	squeue
	sleep 5
done

# Analyze
printf "\n------- Analyzing benchmark logs ------\n"
ramble workspace analyze -f json --where '{n_nodes} <= '"${N_NODES}"

printf "\n------- Archiving ramble workspace ------\n"
ramble workspace archive -t --where '{n_nodes} <= '"${N_NODES}"

printf "\n--------------- SUMMARY ---------------\n"
cd "${TEST_DIR}"
jq -r '["workload","n_nodes","msg_size","busbw"], (.experiments[] as $exp | $exp.CONTEXTS[] as $context |
{
  experiment_name: $exp.name,
  workload: $exp.workload_name,
  n_nodes: $exp.n_nodes,
  Context: $context.name
} +
($context.foms | from_entries )
| [.workload, .n_nodes, .Size, ."Out of Place Bus Bandwidth"])
| @tsv' results.latest.json >summary.tsv

# Print just the large message sizes
printf "\n--- SUMMARY for >1GB Message Sizes --\n"
jq -r '["workload","n_nodes","msg_size","busbw"], (.experiments[] as $exp | $exp.CONTEXTS[] as $context |
{
  experiment_name: $exp.name,
  workload: $exp.workload_name,
  n_nodes: $exp.n_nodes,
  Context: $context.name
} +
($context.foms | from_entries )
| select(.Size | tonumber  > 1000000000)
| [.workload, .n_nodes, .Size, ."Out of Place Bus Bandwidth"])
| @tsv' results.latest.json | column -t
printf "\nFor full results, see %s\n" "${TEST_DIR}"/summary.tsv

printf "\n-------- Benchmarking Complete -------\n"

ARCHIVE_TAR=$(readlink archive/archive.latest.tar.gz)
ARCHIVE_PATH="${RAMBLE_WORKSPACE}"/archive/"${ARCHIVE_TAR}"
RESULTS_FILE=$(basename "$(readlink results.latest.json)")
RESULTS_PATH="${RAMBLE_WORKSPACE}"/"${RESULTS_FILE}"

printf "\n# To view the full results:\n"
printf "cat %s\n" "${RESULTS_PATH}"

printf "\n# To find the ramble workspace archive:\n"
printf "ls %s\n" "${ARCHIVE_PATH}"

printf "\n# To re-activate ramble workspace:\n"
printf "cd %s\n" "${RAMBLE_WORKSPACE}"
printf "source %s/ramble/env/bin/activate\n" "${SOFTWARE_INSTALL}"
printf ". %s/ramble/share/ramble/setup-env.sh\n" "${SOFTWARE_INSTALL}"
printf "ramble workspace activate .\n"
