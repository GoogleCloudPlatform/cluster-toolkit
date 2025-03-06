# Provisioning SlurmGCP nodes with Future Reservations (DWS Calendar Mode)

Use [Future Reservations](https://cloud.google.com/compute/docs/instances/future-reservations-overview) to request assurance of important or difficult-to-obtain capacity in advance.
[Dynamic Workload Scheduler](https://cloud.google.com/blog/products/compute/introducing-dynamic-workload-scheduler) Calendar mode extends the future reservation capabilities and caters to training/experimentation workloads that demand precise start times and have a defined duration.

Compared to on-demand reservations, future reservations provide you with an even higher level of assurance in obtaining capacity for Compute Engine zonal resources.

With Calendar mode, you will be able to request GPU capacity in fixed duration capacity blocks. It will initially support future reservations with durations of 7 or 14 days and can be purchased up to 8 weeks in advance. Your reservation will get confirmed, based on availability, and the capacity will be delivered to your project on your requested start date. Your VMs will be able to target this reservation to consume this capacity block. At the end of the defined duration, the VMs will be terminated, and the reservations will get deleted.

> [!IMPORTANT]  
> To use DWS Calendar mode your project needs to be allowlisted for private preview access.
> Fill out the [form](https://docs.google.com/forms/d/1etaaXMW9jJUTTxfUC7TIIMttLWT5H-3Q8_3-sG6vwKk/edit).

In order to make use of Future Reservations/DWS Calendar mode with SlurmGCP, you must use the `future_reservation` variable in the `schedmd-slurm-gcp-v6-nodeset` module. From there you can specify the name of the future reservation that you would like it to use. Ensure fields like `machine_type` matches with your reservation. Once deployed nodes in your nodeset will appear `DOWN` until your reservation begins, and will appear `DOWN` again once the reservation is complete (no redeploying necessary).

```yaml
  - id: fr_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      future_reservation: name OR project/PROJECT/zone/ZONE/futureReservations/name
      enable_placement: false
      # the rest of the settings, e.g. node_count_static, machine_type, additional_disks, etc.
```
