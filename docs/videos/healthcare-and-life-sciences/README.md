# Deployment Instructions

1. Clone the repo

   ```bash
   git clone https://github.com/nick-stroud/hpc-toolkit.git
   cd hpc-toolkit
   git checkout hcls_mega_blueprint
   ```

1. Build the HPC Toolkit

   ```bash
   make
   ```

1. Generate the deployment folder

   The home and apps server IPs will be populated after the setup stage and are
   needed for later stage deployments. For now we will populate with a dummy
   value to allow HPC Toolkit to run.

   ```bash
   ./ghpc create docs/videos/healthcare-and-life-sciences/hcls-blueprint.yaml -w \
       --vars project_id=<project> \
       --vars homefs_server_ip=1.1.1.1 \
       --vars appsfs_server_ip=1.1.1.1 \
       --vars bucket_name_inputs=<input_bucket_name> \
       --vars bucket_name_outputs=<output_bucket_name> \
       --vars bucket_name_software=<software_bucket_name>
   ```

1. Run terraform commands for `enable_apis` group

   Call the provided `terraform init` and `terraform apply` commands for the
   `enable_apis` group. This will ensure that all of the needed apis are
   enabled before deploying the cluster.

1. Deploy the `setup` group

   This group will create a network and file systems to be used by the cluster.

   Call the provided `terraform init` and `terraform apply` commands for the
   `setup` group.

   > **Note**: You may have to tinker with bucket names to find an unused
   > namespace. If you get errors that the bucket already exists update the
   > bucket names in the `ghpc create` command and repeat the sequence.
   > (`ghpc create ...`, `terraform init`, `terraform apply`).

1. Upload VMD tarball

   VMD is visualization software used by the remote desktop. While the software is
   free the user must perform a registration before downloading it.

   To download the software, complete the registration
   [here](https://www.ks.uiuc.edu/Development/Download/download.cgi?PackageName=VMD)
   and then download the tarball. The blueprint has been tested with the
   `LINUX_64 OpenGL, CUDA, OptiX, OSPRay` version
   (`vmd-1.9.3.bin.LINUXAMD64-CUDA8-OptiX4-OSPRay111p1.opengl.tar.gz`) but should
   work with any compatible 1.9.x version.

   Put the `tar.gz` file in the bucket with the name defined in
   `bucket_name_software`. The virtual desktop will automatically look for this
   file when booting up.

1. Re-generate the deployment folder with updated IP addresses

   Now that the file stores have been deployed we need to populate their ip
   addresses so subsequent deployment groups can use them. Look up the IP
   addresses for the apps and home file system on the filestore page in the
   Google Cloud UI and populate them in the command below.

   ```bash
   ./ghpc create docs/videos/build-your-own-blueprint/hcls-blueprint.yaml -w \
       --vars project_id=<project_id> \
       --vars homefs_server_ip=<home_ip> \
       --vars appsfs_server_ip=<apps_ip> \
       --vars bucket_name_inputs=<input_bucket_name> \
       --vars bucket_name_outputs=<output_bucket_name> \
       --vars bucket_name_software=<software_bucket_name>
   ```

1. Deploy the `software_installation` group.

   This deployment group will deploy a builder VM that will build gromacs and
   save the compiled application on the apps filestore.

   Call the provided `terraform init` and `terraform apply` commands for the
   `software_installation` group.

   This may take several hours to run. You will know that installation has
   completed successfully when the builder VM has shut itself down.

1. Deploy the `cluster` group

   This deployment group contains the Slurm cluster and the Chrome remote
   desktop visualization node.

   Call the provided `terraform init` and `terraform apply` commands for the
   `cluster` group.

1. Set up Chrome Remote Desktop

   - go to https://remotedesktop.google.com/headless and set up crd
   - open terminal and run `vmd` on the command line
