# DEPRECATED: Debian 12 Slurm Image Blueprint for A3-Mega

**This blueprint is deprecated and will be removed on or after August 1, 2025.**
This directory contains the previous Slurm image blueprint for **A3-Megagpu-8G** instances based on **Debian 12**. It has been moved to this `debian/` subdirectory to clearly indicate its deprecated status.

## Reason for Deprecation

Effective immediately, new **A3-Megagpu-8G Slurm** solutions should utilize the **Ubuntu 22.04**-based Slurm blueprint. This transition is being made to standardize on **Ubuntu 22.04** on the **A3** platform.

## What You Should Do

* **For New Deployments:** Please refer to the new **Ubuntu 22.04**-based Slurm blueprint for **A3-Megagpu-8G**. You'll find it at `examples/machine-learning/a3-megagpu-8g/a3mega-slurm-blueprint.yaml`.
* **For Existing Deployments:** While your current **Debian 12**-based deployments may continue to function, we strongly recommend planning a migration to the **Ubuntu 22.04**-based solution as soon as possible.
* **Testing:** This blueprint may still be used for compatibility testing purposes until its final removal.

## Final Removal Date

On or after **August 1, 2025**, this `debian/` directory and its contents will be permanently removed from this repository. Please ensure all your workflows are migrated before this date.
