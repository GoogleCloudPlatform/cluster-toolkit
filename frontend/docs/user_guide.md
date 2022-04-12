## HPC Toolkit FrontEnd - User Guide

This document is for ordinary users of the HPC Toolkit FrontEnd. Ordinary user can access HPC clusters and installed applications as set up by the administrators. They can prepare, submit and run jobs on the cluster through the convenience of the web interface. Administrators should refer to the [Administrator's Guide](admin_guide.md) for guidance on how to provision and manage cloud resources for HPC clusters.

### Access to the system

An administrator should have arranged access to the system for an ordinary user:

- A URL should be provided on which an instance of the HPC Toolkit FrontEnd is deployed.
- The Google identity of the user should be whitelisted to access the instane.
- The user should be set as authorised users on existing HPC clusters.
- Administrators can optionally set up quotas to restrict the amount of resources that can be consumed by individual users.

Discuss requirements with the admin user in your organisation: 

- Applications may require certain instance types to run efficiently on GCP. Administrators may be able to create new Slurm partitions to support new instance types.
- Some applications may requite additional configurations at install time to switch on features. Administrators may be able to customised or provide additiona versions/variants. 

On first visit, click the *Login* link on the home page, then click the *Login with Google* link. The system will then attempt the authenticate the user through OAUTH with Google.

### Clusters

Shown on the cluster page is a list of active clusters the current user is authorised to use. Clusters are created and managed by admin users so messages on those pages are for information only. The underlying infrastructures, such as network and storage components, are also managed by admin users so those pages are not accessible by ordinary users.

### Applications

Shown on the application page is a list of pre-installed applications on those clusters. Applications are set up by admin users so messages on those pages are for information only. Application detail pages provide extra information regarding the pacakges. 

There are three types of applications: 

- those installed by the Spack package manager
- those installed from custom scripts as prepared by the admin users
- those manually installed on the clusters by admin users and then registered with this system

In practice, end users do not need to distinguish these different types.

### Jobs

From an application page, click the *New Job* action to set up a job. 

In most cases, users do not need to concern about the application's software environment as the system handles that automatically. For example, if an application has been set up as a Spack package, the system will automatically invoke `spack load` to configure its environment, e.g. putting the application binaries in `$PATH`, before executing the user job.

On the other hand, users need to provide the exact steps setting up jobs through scripts. A run script can either be located at a URL or provides inline in the new job form. A run script may provide additional steps that download or prepare input files before invoking the application binary. It may also perform post-processing tasks as required.

#### Job input and output

There are several different ways to prepare job input and process job output files

##### Using cloud storage

When submitting a new job, the end user may optionally specify:

- an URL from which input files are downloaded - http:// or https:// URL for an external storage, or a gs:// URL a Google cloud storage
- an gs:// URL to which output files are uploaded

The system supports using Google Cloud Storage (GCS) buckets as external storage. Here the GCS bucket is a personal one belonging to the end user (for admin users this is not to be confused with the GCS bucket that supports the deployment of this system). A one-time set-up for the GCS bucket is required per cluster: from the *Actions* menu of a cluster, click *Authenticate to Google Cloud Storage* and then follow Google's instructions.

##### Building logic directly in run script

Users can prepare the input dataset directly within the run script by using arbitrary scripting.

##### Preparing data manually

It is also possible to prepare job data manually on the cluster. Users can always SSH into cluster login node and run arbitrary commands to prepare data in their home directories. Then job run scripts can access these files as needed.

Manually preparing data is probably the most tedious. However, it can be most cost-effective if a large amount of data is to be transmitted over the Internet, or the data is to be shared by multiple job runs. Moving large datasets from inside the run script can add significant cloud spending as all compute nodes reserved for jobs are being charged.

### Benchmarks

A benchmark is effectively a collection of jobs using the same application on the same dataset. The application version and input dataset should always be the same; the compiler/libraries to build the application and the cloud instance types to run the application can differ.

New benchmarks can be created by an admin user from the Benchmarks section of the website.

For ordinary users, when running a job, there is an option to associated that job to an existing benchmark, as show in the following figure.

![Associate a job with a benchmark](images/benchmark.png)

If a job script contains logic to produce a key performance indicator (KPI), as should be the case for any benchmark run, it will be passed back to the service machine and stored in the database. For this to work, the job script should extract appropriate information from the job output, and use suitable scripting to
create a file called kpi.json and place it in the current working directory. The file content will be sent back to the service machine through an API call. The JSON file should be in the following format:

```
{
  "result_unit": "some unit",
  "result_value": "some value"
}
```

Once a few benchmark runs are registered and their KPIs recorded, it is obvious that further analysis can be performed.
### Vortex AI workbenches
