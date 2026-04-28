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
#
# OPTIMIZED FOR: AMD H4D Nodes
# USAGE: `chmod +x run-hpl-workload.sh && ./run-hpl-workload.sh [tcp|rxm] [n_nodes]`
set -eu

PROVIDER=${1:-tcp}
MPI_FABRICS=${MPI_FABRICS:-"shm:ofi"}

TAG=$(date +%s)
TEST_DIR=${PWD}/hpl-${PROVIDER}-${TAG}

# Detect the number of available nodes in the partition if not provided via override
N_NODES=${2:-$(sinfo -N -h -t idle,mix,alloc | wc -l | xargs)}

if [[ "$N_NODES" -eq 0 ]]; then
    echo "Error: Could not detect any available compute nodes in Slurm."
    exit 1
fi

# --- CONFIGURATION LOGIC ---
case "$PROVIDER" in
    "rxm")
        PROVIDER_NAME="RXM (ofi_rxm)"
        ENV_VARS_BLOCK=$(cat <<END
  env_vars:
    set:
      I_MPI_FABRICS: "${MPI_FABRICS}"
      FI_PROVIDER: "verbs;ofi_rxm"
      FI_VERBS_IFACE: "rdma0"
      FI_VERBS_MR_CACHE_ENABLE: "1"
      FI_LOG_LEVEL: "error"
      OMP_NUM_THREADS: "{n_threads}"
      OMP_PROC_BIND: "TRUE"
      OMP_PLACES: "cores"
END
)
        ;;
    "tcp")
        PROVIDER_NAME="TCP"
        ENV_VARS_BLOCK=$(cat <<END
  env_vars:
    set:
      I_MPI_FABRICS: "${MPI_FABRICS}"
      FI_PROVIDER: "tcp"
      FI_TCP_IFACE: "ens8"
      FI_LOG_LEVEL: "error"
      OMP_NUM_THREADS: "{n_threads}"
      OMP_PROC_BIND: "TRUE"
      OMP_PLACES: "cores"
END
)
        ;;
    *)
        echo "Error: Invalid provider '$PROVIDER'. Use [tcp|rxm]."
        exit 1
        ;;
esac

cat <<EOF
=========================================================
HPL Cluster Run: ORCHESTRATOR (${PROVIDER_NAME})
=========================================================
Workspace:      ${TEST_DIR}
Target Nodes:   ${N_NODES} Node(s) (h4d-standard-192/h4d-highmem-192/h4d-highmem-192-lssd)
Build OS:       Targeting Compute Node (Rocky Linux)
=========================================================
EOF

mkdir -p "${TEST_DIR}/configs_staging"

# Ramble Configuration (Hardware & MPI Specs)
cat <<EOF >"${TEST_DIR}"/configs_staging/ramble.yaml
ramble:
  variants:
    package_manager: spack
${ENV_VARS_BLOCK}
  variables:
    batch_submit: 'sbatch {execute_experiment}'

    # Define MPI implementation for Spack
    mpi_pkg_spec: intel-oneapi-mpi@2021.17.2

    mpirun_args: >-
      --mpi=pmi2
      -N {n_nodes}
      -n {n_ranks}
      -c {n_threads}
      --ntasks-per-node={processes_per_node}
      --cpu-bind=cores
      --distribution=block:block
    mpi_command: srun {mpirun_args}


  applications:
    hpl:
      workloads:
        calculator:
          experiments:
            hpl-${PROVIDER}-${N_NODES}-nodes:
              variables:
                hpl_path: "xhpl"
                processes_per_node: '192'
                memory_per_node: 762
                n_nodes: '${N_NODES}'
                n_threads: 1
                n_ranks: '{processes_per_node}*{n_nodes}'

                # HPL.dat Parameters
                N-NBMINs: '1'
                NBMINs: '4'
                NPFACTs: '1'
                PFACTs: '1'
                N-RFACTs: '1'
                RFACTs: '1'

  software:
    packages:
      impi:
        pkg_spec: intel-oneapi-mpi@2021.17.2 %gcc@14
      mpi:
        pkg_spec: '{mpi_pkg_spec} %gcc@14'
      hpl:
        pkg_spec: hpl@2.3 +openmp ^amdblis threads=openmp %gcc@14
    environments:
      hpl:
        packages:
        - mpi
        - hpl
