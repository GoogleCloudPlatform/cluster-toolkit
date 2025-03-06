# Network topology aware scheduling

Slurm can be [configured](https://slurm.schedmd.com/topology.html) to support topology-aware
resource allocation to optimize job performance.

If you are using Slurm via ClusterToolkit, the Slurm Topology Plugin is automatically configured with:

```ini
TopologyPlugin=topology/tree
TopologyParam=SwitchAsNodeRank 
```

This does two things:

* **Minimizes inter-rack communication:** For jobs smaller than the full cluster size, Slurm will assign the job to as few racks as possible.
* **Optimizes rank placement:** Within a job, the Slurm node rank (used to assign global Slurm / MPI ranks) is ordered by the Switch that the node is on, such that ranks are ordered by rack.

SlurmGCP automatically updates topology information for all nodes in the cluster, according to their [physical location](https://cloud.google.com/compute/docs/instances/use-compact-placement-policies#verify-vm-location).

> [!NOTE]
> The physical location information is available for VMs configured with a placement policy.
> VMs without a defined placement policy will be assigned a less efficient 'fake' topology.

Applications that incorporate either the `SLURM_PROCID`/`NODE_RANK`/etc or the MPI Rank into their task assignment may see performance benefits.
In other cases, such as with PyTorch's `distributed`, you may need to modify the rank assignment to incorporate this information, see [example](../examples/machine-learning/a3-megagpu-8g/topological-pytorch/README.md).

## Inspect topology

You can inspect topology used by Slurm by running:

```sh
scontrol show topology

# Or by listing the configuration file:
cat /etc/slurm/topology.conf
```

To inspect the "real" topology and verify the physical host placement, you can list the `physical_host` property of nodes:

```sh
#!/bin/bash

# /home/where.sh - echo machines hostname and its physicalHost
echo "$(hostname) $(curl 'http://metadata.google.internal/computeMetadata/v1/instance/attributes/physical_host' -H 'Metadata-Flavor: Google' -s)"
```

```sh
srun --nodelist={nodes_to_inspect} -l /home/where.sh | sort -V
```

## Disabling SlurmGCP topology integration

Updates to `topology.conf` require reconfiguration of Slurm controller. This can be a costly operation that affects the responsiveness of the controller.

You have the option to disable the Slurm Topology Plugin (along with automatic updates) by providing the following settings to controller module in your blueprint:

```yaml
settings:
  cloud_parameters:
    topology_plugin: ""
```

Even with the Topology Plugin disabled, you can still optimize rank placement by using the `sort_nodes`
util  in your [sbatch](https://slurm.schedmd.com/sbatch.html) scripts. For example:

```sh
#SBATCH --ntasks-per-node=8
#SBATCH --nodes=64

export SLURM_HOSTFILE=$(sort_nodes.py)

srun -l hostname | sort
```
