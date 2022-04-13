## HPC Toolkit FrontEnd - Developer’s Guide

### Architecture design

The HPC Toolkit FrontEnd is a web application integrating several front-end and back-end technologies. Django, a high-level Python-based web framework, forms the foundation of the web application. The back-end business logics can mostly be delegated to Terraform to create GCP cloud infrastructure required by the HPC clusters. With HPC Toolkit, there is no need to define infrastructure configurations from scratch. Rather, a high-level description of the clusters are provided for it to generate Terraform configurations.

The overall system design is described in the following figure. 

[TODO: insert figure]

In most cases, end users are expected to communicate with the cloud systems via the web front-end. Of course, users from a traditional supercomputing background may wish to work with HPC clusters from the command line. This is entirely possible and is covered in later sections.

<!--  This system is currently NOT capable of being run in parallel, so a Load Balancer will not add any benefits.

In a production environment, it is typically good practice to leave the web application behind a load balancer, which provides additional performance, scalability, and security. -->

A single compute engine virtual machine, referred to as the **service machine** from now on, should be created to host the application server, webserver and database server. In large productions, these servers can of course be hosted on different machines if required.

The web application is built upon Django, a Python framework to develop data-driven dynamic websites. A Nginx server is configured to serve the static files of the website, as well as proxying Django URLs to the application server. Application data is stored in a file-based SQLite database which can be easily replaced by a managed SQL service in large production.

From the web application, HPC clusters can be created on GCP by administrators. A typical HPC cluster contains a single Slurm controller node, and one or more login nodes, typically all running on low- to mid-range virtual machines. The controller node hosts the Slurm job
scheduler. Through the job scheduler, compute nodes can be started or terminated as required. The controller node and login nodes all provide public IP addresses for administrators or users to SSH into, although doing so is not mandatory as day-to-day tasks can be performed via the web interface.

The Slurm job scheduler supports partitions. Each partition can have compute nodes of different instance types. All major HPC capable instance types can be supported from a single cluster if so desired. Of course, it is also possible to create multiple clusters, which is entirely an
operational decision by the adminstrators.

For each cluster, two shared filesystems are created to host a system directory for applications and a home directory for users' job data. Additional filesystems can be created or imported, to be mounted to the clusters. 

For each deployment, a GCS bucket is created to hold supporting files, including configurations to build the service machine, Ansible configurations to set up various Slurm nodes. The same GCS bucket is also served as a long-term backup to application and job data, including log files for most cloud operations and selected files created by jobs.

Communication between the service machine and clusters is handled by Pub/Sub. For technical details, consult the [Cluster Command & Control](ClusterCommandControl.md) document. Alternatively, there is an API layer around Django to allow incoming communication to the service machine.

### Deploy the system

Please follow the deployment section in the [Administrator’s Guide](admin_guide.md) to deploy the system for testing and development.

### Access the service machine

By default, access to the service machine is restricted to authorised users (the owner/editor of the hosting GCP project or other users delegated with sufficient permissions). Use one of the following two methods to access the system after a new deployment:

- SSH into the service machine directly from the GCP console of the hosting GCP project.
- Edit the hosting VM instance by uploading the public SSH key of a client machine to grant SSH access.

Immediately after login, run `sudo su -l gcluster` to become the *gcluster* user. This user account was created during the deployment to be the owner of the frontend files.

### Directory structures on service machine

The home directory of the *gcluster* account is at */opt/gcluster*. For a new deployment, the following four sub-directories are created:

- *go* - the development environment of the Go programming language, required to build Google HPC Toolkit
- *hpc-toolkit* - a clone of the Google HPC Toolkit project. The *ghpc* binary should have already been built during the deployment. The *frontend* sub-directory contains the Django-based web application for the FrontEnd and other supporting files.
- *django-env* - a Python 3 virtual environment containing everything required to support Django development. To activate this environment: `source ~/django-env/bin/activate`.
- *run* -  directory for run-time data, including the following log files:
  - *nginx-access.log* - web server access log.
  - *nginx-error.log* - web server error log.
  - *supvisor.log* -  Django application server log. Python *print* from Django source files will appear in this file for debugging purposes.
  - *django.log* - additional debugging information generated by the Python logging module is writen here.

### Run-time data

#### For cloud resources

Run-time data to support creating and managing cloud resources are generated and stored in the following sub-directories within *hpc-toolkit/frontend*:

- *clusters/cluster_\<id>* - holding run-time data for a cluster. *\<id>* here has a one-to-one mapping to the IDs shown in the frontend's cluster list page. It contains the following:
  - *cluster.yaml* - input file for *ghpc*, generated based on information collected from web interface.
  - *\<cluster_name>_\<random_id>/primary* - Terraform files generated by *ghpc* to create the cluster, and log files from running `terraform init/validate/plan/apply`. Should there be a need to manually clean up the associated cloud resources, run `terraform destroy` here.
- *vpcs/vpc_\<id>* - similar to above but holding run-time data for a virtual network. Currently creating custom mode VPC is not yet supported by HPC Toolkit. A custom set of Terraform configurations are used.
- *fs/fs_\<id>* - similar to above but holding run-time data for a filesystem. Currently only GCP Filestore is supported.

#### For applications

Application data is stored in the shared filesystem `/opt/cluster`. It contains the following sub-directories:

