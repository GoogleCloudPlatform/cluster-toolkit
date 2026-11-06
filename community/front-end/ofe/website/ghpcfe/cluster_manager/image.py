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
from . import cmek
from . import utils
import json
import secrets
import subprocess
import os
from django.conf import settings
from google.api_core.exceptions import NotFound
from google.auth.exceptions import DefaultCredentialsError
from google.cloud import compute_v1

logger = logging.getLogger(__name__)

# The image's key and its IAM grant live here: a group of their own, so the
# builder env can be destroyed after a successful build without taking the
# key with it.
CMEK_GROUP = "image-cmek"


class ImageBackend:
    """Image configuration and management class"""

    def __init__(self, image):
        self.config = utils.load_config()
        self.ghpc_path = "/opt/gcluster/cluster-toolkit/ghpc"
        
        self.image = image
        self.image_dir = (
            self.config["baseDir"]
            / "images"
            / f"image_{self.image.id}"
        )
        self.blueprint_name = f"image_{self.image.id}"
        self.credentials_file = self.image_dir / "cloud_credentials"
        # set by _create_blueprint; the key group only exists when encrypted
        self._cmek_in_use = False


    def prepare(self):
        """
        Prepare the image creation process by following these steps:

        1. Create the necessary directory structure for the image.
        2. Generate a Cluster Toolkit blueprint to build the image.
        3. Run the Cluster Toolkit (`gcluster`) to create the image based on the blueprint.
        4. Set up the builder environment on Google Cloud Platform (GCP) using Terraform.
        5. Create the image on GCP using Packer.
        6. Destroy the builder environment after the image creation is complete.
           The key group is deliberately left standing: the image stays
           encrypted with that key, so it is torn down with the image.

        This method handles the entire image creation process, from setting up the necessary
        directories and configuration files to executing Cluster Toolkit and Packer to build
        and finalize the image. If any step encounters an error, it logs the issue and marks
        the image's status as "error" (status code 'e').

        Note:
        - This method assumes that the necessary tools (Cluster Toolkit, Terraform, and Packer)
          are properly installed and configured on the system.
        - The credentials file required for GCP authentication is created during the image
          directory setup.
        - An image whose key was revoked is refused, and a failure to create the
          directory or the blueprint stops the build there.

        Raises:
            OSError: If there is an error while creating the image directory or writing to
                     the credentials file.
            IOError: If there is an error while writing to the credentials file.
            subprocess.CalledProcessError: If any of the subprocess calls (gcluster, Terraform, or Packer)
                                           encounter an error during execution.
            cmek.CmekConfigError: If the image's key was revoked.
        """
        try:
            cmek.assert_not_revoked(self.image)
        except cmek.CmekConfigError as err:
            # Refused before anything was done: the build's claim on the
            # image is released, so it is back to newly configured, and the
            # message says why.
            self.image.status = "n"
            self.image.status_message = str(err)
            self.image.save(update_fields=["status", "status_message"])
            raise
        except Exception as err:
            self._fail(f"The image build failed: {err}")
            raise
        try:
            # clear any stale failure reason on a fresh build
            self.image.status_message = ""
            self.image.save(update_fields=["status_message"])
            self._create_image_dir()
            self._create_blueprint()
            self._run_ghpc()
            self._create_cmek_env()
            self._create_builder_env()
            self._create_image()
            self._destroy_builder_env()
        except Exception as err:
            # Whichever step failed, the image must not stay "being
            # created". A reason the step already gave is kept.
            self._fail(self.image.status_message
                       or f"The image build failed: {err}")
            raise

    def update_image_status(self, new_status):
        self.image.status = new_status
        cmek.keep_flags_current(self.image)
        self.image.save()

    def _fail(self, message):
        """Mark the build failed and say why on the image's page."""
        self.image.status = "e"
        self.image.status_message = message
        self.image.save(update_fields=["status", "status_message"])

    def _create_image_dir(self):
        try:
            self.image_dir.mkdir(parents=True, exist_ok=True)
            creds = self.image.cloud_credential.detail
            with self.credentials_file.open("w") as fp:
                fp.write(creds)
        except OSError as e:
            # IOError is OSError, so this covers writing the credentials too.
            # Re-raised: nothing after this step can work without them.
            self._fail(f"Could not create the image directory: {e}")
            logger.error(f"Error occurred while creating the image directory: {e}")
            raise

    def _get_credentials_file(self):
        return self.image_dir / "cloud_credentials"
  
    def _create_blueprint(self):
        """
        Create Cluster Toolkit blueprint that will build the image.
        """
        try:
            blueprint_file = self.image_dir / "image.yaml"
            # Removed first, so a failure below cannot leave a previous
            # attempt's blueprint for anything to build from.
            blueprint_file.unlink(missing_ok=True)
            project_id = json.loads(self.image.cloud_credential.detail)["project_id"]

            # kms-key / kms-key-iam for this image, then `use: [cmek_key_iam]` on
            # the packer module: the IAM module exports the key id as
            # image_encryption_key, which is custom-image's own input.
            #
            # They live in a deployment group of their own, not in builder-env.
            # builder-env is destroyed as the last step of a successful build,
            # which would take the key's grant -- and under DELETE the key
            # itself -- with it, leaving an image Compute Engine can no longer
            # decrypt when it boots a node from it. The key belongs to the
            # image, so it is torn down with the image instead.
            cmek_settings = cmek.CmekSettings.from_config(self.config)
            chosen = cmek.resource_settings(self.image, cmek_settings)
            key_modules = ""
            key_use = ""
            self._cmek_in_use = bool(chosen)
            # Store the effective choice, not the box as typed: enforcement
            # may have overridden it, and once built the stored value is what
            # stands (is_frozen), and what the keys page lists.
            if not chosen and self.image.cmek_enabled:
                self.image.cmek_enabled = False
                self.image.save(update_fields=["cmek_enabled"])
            if chosen:
                if not chosen["key_name"]:
                    # Cloud KMS never frees a key name, so an image recreated
                    # under the same name must not derive the same key name.
                    # The suffix makes it unique; storing it below makes
                    # retries reuse it.
                    chosen["key_name"] = cmek.derive_key_name(
                        "image", f"{self.image.name}-{secrets.token_hex(4)}")
                # Record what was deployed, so the detail page reports this key
                # rather than recomputing it from settings that may since have
                # changed. The derived name is not otherwise stored anywhere.
                deployed = cmek.resolve_created_key(
                    chosen, cmek_settings, project_id)
                self.image.cmek_enabled = True
                self.image.cmek_key_name = deployed["key_name"]
                self.image.cmek_key_ring_name = deployed["key_ring_name"]
                self.image.cmek_key_project_id = deployed["key_project_id"]
                self.image.save(update_fields=[
                    "cmek_enabled", "cmek_key_name", "cmek_key_ring_name",
                    "cmek_key_project_id"])
                ring_exists = cmek.ring_already_exists(
                    self.image.cloud_credential.detail, deployed,
                    self.image.cloud_region)
                key_modules = cmek.key_modules_yaml(
                    service="image",
                    project_id=project_id,
                    location=self.image.cloud_region,
                    settings=cmek_settings,
                    ring_exists=ring_exists,
                    # the packer module is in another deployment group, so the
                    # output it needs must be promoted to this group's outputs
                    iam_outputs=["image_encryption_key"],
                    **chosen,
                ) + "\n"
                key_use = f"\n    - {cmek.KEY_IAM_MODULE_ID}"

            cmek_group = ""
            if key_modules:
                cmek_group = f"- group: {CMEK_GROUP}\n  modules:\n{key_modules}"

            scripts = self.image.startup_script.all()
            runners = ""
            for script in scripts:
                script_path = os.path.join(settings.MEDIA_ROOT, script.content.name)
                runners+=f"""
      - type: {script.type}
        destination: {script.name}
        source: {script_path}"""
            if not runners:
                # No startup scripts selected: the startup-script module
                # rejects null runners, so render an explicit
                # empty list for a no-op builder script.
                runners = " []"

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

