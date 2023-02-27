# Healthcare and Life Science Blueprint

The Healthcare and Life Science (HCLS) [blueprint](./hcls-blueprint.yaml) in
this folder captures an advanced architecture that can be used to run GROMACS
with GPUs on Google Cloud.

## Getting Started

There are several ways to get started with the HCLS blueprint.

First you will want deploy the blueprint following the
[Deployment Instructions](#deployment-instructions).

Once deployed, you can test the cluster by running an example workload:

- [Water Benchmark Example](#water-benchmark-example-instructions): All the
  inputs needed to run this example are included as part of the blueprint. This
  makes this example an easy test case to run GROMACS and confirm that the
  cluster is working as expected.
- [Lysozyme Example](./lysozyme-example/README.md): This example demonstrates a
  real life case of simulating the Lysozyme protein in water. It is a multi-step
  GPU enabled GROMACS simulation.

## Architecture

The blueprint includes:

- Auto-scaling Slurm cluster
- Filestore for shared NFS storage
- Input and output Google Cloud Storage bucket
- GPU accelerated remote desktop for visualization
- Software builder VM to compile molecular dynamics software

![blueprint-architecture](https://user-images.githubusercontent.com/22925983/221241948-598d88b7-51e7-4f28-94dd-bd930ccce2f8.png)

### Deployment Stages

This blueprint has 4 deployment groups:

- `enable_apis`: Ensure that all of the needed apis are enabled before deploying
  the cluster.
- `setup`: Setup backbone infrastructure such as networking, file systems, &
  monitoring.
- `software_installation`: Compile and install HPC applications and populate the
  input library.
- `cluster`: Deploys an auto-scaling cluster and remote desktop.

Having multiple deployment groups decouples the life cycle of some
infrastructure. For example a) you can tear down the cluster while leaving the
storage intact and b) you can build software before you deploy your cluster.

## Deployment Instructions

> **Warning**: This tutorial uses the following billable components of Google
> Cloud:
>
> - Compute Engine
> - Filestore
> - Cloud Storage
>
> To avoid continued billing once the tutorial is complete, closely follow the
> [teardown instructions](#teardown-instructions). Additionally, you may want to
> deploy this tutorial into a new project that can be deleted when the tutorial
> is complete. To generate a cost estimate based on your projected usage, use
> the [pricing calculator](https://cloud.google.com/products/calculator).

1. Clone the repo

   ```bash
   git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git
   cd hpc-toolkit
   ```

1. Build the HPC Toolkit

   ```bash
   make
   ```

1. Generate the deployment folder

   Starting with the command below, populate the `project_id` and the three
   bucket name variables with unique bucket names for your deployment. The three
   buckets will be created as part of the deployment and used as follows:

   - **bucket_name_inputs:** name for GCS bucket that will be used as a library
     of pre-loaded static inputs (protein library, benchmarks, examples).
   - **bucket_name_outputs:** name for GCS bucket that will be used to retain,
     share, and utilize results in downstream analysis.
   - **bucket_name_software:** name for GCS bucket where users can upload
     software to be utilized by the cluster. See *Upload VMD tarball* step
     below.

   The `homefs_server_ip` and `appsfs_server_ip` will be populated after the
   setup stage is complete. For now we will populate with a dummy value to allow
   HPC Toolkit to run.

   ```bash
   ./ghpc create docs/videos/healthcare-and-life-sciences/hcls-blueprint.yaml -w \
       --vars project_id=<project> \
       --vars bucket_name_inputs=<input_bucket_name> \
       --vars bucket_name_outputs=<output_bucket_name> \
       --vars bucket_name_software=<software_bucket_name> \
       --vars homefs_server_ip=1.1.1.1 \
       --vars appsfs_server_ip=1.1.1.1
   ```

1. Deploy the `enable_apis` group

   Call the following terraform commands to deploy the `enable_apis` deployment
   group.

   ```bash
   terraform -chdir=hcls-01/enable_apis init && terraform -chdir=hcls-01/enable_apis apply
   ```

   This will ensure that all of the needed apis are enabled before deploying the
   cluster.

1. Deploy the `setup` group

   This group will create a network and file systems to be used by the cluster.

   Call the following terraform commands to deploy the `setup` deployment
   group.

   ```bash
   terraform -chdir=hcls-01/setup init && terraform -chdir=hcls-01/setup apply
   ```

   > **Note**: You may have to tinker with bucket names to find an unused
   > namespace. If you get errors that the bucket already exists update the
   > bucket names in the `ghpc create` command above and repeat the sequence.
   > (`ghpc create ...`, `terraform init`, `terraform apply`).

1. Upload VMD tarball

   VMD is visualization software used by the remote desktop. While the software is
   free the user must register before downloading it.

   To download the software, complete the registration
   [here](https://www.ks.uiuc.edu/Development/Download/download.cgi?PackageName=VMD)
   and then download the tarball. The blueprint has been tested with the
   `LINUX_64 OpenGL, CUDA, OptiX, OSPRay` version
   (`vmd-1.9.3.bin.LINUXAMD64-CUDA8-OptiX4-OSPRay111p1.opengl.tar.gz`) but should
   work with any compatible 1.9.x version.

   Upload the `tar.gz` file in the bucket with the name defined in
   `bucket_name_software`. The virtual desktop will automatically look for this
   file when booting up. To do this using the Google Cloud UI:
   1. navigate to the
      [Cloud Storage page](https://console.cloud.google.com/storage/browser).
   1. click on the bucket with the name provided for `bucket_name_software`.
   1. click on `UPLOAD FILES`.
   1. select the `tar.gz` file for VMD.

1. Re-generate the deployment folder with updated Filestore IP addresses

   Now that the Filestores have been deployed, we need to populate their IP
   addresses so subsequent deployment groups can use them. Look up the IP
   addresses for the `appsshare` and `homeshare` file system and populate them
   in the command below.

   > **Note**: You can use the following command to list information about
   > filestores, including their IP address.

   ```bash
   gcloud filestore instances list --project <project_id>
   ```

   ```bash
   ./ghpc create docs/videos/healthcare-and-life-sciences/hcls-blueprint.yaml -w \
       --vars project_id=<project_id> \
       --vars bucket_name_inputs=<input_bucket_name> \
       --vars bucket_name_outputs=<output_bucket_name> \
       --vars bucket_name_software=<software_bucket_name> \
       --vars homefs_server_ip=<home_ip> \
       --vars appsfs_server_ip=<apps_ip>
   ```

1. Deploy the `software_installation` group.

   This group will deploy a builder VM that will build GROMACS and save the
   compiled application on the apps Filestore.

   Call the following terraform commands to deploy the `software_installation`
   deployment group.

   ```bash
   terraform -chdir=hcls-01/software_installation init && \
     terraform -chdir=hcls-01/software_installation apply
   ```

   This will take **several hours** to run. After the software installation is
   complete the builder VM will automatically shut itself down. This allows you
   to monitor the status of the builder VM to know when installation has
   finished.

   You can check the serial port 1 logs and the Spack logs
   (`/var/log/spack.log`) to check status. If the builder VM never shuts down it
   may be a sign that something went wrong with the software installation.

   This builder VM can be shut down or deleted once the software installation
   has completed successfully.

1. Deploy the `cluster` group

   This deployment group contains the Slurm cluster and the Chrome remote
   desktop visualization node.

   Call the following terraform commands to deploy the `cluster` deployment
   group.

   ```bash
   terraform -chdir=hcls-01/cluster init && terraform -chdir=hcls-01/cluster apply
   ```

1. Set up Chrome Remote Desktop

   - Follow
     [the instructions](https://github.com/GoogleCloudPlatform/hpc-toolkit/blob/develop/community/modules/remote-desktop/chrome-remote-desktop/README.md#setting-up-the-remote-desktop)
     for setting up the Remote Desktop.

## Teardown Instructions

> **Note**: If you created a new project for this tutorial, the easiest way to
> eliminate billing is to delete the project.

When you would like to tear down the deployment, each stage must be destroyed,
with the exception of the `enable_apis` stage. Since the `software_installation`
and `cluster` depend on the network deployed in the `setup` stage, they must be
destroyed first. You can use the following commands to destroy the deployment.

> **Warning**: If you do not destroy all three deployment groups then there may
> be continued associated costs.

```bash
# cluster
terraform -chdir=hcls-01/cluster init && \
   terraform -chdir=hcls-01/cluster destroy --auto-approve
# software_installation
terraform -chdir=hcls-01/software_installation init && \
   terraform -chdir=hcls-01/software_installation destroy --auto-approve
# setup
terraform -chdir=hcls-01/setup init && \
   terraform -chdir=hcls-01/setup destroy --auto-approve
```

> **Note**: You may have to clean out items added to the Cloud Storage buckets
> before terraform will be able to destroy them.

## Water Benchmark Example Instructions

As part of deployment, the GROMACS water benchmark has been placed in the
`/data_input` Cloud Storage bucket. Additionally two sbatch Slurm submission
scripts have been placed in the `/apps/gromacs` directory, one uses CPUs and the
other uses GPUs.

> **Note**: Make sure that you have followed all of the
> [deployment instructions](#deployment-instructions) before running this
> example.

<!--  -->

> **Note**: To run this example you will need quota for `A2 CPUs` (12) and
> `NVIDIA A100 GPUs` (1).

1. SSH into the Slurm login node

   Go to the
   [VM instances page](https://console.cloud.google.com/compute/instances) and
   you should see a VM with `login` in the name. SSH into this VM by clicking
   the `SSH` button or by any other means.

1. Create a submission directory

   ```bash
   mkdir water_run && cd water_run
   ```

1. Submit the GROMACS job

   There are two example sbatch scripts which have been populated at:

   - `/apps/gromacs/submit_gromacs_water_cpu.sh`
   - `/apps/gromacs/submit_gromacs_water_gpu.sh`

   The first of these runs on the `compute` partition, which uses CPUs on a
   `c2-standard-60` machine. The second targets the `gpu` partition. It runs on
   an `a2-highgpu-1g` machine and uses a NVIDIA A100 for GPU acceleration.

   The example below runs the GPU version of the job. You can switch out the
   path of the script to try the CPU version.

   Submit the sbatch script with the following commands:

   ```bash
   sbatch /apps/gromacs/submit_gromacs_water_gpu.sh
   ```

1. Monitor the job

   Use the following command to see the status of the job:

   ```bash
   squeue
   ```

   The job state (`ST`) will show `CF` while the job is being configured. Once
   the state switches to `R` the job is running.

   If you refresh the
   [VM instances page](https://console.cloud.google.com/compute/instances) you
   will see an `a2-highgpu-1g` machine that has been auto-scaled up to run this
   job. It will have a name like `hcls01-gpu-ghpc-0`.

   Once the job is in the running state you can track progress with the
   following command:

   ```bash
   tail -f slurm-*.out
   ```

   When the job has finished end of the `slurm-*.out` file will print
   performance metrics such as `ns/day`.
