# Advanced Slurm Configurations

This document outlines advanced configurations natively supported by the Cluster Toolkit for Slurm clusters (version 25.11 and newer). These features are designed to improve cluster observability, fault tolerance, and minimize system overhead.

## Native OpenMetrics (Prometheus) Telemetry

For Slurm 25.11 clusters, the Toolkit integrates Slurm's native plugin to provide direct support for exporting OpenMetrics (Prometheus) telemetry from the `slurmctld` daemon.

This feature configures the Google Cloud Ops Agent to automatically scrape this telemetry (which is locally accessible on your `SlurmctldPort` by default) and export standard cluster metrics (such as job states, node health, partition usage, and scheduler performance) into Google Cloud Monitoring. For a full, detailed breakdown of every exported metric, please refer to the official [SchedMD Metrics Documentation](https://slurm.schedmd.com/metrics.html).

**How to enable:**
Set the `enable_openmetrics` variable to `true` in your Slurm controller module:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    settings:
      enable_openmetrics: true
```

Once deployed, you can view your cluster metrics directly in the GCP Metrics Explorer under `Prometheus/Slurm`.

## Expedited Requeue for Job Resilience

When a compute node suffers an unexpected hardware failure, kernel panic, or a Spot VM preemption, any active jobs on that node will crash and are typically forced to start over at the back of the queue.

By default, the Cluster Toolkit automatically enables the `enable_expedited_requeue` setting in the `schedmd-slurm-gcp-v6-controller` module. This turns the feature on for the cluster. **To disable it globally**, you can explicitly opt-out in your blueprint:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    settings:
      enable_expedited_requeue: false
```

To take advantage of the feature, simply submit your jobs with the following flag:

```bash
sbatch --requeue=expedite script.sh
```

If the compute node suffers a hard crash, spot VM preemption, or if the batch script returns a non-zero exit code while one or more Epilog scripts fail, Slurm will instantly cancel the job and requeue it.

Expedited requeue jobs are eligible to restart immediately and are treated as the **highest priority** job in the system. Furthermore, Slurm will fence off their previously allocated set of broken nodes to prevent them from launching other work while the job rapidly resumes execution elsewhere.
*(Note: Graceful, administrative shutdowns explicitly bypass this behavior to prevent infinite job loops).*

For more information about requeue behaviors and flags, please refer to the official [sbatch Documentation](https://slurm.schedmd.com/sbatch.html).

## Start-Only Health Checks

If you define a custom `HealthCheckProgram` script in Slurm, it typically runs continuously in the background on a timer. This constant polling can generate unnecessary overhead and distract active compute nodes from performing actual workload calculations.

You can instruct Slurm to run your health check script **exactly once** when a node first boots up, and then stop:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    settings:
      enable_health_check_start_only: true
```

When enabled, the `slurmd` daemon ensures your script executes once during the node's initial boot sequence to verify the node is healthy, but will completely halt the background polling so active jobs are not impacted by the health check.

## Asynchronous Reply Mode (Experimental)

The Slurm controller (`slurmctld`) typically processes network socket connections sequentially in user-space. In large clusters, if clients are extremely slow to read (e.g., due to network lag or a TCP Window stall), the controller's worker threads can get trapped endlessly retrying connections. This consumes CPU cycles, exhausts the thread pool, and can make the entire cluster unresponsive.

For Slurm 25.11+ clusters, the Toolkit supports `enable_async_reply`. This experimental mode allows the controller to instantly disengage and offload RPC responses directly to the Linux kernel network stack for further processing, freeing individual worker threads for new traffic. Because worker threads are instantly freed up, the controller stays heavily responsive even during massive node scale-up events or network latency spikes.

**How to enable:**
To enable this feature, place it under the `experimental` block in your controller settings:

```yaml
  - id: slurm_controller
    source: community/modules/scheduler/schedmd-slurm-gcp-v6-controller
    settings:
      experimental:
        enable_async_reply: true
```

For more technical details on this mode, visit the official [Slurm Configuration Guide](https://slurm.schedmd.com/slurm.conf.html) and search for `enable_async_reply`.
