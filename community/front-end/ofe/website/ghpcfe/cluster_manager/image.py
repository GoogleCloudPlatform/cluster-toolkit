# Copyright 2026 Google LLC
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
        Build the image:

        1. Create the image directory and its credentials file.
        2. Generate a Cluster Toolkit blueprint with two groups: builder-env
           (a VPC and the startup script) and packer-image.
        3. `gcluster create`, then `gcluster deploy`: it applies builder-env,
           passes its outputs (subnetwork, startup script) to the Packer
           group and runs the Packer build.
        4. `gcluster destroy`: removes the builder network (the image stays).

        Any failure marks the image's status as "error" (status code 'e').

        Raises:
            subprocess.CalledProcessError: If a gcluster command fails.
        """
        self._create_image_dir()
        self._create_blueprint()
        self._run_ghpc()
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

    def _get_credentials_file(self):
        return self.image_dir / "cloud_credentials"
  
    def _create_blueprint(self):
        """
        Create Cluster Toolkit blueprint that will build the image.
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
            if not runners:
                # The startup-script module rejects null runners, so render
                # an explicit empty list for a no-op builder script.
                runners = " []"

            with blueprint_file.open("w") as f:
                f.write(
                    f"""blueprint_name: {self.blueprint_name}

terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: {utils.get_terraform_state_bucket()}
    prefix: {utils.terraform_state_prefix("image", self.image.id)}

vars:
  project_id: {project_id}
  deployment_name: {self.blueprint_name}
  region: {self.image.cloud_region}
  zone: {self.image.cloud_zone}
  network_name: {"image-"+ str(self.image.id) + "-network"}
  subnetwork_name: {"image" + str(self.image.id) + "-subnetwork"}
  image_name: {"image-" + self.image.name}
  image_family: {"image-" + self.image.family}

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
    - network1
    - scripts_for_image
    settings:
      source_image_project_id: [{self.image.source_image_project}]
      source_image_family: {self.image.source_image_family}
      disk_size: 50
      image_family: $(vars.image_family)
      state_timeout: 30m
      zone: $(vars.zone)
      image_storage_locations: ["{self.image.cloud_region}"]
      metadata:
        enable-oslogin: {self.image.enable_os_login}
        block-project-ssh-keys: {self.image.block_project_ssh_keys}
"""
            )
        except Exception as e:
            self.update_image_status("e")
            logger.error(f"Error occurred while creating blueprint: {e}")
            raise

    def _run_ghpc(self):
        try:
            logger.info(f"Invoking gcluster create for the image {self.image.id}")
            utils.run_gcluster(
                self.image_dir, "create", "image.yaml", "-w",
                "--validation-level", "WARNING",
                credentials_file=self.credentials_file,
            )
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"gcluster create failed for image {self.image.id}", exc_info=cpe)
            raise

    def _create_image(self):
        """Deploy both groups: the builder network, then the Packer build."""
        try:
            logger.info(f"Invoking gcluster deploy for the image {self.image.id}")
            utils.run_gcluster(
                self.image_dir, "deploy", self.blueprint_name, "--auto-approve",
                credentials_file=self.credentials_file,
            )
            self.update_image_status("r")
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"gcluster deploy failed for image {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("gcluster output:\n%s\n", cpe.stdout.decode("utf-8"))
            raise

    def _destroy_builder_env(self):
        """Destroy the builder network (gcluster keeps Packer images)."""
        if not (self.image_dir / self.blueprint_name).is_dir():
            return
        try:
            logger.info(f"Invoking gcluster destroy for the image {self.image.id}")
            utils.run_gcluster(
                self.image_dir, "destroy", self.blueprint_name, "--auto-approve",
                credentials_file=self.credentials_file,
            )
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"gcluster destroy failed for image {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("gcluster output:\n%s\n", cpe.stdout.decode("utf-8"))
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
