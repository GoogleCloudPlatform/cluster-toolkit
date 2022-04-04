## HPC Toolkit FrontEnd - Administrator’s Guide

This document is for administrators of the HPC Toolkit FronEnd. An administrator can manage the life cycles of HPC clusters, set up networking and storage resources that support clusters, install applications and manage user access. Ordinary HPC users should refer to the [User Guide](user_guide.md) on how to prepare and run jobs on existing clusters.

The HPC Toolkit FronEnd is a web application built upon the Django framework. By default, a Django superuser is created at deployment time. For large organisations, additional Django superusers can be created from the Admin site. 

### System Deployment

To deploy this system, follow these simple steps:

- Make sure the client machine has supporting software installed, such as the Google Cloud CLI and Terraform.
- Make sure that a hosting GCP project is in place and the current user has sufficient permissions to access multiple cloud resources. Project *Onwer*s and *Editor*s are obviously fine. For other users, the following list of roles are likely to be sufficient: *Compute Admin*; *Storage Admin*; *Pub/Sub Admin*; *Create Service Accounts*; *Delete Service Accounts*; *Service Account User*, although permissions can be further fine-tuned to achieve least privilege.
-`git clone` the HPC Toolkit project from GitHub into a client machine [TODO: give official repository name after turning this into a public project].
- Run `hpc-toolkit/frontend/deploy.sh` to deploy the system. Follow instructions to name the hosting VM instance and select its cloud region and zone. The hosting VM will be referred to as the *service machine* from now on.
- For a production deployment, follow on-screen instructions to create a static IP address and then provide a domain name. In this case, an SSL certificate will be automatically obtained to secure the web application. For testing purpose, ignore the IP and domain name - the system can still be successfully deployed and ran on an IP address (although some features may not be fully functioning).
- Follow instructions to provide details for a Django superuser account.
- On prompt, check the generated Terraform settings and make further changes if necessary. This step is optional.
- Confirm to create the VM instance. The VM creation takes a few minutes. However, after the script has completed, it can take up to 15 more minutes to have the software environment set up. Be patient.
- Use the domain name or IP address to access the website. Log in as the Django superuser. The deployment is completed.

To uninstall after testing, make sure to clean up cluster resouces from the web interface first. Then go to `hpc-toolkit/frontend/tf` and run `terraform destroy` to remove the service machine and associated resources.


### Post-deployment Configurations

#### SSH access to the service machine

SSH access to the service machine is possible for administration purpose. Administrators can choose from one of the following options:

- An administrator can add his/her public SSH key to the VM instance after the deployment via GCP console or command line.
- An administrator can SSH directly from the GCP console.
- A user with project-wide SSH set in the hosting GCP project should already have SSH access to the service machine.

#### Set up Google login

While it is possible to use a Django user account to access the FrontEnd website, and indeed doing so is required for some administration tasks, ordinary users must authenticate using their Google identities so that, via Google OSLogin, they can maintain consistent Linux identities across VM instances that form the clusters. This is made possible by the *django-allauth* social login extension. 

For a production deployment, a domain name must be obtained and attached to the website. Next, register the site with the hosting GCP project on the GCP console in the *Credentials* section under *APIs and services* category. Note that the *Authorised JavaScript origins* field should contain a callback URL in the following format: *https://<domain_name>/accounts/google/login/callback/*

![Oauth set-up](images/GCP-app-credential.png)

From the GCP console, note the client ID and client secret. Then return to admin site of the deployment, locate the *social applications* database table. A 'Google API' record should have been created during the deployment. Replace the two placeholders with the client ID and client secret. The site is ready to accept Google login.

![Social login set-up](images/register-social-app.png)]

Next, go to the *Authorised user* table. This is where further access control to the site is applied. Create new entries to grant access to users. A new entry can be:

- a valid domain name to grant access to multiple users from authorised organisations (e.g. @example.com) 
- an email address to grant access to an individual user. 

Logins that do not match these patterns will be rejected.

### Credential Management

To use the web system to create cloud resources, the first task is for an admin user to register a cloud credential with the system. The supplied credential will be validated and stored in the database for future use.