EOF

# Execution Template (Slurm Job Script)
cat <<EOF >"${TEST_DIR}"/configs_staging/execute_experiment.tpl
#!/bin/bash
#SBATCH -J {experiment_name}
#SBATCH --output={experiment_run_dir}/slurm-%j.out
#SBATCH -N {n_nodes}
#SBATCH --exclusive
#SBATCH --ntasks-per-node={processes_per_node}
#SBATCH --cpus-per-task={n_threads}
#SBATCH --threads-per-core=1
#SBATCH --mem=0
#SBATCH --propagate=STACK,MEMLOCK
#SBATCH --export=ALL

cd "{experiment_run_dir}"

ulimit -s unlimited 2>/dev/null || ulimit -s \$(ulimit -H -s)
ulimit -l unlimited 2>/dev/null || ulimit -l \$(ulimit -H -l)
ulimit -n 65536

echo "--- Environment Setup ---"
source /opt/spack/share/spack/setup-env.sh
spack load hpl ^amdblis ^{mpi_pkg_spec} %gcc@14
spack load {mpi_pkg_spec} %gcc@14

echo "--- Starting HPL workload (${PROVIDER}) ---"
{mpi_command} "{hpl_path}" 2>&1 | tee "{log_file}"
EOF

# Create analyzer script
cat <<EOF >"${TEST_DIR}"/launch_analyzer.sh
#!/bin/bash
#SBATCH -N 1
#SBATCH -t 0:15:00
#SBATCH -J analyzer_hpl
#SBATCH -o ${TEST_DIR}/logs/analyzer_%j.out

echo "--- Phase 4: Analyzing Results ---"
cd "${TEST_DIR}"


echo "Extracting HPL results directly from Slurm logs..."
echo -e "Experiment\tN\tNB\tP\tQ\tTime(s)\tGflops" > summary.tsv

EXP_LOG_DIR="${TEST_DIR}/experiments/hpl/calculator/hpl-${PROVIDER}-${N_NODES}-nodes"
if ls "\$EXP_LOG_DIR"/slurm-*.out 1> /dev/null 2>&1; then
    awk '/^WR00C2C4/ {print "hpl-${PROVIDER}-${N_NODES}-nodes\t" \$2 "\t" \$3 "\t" \$4 "\t" \$5 "\t" \$6 "\t" \$7}' "\$EXP_LOG_DIR"/slurm-*.out >> summary.tsv
else
    echo "Error: Could not find slurm-*.out in \$EXP_LOG_DIR"
fi

echo ""
echo "--- HPL RUN SUMMARY ---"
column -t summary.tsv
EOF

# --- STEP 4: CREATE ORCHESTRATOR SCRIPT ---
cat <<EOF >"${TEST_DIR}"/launch_orchestrator.sh
#!/bin/bash
#SBATCH -N 1
#SBATCH -t 4:00:00
#SBATCH -J builder_hpl_${PROVIDER}
#SBATCH -o ${TEST_DIR}/logs/orchestrator_%j.out

set -e

SPACK_ROOT="/opt/spack"
RAMBLE_ROOT="/opt/ramble"

echo "--- Starting Orchestrator on \$(hostname) ---"

source /opt/ramble/venv/bin/activate

# Phase 1: Environment Setup
source "\${SPACK_ROOT}/share/spack/setup-env.sh"
source "\${RAMBLE_ROOT}/share/ramble/setup-env.sh"
if ! spack find -l | grep -q gcc@14.3.0; then
  spack load gcc@14.3.0
fi

cd "${TEST_DIR}"
ramble workspace create -a -d .

