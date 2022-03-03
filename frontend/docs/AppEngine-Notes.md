# Moving to AppEngine

## Benefits
* Increased reliance on GCP products
* Working towards a "Deployment" model
* TLS provided
* DNS provided - Useful for Google Login


## Useful Links:
* [Django in AppEngine Flexible Environment](https://cloud.google.com/python/django/flexible-environment)
* [App Engine Custom Runtime](https://cloud.google.com/appengine/docs/flexible/custom-runtimes)
* [Docker for Custom runtimes](https://cloud.google.com/appengine/docs/flexible/custom-runtimes/build)
* [Django and Docker](https://docs.docker.com/compose/django/)
* [Google Secrets Manager](https://cloud.google.com/secret-manager/docs/how-to)

## Challenges

1. Database
   * Simply migrate to CloudSQL - can use a local CloudSQL Proxy to access same DB from development machines.
1. Invoking Terraform binaries
   * Can use AppEngine "Flexible Environment" With the "Custom Runtime" Docker container, which can contain Terraform dependencies
1. Storage of TF state / vars files
   * Consider storing in DB, and syncing with scratch space (`/tmp/`) as needed
   * May need to consider locking via DB to prevent multiple simultaneous  TF actions on the same cluster
   * Consider storing in Cloud Storage rather than DB
     * TF State is ~ 50KB
     * Would enable storage of cluster log files
1. SSH key management
   * Currently create an SSH key per cluster to allow "citc" access.
   * Can have just 1 SSH key for the deployment - stored in Google Secrets Manager
   * Deployed app would download the SSH private key and use it for connecting to clusters.
     * Clusters would all need to be created using this same key
1. AppEngine scaling / weekly restarts
   * Should be OK (aka, Django piece stateless), provided:
     * Use CloudSQL
     * TF State/Vars stored in DB
     * SSH keys stored


## Changes Required:
### Additions
* Docker file
* Deployment / installation scripting
  * Include creating SSH key secret

### Django code

Probably very little.

### Django DB model

May need to incorporate some state currently stored on disk (Terraform state / vars)

### Backend Scripts

#### Cluster Creation/Destruction
* Need to sync tfvars and tfstate with stable storage
  * Stage from stable storage, run   in scratch space, sync to stable storage
* Sync logs to stable storage?
* Re-use project-wide SSH key
* Need to install project scripts to cluster (Not stored open-source.)

#### Install/Run scripts
* Rather than drive from management node, install scripts on cluster
  * Management node will simply `ssh citc@<headnode> run_job <jobid>`
  * `run_job` will then query the DB via API (back to management) to get required info, build SLURM submit script, etc.. Invoke job as user
* Some scripts need to be able to be run as unprivledged users (ie, update DB at end of job run)

