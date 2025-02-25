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

The run scripts have all been staged into `/opt/apps/system_benchmarks`
on the controller node (and available to all nodes). We recommend running
them using `nohup` and redirecting the stdout/err to a logfile, as the tests can
take 30-60 minutes or longer if other jobs are in the queue. The results can
then be viewed by `tail`ing the log file.

For NCCL tests, run:

   ```bash
   nohup bash /opt/apps/system_benchmarks/run-nccl-tests-via-ramble.sh >& nccl-$(date -Iseconds).log &
   tail -f nccl-*.log
   ```

For HPL tests, run:

   ```bash
   nohup bash /opt/apps/system_benchmarks/run-hpl-via-ramble.sh >& hpl-$(date -Iseconds).log &
   tail -f hpl-*.log
   ```

For NeMo tests, run:

   ```bash
   nohup bash /opt/apps/system_benchmarks/run-nemo-via-ramble.sh >& nemo-$(date -Iseconds).log &
   tail -f nemo-*.log
   ```

Where applicable, the NeMo workloads configurations have been chosen to
reproduce those found in
[AI-Hypercomputer/gpu-recipes](https://github.com/AI-Hypercomputer/gpu-recipes).

For each benchmark, multiple node scales will be submitted, up to your maximum
node scale of your cluster.

Viewing the Results
-------------------

For nccl, at the end of the nccl-$(date -Iseconds).log,
you should see something like:

   ```bash
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

For hpl, you should see something like:

   ```bash
   ...
   --------------- SUMMARY ---------------
   workload        n_nodes GFlop/s         GFlops/s/GPU
   calculator      1       X.XXXe+05       X.XXXe+04
   calculator      2       X.XXXe+05       X.XXXe+04
   calculator      4       X.XXXe+06       X.XXXe+04
   calculator      8       X.XXXe+06       X.XXXe+04

   -------- Benchmarking Complete -------
   ```

For nemo, you should see something like:

   ```bash
   ...
   --------------- SUMMARY ---------------
   nemo_config     n_nodes step    train_step_timing
   mixtral_8x7b    8       0-10/10 XX.XX
   llama3_1_70b    8       0-10/10 XX.XX

   -------- Benchmarking Complete -------
   ```

Cleaning Up
-----------

The ramble workspaces will be located in directories called `nccl-tests-*`,
`hpl-tests-*`, and `nemo-tests-*`. The ramble codebase was installed to
`/opt/apps/ramble`.  Removing all of these directories will remove all of the
files generated during these tests.
