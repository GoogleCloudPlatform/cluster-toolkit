#!/bin/bash

# Spack doesn't place much into the environment for WRF
# But it does add to CMAKE_PREFIX_PATH
# Extract the package info from that
spack_hash=$(echo "$CMAKE_PREFIX_PATH" | sed -e 's/:/\n/g' | sort | uniq | awk -F- '/wrf-3.9.1.1/{print $NF}')
wrf_home=$(spack location -i "/${spack_hash}")

bench_dir=bench_2.5km

# SHOULD HAVE DOWNLOAD:
# gs://mcbench/datasets/WRFv3/bench_2.5km

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
ln -s "$wrf_home/run/VEGPARM.TBL" .
ln -s "$wrf_home/run/SOILPARM.TBL" .
ln -s "$wrf_home/run/GENPARM.TBL" .

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
