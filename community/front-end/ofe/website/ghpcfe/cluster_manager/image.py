# Copyright 2022 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

'''
This is a backend part of custom image creation functionality.
Frontend views will talk with functions here to perform real actions.
'''

import logging
from . import utils
import json
import subprocess
import os
from django.conf import settings
from google.api_core.exceptions import NotFound
from google.cloud import compute_v1

logger = logging.getLogger(__name__)

class ImageBackend:
    """Image configuration and management class"""

    def __init__(self, image):
        self.config = utils.load_config()
        self.ghpc_path = "/opt/gcluster/hpc-toolkit/ghpc"
        
        self.image = image
        self.image_dir = (
            self.config["baseDir"]
            / "images"
            / f"image_{self.image.id}"
        )
        self.blueprint_name = f"image_{self.image.id}"
        self.credentials_file = self.image_dir / "cloud_credentials"


    def prepare(self):
        """
        Prepare the image creation process by following these steps:

        1. Create the necessary directory structure for the image.
        2. Generate a HPC Toolkit blueprint to build the image.
        3. Run the HPC Toolkit (`ghpc`) to create the image based on the blueprint.
        4. Set up the builder environment on Google Cloud Platform (GCP) using Terraform.
        5. Create the image on GCP using Packer.
        6. Destroy the builder environment after the image creation is complete.

        This method handles the entire image creation process, from setting up the necessary
        directories and configuration files to executing HPC Toolkit and Packer to build
        and finalize the image. If any step encounters an error, it logs the issue and marks
        the image's status as "error" (status code 'e').

        Note:
        - This method assumes that the necessary tools (HPC Toolkit, Terraform, and Packer)
          are properly installed and configured on the system.
        - The credentials file required for GCP authentication is created during the image
          directory setup.

        Raises:
            OSError: If there is an error while creating the image directory or writing to
                     the credentials file.
            IOError: If there is an error while writing to the credentials file.
            subprocess.CalledProcessError: If any of the subprocess calls (ghpc, Terraform, or Packer)
                                           encounter an error during execution.
        """
        self._create_image_dir()
        self._create_blueprint()
        self._run_ghpc()
        self._create_builder_env()
        self._create_image()
        self._destroy_builder_env()

    def update_image_status(self, new_status):
        self.image.status = new_status
        self.image.save()

    def _create_image_dir(self):
        try:
            self.image_dir.mkdir(parents=True, exist_ok=True)
            creds = self.image.cloud_credential.detail
            with self.credentials_file.open("w") as fp:
                fp.write(creds)
        except OSError as e:
            self.update_image_status("e")
            logger.error(f"Error occurred while creating the image directory: {e}")
        except IOError as e:
            self.update_image_status("e")
            logger.error(f"Error occurred while writing to the credentials file: {e}")

   
    def _create_blueprint(self):
        """
        Create HPC Toolkit blueprint that will build the image.
        """
        try:
            blueprint_file = self.image_dir / "image.yaml"
            project_id = json.loads(self.image.cloud_credential.detail)["project_id"]
            scripts = self.image.startup_script.all()
            runners = ""
            for script in scripts:
                script_path = os.path.join(settings.MEDIA_ROOT, script.content.name)
                runners+=f"""        
      - type: {script.type}
        destination: {script.name}
        source: {script_path}"""

            with blueprint_file.open("w") as f:
                f.write(
                    f"""blueprint_name: {self.blueprint_name}
vars:
  project_id: {project_id}
  deployment_name: {self.blueprint_name}
  region: {self.image.cloud_region}
  zone: {self.image.cloud_zone}
  network_name: {"image-"+ str(self.image.id) + "-network"}
  subnetwork_name: {"image" + str(self.image.id) + "-subnetwork"}
  image_name: {"image-" + self.image.name}
  image_family: {"image-" + self.image.family}
  tag: ofe-created

deployment_groups:
- group: builder-env
  modules:
  - id: network1
    source: modules/network/vpc
    settings:
      network_name: $(vars.network_name)

  - id: scripts_for_image
    source: modules/scripts/startup-script
    settings:
      runners:{runners}
    outputs: [startup_script]

- group: packer-image
  modules:
  - id: custom-image
    source: modules/packer/custom-image
    kind: packer
    use:
    - scripts_for_image
    settings:
      source_image_project_id: [{self.image.source_image_project}]
      source_image_family: {self.image.source_image_family}
      disk_size: 50
      image_family: $(vars.image_family)
      state_timeout: 30m
      zone: $(vars.zone)
      subnetwork_name: $(vars.subnetwork_name)
      image_storage_locations: ["{self.image.cloud_region}"]
      metadata:
        enable-oslogin: {self.image.enable_os_login}
        block-project-ssh-keys: {self.image.block_project_ssh_keys}
"""
            )
        except Exception as e:
            self.update_image_status("e")
            logger.error(f"Error occurred while creating blueprint: {e}")

    def _run_ghpc(self):
        target_dir = self.image_dir
        try:
            logger.info(f"Invoking ghpc create for the image {self.image.id}")
            log_out_fn = target_dir / "ghpc_create_log.stdout"
            log_err_fn = target_dir / "ghpc_create_log.stderr"
            with log_out_fn.open("wb") as log_out:
                with log_err_fn.open("wb") as log_err:
                    subprocess.run(
                        [self.ghpc_path, "create", "image.yaml"],
                        cwd=target_dir,
                        stdout=log_out,
                        stderr=log_err,
                        check=True,
                    )
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"ghpc exec failed for image {self.image.id}", exc_info=cpe)
            # No logs from stdout/err - get dumped to files
            raise
      
    def _create_builder_env(self):
        """Setup builder environment on GCP."""
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self.credentials_file
        }
        try:
            logger.info("Invoking Terraform Init for builder env.")
            try:
                terraform_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/builder-env")
                packer_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/packer-image")
            except OSError as e:
                self.update_image_status("e")
                logger.error(f"Error occurred while constructing terraform_dir: {e}")
            utils.run_terraform(terraform_dir, "init")
            utils.run_terraform(terraform_dir, "validate", extra_env=extra_env)
            logger.info("Invoking Terraform Plan for builder env.")
            utils.run_terraform(terraform_dir, "plan", extra_env=extra_env)
            logger.info("Invoking Terraform Apply for builder env.")
            utils.run_terraform(terraform_dir, "apply", extra_env=extra_env)
            logger.info("Exporting startup script from builder env.")
            utils.run_terraform(terraform_dir, "output", extra_env=extra_env, 
                                arguments=[
                                    "-raw",
                                    "startup_script_scripts_for_image"],
                                    )
            utils.copy_file(f"{terraform_dir}/terraform_output_log.stdout",f"{packer_dir}/custom-image/startup_script.sh")
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"Terraform exec failed for builder env, image: {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
            raise
        
    def _create_image(self):
        """Create image on GCP."""
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self.credentials_file
        }
        try:
            logger.info("Invoking Packer Init for image.")
            try:
                packer_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/packer-image/custom-image")
            except OSError as e:
                self.update_image_status("e")
                logger.exception(f"Error occurred while constructing packer_dir: {e}")
            utils.run_packer(packer_dir, "init", arguments=["."])
            utils.run_packer(packer_dir, "validate", extra_env=extra_env,
                             arguments=["-var", "startup_script_file=startup_script.sh", "."])
            logger.info("Invoking Packer build for the image")
            utils.run_packer(packer_dir, "build", extra_env=extra_env,
                             arguments=["-var", "startup_script_file=startup_script.sh", "."])
            self.update_image_status("r")
            
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.exception(f"Packer image build failed for image: {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
            raise
        except Exception as e:
            logger.exception(f"Unhandled error happened during image {self.image.id} creation.")
        
    def _destroy_builder_env(self):
        """Destroy builder environment on GCP."""
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self.credentials_file
        }
        try:
            logger.info("Invoking Terraform Destroy for builder env.")
            try:
                terraform_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/builder-env")
            except OSError as e:
                self.update_image_status("e")
                logger.error(f"Error occurred while constructing terraform_dir: {e}")
            logger.info("Invoking Terraform Destroy for builder env.")
            utils.run_terraform(terraform_dir, "destroy", extra_env=extra_env)
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"Terraform exec failed for destroying builder env, image: {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
            raise
        
    def delete_image(self):
        project_id = json.loads(self.image.cloud_credential.detail)["project_id"]
        image_name = f"image-{self.image.name}"
        zone = self.image.cloud_zone

        # Set the GOOGLE_APPLICATION_CREDENTIALS environment variable
        os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = self.credentials_file.as_posix()
        
        # Create a client
        client = compute_v1.ImagesClient()

        try:
            # Make sure that the builder env is destroyed
            self._destroy_builder_env()

            # Delete the image
            operation = client.delete(project=project_id, image=image_name)
            operation.result()
            logger.info(f"Image '{image_name}' deleted successfully from project '{project_id}' in zone '{zone}'")

        except NotFound:
            logger.error(f"Image '{image_name}' not found in project '{project_id}' or zone '{zone}'")

        except Exception as e:
            logger.error(f"An error occurred while deleting the image {image_name}: {e}")

        finally:
            # Clear the GOOGLE_APPLICATION_CREDENTIALS environment variable
            os.environ.pop("GOOGLE_APPLICATION_CREDENTIALS", None)
