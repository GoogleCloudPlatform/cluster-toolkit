# Cluster Toolkit FrontEnd - Administrator’s Guide

<!--
0        1         2         3         4         5         6         7        8
1234567890123456789012345678901234567890123456789012345678901234567890234567890
-->

This document is for administrators of the Cluster Toolkit FrontEnd (TKFE). An
administrator can deploy the TKFE portal, manage the lifecycle of HPC clusters,
set up networking and storage resources that support clusters, install
applications. and manage user access. Normal HPC users should refer to the
[User Guide](user_guide.md) for guidance on how to prepare and run jobs on
clusters that have been set up by administrators.

Basic administrator knowledge of the Google Cloud Platform is needed in order to
create projects and user accounts, but all other low-level administration tasks
are handled by the portal.

TKFE is a web application built upon the Django framework.  By default, a
single Django superuser is created at deployment time - this superuser then has
administrator privileges in TKFE.  For large organisations that need additional
administrators, additional Django superusers can be created from the Admin site
within TKFE, once it is deployed and running.

The TFKE web application server uses the
[Cluster Toolkit](https://github.com/GoogleCloudPlatform/cluster-toolkit) to
provision resources for networks, filesystems and clusters, using a service
account that has its credentials registered to TKFE. The service account is
used for access management and billing.

This guide covers the deployment of TKFE, registering the service account, and
the steps required to then create and manage clusters.  Further guides cover
installation of [Applications](Applications.md) and the
[Workbench](WorkbenchAdmin.md) Jupyter Notebook feature.
An [Advanced Administrators Guide](AdvancedAdmin.md) also covers features for
customising the TKFE deployment.

## TKFE Deployment

TKFE is deployed from a client machine using a script. The script will guide
the admin through the setup process, prompting for input of required
parameters, then deploy TKFE into the Google Cloud Platform.

TKFE can also be deployed using a configuration file specified with the --config option. In this case, the script will read the required parameters from the YAML file instead of prompting the user for input. This allows for more streamlined and automated deployment processes. However, it is important to ensure that all required parameters are properly specified in the configuration file before deploying.

### Prerequisites

#### Client Machine

The client machine must run Linux (or equivalent such as
[WSL](https://docs.microsoft.com/en-us/windows/wsl/install)) and there are a
small number of prerequisites that must be installed ahead of time:

- bash shell
- [Git](https://github.com/git-guides/install-git)
- [Terraform CLI](https://www.terraform.io/downloads) installation
- [Google Cloud CLI](https://cloud.google.com/cli) installation (`gcloud`,
  including the `gcloud storage` command)
  
#### Download TKFE

If not already downloaded, the TKFE repository needs to be cloned to the client
machine.

Clone the repository, checkout the corresponding branch, and switch to
the `front-end/ofe` directory as follows:

```bash
git clone https://github.com/GoogleCloudPlatform/cluster-toolkit.git
cd cluster-toolkit
cd community/front-end/ofe
```

All further deployment actions must be performed from this directory.

#### Google Cloud Platform

Your organisation must already have access to the Google Cloud Platform (GCP)
and be able to create projects and users.  A project and a user account with
enabled APIs and roles/permissions need to be created. The user account must
also be authenticated on the client machine to allow it to provision GCP
resources.

##### GCP project

A GCP project is required with the following APIs enabled:

```Text
 Compute Engine API
 Cloud Monitoring API
 Cloud Logging API
 Cloud Pub/Sub API
 Cloud Resource Manager
 Identity and Access Management (IAM) API
 Cloud OS Login API
 Cloud Filestore API
 Cloud Billing API
 Vertex AI API
```

If these are not enabled, the deployment script will ask to enable them for
you. It's not possible to complete a deployment without these APIs, so the
script will abort if not selected.

##### GCP user

A GCP user that is a member of the project will be able to deploy TKFE, but
must have appropriate privileges.  A user that is an *Owner* of the project
will automatically have all required roles and permissions.  Alternatively,
another account with access to the project, but a limited set of roles can be
used, which can help satisfy security concerns. The `gcloud` command can be
used to [switch to another account](https://cloud.google.com/sdk/gcloud/reference/auth/login)
and apply IAM roles.  IAM roles can also be applied via the GCP Console. The
required roles are:

```Text
 Compute Admin
 Storage Admin
 Pub/Sub Admin
 Create Service Accounts
 Delete Service Accounts
 Service Account User
 Project IAM Admin
```

If required, an even stricter, or least-privilege custom role can be created -
please refer to the [Advanced Admin Guide](AdvancedAdmin.md#custom-rolespermissions-and-apis).

The user account must also be [authenticated to deploy GCP resources](https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login).
This can be done with the following command:

```bash
gcloud auth application-default login --project=<PROJECT_ID>
```

You will be prompted to open your web browser to authenticate.

If further help is needed, please refer to GCP documentation for:

- [Creating and managing projects](https://cloud.google.com/resource-manager/docs/creating-managing-projects)
- [Enabling APIs](https://cloud.google.com/endpoints/docs/openapi/enable-api)
- [Granting and roles/permissions](https://cloud.google.com/iam/docs/creating-custom-roles)

### Deployment Process
#### Manual deployment process

TKFE uses a deployment script run on the client machine, prompting for required
parameters, to configure TKFE.

1. Run `./deploy.sh`
1. Follow prompts to name the FrontEnd VM instance, specify the GCP project,
   zone and subnet (subnet is optional, and one will be created if required).
   The hosting VM will be referred to as the *service machine* from now on.
1. Follow prompts to enter domain name (DNS hostname) and IP address for the
   service machine.
   - For a production deployment, provide a domain name and static IP
     address.  If a static IP is needed, follow the on-screen instructions. An
     SSL certificate will automatically be obtained via
     [LetsEncrypt](https://letsencrypt.org/) to secure the web application.
   - For dev and testing purposes, the domain name and static IP address and
     domain name can be left blank. The system can still be successfully
     deployed and run with an ephemeral IP address, however OAuth2-based login
     (see later) will not be available as this requires a publicly resolvable
     domain name.
1. Follow instructions to provide details for an Admin (the Django superuser)
   account - username, password, email address.
1. Confirm whether to create a service account for managing clusters.  This
   will create a service account, and generate a credential, associated with
   the GCP project specified earlier being used to deploy TKFE.  This service
   account may not be needed if using an existing one, or using multiple
   projects (see [advanced admin guide](AdvancedAdmin.md) for details).
   - If selected, a credential file, `credential.json`. will be created in the
     directory that needs to be registered in the FrontEnd (see below).
1. Confirm the parameters are correct when prompted and the deployment can
   proceed.
   - If confirmed, the VM instance will be created.  This can take a few
     minutes.
   - **Note: after the deploy script has completed, it will still take up to
     another 15 minutes to fully install the TKFE server with its full
     software stack.**

#### Automated deployment process
TKFE can also be deployed automatically using a YAML configuration file with all the required parameters. To deploy using this option, run ./deploy.sh --config <path-to-config-file>. All required parameters for deployment must be specified in the YAML file. If any required parameter is missing or invalid, the script will exit with an error. If all parameters are provided correctly, the deployment process will proceed automatically without any prompts for user input. Note that in case of any errors during deployment, the script will exit with an error and no resources will be created or modified.

If the DJANGO_SUPERUSER_PASSWORD environment variable is set, the script will use it to set the Django superuser password instead of the value in the YAML file. This is a more secure way of handling passwords.

A configuration-file deployment does not create the provisioning service
account (`<deployment_name>-tkfe-sa`), so no account is granted access to the
Terraform state bucket. A credential whose service account has project-level
Cloud Storage access, such as one created with `script/service_account.sh`,
can still use the bucket. Add any other provisioning credential, including one
from another project, to `state_bucket_writers` in
`tf/state_bucket/deployments/<deployment_name>/admin.auto.tfvars`, then run
`terraform -chdir=tf/state_bucket/deployments/<deployment_name> apply`. See
[Terraform State](#terraform-state).

To use this configuration file for automated deployment, follow these steps:

1. Create a file called `config.yaml` (do not place inline `#` comments on value lines — `deploy.sh` parses values directly):

  ```yaml
  deployment_name: MyDeployment
  project_id: my-project-id
  zone: us-west1-a
  # Optional fields: subnet_name, dns_hostname, ip_address, deployment_mode, repo_fork, repo_branch
  subnet_name: my-subnet
  dns_hostname: myhostname.com
  ip_address: 1.2.3.4
  django_superuser_username: sysadm
  # Optional if DJANGO_SUPERUSER_PASSWORD is set in the environment
  django_superuser_password: Passw0rd!
  django_superuser_email: sysadmin@example.com
  deployment_mode: git
  repo_fork: GoogleCloudPlatform
  repo_branch: develop
  ```

 1. Save the file in the same directory as the deploy.sh script.
 1. Open a terminal and navigate to the directory where the files are located.
 1. Run the following command: ./deploy.sh --config config.yaml
 1. The script will read the parameters from the config file and proceed with the deployment process. If DJANGO_SUPERUSER_PASSWORD environment variable is set, the script will use it to set the Django superuser password instead of the value in the YAML file.

<!--
TODO:  Give instruction for a custom deployment.

If a custom TKFE deployment is needed, the final step can be cancelled  Then
hack any files in ./tf and manually run `terraform apply` in the `frontend/tf` directory after properly setting the `terraform.tfvars`.
-->

**Important: To ensure that the TKFE resources can be fully cleaned up at a
later date, ensure that the directory containing the terraform configuration
(`./tf`) is retained.**

The deployment is now complete and after ~15 minutes, it will be possible to
log into the TKFE using the Admin account details given, at the specified
domain name or IP address via a web browser.  The IP address will be output as
`server_ip`.

---

## Post-deployment Configuration

### Credential Management

To allow TKFE to manage cloud resources on behalf of users, a
[service account](https://cloud.google.com/iam/docs/service-accounts)
must be registered by the Admin.  This is done by entering a credential
corresponding to the service account - the credential will be a json format
key file that needs to be copied into the *Credentials* form in TKFE.

The deploy script will usually create a service account and credential (unless
told not to), but other service accounts can be used -see advanced admin guide(AdvancedAdmin.md#service-account-management) for details.

<!--
The supplied credential will be validated and stored in the database for
future use.

Typically, the Owner or Editor of a GCP project have enough permissions. For
other users with custom roles, if certain permissions are missing, GCP will
typically return clear error messages.

For this project, the following roles should be sufficient for the admin users to manage the required service account: *Service Account User*, *Service Account Admin*, and *Service Account Key Admin*.
-->

## Network Management

All cloud systems begin with defining the network that components will
exist in. Before a cluster or stand-alone filesystem can be created, the
administrator must create the virtual private cloud (VPC) network. This is
accomplished under the *Networks* main menu item. Note that network resources
have their own life cycles and are managed independently.

### Create a new VPC
To create a new network, the admin must first select which credential should be
used for this network, then give the VPC a name, and then select the cloud
region for the network.

Upon clicking the *Save* button, the network is not immediately created. The
admin has to click *Edit Subnet* to create at least one subnet in the VPC.
Once the network and subnet(s) are defined, click the ‘Apply Cloud Changes’
button to trigger creation of the VPC and subnet(s).

<!-- Behind the scenes, gcluster is used to get Terraform to provision the cloud resources. -->

### Import an existing VPC

If your organisation already has predefined VPCs within the hosting GCP
project, they can be imported.  Simply select an existing VPC and associated
subnets from the web interface to register them with TKFE.  Imported VPCs
can be used in exactly the same way as newly created ones.

## Filesystem Management

By default each cluster creates two shared filesystems: one at `/opt/cluster` to
hold installed applications and one at `/home` to hold job files for individual
users. Both can be customised if required. Optionally, additional filesystems
may be created and mounted to the clusters. Note that filesystem resources have
their own life cycles and are managed independently, so they persist until
explicitly deleted and can be attached to several clusters in the same subnet.

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
connectivity (such as extablishing network peering) before mounting the
external filesystem located elsewhere on GCP.

## Cluster Management

HPC clusters can be created after setting up the hosting VPC and any
additional filesystems. The Cluster Toolkit FrontEnd can manage the whole life
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
   system.  In general, lower-case alphanumeric names with hyphens are
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
1. Finally, save the configurations and click the *Create* button to trigger
   the cluster creation.

### Deleting a cluster

To destroy a cluster, first find the list of clusters in the *Clusters* menu,
then simply select *Destroy* pop-down *Actions* menu and confirm.
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
`https://<domain_name>/accounts/google/login/callback/`

![Oauth set-up](images/GCP-app-credential.png)

From the GCP console, note the client ID and client secret. Then return to admin
site of the deployment, locate the *social applications* database table. A
'Google API' record should have been created during the deployment. Replace the
two placeholders with the client ID and client secret. The site is ready to
accept Google login.

![Social login set-up](images/register-social-app.png)

#### Set Allowed Users by Email Address

Next, go to the *Authorised user* table. This is where further access control
to the site is applied. Create new entries to grant access to users. A new
entry can be:

- a valid domain name to grant access to multiple users from authorised
organisations (e.g. *@example.com*)
- an email address to grant access to an individual user (e.g
*user.name@example.com*)

All login attempts that do not match these patterns will be rejected.

#### Note on external users

If you wish to allow users from outside your Google Cloud organization to use
the cluster you will need to additionally assign these users the
`roles/compute.osLoginExternalUser` role at an Organization level (there is no
way to assign at the project level).

User accounts will be automatically created for users when they log into the
FrontEnd for the first time, by default new accounts are created with quota
disabled.  To enable job submission for an account, administrators must enable
compute quota from the *Users* page.

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

## Terraform State

Every resource TKFE provisions (clusters, filesystems, networks, image builder
environments and workbenches) is created with Terraform, which records what it
created in a *state* file. Terraform needs that state to modify or destroy the
resource later. **A resource whose state has been
lost cannot be destroyed by Terraform**: it keeps running, keeps accruing cost,
and has to be removed by hand in the Cloud Console.

TKFE therefore stores this state in a dedicated Cloud Storage bucket,
`<deployment_name>-tfstate-<suffix>`, rather than on the service machine's
local disk. `deploy.sh` creates the bucket from the Terraform files in
`tf/state_bucket/`, before the FrontEnd itself, and prints its name. The
suffix is derived from the project and deployment name, so running
`deploy.sh` again from any machine finds and reuses the existing bucket rather
than creating a second one.

To make a valid bucket name, the deployment name is lowercased, any character
other than a-z, 0-9, '-' and '_' becomes '-', it is shortened to 46 characters
(bucket names are limited to 63), and leading and trailing '-' and '_' are
removed. A typical deployment name is used unchanged, and the suffix still
distinguishes deployments whose names clean or shorten to the same text.

On the client machine, each deployment has its own working directory for
these files, `tf/state_bucket/deployments/<deployment_name>/`, holding that
deployment's local state and variables, so deployments with different names
can be deployed from the same directory one after another. Files that an
earlier release kept directly in `tf/state_bucket/` are moved into the
directory of the deployment they belong to on the next deploy. If `deploy.sh`
cannot tell which deployment that is, it stops and says how to move them.

The bucket has three properties that matter to an administrator:

- **It is versioned.** Every previous copy of a state file is retained (by
  default for 90 days), so a state file corrupted by a crash part-way through
  an operation can be rolled back.
- **It survives teardown.** It is a separate Terraform deployment from the
  FrontEnd, so destroying the FrontEnd cannot destroy the state of the
  resources it created, and it is protected against accidental deletion.
- **Access is limited.** The bucket itself grants access only to the
  provisioning service account `deploy.sh` creates and to the accounts listed
  in `state_bucket_writers` in the deployment's `admin.auto.tfvars`. Accounts
  with a project-level Cloud Storage role can also reach it, including the
  `storage.admin` role `script/service_account.sh` gives the provisioning
  account. State contains full resource detail and can contain generated
  passwords, so keep project-level storage roles to accounts you trust.

State is filed under a namespace belonging to the TKFE database, then one
prefix per resource matching the run-time directory names on the service
machine: `gs://<bucket>/20261015-3f2a9c1e/clusters/cluster_3/` holds the
state for the cluster with ID 3. The namespace is created with the database,
so a new database (for example after rebuilding the service machine) gets a
new namespace, and its resources can never be confused with those of an
earlier database. On the service machine, `python manage.py tfstate_namespace`
prints the current namespace.

The state bucket is separate from the support bucket
(`<deployment_name>-storage-<suffix>`), which holds startup scripts, Ansible
configuration and job output.

### Registering a credential for another project

If you register a service account credential from a **different project** to
the one TKFE was deployed into (see
[Advanced Administration](AdvancedAdmin.md#multi-project-configuration)), that
service account also needs access to the state bucket, or provisioning with it
fails when Terraform tries to write state.

Add it to the `state_bucket_writers` list in
`tf/state_bucket/deployments/<deployment_name>/admin.auto.tfvars` on the client
machine and re-apply that directory:

```shell
terraform -chdir=tf/state_bucket/deployments/<deployment_name> apply
```

`deploy.sh` creates `admin.auto.tfvars` on the first deploy and never reads or
overwrites it afterwards, so entries there survive redeploys, and any valid
list layout is accepted. Do not add writers to `terraform.tfvars` in the same
directory: `deploy.sh` regenerates that file on every deploy.

When a credential is registered or updated in the portal, TKFE uses it to
check whether its service account can read and write state in the bucket. If
it cannot, the portal shows a warning naming the account, with the line to add
to `admin.auto.tfvars` and the command above. Registration still succeeds;
TKFE never grants the access itself.

### Recovering resources after losing the service machine

This is the situation the state bucket exists for. If the service machine is
lost or rebuilt, its local disk goes with it, including the TKFE database
(`db.sqlite3`), which is what the portal knows about your resources, and the
per-resource run-time directories. The state bucket is unaffected, so every
resource TKFE created can still be destroyed with Terraform.

Only the **state** is recovered, from the bucket. The **portal's records** are
not: a rebuilt TKFE will not list the old clusters, because they were in the
lost database. Recovery is therefore done from the command line, not through
the web interface.

Old resources are under the old database's namespace. List the namespaces with
`gcloud storage ls gs://<bucket>/`; the date at the start of each shows when
that database was created.

To destroy a resource whose service machine is gone, create an empty directory
on any machine with a `gcloud` login that has access to the state bucket, and
write a single `main.tf` containing only a backend and a provider:

```hcl
terraform {
  backend "gcs" {
    bucket = "<deployment_name>-tfstate-<suffix>"
    prefix = "<namespace>/clusters/cluster_3"
  }
}

provider "google" {
  project = "<your project>"
  region  = "<the resource's region>"
}
```

Use the prefix of the resource you want to remove;
`gcloud storage ls gs://<bucket>/<namespace>/` lists them, and they match the
run-time directory names (`clusters/cluster_<id>`, `fs/fs_<id>`,
`vpcs/vpc_<id>`, `images/image_<id>`, `workbenches/workbench_<id>`). Then:

```shell
terraform init
terraform plan
terraform destroy
```

The resources are in the recovered state but not in this configuration, so
Terraform plans to destroy all of them. Check that the plan lists what you
expect before approving it.

The state alone is enough, so this works even for clusters, filesystems and
image builder environments, whose Terraform configuration was generated by the
Cluster Toolkit and cannot be regenerated once the database is gone. For
networks and workbenches you can instead copy the template from
`infrastructure_files/` into a new directory and add the same backend block;
`terraform init` then adopts the existing state, allowing a normal plan and
destroy.

**Clusters: remove compute nodes before destroying the network.** Slurm creates
compute nodes directly through the Compute Engine API when jobs are scheduled,
so they are not in Terraform state and `terraform destroy` leaves them running.
After destroying a cluster, find them by the cluster's Slurm name, which is the
part before `-controller` in the controller instance's name in the plan above:

```shell
gcloud compute instances list --project=<your project> \
  --filter="name~^<slurm_cluster_name>-" --format="value(name,zone)"
gcloud compute instances delete <name> --zone=<zone> --project=<your project>
```

Do this before destroying the cluster's network: a network still used by
running instances cannot be deleted. In general, destroy clusters,
filesystems, workbenches and image builders before the networks they use.

### A resource reports a stale state lock

While Terraform works on a resource it holds a lock on that resource's state
(a `default.tflock` object beside `default.tfstate` in the state bucket). A
second operation on the same resource waits for the lock, for up to 45
minutes, instead of interfering with the first. Terraform releases the lock
when it finishes, including when it fails. A lock is left behind, or *stale*,
only if Terraform is killed outright while holding it: for example by a
service machine reboot, the out-of-memory killer, or `kill -9`.

Before each Terraform operation, TKFE checks the resource's lock. If the lock
was taken on the service machine and no Terraform process there is running
for that resource, the operation fails at once instead of waiting. The
resource goes into an error state, and the log (see
[Finding Log Files](#finding-log-files)) contains a message starting
`Terraform state for <directory> is locked.` It gives the lock ID, the
operation and host that took the lock, its age, and the command to clear it.
An operation that gives up after waiting 45 minutes for a lock reports the
same message.

Before clearing the lock, confirm on the service machine that no Terraform
process is working in that resource's directory (with `sudo`, so processes run
by any user are shown):

```shell
for pid in $(pgrep -x terraform); do echo "$pid $(sudo readlink /proc/$pid/cwd)"; done
```

If any line shows the directory from the message, an operation is still
running: wait for it to finish. If the lock's holder is a different host,
check that machine instead.

When nothing is running, clear the lock as the `gcluster` user with the
command from the message:

```shell
sudo -u gcluster bash -c 'cd <directory> && GOOGLE_APPLICATION_CREDENTIALS=<credential file> terraform force-unlock -force <lock ID>'
```

The lock ID is the number in the message (the lock object's generation), not
the `ID` field inside the lock object. Then repeat the operation in the portal.

**Never force-unlock while an operation is running.** The lock is the only
thing that stops two Terraform runs writing the same state at once, and
removing it during an operation can corrupt the state. If state is damaged,
earlier versions are kept in the versioned bucket.

### Upgrading an existing TKFE to Cloud Storage state

A TKFE deployed before state was held in Cloud Storage keeps each resource's
state on the service machine's disk, and TKFE has no in-place upgrade command.
Until the steps below are done, creating or reconfiguring a resource fails:
the new code refuses to provision without a state bucket.

These steps assume the new release's code is already in place under
`/opt/gcluster/cluster-toolkit` on the service machine. The support bucket
must also hold the new release's cluster and workbench files, or new clusters
still start with the old release's setup scripts. To copy them, run this from
the new release's `community/front-end/ofe` directory on the client machine:

```shell
gcloud storage cp --recursive infrastructure_files/gcs_bucket/* gs://<deployment_name>-storage-<suffix>/
```

1. On the client machine, create the state bucket from the deployment's own
   directory, linking the Terraform files as `deploy.sh` does:

   ```shell
   D=tf/state_bucket/deployments/<deployment_name>
   mkdir -p "$D"
   for f in tf/state_bucket/*.tf; do ln -s "../../${f##*/}" "$D/"; done
   ```

   Write `$D/terraform.tfvars` with `project_id`, `region` and
   `deployment_name` matching the deployment, and set `default_writers` to the
   provisioning service account if it exists:

   ```hcl
   default_writers = ["serviceAccount:<deployment_name>-tkfe-sa@<project>.iam.gserviceaccount.com"]
   ```

   Put any other provisioning accounts, including those for other projects,
   in `state_bucket_writers` in `$D/admin.auto.tfvars`. Then:

   ```shell
   terraform -chdir="$D" init
   terraform -chdir="$D" apply
   terraform -chdir="$D" output -raw bucket_name
   ```

2. On the service machine, add the bucket name under `config.server` in
   `/opt/gcluster/cluster-toolkit/community/front-end/ofe/configuration.yaml`:

   ```yaml
   terraform_state_bucket: "<bucket name>"
   ```

   On the client machine, add the same value to the FrontEnd's own
   `tf/terraform.tfvars`, which an older `deploy.sh` wrote without it:

   ```hcl
   terraform_state_bucket = "<bucket name>"
   ```

   Without it, `teardown.sh` cannot destroy the FrontEnd ("No value for
   required variable").

3. As the `gcluster` user, create the new database table and restart TKFE:

   ```shell
   cd /opt/gcluster/cluster-toolkit/community/front-end/ofe/website
   /opt/gcluster/django-env/bin/python3 manage.py makemigrations ghpcfe
   /opt/gcluster/django-env/bin/python3 manage.py migrate
   sudo supervisorctl restart 'gcluster-uvicorn-background:*'
   ```

4. Move existing state into the bucket, first as a preview:

   ```shell
   cd /opt/gcluster/cluster-toolkit/community/front-end/ofe
   ./script/migrate_tf_state_to_gcs.sh --dry-run
   ./script/migrate_tf_state_to_gcs.sh --yes
   ```

   The script reports each directory as migrated, skipped or failed and exits
   non-zero if any failed. It keeps the original of each migrated state file
   as `terraform.tfstate.premigration` in the resource's run-time directory,
   beside its `cloud_credentials` (for example `clusters/cluster_<id>/` or
   `vpcs/vpc_<id>/`), not in the Terraform working directory, which the
   Cluster Toolkit replaces whenever a cluster, filesystem or image builder is
   reconfigured.

5. Reconfigure or start resources only after the script reports no failures.
   Reconfiguring a cluster whose state has not been migrated fails at
   `terraform init` rather than moving its state, because the backend has
   changed.

The script only finds working directories that hold a local state file. A
network or workbench that was defined but never created, or a filesystem that
was defined but never started, has none, so it is not migrated. A
filesystem's blueprint would also still lack the state backend, and starting
it would keep its state on the service machine's disk. Delete such a resource
in the portal and create it again.

### Reverting to an older TKFE release

A TKFE release from before state was held in Cloud Storage expects state on
local disk. There is no automated way back. To revert, move each resource's
state back to its run-time directory **before** installing the older release;
an older release that finds a GCS backend block in a working directory will
not find the resource's state.

For each resource, in its Terraform working directory on the service machine
(for example `vpcs/vpc_<id>`, `workbenches/workbench_<id>/terraform/google`, or
`clusters/cluster_<id>/<cloud id>/primary`):

1. Remove the `backend "gcs"` block. If the directory has a `backend.tf`,
   delete that file: networks and workbenches have one, and so does any
   resource that `script/migrate_tf_state_to_gcs.sh` migrated from an older
   release. Otherwise, for clusters, filesystems and image builders created by
   this release, delete the `backend "gcs"` block from the generated `main.tf`.
2. Copy the state back to local disk, using the resource's own credential:

   ```shell
   GOOGLE_APPLICATION_CREDENTIALS=<run-time directory>/cloud_credentials \
     terraform init -migrate-state -force-copy
   ```

3. Confirm `terraform.tfstate` lists the resources and `terraform plan`
   reports no changes.

If you later move these resources to Cloud Storage again, first move each
resource's existing `terraform.tfstate.premigration` out of its run-time
directory, or rename it. The migration script refuses to overwrite a backup
that differs from the current state.

## Teardown Process

The TKFE package contains a `teardown.sh` script that will destroy the running
FrontEnd instance. This script only removes the FrontEnd, not resources started from it.

**Before running the teardown script, ensure that all clusters, VPCs,
workbenches and filestores are removed using the TKFE web interface before
destroying it. These resources will otherwise persist and accrue costs.**

To tear down the web interface and its hosting infrastructure, run
directory `./teardown.sh` on the original client machine
in the same directory that was used to deploy TKFE.

The Terraform state bucket is deliberately **not** removed by the teardown
script, so that any resource still running can still be destroyed with
Terraform. The script prints the bucket name when it finishes. Once you are
certain every resource TKFE created is gone, remove it with:

```shell
./teardown.sh --delete-state-bucket
```

This tears down the FrontEnd too if it is still deployed. Before destroying
anything, the script reads every state file in the bucket, under every
namespace, and stops if any still records resources, cannot be read or is
locked, listing them. It then asks you to type the bucket name to confirm,
unless `-y` is also given. The bucket's deletion protection is lifted only in a
temporary copy of `tf/state_bucket/`; the files in the repository are not
changed. If Terraform would change the bucket in any other way, for example
replace it after a change of region, the script stops and deletes nothing.

To remove the bucket of an earlier deployment made from the same directory,
name it: `./teardown.sh --delete-state-bucket <deployment_name>`. This touches
only that bucket. If that deployment's FrontEnd is, or may still be, deployed
from this directory (its lock file exists or its state cannot be read), the
script refuses; run it without a name to tear the FrontEnd down first.

Note: If the -y flag is passed to the script, all user questions will be answered with "yes". This is intended for automated deployments where user input is not possible.

## Troubleshooting

### Finding Log Files

The service machine produces log files in `/opt/gcluster/run/`. These log
files will show errors from the Django web application.

Cloud resource deployment log files (from Terraform) are typically shown via
the FrontEnd web site.  If those logs are not being shown, they can be found on
the service machine under
`/opt/gcluster/cluster-toolkit/frontend/(clusters|fs|vpc)/...`.
Cluster Toolkit log files will also be found in those directories.  The Terraform
log files and status files will be down a few directories, based off of the
Cluster Number, Deployment ID, and Terraform directory.

On Cluster controllers, most of the useful log files for debugging can be
retrieved by executing the 'Sync Cluster' command.  These include Slurm log
files as well as general system log files.  The daemon which communicates to
the service machine logs to syslog, and can be viewed on the cluster controller
node via `journalctl`, looking at the `ghpcfe_c2` service.

Job logs and Spack application logs are uploaded upon job completion to Google
Cloud Storage and viewable via the HPC Frontend.

### Deployment problems

Most deployment problems are caused by not having the right permissions. If
this is the case, error message will normally show what permissions are
missing. Use the [IAM permissions reference](https://cloud.google.com/iam/docs/permissions-reference)
to research this and identify additional roles to add to your user account.

Before any attempt to redeploy the TKFE, make sure to run
`terraform destroy` in `cluster-toolkit/frontend/tf` to remove cloud resources that
have been already created.

### Cluster problems

The FrontEnd should be quite reliable provisioning clusters. However, in cloud
computing, errors will and do happen from time to time; usually due to changes
in back-end services or other factors beyond scope of the TKFE. For example, a
resource creation could fail because the hosting GCP project has ran out of
certain resource quotas; or an upgrade of an underlying machine image might
have introduced changes that are incompatible to the TKFE, which then needs
updating.
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

- If a cluster is stuck in 'i' state, it is normally OK to find the *Destroy*
  button from its *Actions* menu to destroy it.
- For failed network/filesystem/cluster creations, one may need to SSH into the
  service machine, locate the run-time data directory, and manually run
  `terraform destroy` there for clean up cloud resources.
- Certain database records might get corrupted and need to be removed for
  failed clusters or network/filesystem components. This can be done from the
  Django Admin site, although administrators need to exercise caution while
  modifying the raw data in Django database.
