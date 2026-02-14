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
TEST_DIR=${PWD}/hpl-tests-"${TAG}"
SOFTWARE_INSTALL=/opt/apps

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
. ${SOFTWARE_INSTALL}/ramble/share/ramble/setup-env.sh

# Create a new workspace for this work
ramble workspace create -a -d "${TEST_DIR}"

# Populate ramble.yaml
cat <<EOF >"${TEST_DIR}"/configs/ramble.yaml
# Ramble Configuration for NVIDIA-HPL Tests
ramble:
  modifiers:
  - name: pyxis-enroot
  - name: nccl-gib
  variables:
    batch_submit: sbatch '{execute_experiment}'

    srun_args: >-
      --mpi=pmi2
      --container-env LD_LIBRARY_PATH
      --container-image {container_path}
      --container-mounts "/usr/local/gib,/var/tmp,{container_mounts},{experiment_run_dir}:/workdir"
      --container-writable
      --wait=60
      --kill-on-bad-exit=1

    mpi_command: srun {srun_args}

    hostlist: \${SLURM_JOB_NODELIST}

    container_dir: "/opt/apps/ramble/sqsh"
    container_name: hpl
    container_tag: 24.09
    container_uri: docker://nvcr.io#nvidia/hpc-benchmarks:{container_tag}
    processes_per_node: 8

  env_vars:
    set:
      OMPI_MCA_btl_tcp_if_include: enp0s19
      CUDA_VISIBLE_DEVICES: 0,1,2,3,4,5,6,7
      HPL_FCT_COMM_POLICY: 1
      HPL_P2P_AS_BCAST: '{hpl_p2p_as_bcast}'
      HPL_USE_NVSHMEM: 0
      NVSHMEM_DISABLE_CUDA_VMM: 1
      OMPI_MCA_btl: "^openib"
      OMPI_MCA_pml: "^ucx"
      UCX_MAX_RNDV_RAILS: 4
      UCX_IB_GID_INDEX: 3
      UCX_NET_DEVICES: rocep145s0:1,rocep146s0:1,rocep152s0:1,rocep153s0:1,rocep198s0:1,rocep199s0:1,rocep205s0:1,rocep206s0:1
      NCCL_NET: gIB
      LD_LIBRARY_PATH: /usr/local/gib/lib64:/usr/local/cuda/compat/lib.real:/opt/nvshmem/lib:/usr/local/cuda/compat/lib:/usr/local/nvidia/lib:/usr/local/nvidia/lib64

  applications:
    nvidia-hpl:
      workloads:
        calculator:
          experiments:
            hpl-{n_nodes}:
              variables:
                n_nodes: [1,2,4,8,16,24,32]

                # 0 = ncclBcast, 1 = ncclSend/Recv
                hpl_p2p_as_bcast: '0'

                # Percent of memory to use (default 85)
                percent_mem: 85

                # Memory per node in GB
                memory_per_node: '1200'

                # Other Recommended Settings
                block_size: '1024'
                PMAP: 1
                SWAP: 1
                swapping_threshold: 192
                L1: 1
                U: 0
                Equilibration: 0
                pfact: 0
                nbmin: 2
                rfact: 0
                bcast: 3
                depth: 1

EOF

# Populate slurm sbatch script
cat <<EOF >"${TEST_DIR}"/configs/execute_experiment.tpl
#!/bin/bash
#SBATCH -J {experiment_name}-"${TAG}"
#SBATCH --output={experiment_run_dir}/slurm-%j.out
#SBATCH -N {n_nodes}
#SBATCH --gpus-per-node=8
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
jq -r '["workload","n_nodes","GFlop/s   ","GFlops/s/GPU"], (.experiments[] as $exp | $exp.CONTEXTS[] as $context |
{
  experiment_name: $exp.name,
  workload: $exp.workload_name,
  n_nodes: $exp.n_nodes,
  Context: $context.name
} +
($context.foms | from_entries )
| [.workload, .n_nodes, .GFlops, ."Per GPU GFlops"])
| @tsv' results.latest.json
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
