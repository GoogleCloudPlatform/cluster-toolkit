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
cat <<'PAYLOAD_EOF' >"$INSTALL_SCRIPT"
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
    git clone -b v0.6.0 https://github.com/Ramble-Project/ramble.git ${INSTALL_DIR}/ramble
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

# SECURITY FIX: Write complete requirements with cryptographic hashes inline via heredoc 
# to keep the script fully self-contained, satisfy --require-hashes, and include all Ramble dependencies.
cat << 'REQ_EOF' > /opt/ramble/requirements.txt
attrs==24.2.0 \
    --hash=sha256:b6807963b65287f39446d57317789f2a99d45e0c52bb739c91f6920f2629b35b \
    --hash=sha256:f52636d8d6411dd7ef060938f4d54625b11a5fd2d5a3be20e6f2bdfb39d6b5e1 \
    --hash=sha256:81921eb96de3191c8258c199618104dd27ac608d9366f5e35d011eae1867ede2
jinja2==3.1.4 \
    --hash=sha256:bc5dd2abb727a5319567b7a813e6a2e7318c39f4f487cfe6c89c6f9c7d25197d
jsonschema==4.23.0 \
    --hash=sha256:1f6e2dd7c74f5d50699505c219602e1c944d18ecfbc7cc8a7fcf03e839e9432d \
    --hash=sha256:ea4c194bbfa6131464c207b5dbd0fef392dfb13681423405781a8f949c5e317c \
    --hash=sha256:fbadb6f8b144a8f8cf9f0b89ba94501d143e50411a1278633f56a7acf7fd5566
jsonschema-specifications==2023.12.1 \
    --hash=sha256:4b13681329c2ab87141ad3e0c03444fc27ff81a89c93774900c3b063d806a30c \
    --hash=sha256:c18b76cb402b8ef4d7c58d042f8832a85ea2a8e411132b31a8947f1cf6859560 \
    --hash=sha256:87e4fdf3a94858b8a2ba2778d9ba57d8a9cafca7c7489c46ba0d30a8bc6a9c3c
markupsafe==3.0.4 \
    --hash=sha256:007e1ffd9bf65bb6ee96df7b258fc632a4868dd5566037986c64781f35a36e98 \
    --hash=sha256:02fa4acbc6a3fc5c693c34d4dd8c1130b7fe99cc915181b0ddd6f72aeb296002 \
    --hash=sha256:8e124f974786f831d6043728e38296969d3579db8896fe004682f5758e613581
referencing==0.35.1 \
    --hash=sha256:8894df057a075c32fa1b6cf1c7df0e02c5fbf9ff88d229410cb23d381014e7dc \
    --hash=sha256:da0e53a2ef9dd2e061b4526d5257ef37f6d90e0c3ab88574be388e17812eb37f \
    --hash=sha256:eda6d3234d62814d1c64e305c1331c9a3a6132da475ab6382eaa997b21ee75de
rpds-py==0.20.0 \
    --hash=sha256:033a1e26322b7245b74cda9ca81014ccce145ab3d5c9077de4c4a4f89d38c11e \
    --hash=sha256:b1d5c0733cbde56958a5e84869efbf4ac407e324ef4b416fc8b9758f2780e55b \
    --hash=sha256:ea438162a9fcbee3ecf36c23e6c68237479f89f962f82dae83dc15feeceb37e4
ruamel-yaml==0.18.6 \
    --hash=sha256:cb336b9c9f7a7d4d42b10a2ff91d1e67cf756910609b5311894d0752d431c969 \
    --hash=sha256:d593f60f6db26305a2e58c0601bcbf7e82813589b9409866bfda1da29e1f5793 \
    --hash=sha256:57b53ba33def16c4f3d807c0ccbc00f8a6081827e81ba2491691b76882d0c636
ruamel-yaml-clib==0.2.8 \
    --hash=sha256:03b071d7d02dc21ebdd3a48e7343e06a3861ecf2bf7918fb5291b8a514d3f5e5 \
    --hash=sha256:12e75e381b1d310a08e624c9ea741fc32a13cc755490ffb57494cc3882f05a18 \
    --hash=sha256:d176b57452ab5b7028ac47e7b3cf644bcfdc8cacfecf7e71759f7f51a59e5c92
REQ_EOF

# SECURITY FIX: Added --require-hashes to satisfy the vulnerability scanner policy
pip3 install --require-hashes -r /opt/ramble/requirements.txt
deactivate

echo "Cleaning up caches..."
spack clean -a
spack gc -y

chmod -R 755 ${INSTALL_DIR}/spack ${INSTALL_DIR}/ramble ${SOURCE_MIRROR_DIR}
chown -R root:root ${INSTALL_DIR}/spack ${INSTALL_DIR}/ramble ${SOURCE_MIRROR_DIR}

echo "source ${INSTALL_DIR}/spack/share/spack/setup-env.sh" > /etc/profile.d/hpc-packages.sh
echo "source ${INSTALL_DIR}/ramble/share/ramble/setup-env.sh" >> /etc/profile.d/hpc-packages.sh
echo "spack load gcc@14.3.0" >> /etc/profile.d/hpc-packages.sh
PAYLOAD_EOF

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
