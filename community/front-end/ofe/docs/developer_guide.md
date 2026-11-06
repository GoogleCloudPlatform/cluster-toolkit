# Open Front End (OFE) - Developer Guide
<!--
0        1         2         3         4         5         6         7        8
1234567890123456789012345678901234567890123456789012345678901234567890234567890
-->
## Architecture design

The Open Front End (OFE) is a web application integrating several front-end and
back-end technologies. *Django*, a high-level Python-based web framework, forms
the foundation of the web application. The back-end business logics can mostly
be delegated to *Terraform* to create GCP cloud infrastructure required by the
HPC clusters. With Cluster Toolkit, there is no need to define infrastructure
configurations from scratch. Rather, a high-level description of the clusters
are provided for it to generate Terraform configurations.

The overall system design is described in the following figure:

![System Design](images/system-design.png) <!--TODO: refine this sketch-->

> **Note:** `images/system-design.png` and `images/db-UML.png` (below) predate
> newer models and are not regenerated with each change. For example, the UML
> diagram shows `Workbench.trusted_users` (now `trusted_user`) and has no
> `attached_cluster` field, `WorkbenchPreset` or container registry models.
> Treat `website/ghpcfe/models.py` as the source of truth.

In most cases, end-users are expected to use and communicate with the cloud
systems via the OFE. Of course, users from a traditional supercomputing
background may wish to work with HPC clusters from the command line. This is
entirely possible through the Slurm login nodes.

<!--
In a production environment, it is typically good practice to leave the web
application behind a load balancer, which provides additional performance,
calability, and security.

This system is currently NOT capable of being run in parallel, so a Load
Balancer will not add any benefits.
-->

A single compute engine virtual machine, referred to as the **service machine**
from now on, should be created to host the application server, webserver and
database server. In large productions, these servers can of course be hosted on
different machines if desired.

