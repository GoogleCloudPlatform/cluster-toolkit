# HPL Workload README

This guide provides instructions for using the `run-hpl-workload.sh` script to
deploy, compile, and execute the High-Performance Linpack (HPL) benchmark on H4D
VM clusters.

## Overview

The workload script automates a three-job Slurm pipeline to verify baseline
computational performance and interconnect efficiency:

*   **Orchestrator Job:** Isolates the Spack environment and compiles the HPL
    binary directly on a compute node to ensure OS compatibility.
*   **Workload Job:** Executes the core HPL math benchmark using the underlying
    RDMA hardware for high-bandwidth, low-latency communication.
*   **Analyzer Job:** Scrapes Slurm logs to extract the final Gflops metric into
    a clean `summary.tsv` file.

## Usage

### 1. Install Dependencies

The `install-hpl-dependencies.sh` script installs HPL and its dependencies (such
as Spack, Ramble, GCC 14, and Intel MPI) across all compute nodes in the default
Slurm partition. It uses `srun` to execute the installation in parallel on all
nodes of the default partition, creating log files named
`install_progress_[NODE_NAME].log` for each node in your home directory.

```bash
chmod +x install-hpl-dependencies.sh
./install-hpl-dependencies.sh
```

> [!WARNING]
> The script will block the current terminal during execution. You can monitor
> progress by opening a second SSH session to the login node and tailing one of
> the log files (e.g., `tail -f install_progress_[NODE_NAME].log`).

### 2. Preparation

Copy the workload script to your login node's shared directory and grant
execution permissions:

```bash
chmod +x run-hpl-workload.sh
```

### 3. Running the script

Run the script with the following arguments: `./run-hpl-workload.sh [provider]
[n_nodes]`

*   **provider**: network provider (see table below). Defaults to `tcp`.
*   **n_nodes**: number of nodes to run HPL on. Defaults to all available nodes
    in Slurm.

| Provider | Argument | Description                                          |
| :------- | :------- | :--------------------------------------------------- |
| **RXM**  | `rxm`    | Uses RDMA via `ofi_rxm` for highest GFLOPS and best latency. |
| **TCP**  | `tcp`    | (Default) Standard TCP/IP sockets. Useful for debugging; lowest performance. |

**Example Commands:**

Run on all nodes with RXM provider: `bash ./run-hpl-workload.sh rxm`

Run on 16 nodes with RXM provider: `bash ./run-hpl-workload.sh rxm 16`

## Monitoring Progress

Upon submission, the script will provide specific tail commands to track each
phase:

*   **Build Phase:** `tail -f
    ./hpl-[provider]-[tag]/logs/orchestrator_[JOB_ID].out`
*   **Workload Phase:** `tail -f
    ./hpl-[provider]-[tag]/experiments/hpl/calculator/.../slurm-*.out`
*   **Queue Status:** `watch squeue -u $(whoami)`

## Results

Once the pipeline completes, the Analyzer job generates a summary. Navigate to
your test directory and use the following command to view the results:

```bash
cd hpl-[provider]-[tag]
column -t summary.tsv
```
