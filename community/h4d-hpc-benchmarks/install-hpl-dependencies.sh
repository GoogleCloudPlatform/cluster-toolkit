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
# This script installs HPL and its dependencies on a cluster of VMs.
# All Cluster Director managed HPC images will come with these dependencies installed.
# This script is only needed if the cluster is provisioned using non-Cluster Director managed HPC images.
set -e

INSTALL_SCRIPT="$HOME/install_hpc_stack_payload.sh"

echo "Creating installation payload at $INSTALL_SCRIPT..."

# Write the payload into the shared home directory
cat << 'EOF' > "$INSTALL_SCRIPT"
#!/bin/bash
set -e
INSTALL_DIR="/opt"

SOURCE_MIRROR_DIR="/opt/source-code-mirror"

mkdir -p ${INSTALL_DIR}/spack ${INSTALL_DIR}/ramble ${SOURCE_MIRROR_DIR}
chmod 755 ${INSTALL_DIR}/spack ${INSTALL_DIR}/ramble ${SOURCE_MIRROR_DIR}

if [ ! -d "${INSTALL_DIR}/spack/.git" ]; then
    git clone -c feature.manyFiles=true https://github.com/spack/spack.git ${INSTALL_DIR}/spack
fi
if [ ! -d "${INSTALL_DIR}/ramble/.git" ]; then
    git clone -b v0.6.0 https://github.com/GoogleCloudPlatform/ramble.git ${INSTALL_DIR}/ramble
fi

source ${INSTALL_DIR}/spack/share/spack/setup-env.sh

# CAPTURE SOURCE CODE FOR COMPLIANCE
echo "Archiving GCC 14 and dependency source code..."
spack mirror create -d ${SOURCE_MIRROR_DIR} gcc@14.3.0

# INSTALL COMPILERS
echo "Installing GCC 14..."
spack install gcc@14.3.0
spack load gcc@14.3.0
spack compiler find

# INSTALL MPI & HPL
echo "Installing Intel MPI and HPL..."
spack install intel-oneapi-mpi@2021.17.2 %gcc@14
spack install hpl@2.3 +openmp ^amdblis threads=openmp ^intel-oneapi-mpi %gcc@14

python3 -m venv /opt/ramble/venv
source /opt/ramble/venv/bin/activate
pip3 install -r /opt/ramble/requirements.txt
deactivate

# CLEANUP
echo "Cleaning up caches..."
spack clean -a
spack gc -y

chmod -R 755 ${INSTALL_DIR}/spack ${INSTALL_DIR}/ramble ${SOURCE_MIRROR_DIR}
chown -R root:root ${INSTALL_DIR}/spack ${INSTALL_DIR}/ramble ${SOURCE_MIRROR_DIR}

echo "source ${INSTALL_DIR}/spack/share/spack/setup-env.sh" > /etc/profile.d/hpc-packages.sh
echo "source ${INSTALL_DIR}/ramble/share/ramble/setup-env.sh" >> /etc/profile.d/hpc-packages.sh
echo "spack load gcc@14.3.0" >> /etc/profile.d/hpc-packages.sh
EOF

# Make the payload executable
chmod +x "$INSTALL_SCRIPT"

# Dynamically find the default Slurm partition, node count, and the first node's name
PARTITION=$(sinfo -h -o "%P" | head -n 1 | tr -d '*')
NODE_COUNT=$(sinfo -h -p "$PARTITION" -o "%D")
FIRST_NODE=$(scontrol show hostnames "$(sinfo -h -p "$PARTITION" -o "%N" | head -n 1)" | head -n 1)

echo "Targeting Slurm partition: $PARTITION with $NODE_COUNT nodes."
echo ""
echo "================================================================="
echo "Starting parallel installation via srun..."
echo "This will block your current terminal. Please open a SECOND SSH"
echo "session to this login node to observe the progress."
echo ""
echo "# See the files that were created"
echo "ls -l install_progress_*.log"
echo ""
echo "# Watch the live output of the first compute node"
echo "tail -f install_progress_${FIRST_NODE}.log"
echo "================================================================="
echo ""

# Execute across all compute nodes, outputting to individual log files
srun --partition="$PARTITION" --nodes="$NODE_COUNT" --ntasks-per-node=1 \
     --output="install_progress_%N.log" \
     sudo "$INSTALL_SCRIPT"

echo "Installation complete across all active compute nodes!"
