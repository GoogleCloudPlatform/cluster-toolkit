# Apptainer Enabled Slurm Clusters

The [HPC Toolkit](https://cloud.google.com/hpc-toolkit/docs/overview) streamlines the definition and deployment of HPC Systems via _blueprints_ that it uses to generate and deploy [Terraform](https://www.terraform.io/) configurations. Here we provide two example blueprints that build custom [Apptainer](https://apptainer.org/) enabled images for use in the HPC system configuration created by the HPC Toolkit `ghpc` command line tool. The blueprints are:
- [slurm-apptainer.yaml](./slurm-apptainer.yaml)
- [slurm-apptainer-gpu.yaml](./slurm-apptainer-gpu.yaml)

Blueprint Prep

If you want to deploy a Slurm-based HPC System with a GPU partition use the `slurm-apptainer-gpu.yaml` blueprint, otherwise choose the `slurm-apptainer.yaml` blueprint.

Edit your chosen blueprint to set the `project_id` field appropriately. Use your preferred text editor or the `sed` command below to make the change.

```bash
sed -i s/_YOUR_GCP_PROJECT_ID_/${PROJECT_ID}/g #BLUEPRINT#
```

where _BLUEPRINT_ is your blueprint of choice.

## Deployment

Now you can create the deployment artifacts with the command

```bash
./ghpc create #BLUEPRINT#
```

If you chose the `slurm-apptainer.yaml` blueprint you should see output that looks like

```
To deploy your infrastructure please run:

./ghpc deploy hpctainer

Find instructions for cleanly destroying infrastructure and advanced manual
deployment instructions at:

hpctainer/instructions.txt
```

If you chose `slurm-apptatiner-gpu.yaml` the output will be the same with the exception of the deployment name, which will be `gputainer`.

Enter ```./ghpc deploy hpctainer```, or ```./ghpc deploy gputainer```,to deploy the HPC system.

Once the deployment is complete you can login to the system's login node with the command

```bash
gcloud compute ssh \
  $(gcloud compute instances list \
      --filter="NAME ~ login" \
      --format="value(NAME)") \
  --tunnel-through-iap
```

After you have logged into the login node check to ensure Apptainer is installed using the command

```bash
apptainer
```

You should see output that looks like

```
apptainer 
Usage:
  apptainer [global options...] <command>

Available Commands:
  build       Build an Apptainer image
  cache       Manage the local cache
  capability  Manage Linux capabilities for users and groups
  checkpoint  Manage container checkpoint state (experimental)
  completion  Generate the autocompletion script for the specified shell
  config      Manage various apptainer configuration (root user only)
  delete      Deletes requested image from the library
  exec        Run a command within a container
  inspect     Show metadata for an image
  instance    Manage containers running as services
  key         Manage OpenPGP keys
  oci         Manage OCI containers
  overlay     Manage an EXT3 writable overlay image
  plugin      Manage Apptainer plugins
  pull        Pull an image from a URI
  push        Upload image to the provided URI
  remote      Manage apptainer remote endpoints, keyservers and OCI/Docker registry credentials
  run         Run the user-defined default command within a container
  run-help    Show the user-defined help for an image
  search      Search a Container Library for images
  shell       Run a shell within a container
  sif         Manipulate Singularity Image Format (SIF) images
  sign        Add digital signature(s) to an image
  test        Run the user-defined tests within a container
  verify      Verify digital signature(s) within an image
  version     Show the version for Apptainer

Run 'apptainer --help' for more detailed usage information.
```

Now you can run Apptainer containerized workloads. For examples of containerized development environments, MPI, and GPU-based codes checkout the [examples](../examples/) directory.

## Cleanup

When your work is complete you can teardown the HPC system with

```bash
./ghpc destroy hpctainer # or gputainer
```