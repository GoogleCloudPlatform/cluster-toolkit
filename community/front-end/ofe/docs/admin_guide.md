# Open Front End (OFE) - Administrator’s Guide

<!--
0        1         2         3         4         5         6         7        8
1234567890123456789012345678901234567890123456789012345678901234567890234567890
-->

This document is for administrators of the Open Front End (OFE). An
administrator can deploy the OFE portal, manage the lifecycle of HPC clusters,
set up networking and storage resources that support clusters, install
applications, and manage user access. Normal HPC users should refer to the
[User Guide](user_guide.md) for guidance on how to prepare and run jobs on
clusters that have been set up by administrators.

Basic administrator knowledge of the Google Cloud Platform is needed in order to
create projects and user accounts, but all other low-level administration tasks
are handled by the portal.

OFE is a web application built upon the Django framework.  By default, a
single Django superuser is created at deployment time - this superuser then has
administrator privileges in OFE.  For large organisations that need additional
administrators, additional Django superusers can be created from the Admin site
within OFE, once it is deployed and running.

The OFE web application server uses the
[Cluster Toolkit](https://github.com/GoogleCloudPlatform/cluster-toolkit) to
provision resources for networks, filesystems and clusters, using a service
account that has its credentials registered to OFE. The service account is
used for access management and billing.

This guide covers the deployment of OFE, registering the service account, and
the steps required to then create and manage clusters.  Further guides cover
installation of [Applications](Applications.md) and the
[Workbench](WorkbenchAdmin.md) Jupyter Notebook feature.
An [Advanced Administrators Guide](AdvancedAdmin.md) also covers features for
customising the OFE deployment.

## OFE Deployment

OFE is deployed from a client machine using a script. The script will guide
the admin through the setup process, prompting for input of required
parameters, then deploy OFE into the Google Cloud Platform.

OFE can also be deployed non-interactively from a YAML configuration file with
`--config`. This is convenient for scripting, but it skips several checks that
the interactive mode performs - see
[Automated deployment process](#automated-deployment-process) before using it.

### Prerequisites

#### Client Machine

The client machine must run Linux (or equivalent such as
[WSL](https://docs.microsoft.com/en-us/windows/wsl/install)) and there are a
small number of prerequisites that must be installed ahead of time:

- bash shell
- [Git](https://github.com/git-guides/install-git)
- [Terraform CLI](https://www.terraform.io/downloads) installation, version
  1.12.2 or later (`required_version` in `tf/versions.tf`)
- [Google Cloud CLI](https://cloud.google.com/cli) installation (`gcloud`,
  including the `gcloud storage` command)

> **Note:** `deploy.sh` and `teardown.sh` only check that `terraform` is on
> your `PATH`, and print "version 0.13 or higher" in their error message. That
> text is stale; Terraform itself will refuse to run with a version older than
> 1.12.2.

#### Versions used by the OFE server

These are fixed by the repository, not by the client machine. Check the files
named below if you need to confirm them for a given branch:

| Component | Version | Where it is set |
| --------- | ------- | --------------- |
| Terraform | `>= 1.12.2` | `tf/versions.tf` |
| Terraform `google` / `google-beta` providers | `~> 4.0` | `tf/versions.tf` |
| Terraform `null` provider | `~> 3.0` | `tf/versions.tf` |
| Server VM image | `projects/rocky-linux-cloud/global/images/rocky-linux-8-v20220126` (Rocky Linux 8, pinned) | `tf/main.tf` |
| Python on the server | 3.12 (virtual environment at `/opt/gcluster/django-env`) | `infrastructure_files/gcs_bucket/webserver/startup.sh` |
| Django | 6.1 | `requirements.in` / `requirements.txt` |
| Spack on clusters | v0.21.0 | `infrastructure_files/gcs_bucket/clusters/ansible_setup/roles/spack_install/tasks/main.yaml` |
| Slurm image on clusters | `slurm-gcp-6-12-hpc-rocky-linux-9` family (project `schedmd-slurm-public`) | `website/ghpcfe/templates/blueprint/` |

#### Download OFE

If not already downloaded, the OFE repository needs to be cloned to the client
machine.

Clone the repository, checkout the corresponding branch, and switch to
the `community/front-end/ofe` directory as follows:

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

A GCP project is required with the following APIs enabled. This list is the
`PRJ_API` array in `deploy.sh`:

| API | Service name |
| --- | ------------ |
| Cloud Monitoring API | `monitoring.googleapis.com` |
| Cloud Logging API | `logging.googleapis.com` |
| Compute Engine API | `compute.googleapis.com` |
| Cloud Pub/Sub API | `pubsub.googleapis.com` |
| Cloud Filestore API | `file.googleapis.com` |
| Cloud Resource Manager API | `cloudresourcemanager.googleapis.com` |
| Identity and Access Management (IAM) API | `iam.googleapis.com` |
| Cloud OS Login API | `oslogin.googleapis.com` |
| Cloud Billing API | `cloudbilling.googleapis.com` |
| Vertex AI API | `aiplatform.googleapis.com` |
| BigQuery Connection API | `bigqueryconnection.googleapis.com` |
| Cloud SQL Admin API | `sqladmin.googleapis.com` |
| Service Networking API | `servicenetworking.googleapis.com` |
| Secret Manager API | `secretmanager.googleapis.com` |
| Service Usage API | `serviceusage.googleapis.com` |
| Cloud Storage API | `storage.googleapis.com` |

In interactive mode, if any of these are not enabled, the deployment script
asks to enable each one for you. It's not possible to complete a deployment
without these APIs, so the script aborts if you decline. In `--config` mode
this check is skipped (see [Automated deployment process](#automated-deployment-process)),
so enable them beforehand:

```bash
gcloud services enable --project=<PROJECT_ID> \
  monitoring.googleapis.com logging.googleapis.com compute.googleapis.com \
  pubsub.googleapis.com file.googleapis.com cloudresourcemanager.googleapis.com \
  iam.googleapis.com oslogin.googleapis.com cloudbilling.googleapis.com \
  aiplatform.googleapis.com bigqueryconnection.googleapis.com \
  sqladmin.googleapis.com servicenetworking.googleapis.com \
  secretmanager.googleapis.com serviceusage.googleapis.com \
  storage.googleapis.com
```

##### GCP user

A GCP user that is a member of the project will be able to deploy OFE, but
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

OFE uses a deployment script run on the client machine, prompting for required
parameters, to configure OFE. Run it from the `community/front-end/ofe`
directory:

```bash
./deploy.sh
```

The script asks for the following, in this order:

1. Before asking anything, the script aborts if `tf/.tkfe.lock` exists (an OFE
   was already deployed from this directory) and checks that `terraform` and
   `gcloud` (with `gcloud storage`) are on your `PATH`.
1. **Deployment name.** Lower-case letters, digits, `-` and `.`; at least 3
   characters, starting with a letter and ending with a letter or digit. The
   server VM is named `<deployment name>-server`.
1. **GCP project ID** (defaults to your `gcloud` project). The ID is checked
   with `gcloud projects describe`.
1. **API check.** For each required API that is not enabled, you are asked
   whether to enable it. Declining exits the script. The check lists the APIs
   of your default `gcloud` project, so run
   `gcloud config set project <PROJECT_ID>` before `./deploy.sh`.
1. **Account check.** The script exits if `gcloud` has no authorised account. If
   the account is not an *Owner* or *Editor* of the project, it prints a
   warning and asks `Proceed [y/N]`. Answering `N` exits.
1. **GCP zone** (default `europe-west4-a`). The region is derived from the zone
   by dropping the last `-<letter>`.
1. **Subnet** (optional). Leave blank and a VPC and subnet are created.
1. **DNS hostname** (optional) and **static IP address** (optional).
   - For a production deployment, provide a domain name and static IP
     address.  If a static IP is needed, follow the on-screen instructions. An
     SSL certificate will automatically be obtained via
     [LetsEncrypt](https://letsencrypt.org/) to secure the web application.
   - For dev and testing purposes, both can be left blank. The system can still
     be successfully deployed and run with an ephemeral IP address, however
     OAuth2-based login (see later) will not be available as this requires a
     publicly resolvable domain name. Without a hostname the server uses
     plain HTTP.
1. **Admin (Django superuser)** username, password (entered twice) and email
   address.
1. **Service account and credential.** `Create a service account and
   credential? [y/N]` - the default is **No**. If you answer yes, the script
   creates the account `<deployment name>-tkfe-sa` in the project, grants it
   the roles listed in `script/service_account.sh`, and writes a key to
   `credential.json` next to `deploy.sh`. That file must be registered in OFE
   (see below). If the account already exists you are asked whether to delete
   and recreate it, or reuse it (optionally regenerating the credential). You
   can skip this step if you use an existing service account, or multiple
   projects (see [advanced admin guide](AdvancedAdmin.md) for details).
1. **Deployment method of the server software.**
   - `1` (default): clone the git repository when the server starts. You are
     then asked for the fork owner (default `GoogleCloudPlatform`) and branch
     (default `main`). The repository must be named `cluster-toolkit`.
   - `2`: use a copy of the code from this computer (a tarball of the git
     checkout containing `deploy.sh`).
1. **Summary and confirmation.** The script prints a summary, writes
   `tf/terraform.tfvars`, and asks `Proceed to deploy? [y/N]`. If confirmed,
   Terraform creates the VM instance and its supporting resources. This can take
   a few minutes.
   - **Note: after the deploy script has completed, it will still take up to
     another 15 minutes to fully install the OFE server with its full
     software stack.**

> **Warning:** `tf/terraform.tfvars` contains the admin password in plain text
> (`django_su_password`). The password is also stored in the Terraform state
> in `tf/` and in the `webserver/config` object in the deployment's GCS bucket.
> Restrict access to the `tf/` directory, and never commit it.

#### Automated deployment process

OFE can also be deployed from a YAML configuration file:

```bash
./deploy.sh --config <path-to-config-file>
```

In this mode the script does not prompt for anything, including the final
`Proceed to deploy?` confirmation. It reads a small set of keys, fails if a
required one is missing, writes `tf/terraform.tfvars` and runs
`terraform apply -auto-approve`.

**Config mode is not equivalent to interactive mode.** It does only the
following checks: required keys are present, and a password is available. It
does *not*:

- check for `tf/.tkfe.lock` (running it twice from the same directory
  overwrites `tf/terraform.tfvars`, so do not do this while an OFE is deployed);
- check that `terraform` and `gcloud` are installed, that the project exists,
  that the required APIs are enabled, or that your account has suitable roles;
- validate the deployment name (use lower-case letters, digits and `-`, as in
  interactive mode);
- create a service account or write `credential.json`.

The script does not "validate everything and then deploy": if Terraform fails
part-way, some resources may already exist. See
[Deployment problems](#deployment-problems) for how to recover.

##### Config file format

The file is read as flat `key: value` lines, not full YAML. What this means in
practice:

- Blank lines and comment lines are skipped, as are lines that are not
  `key: value` with a key of letters, digits and `_` (not starting with a
  digit). Indentation before a key is ignored.
- A value wrapped in matching single or double quotes is unquoted. Otherwise a
  `#` that starts the value or follows whitespace starts a comment, so
  `key: value # note` yields `value`. Quote a value to keep a `#` that follows a space.
- Backslash escapes inside double quotes are copied to `terraform.tfvars` as
  written, and Terraform decodes them, so `"a\"b"` gives `a"b`. Use escapes
  Terraform understands, such as `\"` and `\\`. A single-quoted value with
  `''`, or a value with text after the closing quote, is kept as written,
  quotes included. `"` or `\` in an unquoted value, and `${` in any value,
  can break the generated `terraform.tfvars`.
- Leading and trailing spaces around an unquoted value are dropped; spaces
  inside quotes are kept.
- Lists, nesting and multi-line values are not supported. An indented key is
  read as a top-level key, and a later duplicate key wins.

```yaml
# Required
deployment_name: mydeployment
project_id: my-project-id
zone: us-west1-a
django_superuser_username: sysadm
django_superuser_email: sysadmin@example.com
# Not needed if the DJANGO_SUPERUSER_PASSWORD environment variable is set
django_superuser_password: CHANGE_ME
# Optional. Default is the zone without its last -<letter>
region: us-west1
# Optional
subnet_name: my-subnet
dns_hostname: myhostname.com
ip_address: 1.2.3.4
# Optional: git or tarball. Default in config mode is tarball
deployment_mode: git
# Optional, used only with git. Defaults are GoogleCloudPlatform and main
repo_fork: GoogleCloudPlatform
repo_branch: develop
```

If the `DJANGO_SUPERUSER_PASSWORD` environment variable is set (even to an
empty string), its value is used instead of the YAML file. This keeps the
password out of the file.

Note that in config mode `deployment_mode` defaults to `tarball`, whereas the
interactive default is `git`. With `tarball`, run `deploy.sh` from inside a
git checkout of the repository.

##### Setting the region

The region is derived from the zone (the zone without its last `-<letter>`),
as in interactive mode. To use a different region, add `region: <REGION>` to
the config file. The region is used for the Google provider, the storage
bucket location and the VPC created when no subnet is given.

##### Creating the service account manually

Config mode does not create a service account or `credential.json`. Create them
with the helper script, using the same naming as interactive mode, then
register `credential.json` in OFE (see [Credential Management](#credential-management)):

```bash
script/service_account.sh create <PROJECT_ID> <DEPLOYMENT_NAME>-tkfe-sa
script/service_account.sh credential <PROJECT_ID> <DEPLOYMENT_NAME>-tkfe-sa credential.json
```

See the [Advanced Admin Guide](AdvancedAdmin.md#service-account-management)
for other options.

To deploy with a config file:

1. Create the file, for example `config.yaml`, as above.
1. Make sure the project's APIs are enabled and you are authenticated with
   `gcloud auth application-default login --project=<PROJECT_ID>`.
1. From the `community/front-end/ofe` directory run
   `./deploy.sh --config config.yaml`.

<!--
TODO:  Give instruction for a custom deployment.

If a custom OFE deployment is needed, the final step can be cancelled  Then
hack any files in ./tf and manually run `terraform apply` in the `front-end/ofe/tf` directory after properly setting the `terraform.tfvars`.
-->

**Important: To ensure that the OFE resources can be fully cleaned up at a
later date, ensure that the directory containing the terraform configuration
and state (`./tf`) is retained.** After a successful deployment `tf/.tkfe.lock`
is created, which stops a second deployment from the same directory.

The deployment is now complete and after ~15 minutes, it will be possible to
log into the OFE using the Admin account details given, at the specified
domain name or IP address via a web browser.  The IP address will be output as
`server_ip`.

---

## Post-deployment Configuration

### Credential Management

To allow OFE to manage cloud resources on behalf of users, a
[service account](https://cloud.google.com/iam/docs/service-accounts)
must be registered by the Admin.  This is done by entering a credential
corresponding to the service account - the credential will be a json format
key file that needs to be copied into the *Credentials* form in OFE.

The deploy script will usually create a service account and credential (unless
told not to), but other service accounts can be used - see the
[advanced admin guide](AdvancedAdmin.md#service-account-management) for
details. If you deployed with `--config`, no service account was created - see
[Creating the service account manually](#creating-the-service-account-manually).

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
subnets from the web interface to register them with OFE.  Imported VPCs
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
connectivity (such as establishing network peering) before mounting the
external filesystem located elsewhere on GCP.

## Cluster Management

HPC clusters can be created after setting up the hosting VPC and any
additional filesystems. The Open Front End (OFE) can manage the whole life
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
1. Click the *Edit* button to make additional changes, such as creating more
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
     an image is compatible with the Rocky Linux 9 based image family
     (`slurm-gcp-6-12-hpc-rocky-linux-9`, project `schedmd-slurm-public`) that
     the cluster templates use by default. Otherwise additional customisation done by this system might fail.
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

While it is possible to use a Django user account to access the OFE
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
OFE for the first time, by default new accounts are created with quota
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

## Teardown Process

The OFE package contains a `teardown.sh` script that will destroy the running
OFE instance. This script only removes the OFE, not resources started from it.

**Before running the teardown script, ensure that all clusters, VPCs,
workbenches and filestores are removed using the OFE web interface before
destroying it. These resources will otherwise persist and accrue costs.**

To tear down the web interface and its hosting infrastructure, run
`./teardown.sh` on the original client machine, in the same directory that was
used to deploy OFE (it reads `tf/terraform.tfvars` and the Terraform state in
`tf/`). On success it deletes `tf/.tkfe.lock`.

If `tf/terraform.tfvars` does not exist, the script exits with status 1 before
asking anything.

Note: if the `-y` flag is passed, the script answers "yes" to its own
"No lock file found - proceed anyway?" and "Proceed?" questions. With `-y`, the
script also deletes the `<deployment name>-tkfe-sa` service account without
asking, so `./teardown.sh -y` runs without prompts. `-y` must be the first
argument. The service account is handled before the `Proceed?` question, so it
is deleted even if the teardown then stops. Without `-y` it asks
`Delete [y/N]`; answer `n` to keep that service account and its credential.
A leftover `<deployment name>-fe-sa` account (created by Terraform) is removed
without a prompt.

## Troubleshooting

### Finding Log Files

The service machine produces log files in `/opt/gcluster/run/`. These log
files will show errors from the Django web application.

Cloud resource deployment log files (from Terraform) are typically shown via
the OFE web site.  If those logs are not being shown, they can be found on
the service machine under
`/opt/gcluster/cluster-toolkit/community/front-end/ofe/(clusters|fs|vpcs|workbenches)/...`.
Cluster Toolkit log files will also be found in those directories.  The Terraform
log files and status files will be down a few directories, based off of the
Cluster Number, Deployment ID, and Terraform directory.

On Cluster controllers, most of the useful log files for debugging can be
retrieved by executing the 'Sync Cluster' command.  These include Slurm log
files as well as general system log files.  The daemon which communicates to
the service machine logs to syslog, and can be viewed on the cluster controller
node via `journalctl`, looking at the `ghpcfe_c2` service.

Job logs and Spack application logs are uploaded upon job completion to Google
Cloud Storage and viewable via the OFE.

### Deployment problems

Most deployment problems are caused by not having the right permissions. If
this is the case, error message will normally show what permissions are
missing. Use the [IAM permissions reference](https://cloud.google.com/iam/docs/permissions-reference)
to research this and identify additional roles to add to your user account.

Before any attempt to redeploy OFE, remove the cloud resources that have
already been created. On the client machine, in the directory you deployed
from:

```bash
cd community/front-end/ofe/tf
terraform destroy
```

or run `./teardown.sh` from `community/front-end/ofe` (see
[Teardown Process](#teardown-process)).

If `terraform apply` failed part-way, `deploy.sh` exits with "Terraform failed"
and does **not** create `tf/.tkfe.lock`. The Terraform state and
`tf/terraform.tfvars` are kept in `tf/`, so either:

- fix the cause (for example a missing role or API, or a quota) and run
  `terraform apply` again in `tf/`; or
- run `terraform destroy` in `tf/` (or `./teardown.sh`) to start again.

`deploy.sh` also writes the Terraform output to `tf/tfapply.log`. If
`tf/.tkfe.lock` exists and you are sure nothing is deployed, delete it to allow
a new deployment from that directory. Remember that `tf/terraform.tfvars` holds
the admin password in plain text.

### Cluster problems

The OFE should be quite reliable provisioning clusters. However, in cloud
computing, errors will and do happen from time to time; usually due to changes
in back-end services or other factors beyond scope of the OFE. For example, a
resource creation could fail because the hosting GCP project has ran out of
certain resource quotas; or an upgrade of an underlying machine image might
have introduced changes that are incompatible to the OFE, which then needs
updating.
It is not possible to capture all such situations.  Here, a list of tips is
given to help debug cluster creation problems. The
[Developer's Guide](developer_guide.md) contains a lot of details on how the
back-end logic is handled, which can also help with certain issues.

- If a cluster is stuck at status 'c', something is wrong with the provisioning
  of cluster hardware.
  - SSH into the service machine and identify the directory containing the
    run-time data for that cluster at
    `/opt/gcluster/cluster-toolkit/community/front-end/ofe/clusters/cluster_<cluster_id>`
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
