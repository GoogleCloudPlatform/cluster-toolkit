# Deploying and installing the hybrid module

These instructions step you through the deployment, installation and
verification of a hybrid slurm cluster using the
[schedmd-slurm-gcp-v5-controller] Cluster Toolkit module.

They assume that your on-premise or simulated on-premise slurm
cluster has already been set up to allow hybrid partitions from the hybrid
module. If you haven't already, see [on-prem-instructions.md](./on-prem-instructions.md) for
instructions on preparing your on-premise cluster or
[demo-with-cloud-controller-instructions.md] for
instructions on preparing a simulated on premise cluster in gcp using Slurm on
GCP.

> If you came to this page from [demo-with-cloud-controller-instructions.md],
> be aware that these instructions have been generalized and that "Project A" and
> "Project B" are not longer used here. "Project A" will simply be referred to as
> you on-premise environment and "Project B" is your cloud bursting project.
> Additional notes for following these instructions using a Slurm on GCP static
> cluster will be indented similar to this block.
>
> **The indented blocks can be ignored if you are deploying onto an on-premise
> controller.**

[schedmd-slurm-gcp-v5-controller]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md

## Use the Cluster Toolkit to Create the Hybrid Deployment Directory
The blueprint for creating a deploying the hybrid configuration can be found in
the blueprints directory as [hybrid-configuration.yaml]. This blueprint defines
two deployment groups, one to create the network if not already created named
`create_network` and another that creates the hybrid configuration named
`primary`. The `primary` deployment group does the following:

* Creates a pointer to the network in the cloud bursting project.
* Create a filestore instance as a performant cloud scratch network filesystem.
* Create two partitions:
  * A partition named "debug" with a dynamic maximum size of 10 nodes of machine
    type n2-standard-2.
  * A partition named "compute" with a dynamic maximum size of 20 nodes of
    machine type c2-standard-60.
* Creates a hybrid configuration using the
  [schedmd-slurm-gcp-v5-hybrid][hybridmodule] module. This module will do the
  following:
  * Create a directory at `output_dir` locally containing the hybrid
    configuration files and execution scripts.
  * Set metadata in the cloud bursting project that inform the burst compute
    nodes how to configure themselves.
  * Create pubsub actions triggered by changes to the hybrid configuration.

The following deployment variables in the [hybrid-configuration.yaml] blueprint
will be set based on your configuration via the command line:

* **_project\_id:_** The ID of the cloud bursting project.
* **_static\_controller\_hostname:_** The hostname of the controller machine.
  Depending on the network setup, the simple hostname may work, but it's
  possible the fully qualified hostname will be required.
* **_static\_controller\_addr:_** Optional variable for setting either the IP
  address or fully qualified hostname of the controller if it's needed for full
  connectivity from the compute nodes.
  
> If you are working from a static cluster deployed in another project, the
> fully qualified internal hostname of the static cluster's controller in
> project A will have the following format:
> `cluster-controller.c.<<Project_A_ID>>.internal`. In addition, the
> `static_control_addr` should be set to the IP address of the static
> controller.

The hybrid-configuration.yaml is configured to connect to a VPC network that has
already been deployed with a `network_name: compute-vpc-network` and a
`subnetwork_name: primary-subnet`. If you have not already set up a network
(_through a static cluster deployed in another project or otherwise_) then
uncomment the `create_network` deployment group to also create a network with
this blueprint.

To create the deployment directory with deployment variables passed through the
command line, run the following command with the updated values for
`<<Controller_Hostname>>` and `<<Project_ID>>`:

```shell
./gcluster create docs/hybrid-slurm-cluster/blueprints/hybrid-configuration.yaml \
  --vars project_id=<<bursting project>> \
  --vars static_controller_hostname=<<fully qualified controller hostname>> \
  --vars static_controller_addr=<<Controller_Address>>
```

If successful, this command will create a deployment folder. Use the following
command to deploy the hybrid configuration:

