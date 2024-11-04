# Hybrid Slurm Cluster Demonstration With GCP Static Cluster

## Description
These instructions step through the setup and execution of a demo of the Cluster
Toolkit hybrid module. In this process you will:

* Setup networking and internal DNS peering between 2 GCP projects
* Deploy a [static cluster](#deploy-a-static-cluster) that will simulate an
  on-premise cluster using the Cluster Toolkit and
  [SchedMD's Slurm on GCP][slurm-gcp] terraform modules.
* Create and deploy a hybrid deployment directory using the Cluster Toolkit
* Run through a few manual steps of integrating the hybrid configurations
  created with the hybrid deployment directory.
* Test the new hybrid controller.

These instructions are provided for demonstration purposes only. This process
may serve as a first step in evaluating the Cluster Toolkit's hybrid slurm module
for use with an on-premise slurm-cluster.

> **Warning:** The [hybrid module][hybridmodule] is in active development and
> the interface is not guaranteed to be static. As the module matures and
> further testing is done, documentation on applying the hybrid module to
> on-premise slurm clusters will be added and expanded.

[slurm-gcp]: https://github.com/GoogleCloudPlatform/slurm-gcp/tree/5.12.2

## Definitions

**_static cluster:_** The static cluster will simulate an on-premise slurm cluster
for the purposes of this all-GCP demo. The static cluster will be deployed with
slurm-gcp and optionally have a set of statically created VMs populating it's
local partition.

**hybrid deployment:** A deployment using the [schedmd-slurm-gcp-v5-hybrid][hybridmodule]
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
  be used by subsequent blueprints.
* [static-cluster.yaml] defines a slurm cluster with 4 static nodes that will be
  used to simulate an on-premise slurm cluster.
* [hybrid-configuration.yaml] sets up the hybrid project and creates a hybrid
  configuration directory with all required configurations and scripts.

[create-networks.yaml]: ./blueprints/create-networks.yaml
[static-cluster.yaml]: ./blueprints/static-cluster.yaml
[hybrid-configuration.yaml]: ./blueprints/hybrid-configuration.yaml

## Troubleshooting

For general troubleshooting advice related to the hybrid configuration
deployment, visit [troubleshooting.md]. Additional troubleshooting tips related
to this demo are included below.

[troubleshooting.md]: ./troubleshooting.md

### Connectivity Issues
To verify the network and DNS peering setup was successful, you can create a VM
in each project attached to the networks created in these instructions. You can
run ping to verify the settings are correct:

```shell
<VM NAME>.c.<OTHER PROJECT ID>.internal
```

This should succeed in both directions.

If the ping test doesn’t work, the DNS may not be configured correctly, or the
networks may not be able to peer correctly. If it’s the former, you should be
able to ping the internal IP of the other VM. If you cannot, the firewall rule
or network peering setting are likely not correct.

## Instructions

### Before you begin

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
* [Filestore API][fileapi]

[computeapi]: https://cloud.google.com/compute/docs/reference/rest/v1
[clouddnsapi]: https://cloud.google.com/dns/docs/reference/v1
[fileapi]: https://cloud.google.com/filestore/docs/reference/rest

#### Set IAM Roles
The service account attached to the slurm controller in Project A
([see above](#select-or-create-2-gcp-projects))
must have the Editor role in
Project A and Project B. If not specified, this will be the
[default compute engine service account][computesa].

[computesa]:https://cloud.google.com/compute/docs/access/service-accounts#default_service_account

#### Dependencies
This demo has the same baseline dependencies as the Cluster Toolkit that are
outlined in the main [README.md](../../README.md#dependencies).

In addition, some pip packages need to be installed locally. Run the following
command to install the pip packages outlined in
[requirements.txt](./requirements.txt):

```shell
pip install -r docs/hybrid-slurm-cluster/requirements.txt
```

#### Build gcluster

Before you begin, ensure that you have built the `gcluster` tool in the Cluster Toolkit.
For more information see the [README.md](../../README.md#quickstart) Quickstart.

The commands in these instructions assume the gcluster binary is installed in a
directory represented in the PATH environment variable. To ensure this is the
case, run `make install` after building `gcluster`:

```shell
make
make install
```

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

Create a deployment directory for the networks using `gcluster`:

```shell
gcluster create docs/hybrid-slurm-cluster/blueprints/create-networks.yaml --vars project_id="<<Project_A_ID>>",project_id_compute="<<Project_B_ID>>"
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

First, use the Cluster Toolkit to create the deployment directory, replacing
"<<Project A ID>>" with the ID of your project A:

```shell
gcluster create docs/hybrid-slurm-cluster/blueprints/static-cluster.yaml --vars project_id="<<Project A ID>>"
```

If successful, this command will provide 3 terraform operations that can be
performed to deploy the deployment directory. They should look similar to the
following:

```shell
Terraform group was successfully created in directory peering-networks-demo/primary
To deploy, run the following commands:
  terraform -chdir=cluster/primary init
  terraform -chdir=cluster/primary validate
  terraform -chdir=cluster/primary apply
```

Execute the terraform commands to deploy the static Slurm cluster in project A.

### Deploy and Install the Hybrid Configuration

Congratulations! You've configured and deployed your static Slurm cluster in GCP
using Slurm on GCP modules in the Cluster Toolkit. The next step is to create,
deploy and validate a hybrid configuration using your static cluster. To do
that, follow the instructions at [deploy-instructions.md](./deploy-instructions.md).
