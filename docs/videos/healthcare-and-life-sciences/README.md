# Deployment Instructions

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
   > bucket names in the `ghpc create` command and repeat the sequence.
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

   Put the `tar.gz` file in the bucket with the name defined in
   `bucket_name_software`. The virtual desktop will automatically look for this
   file when booting up.

1. Re-generate the deployment folder with updated IP addresses

   Now that the file stores have been deployed we need to populate their IP
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
       --vars homefs_server_ip=<home_ip> \
       --vars appsfs_server_ip=<apps_ip> \
       --vars bucket_name_inputs=<input_bucket_name> \
       --vars bucket_name_outputs=<output_bucket_name> \
       --vars bucket_name_software=<software_bucket_name>
   ```

1. Deploy the `software_installation` group.

   This group will deploy a builder VM that will build gromacs and save the
   compiled application on the apps filestore.

   Call the following terraform commands to deploy the `software_installation`
   deployment group.

   ```bash
   terraform -chdir=hcls-01/software_installation init && \
     terraform -chdir=hcls-01/software_installation apply
   ```

   This may take **several hours** to run. After the software installation is
   complete the builder VM will automatically shut itself down. This allows you
   to monitor the status of the builder VM to know when installation has
   finished.

   You can check the serial port 1 logs and the Spack logs
   (`/var/log/spack.log`) to check status. If the builder VM never shuts down it
   may be a sign that something went wrong with the software installation.

   This builder VM can be deleted once the software installation has completed
   successfully.

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
   - Connect to the remote desktop, open a terminal in the remote session and
     run `vmd` on the command line
