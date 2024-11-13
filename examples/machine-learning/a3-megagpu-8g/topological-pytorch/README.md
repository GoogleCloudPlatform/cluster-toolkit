
# Topologically-aware Pytorch Distributed

This example demonstrates how to incorporate topology information into a
pytorch distributed workload.

Note: This requires that your nodes were created using a compact placement
policy.

The main concept is that pytorch should incorporate the information from topologically-aware Slurm into its `dist.init_process_group` function. [Slurm topology plugin is automatically configured for ClusterToolkit](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-topology.md).

## Quickstart

Run the following commands to demonstrate topologically aware pytorch:

    # Creates a local python3 env and installs pytorch
    jobid=$(sbatch --parsable install.sh)

    # Run an example of setting SLURM_HOSTFILE based on topology
    sbatch --dependency=afterok:$jobid topological_pytorch.sh

Once submitted, you should be able to view the state of the jobs with `squeue`:

    JOBID PARTITION     NAME     USER ST       TIME  NODES NODELIST(REASON)
    124    a3mega topologi username   PD       0:00      8 (Dependency)
    123    a3mega install. username    R       2:14      1 a3mega-a3meganodeset-0

Wait until job 124 is complete, then review the output in `slurm-124.out`.  It
will look something like this (illustative values used, your physical host will
have random characters):

    Standard
    rank    hostname        physical_host
    0       a3mega-a3meganodeset-0.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/00000000000000000000000000000000
    8       a3mega-a3meganodeset-1.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/dddddddddddddddddddddddddddddddd/11111111111111111111111111111111
    16      a3mega-a3meganodeset-2.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/22222222222222222222222222222222
    24      a3mega-a3meganodeset-3.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/cccccccccccccccccccccccccccccccc/33333333333333333333333333333333
    32      a3mega-a3meganodeset-4.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee/44444444444444444444444444444444
    40      a3mega-a3meganodeset-5.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/ffffffffffffffffffffffffffffffff/55555555555555555555555555555555
    48      a3mega-a3meganodeset-6.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/66666666666666666666666666666666
    54      a3mega-a3meganodeset-7.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/ffffffffffffffffffffffffffffffff/77777777777777777777777777777777
    Sorted by topology
    rank    hostname        physical_host
    0       a3mega-a3meganodeset-2.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/22222222222222222222222222222222
    8       a3mega-a3meganodeset-0.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/00000000000000000000000000000000
    16      a3mega-a3meganodeset-6.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/66666666666666666666666666666666
    24      a3mega-a3meganodeset-3.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/cccccccccccccccccccccccccccccccc/33333333333333333333333333333333
    32      a3mega-a3meganodeset-1.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/dddddddddddddddddddddddddddddddd/11111111111111111111111111111111
    40      a3mega-a3meganodeset-4.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee/44444444444444444444444444444444
    48      a3mega-a3meganodeset-5.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/ffffffffffffffffffffffffffffffff/55555555555555555555555555555555
    56      a3mega-a3meganodeset-7.c.<project>.internal      /CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC/ffffffffffffffffffffffffffffffff/77777777777777777777777777777777

