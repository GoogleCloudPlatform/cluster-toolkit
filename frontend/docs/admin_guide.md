# HPC Toolkit FrontEnd - Administrator’s Guide

This document is for administrators of the HPC Toolkit FrontEnd. An administrator can manage the life cycles of HPC clusters, set up networking and storage resources that support clusters, install applications and manage user access. Ordinary HPC users should refer to the [User Guide](user_guide.md) for guidance on how to prepare and run jobs on existing clusters.

The HPC Toolkit FrontEnd is a web application built upon the Django framework. By default, a Django superuser is created at deployment time. For large organisations, additional Django superusers can be created from the Admin site. 

## System Deployment

### Prerequisites

#### Client Machine:

- Linux (WSL) environment with bash interpreter
- Terraform CLI installation
- Google Cloud SDK installation (`gcloud` utility)
- Authenticated google cloud user for deployment (`gcloud auth` - see below for required permissions)

#### Google Cloud:

- GCP project(s):
  - for the HPC Toolkit FrontEnd with at least the following APIs enabled:
     - Compute Engine API
     - Cloud Monitoring API
     - Cloud Logging API
     - Cloud Pub/Sub API
     - Cloud Resource Manager
     - Identity and Access Management (IAM) API


  - A GCP project for deploying Clusters with at least the following APIs enabled:

     - Compute Engine API
     - Cloud Monitoring API
     - Cloud Resource Manager
     - Cloud Logging API
     - Cloud OS Login API
     - Cloud Filestore API
     - Cloud Billing API
     - Vertex AI API

  - These two projects may actually be the same project.  The HPC Toolkit Frontend supports deploying clusters into the same project in which the service machine runs, as well as deploying clusters into other projects.
 
-  Cloud user/service account with suitable roles granting the required permissions for deployment of the HPC Toolkit Frontend:

    - The `Owner` role grants complete project control and is more than sufficient

    - Alternatively, a collection of more limited roles can be used:
   
    ```
    - Compute Admin
    - Storage Admin
    - Pub/Sub Admin
    - Create Service Accounts
    - Delete Service Accounts
    - Service Account User
    - Project IAM Admin
    ```

  Permissions can be further fine-tuned to achieve least privilege e.g using the security insights tool.
  