```sh
./gcluster deploy hybrid-config
```

`gcluster` reports the changes that Terraform is proposing to make for your
cluster. Optionally, you may review them by typing `d` and pressing `enter`. To
deploy the cluster, accept the proposed changes by typing `a` and pressing
`enter`.

After deployment, a directory in `hybrid-configuration/primary` named `hybrid/`
should be created which contains a `cloud.conf` file, `cloud_gres.conf` file and
a set of support scripts.

> [!WARNING]
> There is a known issue that may prevent deployment when terraform is being run
> on a machine other than the controller. The error looks like:
> `FileNotFoundError: [Errno 2] No such file or directory: '/slurm/custom_scripts/prolog.d'`.

[hybridmodule]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md
[hybrid-configuration.yaml]: ./blueprints/hybrid-configuration.yaml

## Install and Configure Hybrid on the Controller Instance

> **_NOTE:_** Many of the manual steps in this section have been adapted from the
> hybrid documentation in [Slurm on GCP][slurm-gcp]. The source document can be
> found at [docs/hybrid.md][slurm-gcp-hybrid]

Now that the hybrid configuration directory has been created, it needs to be
installed on the controller VM instance. First, tar the directory:

```shell
cd hybrid-config/primary
tar czvf hybrid.tar.gz hybrid
```

Copy the `hybrid.tar.gz` file to the controller VM instance and SSH to the
slurm controller machine.

> If you are working with a static Slurm cluster created in
> [demo-with-cloud-controller-instructions.md], the
> following steps can be used to copy the hybrid directory and access your
> controller:
>
> To copy the hybrid configuration directory with `gcloud compute scp`:
>
> ```shell
> gcloud compute scp --project="<<Project_A>>" --zone=us-central1-c ./hybrid.tar.gz "cluster-controller:~"
> ```
>
> Now SSH to the controller VM either using the console or the following gcloud
> command:
>
> ```shell
> gcloud compute ssh --project="<<Project_A>>" --zone=us-central1-c "cluster-controller"
> ```

Decompress the `hybrid.tar.gz` file on the Slurm controller:

```shell
SLURM_CONF_DIR=/etc/slurm
sudo tar xzvf hybrid.tar.gz --directory $SLURM_CONF_DIR
rm hybrid.tar.gz
```

> **_NOTE:_** These instructions assume the slurm configuration exists in
> `/etc/slurm`. If that is not the case, update the `SLURM_CONF_DIR` variable
> before executing these commands.

Set the correct permissions for the hybrid directory and the files contained in
it:

```shell
sudo chown -R slurm: $SLURM_CONF_DIR/hybrid
sudo chmod -R 755 $SLURM_CONF_DIR/hybrid
```

> The following instructions only apply if the static Slurm cluster was created
> following the instructions in [demo-with-cloud-controller-instructions.md]:
>
> Because the static cluster was also created by [Slurm on GCP][slurm-gcp]
> terraform modules, the partition information must be copied from the file
> `/etc/slurm/cloud.conf` to the slurm config file at `/etc/slurm/slurm.conf`. The
> lines that need to be copied will look similar to the following block:
>
> ```text
> NodeName=DEFAULT State=UNKNOWN RealMemory=7552 Boards=1 Sockets=1 CoresPerSocket=1 ThreadsPerCore=1 CPUs=1
> NodeName=cluster-static-ghpc-[0-3] State=CLOUD
> NodeSet=cluster-static-ghpc Nodes=cluster-static-ghpc-[0-3]
> PartitionName=static Nodes=cluster-static-ghpc State=UP DefMemPerCPU=7552 SuspendTime=300 Oversubscribe=Exclusive Default=YES
>
> SuspendExcNodes=cluster-static-ghpc-[0-3]
> ```
>
> Depending on the configuration of the static partitions, the `SuspendExcNodes`
> may not be included.
>
> Also remove `State=CLOUD` text when copying this over.
>
> These lines can be copied to the bottom of the `slurm.conf` file.