The preferred way to access GCP resources from this system is through a [Service Account](https://cloud.google.com/iam/docs/service-accounts). To create a service account, a GCP account with sufficient permissions is required. Typically, the Owner or Editor of a GCP project have enough permissions. For other users with custom roles, if certain permissions are missing, GCP will typically return clear error messages. 

For this project, the following roles should be sufficient for the admin users to manage the required service account: *Service Account User*, *Service Account Admin*, and *Service Account Key Admin*.

To register the GCP credential with the system:

- Log in to the GCP console and select the GCP project that hosts this work.
- From the main menu, select *IAM & Admin*, then *Service Accounts*.
- Click the *CREATE SERVICE ACCOUNT* button.
- Name the service account, optionally provide a description, and then click the *CREATE* button.
- Grant the service account the following roles: *Editor*, *Security Admin*.
- Human users may be given permissions to access this service account but that is not required in this work. Clock *Done* button.
- Locate the new service account from the list, click *Manage Keys* from the *Actions* menu.
- Click *ADD KEY*, then *Create new key*. Select JSON as key type, and click the *CREATE* button.
- Copy the generated JSON content which should then be pasted into the credential creation form on the website.
 
### Network Management

All cloud systems begin with defining the network within which the systems will be deployed. Before a cluster or stand-alone filesystem can be created, the administrator must create the virtual cloud network (VPC). This is accomplished under the *Networks* main menu item. Note that network resources have their own life cycles and are managed independently to cluster.

#### Create a new VPC
To create a new network, the admin must first select which cloud credential should be used for this network, then give the VPC a name, and then select the cloud region for the network.

Upon clicking the *Save* button, the network is not immediately created. The admin has to click *Edit Subnet* to create at least one subnet. Once the network and subnets are appropriately defined, click the ‘Apply Cloud Changes’ button to trigger Terraform to provision the  cloud resources.

#### Import an existing VPC

If the organisation already has pre-defined VPCs on cloud within the hosting GCP project, they can be imported. Simply selecting an existing VPC and associated subnets from the web interface to register them with the system. Imported VPCs can be used in exactly the same way as newly created ones.


### Filesystem Management

By default each cluster creates two shared filesystems: one at */opt/cluster* to hold installed applications and one at */home* to hold job files for individual users. Both can be customised if required. Additional filesystems may be created and mounted to the clusters. Note that filesystem resources have their own life cycles and are managed independently to cluster.

#### Create new filesystems

Currently, only GCP Filestore is supported. GCP Filestore can be created from the *Filesystems* main menu item. A new Filestore has to be associated with an existing VPC and placed in a cloud zone. All performance tiers are supported.

#### Import existing filesystems

External NFS can be registered to this system and subsequently mounted by clusters. For this to work, for each NFS, provide an IP address and an export name. The IP addresses can be both public IP and internal.

An internal address can be used if the cluster shares the same VPC with the imported filesystem. Alternatively, system administrators can set up hybrid connectivity (such as extablishing network peering) beforing mounting the external filesystem located elsewhere on GCP. 

### Cluster Management

HPC clusters can be created after setting up the hosting VPC and, optionally, additional filesystems. The HPC Toolkit FrontEnd can manage the whole life cycles of clusters. Click the *Clusters* item in the main menu to list all existing clusters.

#### Cluster status
Clusters can be in different states and their *Actions* menus adapt to this information to show different actions:

- Status 'n' – Cluster is being newly configured by user. At this stage, a new cluster is being set up by an administrator. Only a database record exists, and no cloud resource has been created yet. User is free to edit this cluster: rename it, re-configure its associated network and storage components, and add authorized users. Click *Start* from the cluster detail page to actually provision the cluster on GCP.
- Status 'c' – Cluster is being created. This is a state when the backend Terraform scripts is being invoked to commission the cloud resources for the Cluster. This transient stage typically lasts for a few minutes.
- Status 'i' – Cluster is being initialised. This is a state when the cluster hardware is already online, and Ansible playbooks are being executed to install and configure the software environment of the Slurm controller and login nodes. This transient stage can last for up to 10 more minutes.
- Status 'r' – Cluster is ready for jobs. The cluster is now ready to use. Applications can be installed and jobs can run on it. A Slurm job scheduler is running on the controller node to orchestrate job activities.
- Status 't' – Cluster is terminating. This is a transient state after Terraform is being invoked to destroy the cluster. This stage can take a few minutes when Terraform is working with the cloud platform to decommission cloud resources.
- Status 'd’ – Cluster has been destroyed. When destroyed, a cluster cannot be brought back online. Only the relevant database record remains for information archival purposes.

A visual indication is shown on the website for the cluster being in creating, initialising or destroying states. Also, relevant web pages will refresh every 15 seconds to pick status changes.

#### Create a new cluster

A typical workflow for creating a new cluster is as follows:

- At the bottom of the cluster list page, click the *Add cluster* button to start creating a new cluster. In the next form, choose a cloud credential. This is the account all cloud spending by this cluster
would be charged to. Click the *Next* button to go to a second form from which details of the cluster can be specified.
- In the *Create a new cluster* form, give the new cluster a name. Cloud resource names are subject to naming constraints and will be validated by the system.
- From the *Subnet* dropdown list, select the subnet within which the cluster resides.
- From the *Cloud zone* dropdown list, select a zone.
- From the *Authorised users* list, select users that are allowed to use this cluster. 
- Click the *Save* button to store the cluster settings in the database. Continue from the *Cluster Detail* page.
- Click the *Edit* button to make additional changes. such as creating more Slurm partitions for differnt compute node instance types, or 
mounting additional filesystems.
  - For filesystems, note the two existing shared filesystems defined by default. Additional ones can be mounted if they have been created earlier. Note the *Mounting order* parameter only matters if the *Mount path* parameter has dependencies.
  - For cluster partitions, note that one *c2-standard-60* partition is defined by default. Additional partitions can be added, supporting different instance types. Enable or disable hyprethreading and node reuse as appropriate. Also, placement group can be enabled (for C2 and C2D partitions only).
- Finally, save the configurations and click the *Create* button to trigger the cluster creation.

### Application Management

Administrators can install and manage applications in the following ways:

###### Install Spack applications

The recommended method of application installation is via Spack. Spack, an established package management system for HPC, contains build recipes of the most widely used open-source HPC applications. This method is completed automated. Spack installation is performed as a Slurm job. Simply choose a Slurm partition to run Spack. Advanced user may also customise the installation by specifying a Spack spec string.

###### Install custom applications

For applications not yet covered by the Spack package repository, e.g., codes developed in-house, or those failed to build by Spack, use custom installations by specifying custom scripts containing steps to build the applications.

###### Register manually installed applications

Complex packages, such as some commercial applications that may require special steps to set up, can be installed manually on the cluster's shared filesystem. Once done, they can be registered with the FrontEnd so that future job submissions can be automated through the FrontEnd.

#### Application status

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

- From the application list page, press the *New application* button. In the next form, select the target cluster and choose *Spack instatllation*.
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

### Debugging problems

#### Deployment problems

Most deployment problems are caused by not having the right permissions. If this is the case, error message will normally show what permissions are missing. Use the [IAM permissions reference](https://cloud.google.com/iam/docs/permissions-reference) to research this and identify additional roles to add to your user account.

Before any attempt to redeploy, make sure to run `terraform destroy` in `hpc-toolkit/frontend/tf` to remove cloud resources that have been already created. Also remove the Terraform state files.

#### Cluster problems

The FrontEnd should be quite reliable provisioning clusters. However, in cloud computing, erroneous situations will happen and do happen from time to time. many outside our controls. For example, a resource creation could fail because the hosting GCP project has ran out of certain resource quotas. Or, an upgrade of an underlying machine image might have introduced changes that are imcompatible to our system. It is not possible to capture all such situations. Here, a list of tips is given to help debug cluster creation problems. The [Developer's Guide](developer_guide.md) contains a lot of detais on how the backend logics are handled, which can also shed light on certain issues.

- If a cluster is stuck at status 'c', something is wrong with the provisioning of cluster hardware. SSH into the service machine and identify the directory containing the run-time data for that cluster at `frontend/clusters/cluster_<cluster_id>` where `<cluster_id>` can be found on the web interface. Check the Terraform log files there for debugging information.
- If a cluster is stuck at status 'i', hardware resources should have been commissioned properly and there is something wrong in the software configuration stage. Locate the IP address of the Slurm controller node and find its VM instance on GCP console. Check its related *Serial port* for system log. If needed, SSH into the controller from the GCP console to check Slurm logs under `/var/log/slurm/`.

#### Application problems

Spack installation is fairly reliable. However, there are throusands of packages in the Spack repository and packages are not always tested on all systems. If a Spack installation returns an error, first locate the Spack logs by clicking the *View Logs* button from the application detail page. Then identify from the *Installation Error Log* the root cause of the problem.

Spack installation problem can happen with not only the package installed, but also its depdendencies. There is not a general way debugging Spack problems. It may be helpful to create a standalone compute engine virtual machine with Centos 7 operating system (same OS used by our Slurm clusters) and debug Spack problems there manually. Alternatively, adminstrators can SSH into the Slurm controller and try the manually run `spack` commands there for debugging.

Complex bugs should be reported to Spack. If an easy fix can be found, note the procedure. This can be then used in a custom installation.

#### General clean-up tips

- If a cluster is stucked in 'i' state, it is normally OK to find the *Destroy* button from its *Actions* menu to destroy it.
- For failed network/filesystem/cluster creations, one may need to SSH into the service machine, locate the run-time data directory, and manually run `terraform destroy` there for clean up cloud resources.
- Certain database records might get corrupted and need to be removed for failed clusters or network/filesystem components. This can be done from the Django Admin site, although adminstrators need to exercise caution while modifying the raw data in Django database.
