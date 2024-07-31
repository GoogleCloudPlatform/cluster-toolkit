# Hybrid Slurm Clusters

## [on-prem-instructions.md](./on-prem-instructions.md)
This document describes how to use the Cluster Toolkit to extend an on-premise Slurm
cluster to add cloud hybrid partitions.

## [demo-with-cloud-controller-instructions.md](./demo-with-cloud-controller-instructions.md)
This document describes how to deploy a simulated hybrid slurm cluster entirely
in GCP. These instructions can be used as a way of trying the
[schedmd-slurm-gcp-v5-hybrid][hybridmodule] in GCP before bringing the
configuration changes to a physical on-premise slurm cluster.

[hybridmodule]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md

## Support Documents

### [deploy-instructions.md](./deploy-instructions.md)
[deploy-instructions.md](./deploy-instructions.md) is a shared document used by
both [demo-with-cloud-controller-instructions.md](./demo-with-cloud-controller-instructions.md)
and [on-prem-instructions.md](./on-prem-instructions.md). This document describes how to create,
deploy and install the hybrid configuration assuming your static cluster is
already created.

### [troubleshooting.md](./troubleshooting.md)
Includes a set of common troubleshooting tips when deploying a hybrid partition
using the [schedmd-slurm-gcp-v5-hybrid][hybridmodule] Cluster Toolkit Module.

## Blueprints
The [blueprints directory](./blueprints/) contains a set of support blueprints
for the documentation in this directory. These blueprints are intended to be
used as is with minimal tweaking of deployment variables either in place or on
the command line.
