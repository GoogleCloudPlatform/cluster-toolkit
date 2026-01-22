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

# CHANGES REQUIRED TO SPACK PACKAGE FOR CHARMPP
#     https://github.com/spack/spack/issues/18535
# then...
# spack install namd @2.15a1 fftw=mkl ^charmpp backend=mpi arch=cascadelake
#
# ALSO, the NAMD package assumes that 'intel-mkl' sets up include directories like Spack expects
# It does not.
# Just yum install fftw-devel so that it can find an fftw3.h header...

# Reference: https://software.intel.com/content/www/us/en/develop/articles/recipe-build-and-run-namd-on-intel-xeon-processors-on-single-node.html

curl -O http://www.ks.uiuc.edu/Research/namd/utilities/stmv.tar.gz
tar xfz stmv.tar.gz
sed -i -e '/numsteps/s/500/1000/' stmv/stmv.namd
sed -i -e '/outputEnergies/s/20/600/' stmv/stmv.namd
sed -i -e 's/;.*$//' stmv/stmv.namd

GET_PERF="\$2==\"Benchmark\"{n++; s+=log(\$8); }END{print 1/exp(s/n)}"

# Run 1 rank per host
mpirun -N 1 -np "${SLURM_JOB_NUM_NODES}" namd2 +p "${SLURM_CPUS_ON_NODE}" +ppn "${SLURM_CPUS_ON_NODE}" +setcpuaffinity ./stmv/stmv.namd >namd-stmv.log 2>&1
res=$?

if [[ "$res" == 0 ]]; then
	kpi=$(awk "${GET_PERF}" <namd-stmv.log)
	echo "{\"result_unit\": \"ns/day\", \"result_value\": $kpi}" >kpi.json
fi
