# Healthcare and Life Science Blueprint

The Healthcare and Life Science (HCLS) [blueprint](./hcls-blueprint.yaml) in
this folder captures an advanced architecture that can be used to run GROMACS
with GPUs or CPUs on Google Cloud.

## Getting Started

There are several ways to get started with the HCLS blueprint.

First you will want deploy the blueprint following the
[Deployment Instructions](#deployment-instructions).

Once deployed, you can test the cluster by running an example workload:

- [Water Benchmark Example](#water-benchmark-example): All the
  inputs needed to run this example are included as part of the blueprint. This
  makes this example an easy test case to run GROMACS and confirm that the
  cluster is working as expected.
- [Lysozyme Example](./lysozyme-example/README.md): This example demonstrates a
  real life case of simulating the Lysozyme protein in water. It is a multi-step
  GPU enabled GROMACS simulation. This example was featured in
  [this YouTube Video](https://youtu.be/kJ-naSow7GQ).

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

> [!WARNING]
> This tutorial uses the following billable components of Google Cloud:
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

<!-- -->

> [!IMPORTANT]
> Before attempting to execute the following instructions, it is important to
> consider your project's quota. The `hcls-blueprint.yaml` blueprint creates an
> autoscaling cluster that, when fully scaled up, can deploy up to 20
> `a2-highgpu-1g` and `c2-standard-60` VMs.
>
> To fully scale up this cluster, the project would require quota for (at least):
>
> - GPU Node Group
>   - 12 CPUs * 20 VMs = **120 `A2 CPUs`**
>   - 1 GPU * 20 VMs = **20 `NVIDIA A100 GPUs`**
> - Compute Node Group
>   - 60 CPUs * 20 VMs = **1200 `C2 CPUs`**
> - Slurm Login VM
>   - **2 `N2 CPUs`**
> - Slurm Controller VM
>   - **4 `C2 CPUs`**
>
> Neither the Water Benchmark Example or the Lysozyme Example require the
> cluster to fully scale up. Please see:
>
> - Water Benchmark Example [Quota Requirements](#quota-requirements)
> - Lysozyme Example [Quota Requirements](./lysozyme-example/README.md#quota-requirements)

1. Clone the repo

   ```bash
   git clone https://github.com/GoogleCloudPlatform/hpc-toolkit.git
   cd hpc-toolkit
   ```

1. Build the HPC Toolkit

   ```bash
   make
   ```

1. Generate the deployment folder after replacing `<project>` with the project
   id.

   If you are running this as a test, and don't care about the files created in
   the cloud buckets being destroyed, it is recommended you run:

   ```bash
   ./ghpc create docs/videos/healthcare-and-life-sciences/hcls-blueprint.yaml -w --vars project_id=<project> --vars bucket_force_delete=true
   ```

   The `bucket_force_delete` variable makes it easier to tear down the
   deployment. If it is set to the default value of `false`, buckets with
   objects (files) will not be deleted and the `./ghpc destroy` command will
   fail partway through.

   If the data stored in the buckets should be preseverved, remove the
   `--vars bucket_force_delete=true` portion of the command or set it to `false`

1. Deploy the `enable_apis` group

   Call the following ghpc command to deploy the the hcls blueprint.

   ```bash
   ./ghpc deploy hcls-01
   ```

   This will prompt you to **display**, **apply**, **stop**, or **continue**
   without applying the `enable_apis` group. Select apply.

   This will ensure that all of the needed apis are enabled before deploying the
   cluster.

   > [!WARNING]
   > This ghpc command will run through 4 groups (`enable_apis`, `setup`,
   > `software_installation`, and `cluster`) and prompt you to apply each one.
   > If the command is cancelled or exited by accident before finishing, it can
   > be rerun to continue deploying the blueprint.

1. Deploy the `setup` group

   The next `ghpc` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `setup` group. Select 'apply'.

   This group will create a network and file systems to be used by the cluster.

   > [!NOTE]
   > At this point do not proceed with the ghpc prompt for the `cluster` group.
   > Continue with the steps below before proceeding.

   This step will create a storage bucket for depositing software. The bucket
   will have the prefix `hcls-user-provided-software` followed by a the
   deployment name (e.g. `hcls-01`) and a random suffix, for example
   `hcls-user-provided-software-hcls-01-34c8749a`.

   Here are two ways to locate the bucket name:

   1. At the end of the `setup` deployment, ghpc should output a line
      `Outputs:`. Under that there should be a line similar to
      `gcs_bucket_path_bucket-software = "gs://hcls-user-provided-software-hcls-01-84d0b51e"`,
      the bucket name is located within the quotes after `gs://`
   1. On the GCP Cloud Console, you can navigate to Cloud Storage -> Buckets and
      assuming you have not created two deployments with the same name, there
      should only be one bucket with a name like
      `hcls-user-provided-software-hcls-01-34c8749a`

   Copy this bucket name for the next step.

1. Upload VMD tarball

   VMD is visualization software used by the remote desktop. While the software is
   free the user must register before downloading it.

   To download the software, complete the registration
   [here](https://www.ks.uiuc.edu/Development/Download/download.cgi?PackageName=VMD)
   and then download the tarball. The blueprint has been tested with the
   `LINUX_64 OpenGL, CUDA, OptiX, OSPRay` version
   (`vmd-1.9.3.bin.LINUXAMD64-CUDA8-OptiX4-OSPRay111p1.opengl.tar.gz`) but should
   work with any compatible 1.9.x version.

   Next, upload the `tar.gz` file to the bucket created during the deployment of
   `setup`, its name was copied at the end of the last step. The virtual desktop
   will automatically look for this file when booting up. To do this using the
   Google Cloud UI:

   1. Navigate to the
      [Cloud Storage page](https://console.cloud.google.com/storage/browser).
   1. Click on the bucket with the name provided for `bucket_name_software`.
   1. Click on `UPLOAD FILES`.
   1. Select the `tar.gz` file for VMD.

1. Deploy the `software_installation` group.

   Once the file from the prior step has been completely uploaded, you can
   return to the ghpc command which will ask you to **display**, **apply**,
   **stop**, or **continue** without applying the `software_installation` group.
   Select 'apply'.

   This group will deploy a builder VM that will build GROMACS and save the
   compiled application on the apps Filestore.

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

   The next `ghpc` prompt will ask you to **display**, **apply**, **stop**, or
   **continue** without applying the `cluster` group. Select 'apply'.

   This deployment group contains the Slurm cluster and the Chrome remote
   desktop visualization node.

1. Set up Chrome Remote Desktop

   - Follow
     [the instructions](../../../community/modules/remote-desktop/chrome-remote-desktop/README.md#setting-up-the-remote-desktop)
     for setting up the Remote Desktop.

## Teardown Instructions

> [!NOTE]
> If you created a new project for this tutorial, the easiest way to eliminate
> billing is to delete the project.

When you would like to tear down the deployment, each stage must be destroyed,
with the exception of the `enable_apis` stage. Since the `software_installation`
and `cluster` depend on the network deployed in the `setup` stage, they must be
destroyed first. You can use the following commands to destroy the deployment.

> [!WARNING]
> If you do not destroy all three deployment groups then there may be continued
> associated costs.

```bash
./ghpc destroy hcls-01 --auto-approve
```

> [!NOTE]
> If you did not create the deployment with `bucket_force_destroy` set to true,
> you may have to clean out items added to the Cloud Storage buckets before
> terraform will be able to destroy them.  This can be done on the GCP Cloud
> Console.

## Water Benchmark Example

As part of deployment, the GROMACS water benchmark has been placed in the
`/data_input` Cloud Storage bucket. Additionally two sbatch Slurm submission
scripts have been placed in the `/apps/gromacs` directory, one uses CPUs and the
other uses GPUs.

> [!NOTE]
> Make sure that you have followed all of the
> [deployment instructions](#deployment-instructions) before running this
> example.

### Quota Requirements

The Water Benchmark Example only deploys one computational VM from the
blueprint, as such you will only need quota for either:

- GPU: 12 `A2 CPUs` and 1 `NVIDIA A100 GPUs`
- CPU: 60 `C2 CPUs`

Note that these quotas are in addition to the quota requirements for the slurm
login node (2x `N2 CPUs`) and slurm controller VM (4x `C2 CPUs`). The
`spack-builder` VM should have completed and stopped, freeing its CPU quota
usage, before the computational VMs are deployed.

<!--  -->

### Instructions

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
