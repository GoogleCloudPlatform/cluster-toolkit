## HPC Toolkit FrontEnd - Administrator’s Guide

### System Deployment

### Post-deployment Configurations

#### SSH access to the service machine

SSH access to the service machine is possible for administration purpose. Administrators can choose from one of the following options:

- An administrator can add his/her public SSH key to the VM instance after the deployment via GCP console or command line.
- An administrator can SSH directly from the GCP console.
- A user with project-wide SSH set in the hosting GCP project should already have SSH access to the service machine.

#### Set up Google login

While it is possible to use a Django user account to access the FrontEnd website, and indeed doing so is required for some administration tasks, ordinary users must authenticate using their Google identities so that, via Google OSLogin, they can maintain consistent Linux identities across VM instances that form the clusters. This is made possible by the *django-allauth* social login extension. 

For a production deployment, a domain name must be obtained and attached to the website. Next, register the site with the hosting GCP project on the GCP console in the *Credentials* section under *APIs and services* category.

![Oauth set-up](images/GCP-app-credential.png)

From the GCP console, note the client ID and client secret. Then return to admin site of the deployment, locate the *social applications* database table. A 'Google API' record should have been created during the deployment. Replace the two placeholders with the client ID and client secret. The site is ready to accept Google login.

![Social login set-up](images/register-social-app.png)]

Next, go to the *Authorised user* table. This is where further access control to the site is applied. Create new entries to grant access to users. A new entry can be:

- a valid domain name to grant access to multiple users from authorised organisations (e.g. @example.com) 
- an email address to grant access to an individual user. 

Logins that do not match these patterns will be rejected.

### Credential Management

### Network Management

All cloud systems begin with defining the network within which the systems will be deployed. Before a cluster or stand-alone filesystem can be created, the administrator must create the virtual cloud network (VPC). This is accomplished under the *Networks* main menu item.

#### Create a new VPC
To create a new network, the admin must first select which cloud credential should be used for this network, then give the VPC a name, and then select the cloud region for the network.

Upon clicking the *Save* button, the network is not immediately created. The admin has to click *Edit Subnet* to create at least one subnet. Once the network and subnets are appropriately defined, click the ‘Apply Cloud Changes’ button to trigger Terraform to provision the  cloud resources.

#### Import an existing VPC

If the organisation already has pre-defined VPCs on cloud within the hosting GCP project, they can be imported. Simply selecting an existing VPC and associated subnets from the web interface to register them with the system. Imported VPCs can be used in exactly the same way as newly created ones.


### Filesystem Management

#### Create new filesystems

#### Import existing filesystems

Import NFS: an external NFS can be registered to this system and subsequently mounted by clusters. For this to work, provide its IP address and export name. The IP address can be:

- A public IP address
- An internal IP address. 

An internal address can be used if the cluster shares the same VPC with the imported filesystem. Alternatively, system administrators can set up hybrid connectivity (such as extablishing network peering) beforing mounting the external filesystem located elsewhere on GCP. 

### Cluster Management

### Application Management

### Job Management

### Benchmarks