The web application is built upon Django (6.1, see `requirements.in`), a Python framework to develop
data-driven dynamic websites. An *nginx* server is configured to serve the
static files of the website, as well as proxying Django URLs to the application
server. ([Uvicorn](https://www.uvicorn.org) is the chosen application server).
Application data is stored in a file-based SQLite database which can be easily
replaced by a managed SQL service for large production environments.

From the web application, HPC clusters can be created on GCP by administrators.
A typical HPC cluster contains a single Slurm controller node, and one or more
login nodes, typically all running on low- to mid-range virtual machines. The
controller node hosts the Slurm job scheduler. Through the job scheduler,
compute nodes can be started or terminated as required. The controller node and
login nodes all provide public IP addresses for administrators or users to SSH
into, although doing so is not mandatory as day-to-day tasks can be performed
via the web interface.

The Slurm job scheduler supports partitions. Each partition can have compute
nodes of different instance types. All major HPC capable instance types can be
supported from a single cluster if so desired. Of course, it is also possible
to create multiple clusters, which is entirely an
operational decision by the administrators.

For each cluster, two shared filesystems are created to host a system directory
for applications and a home directory for users' job data. Additional
filesystems can be created or imported, to be mounted to the clusters.

For each deployment, a GCS bucket is created to hold supporting files,
including configurations to build the service machine, and Ansible
configurations to set up various Slurm nodes. The same GCS bucket is also
served as a long-term backup to application and job data, including log files
for most cloud operations and selected files created by jobs.

Communication between the service machine and clusters is handled by
[Pub/Sub](https://cloud.google.com/pubsub). For technical details, consult the
[Cluster Command & Control](ClusterCommandControl.md) document. Alternatively,
there is an API layer around Django to allow incoming communication to the
service machine.

### Deploy the system

Please follow the deployment section in the
[Administrator’s Guide](admin_guide.md) to deploy the system for testing and
development.

Here are some notes from a developer's perspective:

- The deployment is done using Terraform (`>= 1.12.2`, see `tf/versions.tf`).
  The `tf/` and `tf/network/` directories and `script/` are internal to
  `deploy.sh` and `teardown.sh`.
- When `deploy.sh` is invoked interactively, it validates the client machine's
  development environment, collects configuration information from user, save
  input variables in `tf/terraform.tfvars`, and invoke Terraform. With
  `--config`, many of these checks are skipped; see the
  [Administrator's Guide](admin_guide.md#automated-deployment-process).
  - The deploy script will check the GCP project being used has the correct
    APIs enabled.  The list of required APIs is embedded in `deploy.sh`
    script - **this will need to be maintained**.
  - The deploy script will also, optionally, use an additional script,
    `script/service_account.sh`, to create a GCP service account.  This sets
    the correct roles/permissions on the account based on a list kept in the
    script - **this will need to be maintained**.
- Terraform creates a hosting VPC and a subnetwork for the deployment, together
  with the necessary firewall rules.
- Terraform creates a supporting GCS bucket. This bucket is not only used
  during deployment, but also provides a long-term storage for clusters
  operating within this deployment.
- Terraform sets up a Pub/Sub topic for communication between the service
  machine and clusters.
- Terraform provisions a compute engine virtual machine to be the service
  machine, using the Rocky Linux 8 image pinned in `tf/main.tf`
  (`rocky-linux-8-v20220126`). A startup script is then executed on the service machine to set up
  the software environment for Cluster Toolkit and Django, and start the web and
  application servers.

### Access the service machine

By default, access to the service machine is restricted to authorised users
(the owner/editor of the hosting GCP project or other users delegated with
sufficient permissions). Use one of the following two methods to access the
system after a new deployment:

- SSH into the service machine directly from the GCP console of the hosting GCP
  project.
- Edit the hosting VM instance by uploading the public SSH key of a client
  machine to grant SSH access.

Immediately after login, run `sudo su -l gcluster` to become the *gcluster*
user. This user account was created during the deployment to be the owner of
the OFE files.

### Directory structures on service machine

The home directory of the *gcluster* account is at `/opt/gcluster`. For a new deployment, the following four sub-directories are created:

- `go` - the development environment of the Go programming language, required to build Google Cluster Toolkit
- `cluster-toolkit` - a clone of the Google Cluster Toolkit project. The
   `gcluster` binary (and its compatibility symlink `ghpc`, which is what the
   backend still invokes) should have already been built during the
   deployment. The `community/front-end/ofe` sub-directory contains the
   Django-based web application for the OFE and other supporting files.
- `django-env` - a Python 3.12 virtual environment containing everything required
   to support Django development. To activate this environment:
   `source ~/django-env/bin/activate`. Doing so may be required during the
   development, e.g., when running the Django `manage.py` script for
   administration tasks.
- `run` -  directory for run-time data, including the following log files:
  - `nginx-access.log` - web server access log.
  - `nginx-error.log` - web server error log.
  - `supvisor.log` -  Django application server log. Python `print` from
     Django source files will appear in this file for debugging purposes.
  - `django.log` - additional debugging information generated by the Python
     logging module is written here.

### Run-time data

#### For cloud resources

Run-time data to support creating and managing cloud resources are generated
and stored in the following sub-directories within
`/opt/gcluster/cluster-toolkit/community/front-end/ofe` on the service machine:

- `clusters/cluster_\<id>` - holding run-time data for a cluster. `\<id>` here
has a one-to-one mapping to the IDs shown in the frontend's cluster list page.
The directory contains the following:
  - `cluster.yaml` - input file for `gcluster` (invoked as `ghpc`), generated
    based on information collected from the web interface.
  - `\<cluster_name>_\<random_id>/primary` - Terraform files generated by
   `ghpc create` to create the cluster, and log files from running
   `terraform init/validate/plan/apply`. Should there be a need to manually
    clean up the associated cloud resources, run `terraform destroy` here.
- `vpcs/vpc_\<id>` - similar to above but holding run-time data for a virtual
  network. Currently creating custom mode VPC is not yet supported by HPC
  Toolkit. A custom set of Terraform configurations are used.
- `fs/fs_\<id>` - similar to above but holding run-time data for a filesystem.
  Currently only GCP Filestore is supported.
- `workbenches/workbench_<id>` - run-time data for a Workbench.

Note that life cycles of VPCs and Filestores are managed independently to those
of clusters.

#### For applications

Application data is stored in the shared filesystem `/opt/cluster`. It contains the following sub-directories:

- `/opt/cluster/spack` contains a Spack v0.21.0 installation (cloned by the
  `spack_install` Ansible role).
- When applications are installed via the web interface, supporting files are
  saved in `/opt/cluster/installs/<application_id>` where `<application_id>` can
  be located from the web interface.
  - For a Spack installation, a job script `install.sh` is generated to
    submit a Slurm job to the selected partition to run `spack install` of
    the desired package. Output goes to `<spack_name>.out` and `<spack_name>.err`
    in the same directory.
  - For a custom installation, a job script `install_submit.sh` is generated
    to submit a Slurm job to the selected partition to execute `job.sh` which
    contains the custom installation steps. Output goes to `<name>.out` and
    `<name>.err` in the same directory.
- After each successful installation, Spack application binaries are stored at
  `/opt/cluster/spack/opt/spack/linux-rocky9-<arch>` where `<arch>` is the
  architecture of the processors on which the binaries get built, such as
  `cascadelake` or `zen2`. (The OS part follows the Slurm image family, Rocky
  Linux 9; older CentOS 7 based clusters used `linux-centos7-<arch>`. This
  directory name is derived from Spack's platform detection and has not been
  confirmed against a running cluster.)
- Standard output and error files for Slurm jobs are stored in the working
  directories and also uploaded to the GCS bucket associated with the
  deployment at `gs://<deployment_name>-storage-<suffix>/clusters/<cluster_id>/installs/<application_id>/stdout|stderr`
  so that they remain available even if the clusters are destroyed. The bucket
  is created by `tf/main.tf` as `<deployment_name>-storage-<suffix>`, where
  `<suffix>` is a random string.

#### For jobs

Job data is stored in the shared filesystem `/home/<username>` for each user.
Here `<username>` is the OS Login username, which is generated by Google and
will be different from the user's normal UNIX name. The home directories
contain the following:

- When a job is submitted from the web interface, supporting files are saved in
  `/home/<username>/jobs/<job_id>` where `<job_id>` can be located from the web
  interface.
- When running a Spack application, a job script `submit.sh` is generated to
  submit a Slurm job. This script performs a `spack load` to set up the
  application environment and then invoke `job.sh` which contains the
  user-supplied custom commands to run the job.
- Standard output and error files for Slurm jobs are uploaded to the GCS bucket
  associated with the deployment at the following URLs:
  `gs://<deployment_name>-storage-<suffix>/clusters/<cluster_id>/jobs/<job_id>/stdout|stderr`.

Note that a special home directory is created at `/home/root_jobs` to host jobs
submitted by the Django superusers. For convenience they do not need Google
identities and their jobs are run as *root* on the clusters.

### Django development

#### Database Design

Django is great at building data-driven applications. The major system
components, such as clusters, applications, and jobs, can easily map to Django
data models. The database design of this system is best shown with a UML
diagram. This was generated using a function available in the Python
*django-extensions* package (which needs the Python *pydotplus* package and
the *graphviz* package to create the image output - neither is listed in
`requirements.in`, so install them first, e.g. `pip install pydotplus` and your
OS `graphviz` package). To generate the UML diagram, run from the command line
in the `website` directory:

```bash
python manage.py graph_models -a -X <classes_to_exclude> -o UML_output.png
```

To simplify the output and exclude Django's internal models, append a list of
comma-separated class names after the -X flag. The result is shown below:

![UML](images/db-UML.png)

Note that the *CloudResource* model is at the base of all cloud resources
including network components, storage components, compute instance
(representing a single VM), clusters, and Workbenches.

#### Code Layout

The top few layers of the directory hierarchy of the Open Front End (OFE)
define the major components:

| dir                         | description |
|-----------------------------|-------------|
| `cluster-toolkit/community/front-end/ofe/` | Top level   |
| `.../cli/`                  | client commandline interface |
| `.../docs/`                 | documentation |
| `.../infrastructure_files/` | Support files for deploying cloud infrastructure |
| `.../tf/`                   | Terraform files for deploying the OFE |
| `.../website/`              | Source code for the OFE website |

##### Infrastructure Files

| dir                         | description |
|-----------------------------|-------------|
| `.../ofe/infrastructure_files/` | Top level   |
| `.../cluster_startup/`               | Bootstrap startup scripts |
| `.../gcs_bucket/`                    | Common configuration files (scripts, Ansible) to store in GCS |
| `.../vpc_tf/`                        | Terraform templates for creating VPCs |
| `.../workbench_tf/`                  | Terraform templates for creating Workbenches |

These directories hold all the support infrastructure files which are used to
create, provision, and initialize the cloud resources which may be created via
the Open Front End (OFE).  The VPC Terraform and Workbench Terraform files may
eventually migrate into Cluster Toolkit YAML files.

The files under `gcs_bucket` contain the more in-depth startup scripts and
configuration information for the OFE webserver as well as for new
clusters.  During the initial deployment of the Open Front End (OFE), this
directory is copied to a new Google Cloud Storage bucket which is then used for
storing these startup codes as well as additional cluster information, such as
log files.  When clusters are created in Google Cloud, the initial bootstrap
template startup script (from the `cluster_startup` directory) are set to be
the instance startup script.  These scripts are responsible for downloading
from Google Cloud Storage the Ansible repository which is stored in the
`gcs_bucket/clusters/ansible_setup/`, and running Ansible to initialize the
instance.

The source-code for the cluster client-side Command & Control daemon is stored
here as well, under
`.../ansible_setup/roles/c2_daemon/files/ghpcfe_c2daemon.py`

##### Website

| dir                     | description |
|-------------------------|-------------|
| `.../ofe/website/` | Top level   |
| `.../ghpcfe/`           | OFE Application dir |
| `....../cluster_manager/` | Utilities for cloud & backend operations |
| `....../management/`     | Extra Django setup commands |
| `....../migrations/`     | Database migration scripts |
| `....../static/`         | Images, Javascript and other static web collateral |
| `....../templates/`      | Web view templates |
| `....../views/`          | Python files for model views |
| `.../templates/`        | All-Social plugin Templates |
| `.../website/`          | Django core website configuration (including `settings.py`) |
| `.../manage.py`         | Core Django application management script |

As with many Django-based web applications, the Open Front End (OFE) Django
application is broken across multiple directories, each responsible for some
critical subcomponent of the overall application, implementing the MVT (model,
view, template) architecture.  The `ghpcfe/` directory hosts the pieces
specific to the Open Front End (OFE), whereas the other directories are more
Django-focused.

Under `ghpcfe/`, there are a variety of directories as show in the above
table.  Directly inside this directory is the majority of the Python files
which make up the OFE web application. Of particular interest would be
`models.py`, which stores the DB models and `forms.py`, for custom web forms.
There are a sufficiently large number of Django views in the application, so
the prototypical `views.py` is broken into its own Python package.

Also under `ghpcfe/` is the `cluster_manager` directory, which contains most of
the "backend" code responsible for gathering cloud information as well as
creating, controlling, and configuring cloud resources.  Of particular interest
here are the files:

- `c2.py`: Responsible for bidirectional communication between the OFE and
  any created clusters.
- `cloud_info.py`: Provides many utilities for querying information from Google
  Cloud about instance types, pricing, VPCs and so forth
- `clusterinfo.py`: Responsible for creating clusters and keeping track of
  local-to-frontend cluster metadata

Note that these "backend" functions are often invoked asynchronously if the
tasks performed are time-consuming. Support to asynchronous views were
introduced in Django v3.1.

Finally, `templates` directory contains the web view templates. Django ships
with its own template engine to process these template files and insert dynamic
contents to them.

## Workbenches Architecture

The workbench process is fairly straight-forward. Gather configuration values
from the OFE and pass them to Terraform to control the creation of the
workbench instance. This is done directly via Terraform as the Cluster Toolkit does
not currently support Vertex AI Workbenches.

### Infrastructure files

Workbenches are created using a template configuration in
`cluster-toolkit/community/front-end/ofe/infrastructure_files/workbench_tf`. The Terraform
template was originally based on the Terraform template provided by the
[Google Cloud Platform Rad-Lab git repo](https://github.com/GoogleCloudPlatform/rad-lab)
however the configuration diverged during early development. The main reason
for this divergence was to accommodate running the Jupyter notebook as a
specific OSLogin user rather than the generic Jupyter user which would make it
impossible to interact properly with any mounted shared storage.

The process of creating the workbench files is mostly contained within the file
`cluster-toolkit/community/front-end/ofe/website/ghpcfe/cluster_manager/workbenchinfo.py`. The
`copy_terraform()` routine copies files from the `infrastructure_files`
directory while the `prepare_terraform_vars()` routine creates a
`terraform.tfvars` file within the
`cluster-toolkit/community/front-end/ofe/workbenches/workbench_##` directory to provide the
following info gathered by the OFE during the workbench creation process:

- region
- zone
- project_name
- subnet_name
- machine_type
- boot_disk_type (`PD_STANDARD`, `PD_BALANCED` or `PD_SSD`)
- boot_disk_size_gb
- trusted_user (the email address of the trusted user)
- instance_image (an object with `project = "deeplearning-platform-release"`
  and `family` set to the selected image family)
- owner_id (a list holding the same email address)
- wb_startup_script_name
- wb_startup_script_bucket

### Storage mount points

Storage mount points are configured on the second part of the creation process.
This is done via a Django UpdateView form at
`https://$FRONTEND.URL/workbench/update/##` with the main configuration fields
disabled as Terraform does not support modification of an existing Vertex AI
workbench, the workbench would be destroyed and recreated.

Additionally the mount points are added to the startup script and there is no
method in the frontend to re-run this startup script to mount any additional
mount points therefore the UpdateView form is only presented during the
creation process. Once information on the mount points is collected the startup
script can be generated.

### Startup script

The startup script is generated by the `copy_startup_script()` process in
`cluster_manager/workbenchinfo.py`. This process has two parts. The first part
is generated using information gathered by the frontend and passes the trusted
user's Google social account ID (taken from their `socialaccount` record, not
from `owner_id`, which holds an email address). It also passes any configured mount
points into the startup script before the second part of the startup script is
copied from `infrastructure_files/gcs_bucket/workbench/startup_script_template.sh`.

The startup script runs the following processes when the workbench instance
boots:

- Query instance metadata for list of users and filter based on the users
  social ID number to discover the correct format of their OSLogin username.
- Install nfs-common package via `apt-get`.
- Make temporary jupyterhome directory in /tmp/ and set user ownership.
- Make home directory for the user and set user ownership.
- Copy Jupyter configuration files from `/home/jupyter/.jupyter` to
  `/tmp/jupyterhome/.jupyter`.
- Create `DATA_LOSS_WARNING.txt` file with warning message.
- Configure specified mount points in the order specified on OFE.
- Add symlink to `/tmp/jupyterhome/` which will serve as working directory on
  the web interface.
  - This process of mounting and symlinking means the mountpoint will appear
    in both the jupyter notebook web interface working directory and in the
    expected location in the root filesystem.
- Append mount points to `DATA_LOSS_WARNING.txt` file.
- If `/home` was not mounted as a mount point then create a symlink to
  `/home` in `/tmp/jupyterhome`.
- Modify Jupyter config to reflect username and new working directory.
- Update `/lib/systemd/system/jupyter.service` systemd service file to reflect
  username and new working directory.
- Run `systemctl` daemon-reload and restart Jupyter service.
  - Without updating the Jupyter config and restarting the service then the
    Jupyter notebook would be running as the jupyter user. This would break
    permissions used on any mounted shared storage.
