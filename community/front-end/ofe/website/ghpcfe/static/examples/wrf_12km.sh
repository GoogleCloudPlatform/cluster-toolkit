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

# Spack doesn't place much into the environment for WRF
# But it does add to CMAKE_PREFIX_PATH
# Extract the package info from that
spack_hash=$(echo "$CMAKE_PREFIX_PATH" | sed -e 's/:/\n/g' | sort | uniq | awk -F- '/wrf-3.9.1.1/{print $NF}')
wrf_home=$(spack location -i "/${spack_hash}")

bench_dir=bench_12km

# SHOULD HAVE DOWNLOAD:
# gs://mcbench/datasets/WRFv3/bench_12km

if [ ! -d "${bench_dir}" ]; then
	print "Failed to launch - no ${bench_dir} dir!"
	exit 1
fi

if [ ! -x "$wrf_home/main/wrf.exe" ]; then
	print "Failed to launch - could not find wrf.exe"
	exit 1
fi

pushd "${bench_dir}" || exit
ln -s "$wrf_home/main/wrf.exe" .
ln -s "$wrf_home/run/RRTM_DATA" .

mpirun ./wrf.exe
res=$?

popd || exit

cp ${bench_dir}/namelist.* .
cp ${bench_dir}/rsl.* .
rm -rf "${bench_dir}"

if [[ "$res" == 0 ]]; then
	# Produce kpi.json
	simSecPerSec=$(grep 'Timing for main' rsl.error.0000 | awk '{SUM += $9} END {print 10800./SUM}')
	echo "{\"result_unit\": \"simSecPerSec\", \"result_value\": $simSecPerSec}" >kpi.json
fi