### Deployment Process
  - `git clone` the HPC Toolkit project from GitHub into a client machine [TODO: give official repository name after turning this into a public project].
  - Run `hpc-toolkit/frontend/deploy.sh` to deploy the system. Follow instructions to name the hosting VM instance and select its cloud region and zone. The hosting VM will be referred to as the *service machine* from now on.
    - For a production deployment, follow on-screen instructions to create a static IP address and then provide a domain name. In this case, an SSL certificate will be automatically obtained via [LetsEncrypt](https://letsencrypt.org/) to secure the web application. 
    - For testing purposes the static public IP address and domain name can be left blank. The system can still be successfully deployed and run with an ephemeral IP address, however OAuth-based login will not be available as this requires a publicly-resolvable domain name.
    - Follow instructions to provide details for a Django superuser account.
    - On prompt, check the generated Terraform settings and make further changes if necessary. This step is optional.
    - Confirm to create the VM instance. The VM creation takes a few minutes. **N.B after the script has completed, it can take up to 15 more minutes to have the software environment set up.**
  - Alternatively, manually run `terraform apply` in the `frontend/tf` directory after properly setting the `terraform.tfvars`.
  - Use the domain name or IP address to access the website. Log in as the Django superuser.
  - **Important: To ensure that the web interface resources can be cleaned up fully at a later date, ensure that the directory containing the terraform configuration (`hpc-toolkit/frontend/tf`) is retained.**

The deployment is now complete.

## Post-deployment Configuration

### SSH access to the service machine

SSH access to the service machine is possible for administration purpose. Administrators can choose from one of the following options:

- [SSH directly from the GCP console](https://cloud.google.com/compute/docs/instances/connecting-to-instance).
- [Add his/her public SSH key to the VM instance after deployment via GCP console](https://cloud.google.com/compute/docs/connect/add-ssh-keys#add_ssh_keys_to_instance_metadata).
- [Add his/her SSH key to the GCP project to use on all VMs within the project](https://cloud.google.com/compute/docs/connect/add-ssh-keys#add_ssh_keys_to_project_metadata).

*N.B The service machine is not, by default, configured to use the os-login service.*

### Set up Google OAuth2 login

While it is possible to use a Django user account to access the FrontEnd website, and indeed doing so is required for some administration tasks, ordinary users must authenticate using their Google identities via Google OAuth2.  This, combined with the use of Google OSLogin for access to clusters, ensures consistent Linux identities across VM instances that form the clusters. Web frontend login is made possible by the *django-allauth* social login extension. 

For a working  deployment, a fully-qualified domain name must be obtained and attached to the website as configured in the deployment script.  Next, register the site with the hosting GCP project on the GCP console in the *Credentials* section under *APIs and services* category. Note that the *Authorised JavaScript origins* field should contain a callback URL in the following format: *https://<domain_name>/accounts/google/login/callback/*

![Oauth set-up](images/GCP-app-credential.png)

From the GCP console, note the client ID and client secret. Then return to admin site of the deployment, locate the *social applications* database table. A 'Google API' record should have been created during the deployment. Replace the two placeholders with the client ID and client secret. The site is ready to accept Google login.

![Social login set-up](images/register-social-app.png)]

Next, go to the *Authorised user* table. This is where further access control to the site is applied. Create new entries to grant access to users. A new entry can be:

- a valid domain name to grant access to multiple users from authorised organisations (e.g. @example.com) 
- an email address to grant access to an individual user (e.g user.name@example.com) 

All login attempts that do not match these patterns will be rejected.

### Credential Management

To use the web system to create cloud resources, the first task is for an admin user to register a cloud credential with the system. The supplied credential will be validated and stored in the database for future use.

The preferred way to access GCP resources from this system is through a [Service Account](https://cloud.google.com/iam/docs/service-accounts). To create a service account, a GCP account with sufficient permissions is required. Typically, the Owner or Editor of a GCP project have enough permissions. For other users with custom roles, if certain permissions are missing, GCP will typically return clear error messages. 

For this project, the following roles should be sufficient for the admin users to manage the required service account: *Service Account User*, *Service Account Admin*, and *Service Account Key Admin*.

#### Creating a service account via the GCP Console

- Log in to the GCP console and select the GCP project that hosts this work.
- From the main menu, select *IAM & Admin*, then *Service Accounts*.
- Click the *CREATE SERVICE ACCOUNT* button.
- Name the service account, optionally provide a description, and then click the *CREATE* button.
- Grant the service account the following roles:
  
  ```
  - Cloud Filestore Editor
  - Compute Admin
  - Create Service Accounts
  - Delete Service Accounts
  - Project IAM Admin
  - Notebooks Admin
  - Vertex AI administrator 
  ```

- Human users may be given permissions to access this service account but that is not required in this work. Click *Done* button.
- Locate the new service account from the list, click *Manage Keys* from the *Actions* menu.
- Click *ADD KEY*, then *Create new key*. Select JSON as key type, and click the *CREATE* button.
- Copy the generated JSON content which should then be pasted into the credential creation form on the website.

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

Once complete, the service account key json can be copied from `path_to_key_file` into the credentials form on the frontend.

## Network Management

All cloud systems begin with defining the network within which the systems will be deployed. Before a cluster or stand-alone filesystem can be created, the administrator must create the virtual cloud network (VPC). This is accomplished under the *Networks* main menu item. Note that network resources have their own life cycles and are managed independently to cluster.

### Create a new VPC
To create a new network, the admin must first select which cloud credential should be used for this network, then give the VPC a name, and then select the cloud region for the network.

Upon clicking the *Save* button, the network is not immediately created. The admin has to click *Edit Subnet* to create at least one subnet. Once the network and subnets are appropriately defined, click the ‘Apply Cloud Changes’ button to trigger Terraform to provision the  cloud resources.

### Import an existing VPC

If the organisation already has pre-defined VPCs on cloud within the hosting GCP project, they can be imported. Simply selecting an existing VPC and associated subnets from the web interface to register them with the system. Imported VPCs can be used in exactly the same way as newly created ones.


## Filesystem Management

By default each cluster creates two shared filesystems: one at */opt/cluster* to hold installed applications and one at */home* to hold job files for individual users. Both can be customised if required. Additional filesystems may be created and mounted to the clusters. Note that filesystem resources have their own life cycles and are managed independently to cluster.

### Create new filesystems

Currently, only GCP Filestore is supported. GCP Filestore can be created from the *Filesystems* main menu item. A new Filestore has to be associated with an existing VPC and placed in a cloud zone. All performance tiers are supported.

### Import existing filesystems

Existing filesystems can be registered to this system and subsequently mounted by clusters. These can be existing NFS servers (like Filestore), or other filesystems for which Linux has built-in mount support. For this to work, for each NFS server, provide an IP address and an export name. The IP address must be reachable by the VPC subnets intended to be used for clusters.

An internal address can be used if the cluster shares the same VPC with the imported filesystem. Alternatively, system administrators can set up hybrid connectivity (such as extablishing network peering) beforing mounting the external filesystem located elsewhere on GCP. 

## Cluster Management

HPC clusters can be created after setting up the hosting VPC and, optionally, additional filesystems. The HPC Toolkit FrontEnd can manage the whole life cycles of clusters. Click the *Clusters* item in the main menu to list all existing clusters.

### Cluster status
Clusters can be in different states and their *Actions* menus adapt to this information to show different actions:

- Status 'n' – Cluster is being newly configured by user. At this stage, a new cluster is being set up by an administrator. Only a database record exists, and no cloud resource has been created yet. User is free to edit this cluster: rename it, re-configure its associated network and storage components, and add authorized users. Click *Start* from the cluster detail page to actually provision the cluster on GCP.
- Status 'c' – Cluster is being created. This is a state when the backend Terraform scripts is being invoked to commission the cloud resources for the Cluster. This transient stage typically lasts for a few minutes.
- Status 'i' – Cluster is being initialised. This is a state when the cluster hardware is already online, and Ansible playbooks are being executed to install and configure the software environment of the Slurm controller and login nodes. This transient stage can last for up to 15 minutes.
- Status 'r' – Cluster is ready for jobs. The cluster is now ready to use. Applications can be installed and jobs can run on it. A Slurm job scheduler is running on the controller node to orchestrate job activities.
- Status 't' – Cluster is terminating. This is a transient state after Terraform is being invoked to destroy the cluster. This stage can take a few minutes when Terraform is working with the cloud platform to decommission cloud resources.
- Status 'd’ – Cluster has been destroyed. When destroyed, a cluster cannot be brought back online. Only the relevant database record remains for information archival purposes.

A visual indication is shown on the website for the cluster being in creating, initialising or destroying states. Also, relevant web pages will refresh every 15 seconds to pick status changes.

### Create a new cluster

A typical workflow for creating a new cluster is as follows:

- At the bottom of the cluster list page, click the *Add cluster* button to start creating a new cluster. In the next form, choose a cloud credential. This is the Google Service Account which will create the cloud resources. Click the *Next* button to go to a second form from which details of the cluster can be specified.
- In the *Create a new cluster* form, give the new cluster a name. Cloud resource names are subject to naming constraints and will be validated by the system.  In general, lower-case alpha-numeric names with hyphens are accepted.
- From the *Subnet* dropdown list, select the subnet within which the cluster resides.
- From the *Cloud zone* dropdown list, select a zone.
- From the *Authorised users* list, select users that are allowed to use this cluster. 
- Click the *Save* button to store the cluster settings in the database. Continue from the *Cluster Detail* page.
- Click the *Edit* button to make additional changes. such as creating more Slurm partitions for differnt compute node instance types, or 
mounting additional filesystems.
  - For filesystems, note the two existing shared filesystems defined by default. Additional ones can be mounted if they have been created earlier. Note the *Mounting order* parameter only matters if the *Mount path* parameter has dependencies.
  - For cluster partitions, note that one *c2-standard-60* partition is defined by default. Additional partitions can be added, supporting different instance types. Enable or disable hyprethreading and node reuse as appropriate. Also, placement group can be enabled (for C2 and C2D partitions only).
- Finally, save the configurations and click the *Create* button to trigger the cluster creation.

## Application Management

Administrators can install and manage applications in the following ways:

### Install Spack applications

The recommended method of application installation is via Spack. Spack, an established package management system for HPC, contains build recipes of the most widely used open-source HPC applications. This method is completed automated. Spack installation is performed as a Slurm job. Simply choose a Slurm partition to run Spack. Advanced user may also customise the installation by specifying a Spack spec string.

### Install custom applications

For applications not yet covered by the Spack package repository, e.g., codes developed in-house, or those failed to build by Spack, use custom installations by specifying custom scripts containing steps to build the applications.

### Register manually installed applications

Complex packages, such as some commercial applications that may require special steps to set up, can be installed manually on the cluster's shared filesystem. Once done, they can be registered with the FrontEnd so that future job submissions can be automated through the FrontEnd.

### Application status

Clicking the *Applications* item in the main menu leads to the application list page which displays all existing application installations. Applications can be in different states and their *Actions* menus adapt to this information to show different actions:

- Status 'n' – Application is being newly configured by an admin user through the web interface. At this stage, only a database record exists in the system. The user is free to edit this application, although in the
case of a Spack application, most information is automatically populated. When ready, clicking Spack Install from the Actions menu to initiate the installation process.
- Status 'p' – Application is being prepared. In this state, application build is triggered from the web interface and information is being passed to the cluster.
- Status 'q' – In this state the Slurm job for building this application is queueing on the target cluster. Note that all application installations are performed on a compute node. This leaves the relatively
lightweight controller and login nodes to handle management tasks only, and also ensures the maximum possible compatibility between the generated binary and the hardware to run it in the future.
- Status 'i' – In this state, the Slurm job for building this application is running on the target cluster. Spack is fully responsible for building this application and managing its dependencies.
- Status 'r' - Spack build has completed successfully, and the pplication is ready to run by authorised users on the target cluster.
- Status 'e' - Spack has somehow failed to build this application. Refer to the debugging section of this document on how to debug a failed installation.
- Status 'x' – If a cluster has been destroyed, all applications on this cluster will be marked in this status. Destroying a cluster won’t affect the application and job records stored in the database.

A visual indication is shown on the website for any application installation in progress. Also, the relevant web pages will refresh every 15 seconds to pick status changes.

#### Install a Spack application

A typical workflow for installing a new Spack application is as follows:

- From the application list page, press the *New application* button. In the next form, select the target cluster and choose *Spack installation*.
- In the *Create a new Spack application* form, type a keyword in the *Create a new Spack application* form, and use the auto-
completion function to choose the Spack package to install. The *Name* and *Version* fields are populated automatically. If Spack supports multiple versions of the application, click the dropdown list there to select the desired version.
- Spack supports variants - applications built with customised compile-time options. These may be special compiler flags or optional features that must be switched on manually. Advanced users may
supply additional specs using the optional *Spack spec* field.
  - By default, the GCC 11.2 compiler is used for building all applications.
  - Other compilers may be specified with the % compiler specifier and an optional version number using the @ version specifier (e.g., `%intel@19.1.1.217`). Obviously, admin users are responsible for installing and configuring those additional compilers and, if applicable, arrange their licenses.
  - Spack is configured in this system to use Intel MPI to build application.
  - Other MPI libraries may be specified with the ^ dependency specifier and an optional version number.
- The Description field is populated automatically from the information found in the Spack repository.
- Choose an Slurm partition from the dropdown list to run the Slurm job for applicaiton installation. This, typically, should be the same partitions to run the application in the future.
- Click the *Save* button. A database record is then created for this application in the system. On the next page, click the *Edit* button to modify the application settings; click the *Delete* button to delete
this record if desired; click the *Spack install* button to actually start building this application on the cluster. The last step can take quite a while to complete depending on the application. A visual indication is given on the related web pages until the installation Slurm job is completed.
- A successfully installed application will have its status updated to ‘ready’. A *New Job* button becomes available from the Actions menu on the application list page, or from the application detail page. The [User Guide](user_guide.md) contains additional information on how jobs can be prepared and submitted.

## Workbench Management

The Workbench feature provides a way to create and control VertexAI Workbenches which provide a single interactive development environment using a Jupyter Notebook perfect for pre/post processing of data. Workbenches can be located within the same VPC as other GCP resources managed by the frontend. 

### Workbench Configuration
The first stage of configuration, after selecting the desired cloud credential, is to select the basic profile of the workbench including:
* Workbench Name
* Subnet
* Cloud Zone
* Trusted User

![Workbench create process part 1](images/Workbench-Create-1.png)

The subnet will define which regions the workbench can be located in. Workbenches are not available in all regions, see [Workbench Documentation](https://cloud.google.com/vertex-ai/docs/general/locations#vertex-ai-workbench-locations) for more detail on currently available regions. Once a region is selected the cloud zone field will be populated with the available zones. 

### Trusted Users
The trusted user field will govern which user has access to the workbench. This is a 1:1 relationship as each workbench has a single instance owner that is set by the trusted user value. The workbench is then configured to run the jupyter notebook as the users OSLogin account. Access to the notebook is controlled by a proxy that requires the user to be logged into their google account to gain access. 

Workbench instances have a limited number of configurations:
* Machine type
* Boot disk type
* Boot disk capacity
* Image type

![Workbench create process part 2](images/Workbench-Create-2.png)

### Machine type & Workbench Presets
An administrator can configure any type of machine type that is available. Users with the "Normal User" class will only be able to create workbenches using the preset machine type configurations while users with the "Viewer" class will not be able to create workbenches for themselves. The HPC toolkit frontend comes with some pre-configured workbench presets:
* Small - 1x core with 3840 Memory (n1-standard-1)
* Medium - 2x cores with 7680 Memory (n1-standard-2)
* Large - 4x cores with 15360 Memory (n1-standard-4)
* X-Large - 8x cores with 30720 Memory (n1-standard-8)

Each of these have been created under the category "Recommended". Presets can be edited, deleted or new presets added via the admin panel where you can set the machine type and the category under which the user will see the preset

![Workbench create process - Presets](images/Workbench-Create-Presets.png)

### Workbench Storage

The final setup of the workbench is to select any filesystems that are required to be mounted on the workbench. On this page the configuration fields will be disabled and no changes will be possible to the workbench configuration. 

![Workbench create process - Storage ](images/Workbench-Create-Storage.png)

Within this configuration you can select from existing storage exports, the order they are mounted, and the mouth path in the filesystem. Storage will be mounted in the order according to the mount order which will be important if you are mounting storage within a sub-directory of another storage mount. Another important configuration to be aware of is that filesystems will only be mounted if the filestore or cluster is active and has an accurate IP address or hostname in the frontends database. 

## Debugging problems

#### Finding Log Files

The service machine produces log files in `/opt/gcluster/run/`. These log files will show errors from the Django web application.

Cloud resource deployment log files (from Terraform) are typically shown via the Frontend web site.  If those logs are not being shown, they can be found on the service machine under `/opt/gcluster/hpc-toolkit/frontend/(clusters|fs|vpc)/...`.  HPC Toolkit log files will also be found in those directories.  The Terraform log files and status files will be down a few directories, based off of the Cluster Number, Deployment ID, and Terraform directory.

On Cluster controllers, most of the useful log files for debugging can be retrieved by executing the 'Sync Cluster' command.  These include SLURM log files as well as general system log files.  The daemon which communicates to the service machine logs to syslog, and can be viewed on the cluster controller node via `journalctl`, looking at the `ghpcfe_c2` service.

Job logs and Spack application logs are uploaded upon job completion to Google Cloud Storage and viewable via the HPC Frontend.

#### Deployment problems

Most deployment problems are caused by not having the right permissions. If this is the case, error message will normally show what permissions are missing. Use the [IAM permissions reference](https://cloud.google.com/iam/docs/permissions-reference) to research this and identify additional roles to add to your user account.

Before any attempt to redeploy the Frontend, make sure to run `terraform destroy` in `hpc-toolkit/frontend/tf` to remove cloud resources that have been already created.

### Cluster problems

The FrontEnd should be quite reliable provisioning clusters. However, in cloud computing, erroneous situations will happen and do happen from time to time; many outside our controls. For example, a resource creation could fail because the hosting GCP project has ran out of certain resource quotas. Or, an upgrade of an underlying machine image might have introduced changes that are incompatible to our system. It is not possible to capture all such situations. Here, a list of tips is given to help debug cluster creation problems. The [Developer's Guide](developer_guide.md) contains a lot of detais on how the backend logics are handled, which can also shed light on certain issues.

- If a cluster is stuck at status 'c', something is wrong with the provisioning of cluster hardware. SSH into the service machine and identify the directory containing the run-time data for that cluster at `frontend/clusters/cluster_<cluster_id>` where `<cluster_id>` can be found on the web interface. Check the Terraform log files there for debugging information.
- If a cluster is stuck at status 'i', hardware resources should have been commissioned properly and there is something wrong in the software configuration stage. Locate the IP address of the Slurm controller node and find its VM instance on GCP console. Check its related *Serial port* for system log. If needed, SSH into the controller from the GCP console to check Slurm logs under `/var/log/slurm/`.

### Application problems

Spack installation is fairly reliable. However, there are throusands of packages in the Spack repository and packages are not always tested on all systems. If a Spack installation returns an error, first locate the Spack logs by clicking the *View Logs* button from the application detail page. Then identify from the *Installation Error Log* the root cause of the problem.

Spack installation problem can happen with not only the package installed, but also its depdendencies. There is not a general way to debug Spack compilation problems. It may be helpful submit an interactive job to the cluster and debug Spack problems there manually. It is recommended to not build applications from the controller or login nodes, as the underlying processor may differ on the compute nodes.

Complex bugs should be reported to Spack. If an easy fix can be found, note the procedure. This can be then used in a custom installation.

### Workbench problems

#### Storage not mounted
If the expected filesystem storage has not been mounted or is not available the most likely cause is that the database does not have a hostname or IP address for the filestore or cluster targeted. An admin can resolve this by accessing the instance by SSHing into the GCP instance the runs the workbench and running `mount $IPADDRESS:/$TARGETDIR $MOUNTPOINT`

#### Workbench stuck in "Creating" status
If a workbench is stuck in "Creating" status this can be resolved by manually changing the status back to newly created in the admin portal and then starting the creation process again. Logs for this process can be seen at `$HPCtoolkitHome/frontend/workbenches/workbench_##/terraform/google/` where HPCtoolkitHome is normally /opt/gcluster and ## will be the id number of the workbench in question.

### General clean-up tips

- If a cluster is stucked in 'i' state, it is normally OK to find the *Destroy* button from its *Actions* menu to destroy it.
- For failed network/filesystem/cluster creations, one may need to SSH into the service machine, locate the run-time data directory, and manually run `terraform destroy` there for clean up cloud resources.
- Certain database records might get corrupted and need to be removed for failed clusters or network/filesystem components. This can be done from the Django Admin site, although adminstrators need to exercise caution while modifying the raw data in Django database.


## Teardown Process

  - **Important: First ensure that all clusters, workbenches and filestores are removed using the web interface before destroying it. These resources will otherwise persist and continue to cost.**
  - To tear down the web interface and its hosting infrastructure, navigate to the directory `hpc-toolkit/frontend/tf` on the original client machine and run `terraform destroy` to remove the service machine and associated resources.
