#!/bin/bash

BLOCKMESH_DIMENSIONS="100 40 40"    # 22 million case
#BLOCKMESH_DIMENSIONS="130 52 52"   # 42 million case
NTASKS=32                           # number of processes for meshing
DECOMPOSE="8 2 2"                   # mesh decomposition

# Copy case from OpenFOAM tutorial
cp -r $FOAM_TUTORIALS/incompressible/simpleFoam/motorBike .
cd motorBike

# Customise settings
foamDictionary -entry castellatedMeshControls.maxGlobalCells -set 200000000 system/snappyHexMeshDict
foamDictionary -entry blocks -set "( hex ( 0 1 2 3 4 5 6 7 ) ( $BLOCKMESH_DIMENSIONS ) simpleGrading ( 1 1 1 ) )" system/blockMeshDict
foamDictionary -entry numberOfSubdomains -set $NTASKS system/decomposeParDict
foamDictionary -entry hierarchicalCoeffs.n -set "( $DECOMPOSE )" system/decomposeParDict

# Copy and prepare geometry
cp $FOAM_TUTORIALS/resources/geometry/motorBike.obj.gz constant/triSurface/
surfaceFeatures 2>&1 | tee log.surfaceFeatures

# Generate and decompise base mesh
blockMesh 2>&1 | tee log.blockMesh
decomposePar -copyZero 2>&1 | tee log.decomposePar

# Run mesh generation in parallel
mpirun snappyHexMesh -parallel -overwrite 2>&1 | tee log.snappyHexMesh

# Reconstruct into a single mesh - it will be decomposed again when running solver
reconstructParMesh -constant 2>&1 | tee log.reconstructParMesh
rm -rf ./processor*
renumberMesh -constant -overwrite 2>&1 | tee log.renumberMesh

# Clean up
rm -rf ./processor*
cd ..
tar cvf motorBike.tar motorBike
bzip2 motorBike.tar
rm -rf motorBike
