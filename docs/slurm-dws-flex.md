# Obtaining SlurmGCP nodes with DWS Flex

> [!NOTE]
> DWS Flex Start is currently in early development and undergoing extensive testing.

[Dynamic Workload Scheduler](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) Flex Start mode is designed for fine-tuning models, experimentation, shorter training jobs, distillation, offline inference, and batch jobs.

With Dynamic Workload Scheduler in Flex Start mode, you submit a GPU capacity request for your AI/ML jobs by indicating how many you need, a duration, and your preferred region. It supports capacity requests for up to seven days, with no minimum duration requirement. You can request capacity for as little as a few minutes or hours; typically, the scheduler can fulfill shorter requests more quickly than longer ones.

In order to make use of DWS Flex Start mode with SlurmGCP, you must use the `dws_flex` variable in the `schedmd-slurm-gcp-v6-nodeset` module. From there you can specify the desired maximum duration (in seconds) with `max_run_duration`. You can also use `use_job_duration` which will utilize the job's `TimeLimit` within Slurm as the duration. If `use_job_duration` is enabled but `TimeLimit` is not set, it will default to `max_run_duration`. See the example below:

```yaml
  - id: flex_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      dws_flex:
        max_run_duration: 3600 # 1 hour
      enable_placement: false
      # the rest of the settings, e.g. node_count_static, machine_type, additional_disks, etc.
```

**Node behavior:**

* Static nodes will be re-provisioned when `max_run_duration` ends.
* Dynamic nodes in exclusive partitions will delete instances after the job completes (even if `max_run_duration` has yet to pass).

> [!WARNING]
> DWS Flex Start cannot be used in tandem with a reservation or placement policy.
<p>

> [!WARNING]
> DWS Flex Start support in SlurmGCP is in early development, there are some known issues.

**Known issues:**

* When `max_run_duration` completes instances will be deleted by the MIG.
* Empty MIGs are not cleaned up automatically.

> [!NOTE]
> We also have a legacy implementation (which uses bulkInsert) which can be enabled by using the `use_bulk_insert` variable.
