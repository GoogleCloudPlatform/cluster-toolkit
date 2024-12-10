The examples in this directory are used to show how enroot + pyxis can be used
to launch containerized workloads via Slurm.

Contents:

* `import_container.sh`: Uses enroot to create a squashfs container image.
* `build-nccl-tests.sh`: A Slurm batch script for building the nccl-tests.
* `run-nccl-tests.sh`: A Slurm batch script for running the nccl-tests
  `all_reduce_perf` benchmark.

# Running NCCL-Tests via Enroot/Pyxis

In general the workflow to deploy GPUDirect-RDMA-enabled workloads via enroot-pyxis is
the following:

	1. Convert your container into a squashfs based container image
	2. Set required environment variables
	3. Run your application workload

## TLDR

For an end-to-end example, copy the `build-nccl-tests.sh` and
`run-nccl-tests.sh` to your login node.

And run the following:

	sbatch build-nccl-tests.sh # takes ~4 minutes
	sbatch run-nccl-tests.sh # takes ~3 minutes

That should result in a slurm-XX.out file that contains the result of the nccl
`all_gather_perf` benchmark:

```
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

## Convert container to squashfs image

All of the following should be done on the login node of your slurm cluster,
and while somewhere on the shared Filestore filesystem (typically the user's
home directory).

First we'll want to create a squash-fs version of the container we want to
launch. We do this because otherwise we'd be pulling the (typically >10GB)
image multiple times from the source on each node, converting to sqsh each
time, etc, which would make the job launch longer. For example, to use nvidia's
latest pytorch container, we run:

	enroot import docker://nvcr.io#nvidia/pytorch:24.09-py3

This will create a (large) file named "nvidia+pytorch+24.09-py3.sqsh".

## Building NCCL-tests

For building the nccl-tests binaries, we spawn a job that runs on the a3-ultra nodes
within the same application container. See build-nccl-tests.sh for an example,
which can be run with:

       sbatch build-nccl-tests.sh

## Running your application on a3-ultra instances

For a complete example, run:

	sbatch run-nccl-tests.sh

The output will appear in in a `slurm-<job#>.log` file. If the name of your a3-ultragpu
nodeset is different than "a3ultra", you will need to modify the `build-nccl-tests.sh`
and `run-nccl-tests.sh` scripts's  `#SBATCH --partition` setting. Alternativel, you
can run `sbatch -p <your partition> <script>`.
