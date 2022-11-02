# Troubleshooting Hybrid Slurm Deployments

## Logging
The logs from VMs created by the hybrid configuration will be populated under
`/var/log/slurm/*.log`, a selection of pertinent logs are described below:

* `slurmctld.log`: The logging information for the slurm controller daemon. Any
  issues with the config or permissions will be logged here.
* `slurmd.log`: The logging information for the slurm daemon on the compute
  nodes. Any issues with the config or permissions on the compute node can be
  found here. Note: These logs require SSH'ing to the compute nodes and viewing
  them directly.
* `resume.log`: Output from the resume.py script that is used by hybrid
  partitions to create the burst VM instances. Any issues creating new compute
  VM nodes will be logged here.

In addition, any startup failures can be tracked through the logs at
`/var/log/messages` for centos/rhel based images and `/var/log/syslog` for
debian/ubuntu based images. Instructions for viewing these logs can be found in
[Google Cloud docs][view-ss-output].

[view-ss-output]: https://cloud.google.com/compute/docs/instances/startup-scripts/linux#viewing-output

## Debug Settings

If the standard logging information is not sufficient, it is possible to
increase the verbosity of the `slurmctld` and `slurmd` logs with the following
variables in the `slurm.conf` file:

* [`SlurmctldDebug`](https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmctldDebug)
* [`SlurmdDebug`](https://slurm.schedmd.com/slurm.conf.html#OPT_SlurmdDebug)

For more information on which logging level to select, click the links above to
view the official SchedMD documentation.

In addition to the logging levels, specific debug flags can be set that are
relevant to the hybrid configuration. Specifically, the [`Power`][powerflag]
[`DebugFlag`][flags] will provide useful information about the
[power saving][powersaving] functionality used to implement the hybrid
partitions.

[powerflag]: https://slurm.schedmd.com/slurm.conf.html#OPT_Power
[flags]: https://slurm.schedmd.com/slurm.conf.html#OPT_DebugFlags
[powersaving]: https://slurm.schedmd.com/power_save.html
