#!/bin/bash
# Lysozyme example (c) by Google LLC
# Derived from Justin Lemhul (http://www.mdtutorials.com) - licensed under CC-BY-4.0 &
# Alessandra Villa (https://tutorials.gromacs.org/) - licensed under CC-BY-4.0
#
# Lysozyme example is licensed under a
# Creative Commons Attribution 4.0 International (CC BY 4.0) License.
# See <https://creativecommons.org/licenses/by/4.0/>.

#SBATCH -N 1
#SBATCH --ntasks-per-node 1
#SBATCH --partition gpu
#SBATCH --gpus 1

PDB_FILE=1AKI.pdb
PROTEIN="${PDB_FILE%.*}"

echo "$PDB_FILE"
echo "$PROTEIN"

# Activate GROMACS environment
source /apps/spack/share/spack/setup-env.sh
spack env activate gromacs

# Check that gmx_mpi exists
which gmx_mpi

# Prepare Inputs
# Note: The protein in this example has been deliberately chosen because it requires minimal preparation.
# This procedure is not universally appropriate (e.g., for proteins with missing residues, occupancies less than 1, etc).
grep -v -e HETATM -e CONECT "${PDB_FILE}" >"${PROTEIN}"_protein.pdb

# Generate Topology
mpirun -n 1 gmx_mpi pdb2gmx -f "${PROTEIN}"_protein.pdb -o "${PROTEIN}"_processed.gro -water tip3p -ff "charmm27"

# Solvate System
mpirun -n 1 gmx_mpi editconf -f "${PROTEIN}"_processed.gro -o "${PROTEIN}"_newbox.gro -c -d 1.0 -bt dodecahedron
mpirun -n 1 gmx_mpi solvate -cp "${PROTEIN}"_newbox.gro -cs spc216.gro -o "${PROTEIN}"_solv.gro -p topol.top

# Add Ions
mpirun -n 1 gmx_mpi grompp -f config/ions.mdp -c "${PROTEIN}"_solv.gro -p topol.top -o ions.tpr
printf "SOL\n" | mpirun -n 1 gmx_mpi genion -s ions.tpr -o "${PROTEIN}"_solv_ions.gro -p topol.top -pname NA -nname CL -neutral

MDRUN_GPU_PARAMS=(-gputasks 00 -bonded gpu -nb gpu -pme gpu -update gpu)
MDRUN_MPIRUN_PREAMBLE=(mpirun -n 1 -H localhost env GMX_ENABLE_DIRECT_GPU_COMM=1)

# Run Energy Minimization
mpirun -n 1 gmx_mpi grompp -f config/emin-charmm.mdp -c "${PROTEIN}"_solv_ions.gro -p topol.top -o em.tpr
"${MDRUN_MPIRUN_PREAMBLE[@]}" gmx_mpi mdrun -v -deffnm em

# Run Temperature Equilibration
mpirun -n 1 gmx_mpi grompp -f config/nvt-charmm.mdp -c em.gro -r em.gro -p topol.top -o nvt.tpr
"${MDRUN_MPIRUN_PREAMBLE[@]}" gmx_mpi mdrun -v -deffnm nvt "${MDRUN_GPU_PARAMS[@]}"

# Run Pressure Equilibration
mpirun -n 1 gmx_mpi grompp -f config/npt-charmm.mdp -c nvt.gro -r nvt.gro -t nvt.cpt -p topol.top -o npt.tpr
"${MDRUN_MPIRUN_PREAMBLE[@]}" gmx_mpi mdrun -v -deffnm npt "${MDRUN_GPU_PARAMS[@]}"

# Run Production Run
mpirun -n 1 gmx_mpi grompp -f config/md-charmm.mdp -c npt.gro -t npt.cpt -p topol.top -o md.tpr
"${MDRUN_MPIRUN_PREAMBLE[@]}" gmx_mpi mdrun -v -deffnm md "${MDRUN_GPU_PARAMS[@]}"

# Post Process Trajectory
printf "1\n" | mpirun -n 1 gmx_mpi trjconv -s md.tpr -f md.xtc -o md_protein.xtc -pbc mol
printf "1\n1\n" | mpirun -n 1 gmx_mpi trjconv -s md.tpr -f md_protein.xtc -fit rot+trans -o md_fit.xtc

# Copy Protein and Trajectory to Output Directory
cp "${PROTEIN}"_newbox.gro /data_output/
cp md_fit.xtc /data_output/"${PROTEIN}"_md.xtc
