# Slurm Image Upgrade Guide for Cluster Toolkit

This guide shows how to move a running Slurm cluster to a newer VM image, for
example to pick up security fixes. It covers how to check which image each node
runs, how to update each type of node, how to hold a cluster on one image, and
how to move to a different image family.

The steps apply to clusters with a single controller. Clusters with a backup
controller (`enable_backup_controller`) are not covered.

## 1. How a cluster picks its image

A VM keeps the image it was created with. A node moves to a newer image only
when its VM is created again.

The `instance_image` setting in the blueprint decides which image a new VM gets:

```yaml
instance_image:
  family: aci-cpu-rocky-linux-9-slurm-2605-amd64
  project: advanced-compute-images
```

* With `family`, each new VM boots the newest image in that family at the moment
  the VM is created.
* With `name` instead of `family`, each new VM boots that exact image.

If the blueprint does not set `instance_image`, each module uses its default.
The default is listed in the module README, for example
[schedmd-slurm-gcp-v6-controller](../community/modules/scheduler/schedmd-slurm-gcp-v6-controller/README.md#inputs).

The image contains the operating system and kernel, Slurm and, on GPU images,
the NVIDIA driver and CUDA. A newer image in the same family is a routine
update. A different family usually means a different Slurm, driver or operating
system version; see [section 6](#6-move-to-a-different-image-family).

## 2. Check which image each node runs

List the image of every node in the deployment:

```shell
gcloud compute disks list --project PROJECT_ID \
    --filter="labels.ghpc_deployment=DEPLOYMENT_NAME AND sourceImage:*" \
    --format="table(name,labels.slurm_instance_role:label=ROLE,sourceImage.basename():label=IMAGE,creationTimestamp)" \
    --sort-by=name
```

Example output, from a cluster where a newer image was published after the
cluster was deployed:

```text
NAME                 ROLE        IMAGE                                               CREATION_TIMESTAMP
hpc-computeset-0     compute     aci-cpu-rocky-linux-9-slurm-2605-amd64-v20261111-2  2026-11-16T05:27:10.000-08:00
hpc-controller       controller  aci-cpu-rocky-linux-9-slurm-2605-amd64-v20261008-1  2026-11-16T04:03:22.000-08:00
hpc-slurm-login-001  login       aci-cpu-rocky-linux-9-slurm-2605-amd64-v20261008-1  2026-11-16T04:06:41.000-08:00
hpc-staticset-0      compute     aci-cpu-rocky-linux-9-slurm-2605-amd64-v20261008-1  2026-11-16T04:09:05.000-08:00
```

Then find the newest image in the family:

```shell
gcloud compute images describe-from-family IMAGE_FAMILY \
    --project IMAGE_PROJECT --format="value(name)"
```

A node is up to date when its `IMAGE` matches that name. In the example, only
the compute node that was created after the new image was published is up to
date.

## 3. What each node type needs

| Node type | When it gets a newer image | What you do |
| --- | --- | --- |
| Compute nodes that are created per job | Every time a node is created | Nothing |
| Always-on (static) compute nodes | When the node is created again | [Section 4.1](#41-always-on-compute-nodes) |
| Login nodes | When the VM is replaced | [Section 4.2](#42-login-nodes) |
| Controller | When the VM is replaced | [Section 4.3](#43-controller) |

Running `gcluster deploy` again with an unchanged blueprint does not move any
node to a newer image.

## 4. Update to the newest image in the same family

Each step can be done on its own. If you update the controller, do it first: it
requires the always-on compute nodes to be created again afterwards.

| Step | Running jobs | Users | Time in our test |
| --- | --- | --- | --- |
| [4.1 Always-on compute nodes](#41-always-on-compute-nodes) | Finish first | Not affected | About 4 minutes per node after its job ends (up to 12 minutes if Slurm retries a slow boot) |
| [4.2 Login nodes](#42-login-nodes) | Not affected | Sessions on that node end | About 5 minutes |
| [4.3 Controller](#43-controller) | Drain first and wait for them to finish | Scheduler unavailable for 3 to 7 minutes; login sessions end | 10 to 15 minutes after running jobs finish |

Times are from a small CPU cluster. Larger machine types and long startup
scripts take longer.

### 4.1 Always-on compute nodes

1. Find the node names:

   ```shell
   sinfo -N -p PARTITION
   ```

1. On the controller, tell Slurm to create the nodes again:

   ```shell
   sudo $(which scontrol) update nodename=NODE_LIST \
       state=power_down_asap reason="image-update"
   ```

   `NODE_LIST` can be a range, for example `hpc-staticset-[0-9]`. Running jobs
   finish first. Each node is then deleted and created again from the newest
   image. Update the nodes in batches if you need to keep some capacity
   available.

1. Watch the nodes come back:

   ```shell
   sinfo -N -p PARTITION
   ```

   | State | Meaning |
   | --- | --- |
   | `drng!` | Waiting for a running job to finish |
   | `idle%` | Shutting down |
   | `idle~` | Deleted, waiting to be created again |
   | `idle#` | Starting |
   | `idle` | Ready |

   If a node goes back from `idle#` to `idle~`, it did not start within
   `ResumeTimeout` (300 seconds by default) and Slurm is trying again
   automatically. No action is needed.

1. Check the images again as in [section 2](#2-check-which-image-each-node-runs).

Keep a reason of your own in the command. The reasons `XID`, `SDC`,
`PERFORMANCE` and `unspecified` are reserved: they report the host to Google
Cloud as faulty.

Creating a node again deletes its boot disk and any Local SSDs (such as local
scratch or Enroot container caches). Shared storage such as Filestore, Managed
Lustre, or Cloud Storage FUSE is not touched.

Creating a node again also releases its capacity for a moment. Without a
reservation, a node may not come back if the zone has no capacity left for that
machine type.

### 4.2 Login nodes

Replacing a login node ends the sessions of users who are logged in to it.

1. Find the address of the login node in the deployment folder. `GROUP` is the
   deployment group that contains the Slurm controller, for example `primary`:

   ```shell
   terraform -chdir=DEPLOYMENT_FOLDER/GROUP state list | grep instance_from_template
   ```

   Example output:

   ```text
   module.slurm_controller.google_compute_instance_from_template.controller[0]
   module.slurm_controller.module.login["slurm-login"].module.instance.google_compute_instance_from_template.slurm_instance[0]
   ```

1. Replace the login node. Use the address from the previous step, in single
   quotes:

   ```shell
   terraform -chdir=DEPLOYMENT_FOLDER/GROUP apply \
       -replace='module.slurm_controller.module.login["slurm-login"].module.instance.google_compute_instance_from_template.slurm_instance[0]'
   ```

   Terraform shows what it will replace and asks for confirmation. The plan
   should list only the login node: `1 to add, 0 to change, 1 to destroy`.

1. Check the image as in [section 2](#2-check-which-image-each-node-runs).

Repeat the second step for each login node address.

### 4.3 Controller

Replace the controller in a maintenance window. Read this list before you start.

* **The scheduler is unavailable** for 3 to 7 minutes.
* **Every login node is replaced** together with the controller.
* **Kept:** queued jobs, node drain states, and the accounting history, when
  `controller_state_disk` is enabled (the default). If your blueprint sets
  `controller_state_disk: null`, these live on the controller boot disk instead
  and are lost when the controller is replaced.
* **Lost:** everything stored on the controller boot disk. That is `/opt/apps`,
  and also `/home` unless your blueprint mounts other storage at `/home`. Copy
  what you need before you start. Storage outside the controller, such as
  Filestore or Managed Lustre, is not touched.
* **Compute nodes that are running lose `/home` and `/opt/apps`** when these are
  served by the controller. Jobs and logins on those nodes hang until the node
  is created again. For this reason the steps below drain the cluster first and
  create the always-on nodes again at the end.

To see where `/home` and `/opt/apps` are stored, run this on a login node. A
source that starts with the controller name means the controller serves them:

```shell
df -h /home /opt/apps
```

Steps:

1. On the controller, stop new jobs from starting:

   ```shell
   sudo $(which scontrol) update nodename=ALL \
       state=drain reason="controller-update"
   ```

1. Wait until no job is running:

   ```shell
   squeue --states=RUNNING
   ```

   Nodes show `drain` (always-on) or `drain~` (powered-down per-job nodes) in
   `sinfo -N`.

1. Find the controller address as in [section 4.2](#42-login-nodes), then
   replace the controller:

   ```shell
   terraform -chdir=DEPLOYMENT_FOLDER/GROUP apply \
       -replace='module.slurm_controller.google_compute_instance_from_template.controller[0]'
   ```

   The plan should list the controller and the login nodes:
   `3 to add, 0 to change, 3 to destroy` for a cluster with one login node.

1. When Terraform finishes, wait until the scheduler answers. On the new
   controller:

   ```shell
   scontrol ping
   ```

1. Clear the drain and create the always-on nodes again:

   ```shell
   sudo $(which scontrol) update nodename=ALL \
       state=power_down_force reason="controller-update"
   ```

   Watch the nodes with `sinfo -N`. Always-on nodes go through `idle%`, `idle~`
   and `idle#` and return to `idle` on their own (no separate `state=resume`
   command is needed), and waiting jobs start automatically. Per-job nodes
   return to `idle~`.

1. Check the images as in [section 2](#2-check-which-image-each-node-runs).

## 5. Stay on one image, or go back to an older one

To hold a nodeset on one exact image, set `name` instead of `family` in that
nodeset:

```yaml
  - id: compute_nodeset
    source: community/modules/compute/schedmd-slurm-gcp-v6-nodeset
    use: [network]
    settings:
      node_count_dynamic_max: 20
      machine_type: c2-standard-60
      instance_image:
        name: aci-cpu-rocky-linux-9-slurm-2605-amd64-v20261008-1
        project: advanced-compute-images
```

Then redeploy:

```shell
./gcluster deploy <path-to-blueprint.yaml> -w
```

Only that nodeset's instance template is replaced; no running VM is touched.
Nodes created from now on use the named image. Nodes that are already running
keep theirs; create them again as in [section 4.1](#41-always-on-compute-nodes)
if needed.

To follow the family again, put `family` back and redeploy.

Two things to know:

* **When `instance_image` is set under `vars`:** To hold the whole cluster on
  one image, replace `family` with `name` directly under `vars` (this also
  updates the controller and login templates; see the next point). To hold only
  one nodeset on one image, move `instance_image` out of `vars` and into the
  `settings` of each module first: a `name` in one module cannot be combined
  with a `family` under `vars` (`at most one of the "family" or "name" fields
  can be set`). Moving the setting into each module changes nothing in the
  deployed cluster. Do not add `family: null` to get past the error: it passes
  validation, but points the instance template at a non-existent image family
  named after the image.
* **Changing `instance_image` on the controller or a login node replaces that
  VM** on the next deploy—even when switching from `family` to `name` for the
  image it is already running. Prepare as in [section 4.3](#43-controller).

## 6. Move to a different image family

Move to a different family when you need a newer Slurm, NVIDIA driver or
operating system version, or when the family you use reaches its end of support.
The families and their support dates are listed in the
[Advanced Compute Images documentation](https://cloud.google.com/compute/docs/images/workload/aci-images).

1. Change `instance_image` for the controller, the login nodes and the nodesets
   in the same blueprint edit, so that all of them move together.
1. Drain the cluster and wait for running jobs to finish as in
   [section 4.3](#43-controller), steps 1 and 2: changing the controller image
   replaces the controller and the login nodes.
1. Redeploy with `./gcluster deploy <path-to-blueprint.yaml> -w`. At the
   `[d,a,s,c]` prompt, press `d` to inspect the full Terraform plan before
   pressing `a` to apply.
1. Clear the drain and create the always-on compute nodes again as in
   [section 4.3](#43-controller), step 5.

When the new family contains a different Slurm version:

* Your Cluster Toolkit release must support that Slurm version. Use the release
  whose default image is the family you are moving to, or a newer one.
* The accounting database is upgraded when the new controller starts, and it
  cannot be downgraded. Follow the
  [Slurm upgrade guide](https://slurm.schedmd.com/upgrades.html) and try the
  move on a test cluster first.

The steps in this guide were tested between images with the same Slurm version.

## 7. Clusters that use a custom image

If your nodes use an image family in your own project, for example one built
with the [Packer module](image-building.md), updates to the public images do not
reach the cluster by themselves. Build a new image into your family first. After
that, everything in this guide applies to your family in the same way.

## 8. After a Cluster Toolkit upgrade

A new Cluster Toolkit release can change the default image of a module. If your
blueprint relies on the default, redeploying an existing cluster with the newer
release behaves like [section 6](#6-move-to-a-different-image-family): the
controller and the login nodes are replaced.

To decide for yourself when the image changes, set `instance_image` in the
blueprint. Whenever you run `gcluster deploy` on a running cluster, press `d` at
the `[d,a,s,c]` prompt to inspect the full plan before approving it. If it lists
the controller as replaced, prepare as in [section 4.3](#43-controller).
