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
set -eu

trap "printf '\nCaught Ctrl+c. Exiting...\n'; exit" INT

# Use current unix timestamp as a unique tag
# for jobs submitted
TAG=$(date +%s)
TEST_DIR=nccl-tests-${TAG}
SOFTWARE_INSTALL=/opt/apps

cat <<EOF
This script will install the following packages using on this VM:
  build-essential
  g++-12
  gcc-12
  gfortran-12
  jq
  libgcc-12-dev
  libgfortran-12-dev
  libopenmpi-dev
  openmpi-bin
  python3-venv

And will clone spack (https://github.com/spack/spack.git)
and ramble (https://github.com/GoogleCloudPlatform/ramble.git)
to ${SOFTWARE_INSTALL}/. Afterwards it will create a ramble workspace to run a
number of NCCL tests in $(readlink -f ${TEST_DIR}/). As part of the build
process, spack will add some configuration files to your ${HOME}/.spack
directory.

EOF
read -p "To continue, hit any key. To cancel, [Ctrl-c]"

mkdir -p ${TEST_DIR}

# Install prerequisites
sudo apt-get install -y g++-12 gfortran-12 build-essential gcc-12 libgfortran-12-dev libgcc-12-dev python3-venv jq libopenmpi-dev openmpi-bin

# Install ramble and spack, and make world read/writeable.
sudo git clone --depth 1 -c feature.manyFiles=true https://github.com/GoogleCloudPlatform/ramble.git ${SOFTWARE_INSTALL}/ramble || true
sudo git clone --depth 1 -c feature.manyFiles=true -b develop https://github.com/spack/spack.git ${SOFTWARE_INSTALL}/spack || true
sudo chmod -R a+w ${SOFTWARE_INSTALL}/{ramble,spack}

# Create python environment for ramble, and install requirements
python3 -m venv ${SOFTWARE_INSTALL}/ramble/env || true
source ${SOFTWARE_INSTALL}/ramble/env/bin/activate
pip install -q -r ${SOFTWARE_INSTALL}/ramble/requirements.txt

# Activate ramble and spack
. ${SOFTWARE_INSTALL}/ramble/share/ramble/setup-env.sh
. ${SOFTWARE_INSTALL}/spack/share/spack/setup-env.sh

# Set up Spack external packages
spack external find python diffutils xz ncurses flex curl openssl m4 openssh
spack external find -p /usr/local/cuda cuda

# Create a new workspace for this work
ramble workspace create -a -d ${TEST_DIR}

# Populate ramble.yaml
cat <<EOF > ${TEST_DIR}/configs/ramble.yaml
# Ramble Configuration for NCCL Tests
ramble:
  env_vars:
    set:
      OMPI_MCA_pml: "^ucx"
      OMPI_MCA_btl: "^openib"
      OMPI_MCA_btl_tcp_if_include: enp0s19

      CUDA_VISIBLE_DEVICES: 0,1,2,3,4,5,6,7
      NCCL_NET: gIB
      NCCL_SOCKET_IFNAME: enp0s19,enp192s20
      NCCL_CROSS_NIC: 0
      NCCL_NET_GDR_LEVEL: PIX
      NCCL_P2P_NET_CHUNKSIZE: 131072
      NCCL_P2P_PCI_CHUNKSIZE: 131072
      NCCL_P2P_NVL_CHUNKSIZE: 524288
      NCCL_NVLS_CHUNKSIZE: 524288
      NCCL_IB_GID_INDEX: 3
      NCCL_IB_ADAPTIVE_ROUTING: 1
      NCCL_IB_QPS_PER_CONNECTION: 4
      NCCL_IB_TC: 52
      NCCL_IB_FIFO_TC: 84
      NCCL_SHIMNET_GUEST_CONFIG_CHECKER_CONFIG_FILE: /usr/local/gib/configs/guest_config.txtpb
      NCCL_TUNER_CONFIG_PATH: /usr/local/gib/configs/tuner_config.txtpb
    prepend:
    - paths:
        LD_LIBRARY_PATH: /usr/local/gib/lib64

  variables:
    mpi_command: srun --mpi=pmix
    batch_submit: 'sbatch {execute_experiment}'
    processes_per_node: '{gpus_per_node}'
    gpus_per_node: '8'
  applications:
    nccl-tests:
      workloads:
        '{workload}':
          experiments:
            '{workload}-{n_nodes}':
              variants:
                package_manager: spack
              variables:
                workload: [all-gather, all-reduce, reduce-scatter]
                n_nodes: [2, 4, 8, 16, 32]
              matrix:
              - n_nodes
              - workload

  software:
    packages:
      pmix:
        pkg_spec: pmix
      mpi:
        pkg_spec: openmpi +cuda cuda_arch=90
      cuda:
        pkg_spec: cuda@12.4.0
      nccl:
        pkg_spec: nccl@2.23.4-1 cuda_arch=90
      nccl-tests:
        pkg_spec: nccl-tests cuda_arch=90
    environments:
      nccl-tests:
        packages: [cuda, mpi, nccl, nccl-tests, pmix]

EOF

# Populate slurm sbatch script
cat <<EOF > ${TEST_DIR}/configs/execute_experiment.tpl
#!/bin/bash
#SBATCH -J {experiment_name}-${TAG}
#SBATCH --output={experiment_run_dir}/slurm-%j.out
#SBATCH -N {n_nodes}
#SBATCH --gpus-per-node=8
#SBATCH --exclusive
#SBATCH --ntasks-per-node={processes_per_node}

cd "{experiment_run_dir}"
{command}
EOF

# Get number of nodes available
N_NODES=$(sinfo -h -o %D)

# Print available benchmarks
printf "\n--------- Setting up Benchmarks ----------\n"
ramble workspace info --where '{n_nodes} <= '$N_NODES

printf "\n------- About to run the following: ------\n\n"
printf "source ${SOFTWARE_INSTALL}/ramble/env/bin/activate\n"
printf ". ${SOFTWARE_INSTALL}/ramble/share/ramble/setup-env.sh\n"
printf ". ${SOFTWARE_INSTALL}/spack/share/spack/setup-env.sh\n"
printf "ramble workspace activate ${TEST_DIR}\n"
printf "ramble workspace setup --where '{n_nodes} <= ${N_NODES}'\n"
printf "ramble on --where '{n_nodes} <= ${N_NODES}' \n"

# Set up experiments
printf "\n--------- Setting up Benchmarks -------\n"
printf "         This may take 20-30 minutes     \n"
ramble workspace setup --where '{n_nodes} <= '${N_NODES}

# Submit Experiments to Slurm
printf "\n----------- Running Benchmarks --------\n"
ramble on --where '{n_nodes} <= '${N_NODES}

# Wait for all to be done
# Use the TAG in the slurm jobs
until [[ $(squeue -h -o %j | grep -c ${TAG}) -eq 0 ]]; do
  clear
  echo "waiting for $(squeue -h -o %j | grep -c ${TAG}) jobs to finish"
  squeue
  sleep 5
done

# Analyze
ramble workspace analyze -f json --where '{n_nodes} <= '${N_NODES}

# Summarize all results in summary.tsv
cd ${TEST_DIR}
cat results.latest.json | jq -r '["workload","n_nodes","msg_size","busbw"], (.experiments[] as $exp | $exp.CONTEXTS[] as $context |
{
  experiment_name: $exp.name,
  workload: $exp.workload_name,
  n_nodes: $exp.n_nodes,
  Context: $context.name
} +
($context.foms | from_entries ) | [.workload, .n_nodes, .Size, ."Out of Place Bus Bandwidth"]) | @tsv' > summary.tsv

# Print just the 8GB message sizes
printf "\n--- SUMMARY for 8GB Message Sizes --\n"
cat results.latest.json | jq -r '["workload","n_nodes","msg_size","busbw"], (.experiments[] as $exp | $exp.CONTEXTS[] as $context |
{
  experiment_name: $exp.name,
  workload: $exp.workload_name,
  n_nodes: $exp.n_nodes,
  Context: $context.name
} +
($context.foms | from_entries ) | select(.Size | tonumber  > 8000000000) | [.workload, .n_nodes, .Size, ."Out of Place Bus Bandwidth"]) | @tsv'
printf "\nFor full results, see \"summary.tsv\"\n"

printf "\n- To reactivate this ramble workspace, run -\n\n"
printf "source ${SOFTWARE_INSTALL}/ramble/env/bin/activate\n"
printf ". ${SOFTWARE_INSTALL}/ramble/share/ramble/setup-env.sh\n"
printf ". ${SOFTWARE_INSTALL}/spack/share/spack/setup-env.sh\n"
printf "ramble workspace activate ${TEST_DIR}\n"
