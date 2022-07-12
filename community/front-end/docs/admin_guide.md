# HPC Toolkit FrontEnd - Administrator’s Guide
<!--
0        1         2         3         4         5         6         7        8
1234567890123456789012345678901234567890123456789012345678901234567890234567890
-->
This document is for administrators of the HPC Toolkit FrontEnd (TKFE). An
administrator can manage the life cycle of HPC clusters, set up networking and
storage resources that support clusters, install applications and manage user
access. Ordinary HPC users should refer to the [User Guide](user_guide.md) for
guidance on how to prepare and run jobs on existing clusters.

The HPC TKFE is a web application built upon the Django framework.
By default, a single Django superuser is created at deployment time that has
administrator privileges. For large organisations that need additional
administrators, additional Django superusers can be created from the Admin site
within the FrontEnd, once it is deployed and running.

This guide covers the deployment of the TKFE and the steps required to create
and manage clusters.  Further guides cover installation of
[applications](Applications.md) and the [Workbench](WorkbenchAdmin.md) Jupyter
Notebook feature.



## TKFE Deployment

### Prerequisites

#### Download TKFE

If not already downloaded, the TKFE repository needs to be cloned to a client machine from where deployment will take place.
<!-- TODO: insert link to official repository name when released as public project]
[TKFE repository](https://github.com/nagconsulting/hpc-toolkit.git)
-->
Clone the repository for TKFE, checkout the corresponding branch, and switch to the working directory as follows:
```bash
$ git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git
$ cd hpc-toolkit
$ git checkout new_frontend
$ cd community/frontend
```
 
Deployment must be started from this directory location.

#### Client Machine

The client deploying TKFE has a number of prerequisites - these must be
installed ahead of time.

- Linux environment (or an equivalent such as WSL) with bash shell
- [Terraform CLI](https://www.terraform.io/downloads) installation
- Google [Cloud CLI](https://cloud.google.com/cli) installation (`gcloud`
  utility)
- Google cloud user for deployment
  [authenticated in gcloud](https://cloud.google.com/sdk/gcloud/reference/auth/login) (see below for required permissions)

#### Google Cloud Platform

A GCP Project and user account with enabled APIs and roles/permissions are
also needed to deploy TKFE.

Please refer to GCP documentation for
[how to add APIs](https://cloud.google.com/endpoints/docs/openapi/enable-api) 
and
[granting and roles/permissions](https://cloud.google.com/iam/docs/creating-custom-roles) to Projects and Users respectively.

The GCP Project requires the following APIs :

- Compute Engine API
- Cloud Monitoring API
- Cloud Logging API
- Cloud Pub/Sub API
- Cloud Resource Manager
- Identity and Access Management (IAM) API
- Cloud OS Login API
- Cloud Filestore API
- Cloud Billing API
- Vertex AI API

<!--
 TODO:  TKFE has the mechanics to allow the FE to be created from one GCP
  project, but the clusters deployed against another.   This isn't expected to
  be used in anger, at least in the short term, and does complicate things.
  The interface and docs will need updating and simplifying.  For now, assume
  a single project for both TKFE and all clusters from it.

- GCP project(s):
- for the HPC Toolkit FrontEnd with at least the following APIs enabled:
    - Compute Engine API
    - Cloud Monitoring API
    - Cloud Logging API
    - Cloud Pub/Sub API
    - Cloud Resource Manager
    - Identity and Access Management (IAM) API

  - A GCP project for deploying Clusters with at least the following APIs
    enabled:

    - Compute Engine API
    - Cloud Monitoring API
    - Cloud Resource Manager
    - Cloud Logging API
    - Cloud OS Login API
    - Cloud Filestore API
    - Cloud Billing API
    - Vertex AI API

- These two projects may actually be the same project.  The HPC Toolkit
    FrontEnd supports deploying clusters into the same project in which the
    service machine runs, as well as deploying clusters into other projects.
-->

The *Owner* of the project will automatically have all required roles and
permissions, so this account can be used without any further modification.
Alternatively, another account with a more limited collections of roles, which
can help satisfy security concerns, can be used.  The `gcloud` command can be used [to switch to another account](https://cloud.google.com/sdk/gcloud/reference/auth/login). The required roles are:

- Compute Admin
- Storage Admin
- Pub/Sub Admin
- Create Service Accounts
- Delete Service Accounts
- Service Account User
- Project IAM Admin

If required, a strict, least-privilege custom role can be created - please refer
to the [complete list](CompletePermissions.md) of required permissions.



### Deployment Process
TKFE uses a deployment script run on the client machine, prompting for required parameters, to configure TKFE.

1. Run `./deploy.sh`
1. Follow prompts to name the FrontEnd VM instance, specify the GCP project, zone and subnet (subnet is optional, and one will be created if required). The hosting VM will be referred to as the *service machine* from now on.
    - For a production deployment, provide a domain name and static IP address.  If a static IP is needed, follow the on-screen instructions. An SSL certificate will automatically be obtained via [LetsEncrypt](https://letsencrypt.org/) to secure the web application.
    - For testing purposes the domain name and static public IP address and domain name can be left blank. The system can still be successfully deployed and run with an ephemeral IP address, however OAuth2-based login (see later) will not be available as this requires a publicly resolvable domain name.
1. Follow instructions to provide details for an Admin (the Django superuser) account.
1. Confirm the parameters are correct when prompted and the deployment can proceed.
    - If confirmed, the VM instance will be created.  This can take a few
      minutes    .
    - **Note: after the deploy script has completed, it will still take up to
      another 15 minutes to fully install the TKFE server will the full software
      stack.**

<!--
TODO:  Give instruction for a custom deployment.

If a custom TKFE deployment is needed, the final step can be cancelled  Then hack any files in ./tf and manually run `terraform apply` in the `frontend/tf` directory after properly setting the `terraform.tfvars`.
-->

**Important: To ensure that the TKFE resources can be cleaned up fully at a later date, ensure that the directory containing the terraform configuration (`./tf`) is retained.**

The deployment is now complete and after ~15 minutes, it will be possible to log
into the TKFE using the Admin account details given, at the specified domain
name or IP address via a web browser.   The IP address will be output as
`server_ip`.


## Post-deployment Configuration

### Credential Management

To allow TKFE to manage cloud resources on behalf of users, a
[service account](https://cloud.google.com/iam/docs/service-accounts)
with credentials must be registered to the system by the Admin user. These are
first created within GCP console, then entered in the *Credentials* menu of the
TKFE.


<!-- The supplied credential will be validated and stored in the database for
 future use.

The preferred way to access GCP resources from this system is through a
[service account](https://cloud.google.com/iam/docs/service-accounts).
To create a service account, a GCP account with sufficient permissions is
required.
Typically, the Owner or Editor of a GCP project have enough permissions. For
other users with custom roles, if certain permissions are missing, GCP will
typically return clear error messages.

For this project, the following roles should be sufficient for the admin users to manage the required service account: *Service Account User*, *Service Account Admin*, and *Service Account Key Admin*.-->

#### Creating a service account via the GCP Console

1. Log in to the [GCP console](https://console.cloud.google.com/) and select the 
   GCP project that hosts the TKFE.
   
1. From the Navigation menu, select *IAM & Admin*, then *Service Accounts*.
    - Click the *CREATE SERVICE ACCOUNT* button.
    - Name the service account, optionally provide a description, and then click
      the *CREATE* button.
      
1. Grant the service account the following roles:
    - Cloud Filestore Editor
    - Compute Admin
    - Create Service Accounts
    - Delete Service Accounts
    - Project IAM Admin
    - Notebooks Admin
    - Vertex AI administrator

- <!-- Human users may be given permissions to access this service account but that is not required in this work.--> Click *Done* button.

1. Locate the new service account from the list, click *Manage Keys* from the *Actions* menu.
    - Click *ADD KEY*, then *Create new key*.
        - Select JSON as key type, and click the *CREATE* button.
        - A JSON key file will then be downloaded.
        - Copy the generated JSON content which should then be pasted into the
          credential creation form within the TKFE.

1.  Click *Validate and Save* to register the new credential to TKFE.

<!--
TODO: check this actually works before documenting as a method

#### Creating a service account using the `gcloud` tool

Alternatively the `gcloud` command line tool can be used to create a suitable service account:

```bash
$ gcloud iam service-accounts create <service_account_name>
$ for roleid in file.editor \
              compute.admin \
              iam.serviceAccountCreator \
              iam.serviceAccountDelete \
              resourcemanager.projectIamAdmin \
              notebooks.admin aiplatform.admin; \
  do gcloud projects add-iam-policy-binding <project_name> \
      --member="serviceAccount:<service_account_name>@<project_name>.iam.gserviceaccount.com" \
      --role="roles/$roleid"; \
  done

$ gcloud iam service-accounts keys create <path_to_key_file> \
    --iam-account=<service_account_name>@<project_name>.iam.gserviceaccount.com
```

Once complete, the service account key json can be copied from
`path_to_key_file` into the credentials form on the frontend.
-->

The credential can now be used to create network, storage and compute resources
from the TKFE.

## Network Management

All cloud systems begin with defining the network within which the systems will
be deployed. Before a cluster or stand-alone filesystem can be created, the
administrator must create the virtual private cloud (VPC) network. This is
accomplished under the *Networks* main menu item. Note that network resources
have their own life cycles and are managed independently.

### Create a new VPC
To create a new network, the admin must first select which cloud credential
should be used for this network, then give the VPC a name, and then select the
cloud region for the network.

Upon clicking the *Save* button, the network is not immediately created. The
admin has to click *Edit Subnet* to create at least one subnet. Once the network
and subnets are appropriately defined, click the ‘Apply Cloud Changes’ button to
trigger Terraform to provision the  cloud resources.

### Import an existing VPC

If the organisation already has predefined VPCs on cloud within the hosting GCP
project, they can be imported. Simply selecting an existing VPC and associated
subnets from the web interface to register them with the system. Imported VPCs
can be used in exactly the same way as newly created ones.

## Filesystem Management

By default each cluster creates two shared filesystems: one at `/opt/cluster` to
hold installed applications and one at `/home` to hold job files for individual
users. Both can be customised if required. Optionally, additional filesystems
may be created and mounted to the clusters. Note that filesystem resources have
their own life cycles and are managed independently, so they persist until
explicitly deleted and can be attached to several clusters.

### Create new filesystems

Currently, only GCP Filestore is supported. GCP Filestore can be created from
the *Filesystems* main menu item. A new Filestore has to be associated with an
existing VPC and placed in a cloud zone. All performance tiers are supported.

### Import existing filesystems

Existing filesystems can be registered to this system and subsequently mounted
by clusters. These can be existing NFS servers (like Filestore), or other
filesystems for which Linux has built-in mount support. For this to work, for
each NFS server, provide an IP address and an export name. The IP address must
be reachable by the VPC subnets intended to be used for clusters.

An internal address can be used if the cluster shares the same VPC with the
imported filesystem. Alternatively, system administrators can set up hybrid
connectivity (such as extablishing network peering) beforing mounting the
external filesystem located elsewhere on GCP.

## Cluster Management

HPC clusters can be created after setting up the hosting VPC and any
additional filesystems. The HPC Toolkit FrontEnd can manage the whole life
cycles of clusters. Click the *Clusters* item in the main menu to list all
existing clusters.

### Cluster Status
Clusters can be in different states and their *Actions* menus adapt to this
information to show different actions:

- `n` – Cluster is being newly configured by user. At this stage, a new
  cluster is being set up by an administrator. Only a database record exists,
  and no cloud resource has been created yet. User is free to edit this cluster:
  rename it, re-configure its associated network and storage components, and add
  authorized users. Click *Start* from the cluster detail page to actually
  provision the cluster on GCP.
- `c` – Cluster is being created. This is a state when the backend
  Terraform scripts is being invoked to commission the cloud resources for the
  Cluster. This transient stage typically lasts for a few minutes.
- `i` – Cluster is being initialised. This is a state when the cluster
  hardware is already online, and Ansible playbooks are being executed to
  install and configure the software environment of the Slurm controller and
  login nodes. This transient stage can last for up to 15 minutes.
- `r` – Cluster is ready for jobs. The cluster is now ready to use.
  Applications can be installed and jobs can run on it. A Slurm job scheduler is
  running on the controller node to orchestrate job activities.
- `t` – Cluster is terminating. This is a transient state after Terraform
  is being invoked to destroy the cluster. This stage can take a few minutes
  when Terraform is working with the cloud platform to decommission cloud
  resources.
- `d` – Cluster has been destroyed. When destroyed, a cluster cannot be
  brought back online. Only the relevant database record remains for information
  archival purposes.

A visual indication is shown on the website for the cluster being in creating,
initialising or destroying states. Also, relevant web pages will refresh every
15 seconds to pick status changes.

### Create a new cluster

A typical workflow for creating a new cluster is as follows:

1. At the bottom of the cluster list page, click the *Add cluster* button to
   start creating a new cluster. In the next form, choose a cloud credential.
   This is the Google Service Account which will create the cloud resources.
   Click the *Next* button to go to a second form from which details of the
   cluster can be specified.
1. In the *Create a new cluster* form, give the new cluster a name. Cloud
   resource names are subject to naming constraints and will be validated by the
   system.  In general, lower-case alpha-numeric names with hyphens are
   accepted.
1. From the *Subnet* dropdown list, select the subnet within which the cluster
   resides.
1. From the *Cloud zone* dropdown list, select a zone.
1. From the *Authorised users* list, select users that are allowed to use this
   cluster.
1. Click the *Save* button to store the cluster settings in the database.
   Continue from the *Cluster Detail* page.
1. Click the *Edit* button to make additional changes. such as creating more
   Slurm partitions for different compute node instance types, or mounting
   additional filesystems.
    - For filesystems, note the two existing shared filesystems defined by
      default. Additional ones can be mounted if they have been created earlier.
      Note the *Mounting order* parameter only matters if the *Mount path*
      parameter has dependencies.
    - For cluster partitions, note that one *c2-standard-60* partition is
      defined by default. Additional partitions can be added, supporting
      different instance types. Enable or disable hyperthreading and node reuse
      as appropriate. Also, placement group can be enabled (for C2 and C2D
      partitions only). In the *image* field one can optionally supply a custom
      image to be used by the compute nodes. Administrators should ensure such
      an image is compatible to the CentOS 7 based machine image used by Slurm 
      GCP. Otherwise additional customisation done by this system might fail.
1.  Finally, save the configurations and click the *Create* button to trigger
    the cluster creation.

### Deleting a cluster

To destroy a cluster, first find the list of clusters in the *Clusters* menu, then simply select *Destroy* pop-down *Actions* menu and confirm.
Any jobs still running on the Cluster will be automatically killed.

## User Management

### SSH access to the service machine

SSH access to the service machine is possible for administration purposes.
Administrators can choose from one of the following options:

- [SSH directly from the GCP console](https://cloud.google.com/compute/docs/instances/connecting-to-instance).
- [Add their public SSH key to the VM instance after deployment via GCP console](https://cloud.google.com/compute/docs/connect/add-ssh-keys#add_ssh_keys_to_instance_metadata).
- [Add their SSH key to the GCP project to use on all VMs within the project](https://cloud.google.com/compute/docs/connect/add-ssh-keys#add_ssh_keys_to_project_metadata).

*N.B The service machine is not, by default, configured to use the os-login service.*

### Set up Google OAuth2 login

While it is possible to use a Django user account to access the FrontEnd
website, and indeed doing so is required for some administration tasks, standard
users must authenticate using their Google identities via Google OAuth2.  This,
combined with the use of Google OSLogin for access to clusters, ensures
consistent Linux identities across VM instances that form the clusters. Web 
frontend login is made possible by the *django-allauth* social login extension. 

For a working  deployment, a fully-qualified domain name must be obtained and
attached to the website as configured in the deployment script.  Next, register
the site with the hosting GCP project on the GCP console in the *Credentials*
section under *APIs and services* category. Note that the *Authorised JavaScript
origins* field should contain a callback URL in the following format:
*https://<domain_name>/accounts/google/login/callback/*

![Oauth set-up](images/GCP-app-credential.png)

From the GCP console, note the client ID and client secret. Then return to admin
site of the deployment, locate the *social applications* database table. A
'Google API' record should have been created during the deployment. Replace the
two placeholders with the client ID and client secret. The site is ready to
accept Google login.

![Social login set-up](images/register-social-app.png)]

#### Set Allowed Users by Email Address

Next, go to the *Authorised user* table. This is where further access control to
the site is applied. Create new entries to grant access to users. A new entry
can be:

- a valid domain name to grant access to multiple users from authorised 
organisations (e.g. *@example.com*)
- an email address to grant access to an individual user (e.g
*user.name@example.com*)

All login attempts that do not match these patterns will be rejected.

#### Note on external users

If you wish to allow users from outside your Google Cloud organization to use the cluster you will need to additionally assign these users the `roles/compute.osLoginExternalUser` role at an Organization level (there is no way to assign at the project level).

User accounts will be automatically created for users when they log into the frontend for the first time, by default new accounts are created with quota disabled.  To enable job submission for an account, administrators must enable compute quota from the *Users* page.

### User Compute Quota

Currently three quota modes are supported:

- **Unlimited quota** - User may submit an unlimited number of jobs
- **Limited quota** - User may submit jobs up to a total spend limit in USD
- **Quota Disabled** - User may not submit jobs - this is the default for newly created accounts

When **limited quota** is selected, an additional field **quota amount** will be available to set the total spend available to the user.

## Application Management

Please see the [application installation guide](Applications.md).

## Workbench Management

Please see the [Workbench Admin Guide](WorkbenchAdmin.md).


## Teardown Process

The TKFE package contains a `teardown.sh` script that will destroy the running
FrontEnd instance. This script only removes the FrontEnd, not resources started from it.

**Before running the teardown script, ensure that all clusters, VPCs,
workbenches and filestores are removed using the TKFE web interface before
destroying it. These resources will otherwise persist and accrue costs.**

To tear down the web interface and its hosting infrastructure, run 
directory `./teardown.sh` on the original client machine
in the same directory that was used to deploy TKFE.


## Troubleshooting

### Finding Log Files

The service machine produces log files in `/opt/gcluster/run/`. These log files will show errors from the Django web application.

Cloud resource deployment log files (from Terraform) are typically shown via the Frontend web site.  If those logs are not being shown, they can be found on the service machine under `/opt/gcluster/hpc-toolkit/frontend/(clusters|fs|vpc)/...`.  HPC Toolkit log files will also be found in those directories.  The Terraform log files and status files will be down a few directories, based off of the Cluster Number, Deployment ID, and Terraform directory.

On Cluster controllers, most of the useful log files for debugging can be
retrieved by executing the 'Sync Cluster' command.  These include Slurm log
files as well as general system log files.  The daemon which communicates to the
service machine logs to syslog, and can be viewed on the cluster controller node
via `journalctl`, looking at the `ghpcfe_c2` service.

Job logs and Spack application logs are uploaded upon job completion to Google
Cloud Storage and viewable via the HPC Frontend.

### Deployment problems

Most deployment problems are caused by not having the right permissions. If this
is the case, error message will normally show what permissions are missing. Use
the [IAM permissions reference](https://cloud.google.com/iam/docs/permissions-reference)
to research this and identify additional roles to add to your user account.

Before any attempt to redeploy the TKFE, make sure to run
`terraform destroy` in `hpc-toolkit/frontend/tf` to remove cloud resources that
have been already created.

### Cluster problems

The FrontEnd should be quite reliable provisioning clusters. However, in cloud
computing, errors will and do happen from time to time; usually due to changes
in back-end services or other factors beyond scope of the TKFE. For example, a
resource creation could fail because the hosting GCP project has ran out of
certain resource quotas; or an upgrade of an underlying machine image might have
introduced changes that are incompatible to the TKFE, which then needs updating.
It is not possible to capture all such situations.  Here, a list of tips is
given to help debug cluster creation problems. The
[Developer's Guide](developer_guide.md) contains a lot of details on how the
back-end logic is handled, which can also help with certain issues.

- If a cluster is stuck at status 'c', something is wrong with the provisioning
  of cluster hardware.
    - SSH into the service machine and identify the directory containing the
      run-time data for that cluster at `frontend/clusters/cluster_<cluster_id>`
      where `<cluster_id>` can be found on the web interface. Check the
      Terraform log files there for debugging information.
- If a cluster is stuck at status 'i', hardware resources should have been
  commissioned properly and there is something wrong in the software
  configuration stage.
    - Locate the IP address of the Slurm controller node and find its VM
      instance on GCP console. Check its related *Serial port* for system log. 
    - If needed, SSH into the controller from the GCP console to check Slurm
      logs under `/var/log/slurm/`.

### General clean-up tips

- If a cluster is stucked in 'i' state, it is normally OK to find the *Destroy*
button from its *Actions* menu to destroy it.
- For failed network/filesystem/cluster creations, one may need to SSH into the
service machine, locate the run-time data directory, and manually run
`terraform destroy` there for clean up cloud resources.
- Certain database records might get corrupted and need to be removed for failed
clusters or network/filesystem components. This can be done from the Django
Admin site, although adminstrators need to exercise caution while modifying the
raw data in Django database.