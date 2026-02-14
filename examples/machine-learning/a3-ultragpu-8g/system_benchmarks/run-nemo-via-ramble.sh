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
TEST_DIR=${PWD}/nemo-tests-"${TAG}"
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
# Ramble Configuration for NeMo LLama3.1 70B and Mixtral 8x7B
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
      --container-mounts "\${HOME}/.cache,/usr/local/gib,{container_mounts},{experiment_run_dir}:/workdir"
      --container-writable
      --wait=60
      --kill-on-bad-exit=1

    mpi_command: srun {srun_args}

    hostlist: \${SLURM_JOB_NODELIST}

    container_dir: "/opt/apps/ramble/sqsh"
    container_name: 'nemo-{nemo_version}'
    container_uri: docker://nvcr.io#nvidia/nemo:{nemo_version}

    n_threads: 12
    nemo_launcher_tag: 24.07
    nemo_version: 24.07
    processes_per_node: 8
    gpus_per_node: 8

    # Shared NeMo Configurations
    trainer.max_steps: 10
    trainer.val_check_interval: null
    trainer.limit_val_batches: 0.0
    trainer.log_every_n_steps: 1
    trainer.enable_model_summary: false

    model.tokenizer.library: megatron
    model.tokenizer.type: GPT2BPETokenizer
    model.tokenizer.model: null
    model.tokenizer.delimiter : null
    model.tokenizer.vocab_file: /workdir/gpt2-vocab.json
    model.tokenizer.merge_file: /workdir/gpt2-merges.txt
    model.data.data_impl: mock
    model.data.data_prefix: []
    model.data.splits_string: 98,1,1

    exp_manager.resume_if_exists: false
    exp_manager.create_checkpoint_callback: false
    exp_manager.create_dllogger_logger: false
    exp_manager.checkpoint_callback_params.save_top_k: 1
    exp_manager.checkpoint_callback_params.model_parallel_size: \${multiply:$\{model.tensor_model_parallel_size}, $\{model.pipeline_model_parallel_size}}
    exp_manager.exp_dir: '{experiment_run_dir}'

  env_vars:
    set:
      UCX_NET_DEVICES: rocep145s0:1,rocep146s0:1,rocep152s0:1,rocep153s0:1,rocep198s0:1,rocep199s0:1,rocep205s0:1,rocep206s0:1

      CUDA_VISIBLE_DEVICES: 0,1,2,3,4,5,6,7
      OMP_NUM_THREADS: '{n_threads}'
      TRANSFORMERS_OFFLINE: 0
      TORCH_NCCL_AVOID_RECORD_STREAMS: 1
      NCCL_NVLS_ENABLE: 0
      GLOO_SOCKET_IFNAME: enp0s19,enp192s20
      # SM_MARGIN environment vars prevent send-receive stalling execution
      # (results in reduced step time)
      NVTE_FWD_LAYERNORM_SM_MARGIN: 8
      NVTE_BWD_LAYERNORM_SM_MARGIN: 8
      OMPI_MCA_btl_tcp_if_include: enp0s19
      OMPI_MCA_btl: "^openib"
      OMPI_MCA_pml: "^ucx"
      NCCL_NET: gIB

      LD_LIBRARY_PATH: /usr/local/gib/lib64:/usr/local/lib/python3.10/dist-packages/torch/lib:/usr/local/lib/python3.10/dist-packages/torch_tensorrt/lib:/usr/local/cuda/compat/lib:/usr/local/nvidia/lib:/usr/local/nvidia/lib64:/usr/local/cuda/lib64:/usr/local/tensorrt/lib:/usr/local/cuda/lib64:/usr/local/tensorrt/lib:/usr/local/cuda/lib64:/usr/local/tensorrt/lib

  applications:
    py-nemo:
      workloads:
        pretraining:
          experiments:
            mixtral-{n_nodes}-nodes:
              variables:
                n_nodes: [8,16,32]

                nemo_stage: training
                nemo_model: mixtral
                nemo_config_name: mixtral_8x7b

                model.data.num_workers: 4
                model.fp8_params: true
                model.gc_interval: 0
                model.global_batch_size: 1024
                model.micro_batch_size: 2
                model.moe_grouped_gemm: false
                model.optim.contiguous_grad_buffer: true
                model.optim.contiguous_param_buffer: true
                model.pipeline_model_parallel_size: 1
                model.virtual_pipeline_model_parallel_size: null
                model.sequence_parallel: false

            llama3-{n_nodes}-nodes:
              variables:
                n_nodes: [8,16,32]

                nemo_stage: training
                nemo_model: llama
                nemo_config_name: llama3_1_70b

                model.data.num_workers: 2
                model.context_parallel_size: 1
                model.fp8: true
                model.fp8_e4m3: true
                model.fp8_hybrid: true
                model.fp8_params: true
                model.global_batch_size: 1024
                model.optim.grad_sync_dtype: bf16
                model.tensor_model_parallel_size: 2
                model.ub_tp_comm_overlap: false
                model.virtual_pipeline_model_parallel_size: 20


      internals:
        custom_executables:
          get_gpt2:
            template:
            - echo "Downloading GPT vocabulary files"
            - wget -P {experiment_run_dir} https://s3.amazonaws.com/models.huggingface.co/bert/gpt2-vocab.json
            - wget -P {experiment_run_dir} https://s3.amazonaws.com/models.huggingface.co/bert/gpt2-merges.txt
            redirect: ''
            log_file: ''
        executable_injection:
        - name: get_gpt2
          order: before

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
jq -r '["nemo_config","n_nodes","step","train_step_timing"], (.experiments[] as $exp | $exp.CONTEXTS[] as $context |
{
  name: $exp.RAMBLE_VARIABLES.nemo_config_name,
  workload: $exp.workload_name,
  n_nodes: $exp.n_nodes,
  Context: $context.name
} +
($context.foms | from_entries )
| select (.Context == "0-10/10")
| [.name, .n_nodes, .Context, .train_step_timing])
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
