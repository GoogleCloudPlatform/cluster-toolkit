# Hybrid Slurm Cluster Demonstration With GCP Static Cluster

## Description
These instructions step through the setup and execution of a demo of the HPC
Toolkit hybrid module. In this process you will:

* Setup networking and internal DNS peering between 2 GCP projects
* Deploy a [static cluster](#deploy-a-static-cluster) that will simulate an
  on-premise cluster using the HPC Toolkit and
  [SchedMD's Slurm on GCP][slurm-gcp] terraform modules.
* Create and deploy a hybrid deployment directory using the HPC Toolkit
* Run through a few manual steps of integrating the hybrid configurations
  created with the hybrid deployment directory.
* Test the new hybrid controller.

These instructions are provided for demonstration purposes only. This process
may serve as a first step in evaluating the HPC Toolkit's hybrid slurm module
for use with an on-premise slurm-cluster.

> **Warning:** The [hybrid module][hybridmodule] is in active development and
> the interface is not guaranteed to be static. As the module matures and
> further testing is done, documentation on applying the hybrid module to
> on-premise slurm clusters will be added and expanded.

[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/v5.1.0

## Definitions

**_static cluster:_** The static cluster will simulate an on-premise slurm cluster
for the purposes of this all-GCP demo. The static cluster will be deployed with
slurm-gcp and optionally have a set of statically created VMs populating it's
local partition.

**hybrid deployment:** A deployment using the [schedmd-slurm-gcp-v5-hybrid]
module. The deployment itself includes the hybrid configuration directory as
well as metadata in the cloud bursting project.

**hybrid configuration directory:** The directory created locally by the
[hybrid module][hybridmodule]. This directory contains the required
configuration files and scripts needed to convert a static cluster to a cloud
hybrid cluster.

[hybridmodule]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md

**cloud bursting:** Cloud bursting refers to creating new compute VM instances
in the cloud elastically that can be used to complete slurm jobs.

**compute node:** In this document, a compute node specifically will refer to
the compute VM instances created by the hybrid configuration.

## More Information
To learn more about the underlying terraform modules that support this demo, you
can visit the [slurm-gcp] repo. Specifically, the hybrid documentation can be
found at [docs/hybrid.md][slurm-gcp-hybrid].

## Blueprints

* [create-networks.yaml] creates VPC networks in 2 projects with IP ranges that
  are suitable for setting up bidirectional network peering. These networks will
  be used by subequent blueprints.
* [static-cluster.yaml] defines a slurm cluster with 4 static nodes that will be
  used to simulate an on-premise slurm cluster.
* [hybrid-configuration.yaml] sets up the hybrid project and creates a hybrid
  configuration directory with all required configurations and scripts.

[create-networks.yaml]: ./blueprints/create-networks.yaml
[static-cluster.yaml]: ./blueprints/static-cluster.yaml
[hybrid-configuration.yaml]: ./blueprints/hybrid-configuration.yaml

## Debugging Suggestions

### Logging
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

### Connectivity Issues
To verify the network and DNS peering setup was successful, you can create a VM
in each project attached to the networks created in these instructions. You can
run ping `<VM NAME>.c.<OTHER PROJECT ID>.internal` to verify the settings are
correct. This should succeed in both directions.

If the ping test doesn’t work, the DNS may not be configured correctly, or the
networks may not be able to peer correctly. If it’s the former, you should be
able to ping the internal IP of the other VM. If you cannot, the firewall rule
or network peering setting are likely not correct.

## Instructions

### Before you begin
* Build ghpc

#### Select or Create 2 GCP Projects

This process will require 2 projects:

* Project A: Where the simulated “On-premise” static slurm cluster will be
  deployed.
* Project B: Where the cloud partitions will create new compute VM instances to
  complete slurm jobs.

Identify the 2 projects you intend to use. "Project A" and "Project B" will be
referred to in the rest of this document based on these definitions.

#### Enable Required APIs

The following APIs are required to complete this demo:

* [Compute Engine API][computeapi]
* [Cloud DNS API][clouddnsapi]

[computeapi]: https://cloud.google.com/compute/docs/reference/rest/v1
[clouddnsapi]: https://cloud.google.com/dns/docs/reference/v1

#### Set IAM Roles
The service account attaches to the slurm controller in Project A
([see above](#select-or-create-2-gcp-projects))
must have the Editor role in
Project A and Project B. If not specified, this will be the
[default compute engine service account][computesa].

[computesa]:https://cloud.google.com/compute/docs/access/service-accounts#default_service_account

#### Dependencies
This demo has the same baseline dependencies as the HPC Toolkit that are
outlined in the main [README.md](../../README.md#dependencies).

In addition, some pip packages need to be installed locally. Run the following
command to install the pip packages outlined in
[requirements.txt](./requirements.txt):

```shell
pip install -r requirements.txt
```

#### Build ghpc

Before you begin, ensure that you have built the `ghpc` tool in the HPC Toolkit.
For more information see the [README.md](../../README.md#quickstart) Quickstart.

### Create VPC Networks
A blueprint for creating VPC networks in each project that can support network
and DNS peering can be found at [create-networks.yaml]. This
blueprint will do the following:

* Create a network named `static-cluster-network` in project A.
* Create a subnetwork of `static-cluster-network` named `primary-subnet` with
  an internal IP range of 10.0.0.0/16.
* Create a network named `compute-vpc-network` in project B.
* Create a subnetwork of `compute-vpc-network` named `primary-subnet` with an
  internal IP range of 10.1.0.0/16

Create a deployment directory for the networks using `ghpc`:

```shell
ghpc create blueprints/create-networks.yaml --vars project_id="<<Project_A_ID>>",project_id_compute="<<Project_B_ID>>"
```

If successful, this command will provide 3 terraform operations that can be
performed to deploy the deployment directory. They should look similar to the
following:

```shell
Terraform group was successfully created in directory peering-networks-demo/primary
To deploy, run the following commands:
  terraform -chdir=peering-networks-demo/primary init
  terraform -chdir=peering-networks-demo/primary validate
  terraform -chdir=peering-networks-demo/primary apply
```

Execute the terraform commands to deploy the two networks.

### Allow Peering Between VPC Networks
Bidirectional VPC and DNS peering is needed between both networks created
in the last step. [VPC peering][netpeering] allows internal IP address
connectivity between the projects. [DNS peering][dnspeering] allows resolution
of the fully qualified hostname of instances in the other project in the current
project.

These instructions will step you through how to set up both of these peering
connections via the [cloud console][console].

[netpeering]: https://cloud.google.com/vpc/docs/vpc-peering
[dnspeering]: https://cloud.google.com/dns/docs/overview
[console]: https://cloud.google.com/cloud-console

#### Setup VPC Peering
First, set up VPC peering from Project A to Project B:

* Navigate to the [VPC Network Peering][netpeeringconsole] page in the GCP
  console.
* Click on [Create Peering Connection][createpeering].
* Click "CONTINUE" if prompted to gather additional information (project ID, IP
  ranges, etc)
* Provide the following information:
  * **_Name:_** The name of the peering connection, for example
    "hybrid-demo-network-peering".
  * **_Your VPC Network:_** The name of the VPC network in this project created
    in the last step, by default "static-cluster-network" for project A and
    "compute-vpc-network" for project B.
  * **_Peered VPC Network_** Select "In another project"
    * **_Project ID:_** The name of the other project.
    * **_VPC network name:_** The name of the VPC network in the other project,
      "compute-vpc-network" if creating from project A or
      "static-cluster-network" if creating from project B.
  * All other fields can be left alone.
* Click "CREATE".

Repeat these same steps in Project B.

When complete, both [network peering connections][netpeeringconsole] should show
a green check icon and be listed as "Active".

Next, set up firewall rules in each project that allow data to pass between the
peered networks. Starting in project A, do the following:

* Navigate to the [VPC Networks][vpcnetworks] page in the GCP console.
* Click on the network created in the prior step, "static-cluster-network" for
  project A and "compute-vpc-network" for project B.
* Click on the tab titled "FIREWALLS".
* Click on "ADD FIREWALL RULE".
* Provide the following information:
  * **_Name:_** The name of the firewall rule, for example
    "allow-peering-connection".
  * **_Network:_** The name of the network, this should already be filled in.
  * **_Direction of traffic:_** Ingress
  * **_Action on match:_** Allow
  * **_Targets:_** All instances in the network
  * **_Source filter:_** IPv4 ranges
  * **_Source IPv4 ranges:_** 10.0.0.0/8
  * **_Protocols and Ports:_** Specified protocols and ports
    * TCP: 0-65532
    * UDP: 0-65532
    * Other: icmp
* Click "CREATE"

Repeat these same steps in Project B.

[netpeeringconsole]: https://console.cloud.google.com/networking/peering/list
[createpeering]: https://console.cloud.google.com/networking/peering/add
[vpcnetworks]: https://console.cloud.google.com/networking/networks/list

#### Setup DNS Peering
First, set up private DNS peering from Project A to Project B:

* Navigate to the [Cloud DNS][dnszones] page in the GCP console.
* Click on "CREATE ZONE".
* Provide the following information:
  * **_Zone Type:_** Private
  * **_Zone name:_** The name of the DNS zone, for example
    "hybrid-demo-dns-zone".
  * **_DNS name:_** `c.<<Project_B_ID>>.internal` replacing `<<Project_B_ID>>`
    with the project ID of project B. When adding the zone in project B, the
    DNS name will be `c.<<Project_A_ID>>.internal`.
  * **_Options:_** DNS Peering
  * **_Networks:_** The network created in the prior step in this project,
    "static-cluster-network" for project A and  "compute-vpc-network" for
    project B.
  * **_Peer Project:_** The project ID of the other project.
  * **_Peer Network:_** The network name created in the last step in the peer
    project, "compute-vpc-network" if creating from project A or
    "static-cluster-network" if creating from project B.
* Click "CREATE"

Repeat these same steps in Project B.

[dnszones]: https://console.cloud.google.com/net-services/dns/zones

### Deploy a Static Cluster

The blueprint defined by [static-cluster.yaml] in the blueprints directory will
create a new slurm cluster with the following:

* A pointer to the network created in [Create VPC Networks](#create-vpc-networks)
  in project A, "static-cluster-network".
* A new filestore instance that will serve as the local scratch network
  filesystem.
* One partition with 4 static nodes (compute VMs that are always up) of machine
  type n2-standard-2. This will be the default partition.
* A Slurm controller and login node.

First, use the HPC Toolkit to create the deployment directory, replacing
"<<Project A ID>>" with the ID of your project A:

```shell
ghpc create blueprints/static-cluster.yaml --vars project_id="<<Project A ID>>"
```

If successful, this command will provide 3 terraform operations that can be
performed to deploy the deployment directory. They should look similar to the
following:

```shell
Terraform group was successfully created in directory peering-networks-demo/primary
To deploy, run the following commands:
  terraform -chdir=static-slurm-cluster/primary init
  terraform -chdir=static-slurm-cluster/primary validate
  terraform -chdir=static-slurm-cluster/primary apply
```

Execute the terraform commands to deploy the static Slurm cluster in project A.

### Use the Cloud HPC Toolkit to Create the Hybrid Deployment Directory
The blueprint for creating a deploying the hybrid configuration can be found in
the blueprints directory as [hybrid-configuration.yaml]. This blueprint defines
a deployment that does the following:

* Create a pointer to the network in project B created in
  [Create VPC Networks](#create-vpc-networks).
* Create a filestore for a cloud scratch network filesystem.
* Create a single partition named "cloud" with a dynamic maximum size of 10
  nodes of machine type n2-standard-2.
* Creates a hybrid configuration using the
  [`schedmd-slurm-gcp-v5-hybrid`][hybridmodule] module. This module will do the
  following:
  * Create a directory at `output_dir` locally containing the hybrid
    configuration files and execution scripts.
  * Set metadata in project B that inform the burst compute nodes how to
    configure themselves.
  * Create pubsub actions triggered by changes to the hybrid configuration.

Either in the blueprint directly or on the command line, update the following
deployment variables in the [hybrid-configuration.yaml] blueprint:

* **_project\_id:_** The ID of project B.
* **_static\_controller\_hostname:_** The fully qualified internal hostname of
  the static cluster's controller in project A. The format is
  `<<instance_name>>.c.<<Project_A_ID>>.internal`.

If the deployment vars have been added directly to the blueprint, the following
command will create the deployment directory:

```shell
ghpc create blueprints/hybrid-configuration.yaml
```

To create the deployment directory with deployment variables passed through the
command line, run the following command with the updated values of
`<<Project_B>>`, `<<Hostname>>` and `<<Homefs_IP>>` instead:

```shell
ghpc create blueprints/hybrid-configuration.yaml --vars project_id="<<Project_B>>",static_controller_hostname="<<Hostname>>.c.<<Project_A>>.internal"
```

If successful, this command will provide 3 terraform operations that can be
performed to deploy the deployment directory. They should look similar to the
following:

```shell
Terraform group was successfully created in directory peering-networks-demo/primary
To deploy, run the following commands:
  terraform -chdir=hybrid-config/primary init
  terraform -chdir=hybrid-config/primary validate
  terraform -chdir=hybrid-config/primary apply
```

Execute the terraform commands to create the hybrid configuration. A directory
in `hybrid-configuration/primary` named `hyrid/` should be created which
contains a `cloud.conf` file, `cloud_gres.conf` file and a set of support
scripts.

[hybridmodule]: ../../community/modules/scheduler/schedmd-slurm-gcp-v5-hybrid/README.md

### Install and Configure Hybrid on the Controller Instance

> **_NOTE:_** Many of the manual steps in this section have been adapted from the
> hybrid documentation in [Slurm on GCP][slurm-gcp]. The source document can be
> found at [docs/hybrid.md][slurm-gcp-hybrid]

Now that the hybrid configuration directory has been created, it needs to be
installed on the controller VM instance. First, tar the directory:

```shell
cd hybrid-config/primary
tar czvf hybrid.tar.gz hybrid
```

Copy the `hybrid.tar.gz` file to the controller VM instance. This can be done
in whichever way is easiest for you, `gcloud compute scp` is used here.

```shell
gcloud compute scp --project="<<Project_A>>" --zone=us-central1-c ./hybrid.tar.gz "<<Controller_Name>>:~"
```

Now SSH to the controller VM either using the console or the following gcloud
command:

```shell
gcloud compute ssh --project="<<Project_A>>" --zone=us-central1-c "<<Controller_Name>>"
```

Decompress the `hybrid.tar.gz` file:

```shell
sudo tar xzvf hybrid.tar.gz --directory /etc/slurm
rm hybrid.tar.gz
```

Set the correct permissions for the hybrid directory and the files contained in
it:

```shell
sudo chown -R slurm: /etc/slurm/hybrid
sudo chmod 644 /etc/slurm/hybrid/cloud.conf
sudo chmod 755 /etc/slurm/hybrid
```

Because the static cluster was also created by [Slurm on GCP][slurm-gcp]
terraform modules, the partition information must be copied from the file
`/etc/slurm/cloud.conf` to the slurm config file at `/etc/slurm/slurm.conf`. The
lines that need to be copied will look similar to the following block:

```text
NodeName=DEFAULT State=UNKNOWN RealMemory=7552 Boards=1 Sockets=1 CoresPerSocket=1 ThreadsPerCore=1 CPUs=1
NodeName=staticslur-static-ghpc-[0-3] State=CLOUD
NodeSet=staticslur-static-ghpc Nodes=staticslur-static-ghpc-[0-3]
PartitionName=static Nodes=staticslur-static-ghpc State=UP DefMemPerCPU=7552 SuspendTime=300 Oversubscribe=Exclusive Default=YES

SuspendExcNodes=staticslur-static-ghpc-[0-3]
```

Depending on the configuration of the static partitions, the `SuspendExcNodes`
may not be included.

These lines can be copied to the bottom of the `slurm.conf` file.

Make the following changes to the `/etc/slurm/slurm.conf` file:

* replace `include cloud.conf` with `include hybrid/cloud.conf`
* Add the fully qualified hostname in parentheses after the controller hostname
  in the parameter `SlurmctldHost`.

```text
# slurm.conf
...
SlurmctldHost=<<Controller_Hostname>>(<<Controller_Hostname>>.c.<<Project_A_ID>>.internal)
...
include hybrid/cloud.conf
...
```

Make the following changes to the `/etc/slurm/hybrid/cloud.conf` file:

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
communicate rather than the hostnames.

Next, create a new cronjob as the slurm user that will periodically call the
`/etc/slurm/hybrid/slurmsync.py` file.

```shell
sudo su slurm
crontab -e
```

Since the controller was deployed using [Slurm on GCP][slurm-gcp], there will
already be a cronjob pointing to the `slurmsync.py` script in `/etc/slurm/`,
simply update it to the following:

```text
*/1 * * * * /etc/slurm/hybrid/slurmsync.py
```

Exit the editor and the slurm user when complete.

Finally, restart the slurmctld service to enable the changes made:

```shell
sudo systemctl restart slurmctld
```

If the restart did not succeed, the logs at `/var/log/slurm/slurmctld.log`
should point you in the right direction.

[slurm-gcp]: https://github.com/SchedMD/slurm-gcp/tree/v5.1.0
[slurm-gcp-hybrid]: https://github.com/SchedMD/slurm-gcp/blob/v5.1.0/docs/hybrid.md

### Validate the Hybrid Cluster

Now that the hybrid configuration has been installed, you can test your new
cloud partition. First off, run `sinfo` to see your partitions listed side by
side:

```shell
$ sinfo
PARTITION AVAIL  TIMELIMIT  NODES  STATE NODELIST
static*      up   infinite      4   idle staticslur-static-ghpc-[0-3]
cloud        up   infinite     10  idle~ hybridconf-cloud-ghpc-[0-9]
```

To verify that your local partitions are still active, run a simple test with
`srun`:

```shell
$ srun -N 1 hostname
staticslur-static-ghpc-0
```

Now verify the cloud partition is running with a similar test. Note that since a
node is being created, the same command will take much longer the first time.
Subsequent uses of the cloud nodes before being suspended will be near
instantaneous after the initial startup cost.

```shell
$ srun -N 1 -p cloud hostname
hybridconf-cloud-ghpc-0
```
