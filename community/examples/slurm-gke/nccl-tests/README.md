The examples in this directory are used to show how enroot + pyxis can be used
to launch containerized workloads via Slurm running on GKE.

Contents:

* `build-nccl-tests.sh`: A Slurm batch script for building the nccl-tests.
* `run-nccl-tests-rdma.sh`: A Slurm batch script for running the nccl-tests for A3 Ultra, A4, A4X with RDMA.
  `all_gather_perf` benchmark.
* `run-nccl-tests-tcpxo.sh`: A Slurm batch script for running the nccl-tests for A3 Mega with TCPXO.
  `all_gather_perf` benchmark.

# Running NCCL-Tests via Enroot/Pyxis

In general the workflow to deploy GPUDirect-RDMA-enabled workloads via enroot-pyxis is
the following:

1. Convert your container into a squashfs based container image
2. Set required environment variables
3. Run your application workload

## TLDR

For an end-to-end example, copy the `build-nccl-tests.sh` and
`run-nccl-tests-rdma.sh` or `run-nccl-tests-tcpxo.sh` to your login node.

And run the following:

```text
BUILD_JOB=$(sbatch --parsable build-nccl-tests.sh) # takes ~4 minutes
sbatch -d afterok:${BUILD_JOB} run-nccl-tests-rdma.sh # takes ~3 minutes
```

The latter should result in a slurm-XX.out file that contains the result of the nccl
`all_gather_perf` benchmark:

```text
#
#                                                              out-of-place                       in-place
#       size         count      type   redop    root     time   algbw   busbw #wrong     time   algbw   busbw #wrong
#        (B)    (elements)                               (us)  (GB/s)  (GB/s)            (us)  (GB/s)  (GB/s)
   268435456       4194304     float    none      -1    XXXXX  XXX.XX  XXX.XX    N/A   XXXXXX  XXX.XX  XXX.XX      0
   536870912       8388608     float    none      -1    XXXXX  XXX.XX  XXX.XX    N/A   XXXXXX  XXX.XX  XXX.XX      0
  1073741824      16777216     float    none      -1    XXXXX  XXX.XX  XXX.XX    N/A   XXXXXX  XXX.XX  XXX.XX      0
  2147483648      33554432     float    none      -1    XXXXX  XXX.XX  XXX.XX    N/A   XXXXXX  XXX.XX  XXX.XX      0
  4294967296      67108864     float    none      -1    XXXXX  XXX.XX  XXX.XX    N/A   XXXXXX  XXX.XX  XXX.XX      0
  8589934592     134217728     float    none      -1    XXXXX  XXX.XX  XXX.XX    N/A   XXXXXX  XXX.XX  XXX.XX      0
# Out of bounds values : 0 OK
# Avg bus bandwidth    : XXX.XX
#
```

For more details, follow the remainder of this README.

## Detailed Instructions

All of the following should be done on the login node of your slurm cluster,
and while somewhere on the shared Filestore filesystem (typically the user's
home directory).

### Building NCCL-tests

See build-nccl-tests.sh for an example. Within it, you will see that first we'll
create a squashfs version of the container using we want to launch using `enroot
import`. We do this because otherwise we'd be pulling the (typically more than
10GB) image multiple times from the source on each node, converting to sqsh each
time, etc, which would make the job launch longer.

For building the nccl-tests binaries, we use `pyxis` to run the enroot container
and build the nccl-tests within that container to ensure the resulting binaries
are compatible with the container environment.

Both of the above (importing and building) are accomplished by running:

```text
sbatch build-nccl-tests.sh
```

### Running your application on a3-mega instances

For a complete example, run:

```text
sbatch run-nccl-tests-tcpxo.sh
```

### Running your application on a3-ultra instances

For a complete example, run:

```text
sbatch run-nccl-tests-rdma.sh
```

The output will appear in in a `slurm-<job#>.log` file. If the name of your a3-ultragpu
partition is different than "gke", you will need to modify the `build-nccl-tests.sh`
and `run-nccl-tests-*.sh` scripts  `#SBATCH --partition` setting. Alternatively, you
can run `sbatch -p <your partition> <script>`.