- `/opt/cluster/spack` contains a Spack v0.17.1 installation.
- When applications are installed via the web interface, supporting files are saved in `/opt/cluster/install/<application_id>` where `<application_id>` can be located from the web interface.
  - For a Spack installation, a job script `install.sh` is generated to submit a Slurm job to the selected partition to run `spack install` of the desired package.
  - For a custom installation, a job script `install_submit.sh` is generated to submit a Slurm job to the selected partition to execute `job.sh` which contains the custom installation steps.
- After each successful installation, Spack application binaries are stored at `/opt/cluster/spack/opt/spack/linux-centos7-<arch>` where `<arch>` is the architecture of the processors on which the binaries get built, such as `cascadelake` or `zen2`.
- Standard output and error files for Slurm jobs are uploaded to the GCS bucket associated with the deployment at the following URLs: `gs://<deployment_name>-<deployment_zone>-storage/clusters/<cluster_id>/installs/<application_id>/stdout|err`.

#### For jobs

Job data is stored in the shared filesystem `/home/<username>` for each user. Here `<username>` is the OS Login username, which is generated by Google and will be different from the user's normal UNIX name. The home directories contain the following:

- When a job is submitted from the web interface, supporting files are saved in `/home/<username>/jobs/<job_id>` where `<job_id>` can be located from the web interface.
- When running a Spack application, a job script `submit.sh` is generated to submit a Slurm job. This script performs a `spack load` to set up the application environment and then invoke `job.sh` which contains the user-supplied custom commands to run the job.
- Standard output and error files for Slurm jobs are uploaded to the GCS bucket associated with the deployment at the following URLs: `gs://<deployment_name>-<deployment_zone>-storage/clusters/<cluster_id>/jobs/<job_id>/stdout|err`.

[comment]: <> (Can this resolve the conflict in the line below?)
Note that a special home directory is created at `/home/root_jobs` to host jobs submitted by the Django superusers. For convenience they do not need Google identities and their jobs are run as *root* on the clusters.


## Workbenches Architecture

The workbench process is fairly simple. Gather configuration values from the frontend and pass them to terraform to control the creation of the workbench instance. This is done directly via terraform as the HPC Toolkit does not currently support VertexAI Workbenches. 

### Infrastructure files
Workbenches are created using a template configuration in `hpc-toolkit/frontend/infrastructure_files/workbench_tf`. The terraform template was originally based on the terraform template provided by the [Google Cloud Platform Rad-Lab git repo](https://github.com/GoogleCloudPlatform/rad-lab) however the configuration diveraged during early development. The main reason for this divergance was to accomidate running the jupyter notebook as a specific OSLogin user rather than the generic jupyter user which would mean we were unable to interact properly with any mounted shared storage.

The process of creating the workbench files is mostly contained within the file `hpc-toolkit/frontend/website/ghpcfe/cluster_manager/workbenchinfo.py`. The copy_terraform() routine copies files from the infrastructure_files directory while the prepare_terraform_vars() routine creates a `terraform.tfvars` file within the `hpc-toolkit/frontend/workbenches/workbench_##` directory to provide the following info gathered by the frontend during the create workbench process:
* region
* zone
* project_name
* subnet_name
* machine_type
* boot_disk_type
* boot_disk_size_gb
* trusted_users
* image_family
* owner_id
* wb_startup_script_name
* wb_startup_script_bucket

### Storage Mount points 
Storage mount points are configured on the 2nd part of the creation process. This is done via a django updateview form at `https://$FRONTEND.URL/workbench/update/##` with the main configuration fields disabled as terraform does not support modification of an existing VertexAI workbench, the workbench would be destroyed and recreated. 

Additionally the mount points are added to the startup script and there is no method in the frontend to re-run this startup script to mount any additional mount points therefore the updateview form is only presented during the creation process. Once information on the mountpoints is collected the startup script can be generated.

### Startup script
The startup script is generated by the copy_startup_script() process in `cluster_manager/workbenchinfo.py`. This process is in two parts. The first part is generated using information gathered by the frontend and passes the user's social ID number set by the owner_id field. It also passes any configured mount points into the startup script before the second part of the startup script is copied from `infrastructure_files/gcs_bucket/workbench/startup_script_template.sh` 

The startup script runs the following processes when the workbench instance boots
1. Query instance metadata for list of users and filter based on the users social ID number to discover the correct format of their OSLogin username
2. Install nfs-common package via apt-get
3. Make temporary jupyterhome directory in /tmp/ and set user ownership
4. Make home directory for the user and set user ownership
5. Vopy jupyter configuration files from /home/jupyter/.jupyter to /tmp/jupyterhome/.jupyter
6. Create DATA_LOSS_WARNING.txt file with warning message
7. Configure specified mountpoints in order specified on frontend
8. Sdd symlink to /tmp/jupyterhome/ which will serve as working directory on the web interface
    * This process of mounting and symlinking means the mountpoint will appear in both the jupyter notebook web interface working directory and in the expected location in the root filesystem
9. Append mount points to DATA_LOSS_WARNING.txt file
10. If /home was not mounted as a mount point then create a symlink to /home in /tmp/jupyterhome
11. Modify jupyter config to reflect username & new working directory
12. Update `/lib/systemd/system/jupyter.service` systemd service file to reflect username and new working directory
13. Run systemctl daemon-reload and restart jupyter service
    * Without updating the jupyter config and restarting the service then the jupyter notebook would be running as the jupyter user. This would break permissions used on any mounted shared storage.


 
