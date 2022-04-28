#!/bin/bash
#SBATCH --nodes=4
#SBATCH --ntasks-per-node=30
#SBATCH --time=01:00:00
#SBATCH --job-name=clckjob
#SBATCH --output=job_%j.log
#SBATCH --partition=compute
 
. /opt/intel/oneapi/setvars.sh

export CLCK_SHARED_TEMP_DIR=$HOME

cd $SLURM_SUBMIT_DIR

# dgemm_cpu_performance | sgemm_cpu_performance
FWD=dgemm_cpu_performance

# Change to _SGEMM_ if running sgemm_cpu_performance
CLCK_PROVIDER_DGEMM_MEMORY_USAGE=40 clck -D ${FWD}.db -F ${FWD} -l debug