deployment_groups:
{cmek_group}- group: builder-env
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
    - scripts_for_image{key_use}
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
            self._cmek_in_use = False
            self._fail(f"Could not generate the image blueprint: {e}")
            logger.error(f"Error occurred while creating blueprint: {e}")
            # Re-raised so the build stops: nothing after this step has a
            # blueprint to work from.
            raise

    def _run_ghpc(self):
        target_dir = self.image_dir
        try:
            logger.info(f"Invoking gcluster create for the image {self.image.id}")
            log_out_fn = target_dir / "ghpc_create_log.stdout"
            log_err_fn = target_dir / "ghpc_create_log.stderr"

            env = os.environ.copy()
            env['GOOGLE_APPLICATION_CREDENTIALS'] = self._get_credentials_file()

            with log_out_fn.open("wb") as log_out:
                with log_err_fn.open("wb") as log_err:
                    subprocess.run(
                        [self.ghpc_path, "create", "image.yaml", "-w",
                         "--validation-level", "WARNING"],
                        cwd=target_dir,
                        stdout=log_out,
                        stderr=log_err,
                        check=True,
                        env=env,
                    )
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(f"gcluster exec failed for image {self.image.id}", exc_info=cpe)
            # No logs from stdout/err - get dumped to files
            raise
      
    def _run_ghpc_import_inputs(self):
        """Bridge builder-env outputs into the packer group.

        Mirrors the deployment instructions ghpc generates for
        cross-group `use`: export-outputs from each applied group the packer
        group draws on (the builder env, and the key group when encrypted),
        then import-inputs into the packer group.
        """
        target_dir = self.image_dir
        log_out_fn = target_dir / "ghpc_import_log.stdout"
        log_err_fn = target_dir / "ghpc_import_log.stderr"
        env = os.environ.copy()
        env["GOOGLE_APPLICATION_CREDENTIALS"] = self._get_credentials_file()
        try:
            with log_out_fn.open("ab") as log_out:
                with log_err_fn.open("ab") as log_err:
                    # The packer group takes inputs from every group it `use`s,
                    # so each of those groups' outputs must be exported first.
                    # The key group was applied with plain terraform, which
                    # writes no export artifact, so it is exported here too.
                    groups = [f"{self.blueprint_name}/builder-env"]
                    if self._cmek_in_use:
                        groups.append(f"{self.blueprint_name}/{CMEK_GROUP}")
                    for group in groups:
                        subprocess.run(
                            [self.ghpc_path, "export-outputs", group],
                            cwd=target_dir,
                            stdout=log_out,
                            stderr=log_err,
                            check=True,
                            env=env,
                        )
                    subprocess.run(
                        [self.ghpc_path, "import-inputs",
                         f"{self.blueprint_name}/packer-image"],
                        cwd=target_dir,
                        stdout=log_out,
                        stderr=log_err,
                        check=True,
                        env=env,
                    )
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(
                f"gcluster export/import bridge failed for image "
                f"{self.image.id}", exc_info=cpe)
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
            # The packer module consumes the builder VPC via `use`
            # (ghpc create hard-errors on unused modules), so
            # its subnetwork input arrives through the toolkit's own
            # cross-group bridge. startup_script also flows through it,
            # but the explicit startup_script_file -var below still
            # overrides it, keeping the historical behavior.
            logger.info("Importing packer group inputs from builder env.")
            self._run_ghpc_import_inputs()
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
            # The build succeeded, so the encryption choice is now in the
            # image: it stays fixed from here on. Stored at once, on its own,
            # so nothing after this that fails can lose it.
            self.image.cmek_applied = True
            self.image.save(update_fields=["cmek_applied"])
            self.update_image_status("r")

        # Not re-raised, so prepare() still destroys the builder env: the
        # failure is recorded on the image instead of leaving it "creating".
        except subprocess.CalledProcessError as cpe:
            self._fail(f"The Packer build failed: {cpe}")
            logger.exception(f"Packer image build failed for image: {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
        except Exception as e:  # pylint: disable=broad-except
            # utils.run_packer raises RuntimeError for any Packer failure.
            self._fail(f"The Packer build failed: {e}")
            logger.exception(f"Unhandled error happened during image {self.image.id} creation.")
        
    def _cmek_terraform_dir(self):
        return os.path.join(self.image_dir, f"{self.blueprint_name}/{CMEK_GROUP}")

    def _create_cmek_env(self):
        """Create this image's key and its IAM grant.

        Applied before the builder env, so the key and the grant Compute Engine
        needs to encrypt the image exist before packer runs.
        """
        if not self._cmek_in_use:
            return
        extra_env = {"GOOGLE_APPLICATION_CREDENTIALS": self.credentials_file}
        terraform_dir = self._cmek_terraform_dir()
        try:
            logger.info("Invoking Terraform Apply for the image key.")
            utils.run_terraform(terraform_dir, "init")
            utils.run_terraform(terraform_dir, "validate", extra_env=extra_env)
            utils.run_terraform(terraform_dir, "apply", extra_env=extra_env)
        except subprocess.CalledProcessError as cpe:
            self.update_image_status("e")
            logger.error(
                f"Terraform exec failed for the image key, image: {self.image.id}",
                exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
            raise

    def _destroy_cmek_env(self):
        """Tear down this image's key group.

        Only ever called when the image itself is being deleted: the image is
        encrypted with this key, so destroying the grant (or, under DELETE, the
        key) any earlier would leave an image nothing can boot from.
        """
        # also covers a build that never created the key group at all
        if not utils.terraform_may_have_applied(self._cmek_terraform_dir()):
            return
        extra_env = {"GOOGLE_APPLICATION_CREDENTIALS": self.credentials_file}
        try:
            logger.info("Invoking Terraform Destroy for the image key.")
            utils.run_terraform(
                self._cmek_terraform_dir(), "destroy", extra_env=extra_env)
        except subprocess.CalledProcessError as cpe:
            logger.error(
                f"Terraform exec failed destroying the image key, image: "
                f"{self.image.id}", exc_info=cpe)
            raise

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
        """Delete the image, its builder env and its key group.

        Returns True only when the builder env and the key group were both
        torn down. The caller keeps the image's record otherwise: it is what
        still points at a key that would be left behind, and lets the delete
        be retried.
        """
        project_id = json.loads(self.image.cloud_credential.detail)["project_id"]
        image_name = f"image-{self.image.name}"
        zone = self.image.cloud_zone

        # Set the GOOGLE_APPLICATION_CREDENTIALS environment variable
        os.environ["GOOGLE_APPLICATION_CREDENTIALS"] = self.credentials_file.as_posix()

        builder_ok = False
        cmek_ok = False
        try:
            # Make sure that the builder env is destroyed. Its own try, so a
            # failure here still lets the key group below be torn down. A
            # build that never got as far as applying it leaves nothing to
            # destroy, as with the key group.
            try:
                if utils.terraform_may_have_applied(os.path.join(
                        self.image_dir, f"{self.blueprint_name}/builder-env")):
                    self._destroy_builder_env()
                builder_ok = True
            except Exception as e:
                logger.error(f"Destroying the builder env failed while deleting the image {image_name}: {e}")

            # Delete the image. The client is built here, inside the guard:
            # it reads the credentials file, which a build that failed early
            # never wrote.
            try:
                client = compute_v1.ImagesClient()
                operation = client.delete(project=project_id, image=image_name)
                operation.result()
                logger.info(f"Image '{image_name}' deleted successfully from project '{project_id}' in zone '{zone}'")
            except NotFound:
                logger.error(f"Image '{image_name}' not found in project '{project_id}' or zone '{zone}'")
            except (DefaultCredentialsError, FileNotFoundError) as e:
                # A built image cannot be deleted without credentials, and
                # its key must outlive it: a real failure, so the record is
                # kept for a retry. An image that was never built has nothing
                # to delete, so the teardown carries on.
                if self.image.status == "r":
                    raise
                logger.warning(f"No credentials to delete the image {image_name} with; it was never built, so there is none to delete: {e}")

            # The key outlives the build, not the image: tear it down only once
            # the image it encrypted is gone. Reached even when the image was
            # already absent, so its key group is not left behind.
            try:
                self._destroy_cmek_env()
                cmek_ok = True
            except Exception as e:
                logger.error(f"Destroying the key group failed while deleting the image {image_name}: {e}")

        except Exception as e:
            logger.error(f"An error occurred while deleting the image {image_name}: {e}")

        finally:
            # Clear the GOOGLE_APPLICATION_CREDENTIALS environment variable
            os.environ.pop("GOOGLE_APPLICATION_CREDENTIALS", None)

        return builder_ok and cmek_ok
