Running System Benchmarks with Ramble
=====================================

[Ramble](https://github.com/GoogleCloudPlatform/ramble) is an open source
multi-platform experimentation framework written in python. It can be used
to easily reproduce benchmark results across systems, and here
we will use it to run a series of system benchmarks.

Currently the following benchmarks are supported:

* NCCL tests (all-gather, all-reduce, reduce-scatter)
* HPL-NVIDIA
* Mixtral 8x7b and LLama3.1 70B via NeMo

All benchmarks use Kueue for topology aware scheduling, and use JobSet
to orchestrate multi-node workloads.

For NCCL tests, run:

   ```bash
   kubectl apply -f ramble-nccl.yaml
   ```

For HPL tests, run:

   ```bash
   kubectl apply -f ramble-hpl.yaml
   ```

For NeMo tests, run:

   ```bash
   kubectl apply -f ramble-nemo.yaml
   ```

Where applicable, the NeMo workloads configurations have been chosen to
reproduce those found in
[AI-Hypercomputer/gpu-recipes](https://github.com/AI-Hypercomputer/gpu-recipes).

For any of the above, the following will be created:

* A `ramble` namespace in your K8s cluster
* A Kueue `LocalQueue` in the `ramble` namespace.
* A "ramble" service account (and associated RBAC configs) that has access to
  the core, batch, jobset, and kueue apis in the `ramble` namespace, as well as
  read access to the kueue "clusterqueues" resources across the cluster.
* Configmaps to various scripts/configurations.
* A K8s `Job` that works as the ramble controller process, which creates a
  series of `Jobset` objects for each individual benchmark.

Once created, this will first create a K8s job called
"ramble-{nccl,hpl,nemo}-runner" in the ramble workspace. This controller job
orchestrates the running and analysis of the benchmarks. It installs everything
it needs within a self-contained pod, creates an ssh keypair for multi-node
communication, and uses Ramble to create JobSet's for each benchmark. Once those
benchmarks are complete, it provides a summary of the results. Full benchmark
logs can otherwise be found in the logs for each of the created JobSet/Job/Pod's
themselves.

If you were to run all of the above commands, you would initially see something
like this:

   ```bash
   $ kubectl -n ramble get jobs
   NAME                 STATUS    COMPLETIONS   DURATION   AGE
   ramble-hpl-runner    Running   0/1           30s        30s
   ramble-nccl-runner   Running   0/1           43s        43s
   ramble-nemo-runner   Running   0/1           22s        22s
   ```

For each benchmark, multiple node scales will be submitted, up to your maximum
node scale of your cluster.  This can be controlled with the `n_nodes` variable
in the `ramble.yaml` configMap.

Note: The benchmarks depends on several tightly coupled settings, in particular
making sure that the subnet names in your GKE cluster match those defined in
the "ramble.yaml" config file. If you modify the names of your subnets
(including by changing the "deployment" name), then you will need to modify
the K8s yaml files. Specifically, the following variables may need to be
modified in the `ramble.yaml` configmap in each of the
ramble-{nccl,hpl,nemo}.yaml files:

        gke_nodepool: a3-ultragpu-8g-a3-ultragpu-pool  # The nodepool name
        sysnet_subnet_prefix: a3u-gke-gcs-sub
        gpu_subnet_prefix: a3u-gke-gcs-rdma-sub
        cluster_queue: a3u

Viewing the Results
-------------------

For ramble-nccl.yaml, at the end of the logs of the created `ramble-nccl-runner`
job, you should see something like:

   ```bash
   kubectl -n ramble logs job/ramble-nccl-runner
   ...
   ---- SUMMARY for >1GB Message Sizes ----
   workload        n_nodes msg_size        busbw
   all-gather      2       1073741824      XXX.XX
   all-gather      2       2147483648      XXX.XX
   all-gather      2       4294967296      XXX.XX
   all-gather      2       8589934592      XXX.XX
   ...
   all-reduce      2       1073741824      XXX.XX
   ...
   reduce-scatter  2       1073741824      XXX.XX
   ...

   -------- Benchmarking Complete -------
   ```

   ```bash
   kubectl -n ramble logs job/ramble-hpl-runner
   ...
   --------------- SUMMARY ---------------
   workload        n_nodes GFlop/s         GFlops/s/GPU
   calculator      1       X.XXXe+05       X.XXXe+04
   calculator      2       X.XXXe+05       X.XXXe+04
   calculator      4       X.XXXe+06       X.XXXe+04
   calculator      8       X.XXXe+06       X.XXXe+04

   -------- Benchmarking Complete -------
   ```

   ```bash
   kubectl -n ramble logs job/ramble-nemo-runner
   ...
   --------------- SUMMARY ---------------
   nemo_config     n_nodes step    train_step_timing
   mixtral_8x7b    8       0-10/10 XX.XX
   llama3_1_70b    8       0-10/10 XX.XX

   -------- Benchmarking Complete -------
   ```

Cleaning Up
-----------

To remove all resources created by these benchmarks, you can run:

    kubectl delete -f ramble-nccl.yaml
    kubectl delete -f ramble-hpl.yaml
    kubectl delete -f ramble-nemo.yaml