Next copy the hybrid `cloud.conf` file to the slurm directory so that it is
visible to the `slurm.conf` file both on the controller and on the compute VMs:

```shell
sudo cp $SLURM_CONF_DIR/hybrid/cloud.conf $SLURM_CONF_DIR
sudo chown -R slurm: $SLURM_CONF_DIR/cloud.conf
sudo chmod 644 $SLURM_CONF_DIR/cloud.conf
```

In the `$SLURM_CONF_DIR/slurm.conf` file, add the fully qualified hostname in
parentheses after the controller hostname in the parameter `SlurmctldHost` if
not already provided.

```text
# slurm.conf
...
SlurmctldHost=cluster-controller(<<Fully_Qualified_Hostname>>)
...
```

Make the following changes to the `$SLURM_CONF_DIR/cloud.conf` file:

* `SlurmctldParameters`
  * Remove `cloud_dns`
  * Add `cloud_reg_addrs`
* Add `TreeWidth=65533`

```text
# cloud.conf
...
SlurmctldParameters=idle_on_node_suspend,cloud_reg_addrs
...
TreeWidth=65533
...
```

These changes will inform the controller to use the IP of compute nodes to
communicate rather than the hostnames. This step may not be required if the DNS
is configured to identify the cloud compute VM instances by hostname.

Next, create a new cronjob as the slurm user that will periodically call the
`$SLURM_CONF_DIR/hybrid/slurmsync.py` file. To do that, edit the `/etc/crontab` file
as root:

```shell
sudo vim /etc/crontab
```

Add the following line to run `slurmsync.py` every minute as the slurm user,
updating `<<SLURM_CONF_SIR>>` based on your configuration:

```text
1 * * * * slurm <<SLURM_CONF_DIR>>/hybrid/slurmsync.py
```

> If the controller was deployed using [Slurm on GCP][slurm-gcp] following the
> instructions in [demo-with-cloud-controller-instructions.md], there will
> already be a cronjob pointing to the `slurmsync.py` script in `/etc/slurm/`.
> This cronjob can be removed by following these steps:
>
> Become the slurm user and edit the crontab:
>
> ```shell
> sudo su slurm
> crontab -e
> ```
>
> Clear the contents of the file and exit the editor.

Finally, restart the slurmctld service to enable the changes made:

```shell
sudo systemctl restart slurmctld
```

If the restart did not succeed, the logs at `/var/log/slurm/slurmctld.log`
should point you in the right direction.

[slurm-gcp]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/5.12.2
[slurm-gcp-hybrid]: https://github.com/GoogleCloudPlatform/slurm-gcp/blob/5.12.2/docs/hybrid.md
[demo-with-cloud-controller-instructions.md]: ./demo-with-cloud-controller-instructions.md

## Validate the Hybrid Cluster

Now that the hybrid configuration has been installed, you can test your new
cloud partitions. First off, run `sinfo` to see your partitions listed side by
side:

```shell
$ sinfo
PARTITION AVAIL  TIMELIMIT  NODES  STATE NODELIST
static*      up   infinite      4   idle cluster-static-ghpc-[0-3]
compute      up   infinite     20  idle~ hybridconf-compute-ghpc-[0-19]
debug        up   infinite     10  idle~ hybridconf-debug-ghpc-[0-9]
```

To verify that your local partitions are still active, run a simple test with
`srun`:

```shell
$ srun -N 1 -p "<<Local_Partition_Name>>" hostname
cluster-static-ghpc-0
```

Now verify the cloud partition is running with a similar test. Note that since a
node is being created, the same command will take much longer the first time.
Subsequent uses of the cloud nodes before being suspended will be near
instantaneous after the initial startup cost.

```shell
$ srun -N 1 -p debug hostname
hybridconf-debug-ghpc-0
```