# Move staged configs
mv configs_staging/ramble.yaml configs/ramble.yaml
mv configs_staging/execute_experiment.tpl configs/execute_experiment.tpl
rmdir configs_staging

echo "Isolating Spack Configuration"
JOB_SPACK_CONFIG="${TEST_DIR}/spack_config_isolation"
mkdir -p "\$JOB_SPACK_CONFIG"
mkdir -p "${TEST_DIR}/spack_build_stage"

cat <<YAML > "\$JOB_SPACK_CONFIG/config.yaml"
config:
  install_tree:
    root: ${TEST_DIR}/spack_store
  source_cache: ${TEST_DIR}/spack_cache/source
  misc_cache: ${TEST_DIR}/spack_cache/misc
  test_cache: ${TEST_DIR}/spack_cache/test
  build_stage:
    - ${TEST_DIR}/spack_build_stage
YAML

# Redirect Bootstrap
cat <<YAML > "\$JOB_SPACK_CONFIG/bootstrap.yaml"
bootstrap:
  root: ${TEST_DIR}/spack_bootstrap
YAML

# Force Read-Only Access to System Spack
cat <<YAML > "\$JOB_SPACK_CONFIG/upstreams.yaml"
upstreams:
  system_spack:
    install_tree: /opt/spack/opt/spack
YAML

# ACTIVATE THE ISOLATION
export SPACK_USER_CONFIG_PATH="\$JOB_SPACK_CONFIG"
echo "Spack is now restricted to: \$JOB_SPACK_CONFIG"
echo "Registering Compiler in Job Scope"

if spack location -i gcc@14 > /dev/null 2>&1; then
    GCC_LOC=\$(spack location -i gcc@14 | head -n 1)
    echo "Found GCC 14 at: \$GCC_LOC"
    spack compiler find "\$GCC_LOC"
else
    echo "WARNING: Could not pin-point gcc@14. Scanning all common paths..."
    spack compiler find
fi

if ! spack compiler list | grep -q "gcc@14"; then
    echo "CRITICAL ERROR: GCC 14 not found in job scope."
    spack compiler list
    exit 1
fi

echo "Phase 2: Concretizing & Installing Software"
ramble workspace setup

echo "Phase 3: Submitting ${N_NODES}-Node Benchmark Job"
EXP_SCRIPT=\$(find "${TEST_DIR}/experiments" -name "execute_experiment" | head -n 1)

if [[ -f "\$EXP_SCRIPT" ]]; then
    echo "Found experiment script: \$EXP_SCRIPT"
    JOB_ID=\$(sbatch "\$EXP_SCRIPT" | awk '{print \$4}')
    echo ">>> HPL JOB SUBMITTED: ID \$JOB_ID <<<"

    ANALYZER_ID=\$(sbatch --dependency=afterok:\$JOB_ID "${TEST_DIR}/launch_analyzer.sh" | awk '{print \$4}')
    echo ">>> ANALYZER JOB SUBMITTED: ID \$ANALYZER_ID <<<"
else
    echo "Error: Experiment script not found. Build may have failed."
    exit 1
fi
EOF

echo "Submitting Orchestrator Job..."
mkdir -p "${TEST_DIR}/logs"
cd "${TEST_DIR}"

JOB_ID=$(sbatch launch_orchestrator.sh | awk '{print $4}')

echo ""
echo "========================================================="
echo "  Orchestrator Job Submitted: $JOB_ID"
echo "  Provider: $PROVIDER_NAME"
echo "========================================================="
echo "1. Monitoring Build: "
echo "tail -f ${TEST_DIR}/logs/orchestrator_${JOB_ID}.out"
echo "2. Check queue: watch squeue -u \$(whoami)"
echo "3. Monitor workload execution (Note: File appears after build completes):"
echo "tail -f ${TEST_DIR}/experiments/hpl/calculator/hpl-${PROVIDER}-${N_NODES}-nodes/slurm-*.out"
echo "4. Once complete, your results will be automatically parsed to:"
echo "   ${TEST_DIR}/summary.tsv"
echo "========================================================="
