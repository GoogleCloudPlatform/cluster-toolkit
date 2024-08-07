# Obtaining SlurmGCP nodes with DWS Flex

[Dynamic Workload Scheduler](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) Flex Start mode is designed for fine-tuning models, experimentation, shorter training jobs, distillation, offline inference, and batch jobs.

With Dynamic Workload Scheduler in Flex Start mode, you submit a GPU capacity request for your AI/ML jobs by indicating how many you need, a duration, and your preferred region. It supports capacity requests for up to seven days, with no minimum duration requirement. You can request capacity for as little as a few minutes or hours; typically, the scheduler can fulfill shorter requests more quickly than longer ones.

> [!IMPORTANT]  
> The project needs to be allowlisted for private preview access.
> Fill out the [form](https://docs.google.com/forms/d/1etaaXMW9jJUTTxfUC7TIIMttLWT5H-3Q8_3-sG6vwKk/edit).

In order to make use of DWS Flex Start mode with SlurmGCP, you must specify a proper set of `instance_properties` in the `schedmd-slurm-gcp-v6-nodeset` module. See the example below:

```yaml
  - id: flex_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      instance_properties:
        reservationAffinity:
          consumeReservationType: NO_RESERVATION
        scheduling:
          maxRunDuration: { seconds: $(2 * 60 * 60) } # 2 hours
          onHostMaintenance: TERMINATE
          instanceTerminationAction: DELETE
      # the rest of the settings, e.g. node_count_static, machine_type, additional_disks, etc.
```

**All** fields in `instance_properties` should match provided values, except for `maxRunDuration`, which should be set to the desired duration in seconds (up to 604800 = 7 days).

> [!WARNING]
> The use of the `instance_properties` setting directly overrides bulkInsert API parameters. While the documented sample
> was tested at the time of publication, it is not regression tested and may cease to work based on changes in the bulkInsert API.
