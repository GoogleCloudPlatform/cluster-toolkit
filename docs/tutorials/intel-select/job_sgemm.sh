#!/bin/bash
#SBATCH --nodes=4
#SBATCH --ntasks-per-node=30
#SBATCH --time=01:00:00
#SBATCH --job-name=clckjob
#SBATCH --output=job_%j.log
#SBATCH --partition=compute

. /apps/clck/2019.10/bin/clckvars.sh

FWD=sgemm_cpu_performance
clck -D ${FWD}.db \
	-F ${FWD} \
	-l debug
