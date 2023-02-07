#!/bin/bash
#SBATCH -N 2
#SBATCH --ntasks-per-node 30
#SBATCH --partition compute

PDB_FILE=1FJS.pdb
PROTEIN="${PDB_FILE%.*}"

echo $PDB_FILE
echo $PROTEIN

# Activate GROMACS environment
source /apps/spack/share/spack/setup-env.sh
spack env activate gromacs

# Check that gmx_mpi exists
which gmx_mpi

# Create hostname file
scontrol show hostnames "${SLURM_JOB_NODELIST}" >hostfile

# Prepare Inputs
grep -v -e HETATM -e CONECT ${PDB_FILE} >${PROTEIN}_protein.pdb

# Generate Topology
gmx_mpi pdb2gmx -f ${PROTEIN}_protein.pdb -o ${PROTEIN}_processed.gro -water tip3p -ff "charmm27"

# Solvate System
gmx_mpi editconf -f ${PROTEIN}_processed.gro -o ${PROTEIN}_newbox.gro -c -d 1.0 -bt dodecahedron
gmx_mpi solvate -cp ${PROTEIN}_newbox.gro -cs spc216.gro -o ${PROTEIN}_solv.gro -p topol.top

# Add ions
gmx_mpi grompp -f config/ions.mdp -c ${PROTEIN}_solv.gro -p topol.top -o ions.tpr
printf "SOL\n" | gmx_mpi genion -s ions.tpr -o ${PROTEIN}_solv_ions.gro -conc 0.15 -p topol.top -pname NA -nname CL -neutral

# Launch MPI jobs

# Run energy minimization
gmx_mpi grompp -f config/emin-charmm.mdp -c ${PROTEIN}_solv_ions.gro -p topol.top -o em.tpr
mpirun -n 60 -hostfile hostfile -ppn 30 gmx_mpi mdrun -v -deffnm em

# Run temperature equilibration
gmx_mpi grompp -f config/nvt-charmm.mdp -c em.gro -r em.gro -p topol.top -o nvt.tpr
mpirun -n 60 -hostfile hostfile -ppn 30 gmx_mpi mdrun -v -deffnm nvt

# Run pressure equilibration
gmx_mpi grompp -f config/npt-charmm.mdp -c nvt.gro -r nvt.gro -t nvt.cpt -p topol.top -o npt.tpr
mpirun -n 60 -hostfile hostfile -ppn 30 gmx_mpi mdrun -v -deffnm npt

# Run production run
gmx_mpi grompp -f config/md-charmm.mdp -c npt.gro -t npt.cpt -p topol.top -o md.tpr
mpirun -n 60 -hostfile hostfile -ppn 30 gmx_mpi mdrun -v -deffnm md

# Post process trajectory
printf "1\n1\n" | gmx_mpi trjconv -s md.tpr -f md.xtc -o md_center.xtc -center -pbc mol
