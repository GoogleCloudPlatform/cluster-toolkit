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

# Mesh was previously generated and saved. Refer to the separate meshing script.
tar xvf motorBike.tar.bz2
pushd motorBike || exit
NODES=${SLURM_JOB_NUM_NODES}
PPN=${SLURM_CPUS_ON_NODE} # or underpopulate if needed
NTASKS=$((NODES * PPN))

# Customise settings to decompose the mesh
foamDictionary -entry numberOfSubdomains -set "$NTASKS" system/decomposeParDict
foamDictionary -entry method -set multiLevel system/decomposeParDict
foamDictionary -entry multiLevelCoeffs -set "{}" system/decomposeParDict
foamDictionary -entry scotchCoeffs -set "{}" system/decomposeParDict
foamDictionary -entry multiLevelCoeffs.level0 -set "{}" system/decomposeParDict
foamDictionary -entry multiLevelCoeffs.level0.numberOfSubdomains -set "$NODES" system/decomposeParDict
foamDictionary -entry multiLevelCoeffs.level0.method -set scotch system/decomposeParDict
foamDictionary -entry multiLevelCoeffs.level1 -set "{}" system/decomposeParDict
foamDictionary -entry multiLevelCoeffs.level1.numberOfSubdomains -set "$PPN" system/decomposeParDict
foamDictionary -entry multiLevelCoeffs.level1.method -set scotch system/decomposeParDict

# Customise solver algorithms
foamDictionary -entry solvers.p.nPreSweeps -set 0 system/fvSolution
foamDictionary -entry solvers.p.nPostSweeps -set 2 system/fvSolution
foamDictionary -entry solvers.p.cacheAgglomeration -set on system/fvSolution
foamDictionary -entry solvers.p.agglomerator -set faceAreaPair system/fvSolution
foamDictionary -entry solvers.p.nCellsInCoarsestLevel -set 10 system/fvSolution
foamDictionary -entry solvers.p.mergeLevels -set 1 system/fvSolution
foamDictionary -entry relaxationFactors.equations.U -set 0.7 system/fvSolution
foamDictionary -entry relaxationFactors.fields -add "{}" system/fvSolution
foamDictionary -entry relaxationFactors.fields.p -set 0.3 system/fvSolution

# Decompose the mesh
decomposePar -copyZero 2>&1 | tee log.decomposeParMultiLevel

# Run the solver
foamDictionary -entry writeInterval -set 1000 system/controlDict
foamDictionary -entry runTimeModifiable -set "false" system/controlDict
foamDictionary -entry functions -set "{}" system/controlDict
foamDictionary -entry endTime -set 250 system/controlDict
mpirun potentialFoam -parallel 2>&1 | tee log.potentialFoam
mpirun simpleFoam -parallel 2>&1 | tee log.simpleFoam

# Extract the total execution time as KPI
popd || exit
kpi=$(tail -n 5 motorBike/log.simpleFoam | head -n1 | awk '{print $3}')
echo "{\"result_unit\": \"seconds\", \"result_value\": $kpi}" >kpi.json