Which shows that the ranks are ordered by the "rack" component of the `physical_host`.
See [here](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies#verify-vm-location)
for more information on compact placement policies.

## Detailed Explanation

### Setup

First we need to install pytorch. While these same concepts transfer to using
enroot/pyxis to launch containerized workloads, in this example we will just
use a local python environment:

    # Creates a local python3 env and installs pytorch
    sbatch install.sh

### Job Submission Script
Now let's review the `topological_pytorch.sh` batch job submission script.

First we set the requisite GPUDirect-TCPXO environment variables:

    NCCL_LIB_DIR="/var/lib/tcpxo/lib64" source /var/lib/tcpxo/lib64/nccl-env-profile.sh
    export NCCL_FASTRAK_CTRL_DEV=enp0s12
    export NCCL_FASTRAK_IFNAME=enp6s0,enp7s0,enp13s0,enp14s0,enp134s0,enp135s0,enp141s0,enp142s0
    export NCCL_SOCKET_IFNAME=enp0s12
    export NCCL_FASTRAK_LLCM_DEVICE_DIRECTORY=/dev/aperture_devices

and activate our python environment:

    source env/bin/activate

Next we demonstrate the standard behavior that torchrun would use, which does
not incorporate topology into how it orders ranks among the nodes.

    # Demonstrate standard behavior
    echo "Standard"
    # Set the MASTER_ADDR to the first node in the Slurm Job Nodelist
    export MASTER_ADDR=$(scontrol show hostnames $SLURM_JOB_NODELIST | head -n 1)
    # For torchrun, we only launch 1 task per node, and instruct torchrun to create
    # 8 (SLURM_GPUS_PER_NODE) processes per node.
    srun --ntasks-per-node=1 --nodes "${SLURM_NNODES}" \
        python -m torch.distributed.run \
        --nproc_per_node "${SLURM_GPUS_PER_NODE}" \
        --rdzv_endpoint "${MASTER_ADDR}":"${MASTER_PORT}" \
        --rdzv_backend c10d \
        --nnodes "${SLURM_NNODES}" topological_pytorch.py

torchrun will launch 8 tasks per node, and assign ranks lexiconographically
across nodes according to the hostnames.

For topologically-aware behavior, we launch all the tasks using Slurm's `srun`,
and will use the Slurm environment variables to initialize the torch distributed
process group, as we'll describe in the next section.

Note: [Topology aware Slurm is enabled by default in ClusterToolkit](https://github.com/GoogleCloudPlatform/cluster-toolkit/blob/main/docs/slurm-topology.md)

Slurm sets the `SLURM_PROCID` according to topology, which we will later use to
order NCCL ranks in Pytorch. The last thing we need to do is launch the job,
adding `--topology` to the script arguments to trigger the topology logic.

    srun python topological_pytorch.py --topology

Note: Alternatively you can set the required environment variables to be populated by Slurm in the srun command.

    srun sh -c "WORLD_SIZE=\${SLURM_NPROCS} RANK=\${SLURM_PROCID} LOCAL_RANK=\${SLURM_LOCALID} LOCAL_WORLD_SIZE=\${SLURM_NTASKS_PER_NODE} python topological_pytorch.py"

### Test Script
Next review the `topological_pytorch.py` script. There is a top level flag of
`--topology`, which controls whether pytorch is initialized using torchrun (when
`False`) or using Slurm (when `True`). The Slurm environment variables ensure
that the node ordering that Slurm uses gets translated to the Pytorch ranks.

    if args.topology:
        # These are populated by Slurm
        local_rank = int(os.environ["SLURM_LOCALID"])
        global_rank = int(os.environ["SLURM_PROCID"])
        world_size = int(os.environ["SLURM_NPROCS"])
        procs_per_node = int(os.environ["SLURM_NTASKS_PER_NODE"])

        # Must set rank and world_size based on SLURM_PROCID and SLURM_NPROCS
        dist.init_process_group("nccl", rank=global_rank, world_size=world_size)
    else:
        # These are populated by torchrun
        local_rank = int(os.environ["LOCAL_RANK"])
        global_rank = int(os.environ["RANK"])
        world_size = int(os.environ["WORLD_SIZE"])
        procs_per_node = int(os.environ["LOCAL_WORLD_SIZE"])

        # Torchrun handles rank allocation
        dist.init_process_group("nccl")

The remainder of the script is meant to demonstrate functionality. We use
`dist.all_gather_object` to collect the rank, hostname, and `physical_host` from
each pytorch worker, and then print the order out from global rank 0.  What you
should see is that depending on the topology that Slurm uses to launch the jobs,
the ordering of this output will vary.

### Running the Test

Run the following commands to demonstrate topologically aware pytorch:

    sbatch topological_pytorch.sh

The output shows the standard vs topologically-aware behavior. See
the Quickstart section above for an example.
