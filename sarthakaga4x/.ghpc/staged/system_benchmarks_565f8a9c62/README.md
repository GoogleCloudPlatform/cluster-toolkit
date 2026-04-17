Running System Benchmarks with Ramble
=====================================

[Ramble](https://github.com/GoogleCloudPlatform/ramble) is an open source
multi-platform experimentation framework written in python. It can be used
to easily reproduce benchmark results across systems, and here
we will use it to run a series of system benchmarks.

Currently the following benchmarks are supported:

* NCCL tests (all-gather, all-reduce, reduce-scatter)

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

Cleaning Up
-----------

The ramble workspaces will be located in directories called `nccl-tests-*`.
The ramble codebase was installed to
`/opt/apps/ramble`.  Removing all of these directories will remove all of the
files generated during these tests.
