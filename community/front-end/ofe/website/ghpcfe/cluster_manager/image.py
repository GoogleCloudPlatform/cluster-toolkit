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
from . import credentials
from . import utils
from . import image_startup
import secrets
import subprocess
import os
from django.conf import settings
from google.api_core.exceptions import NotFound
from google.cloud import compute_v1

logger = logging.getLogger(__name__)

# Name of the build VM's account in the image-builder blocks. The account id is
# <deployment name>-<name>, limited to 30 characters.
BUILDER_ACCOUNT_NAME = "bld"
CLOUD_PLATFORM_SCOPE = "https://www.googleapis.com/auth/cloud-platform"


def new_role_id_suffix(previous=""):
    """6 hex characters for the profile's role_id_suffix, never `previous`"""
    while True:
        suffix = secrets.token_hex(3)
        if suffix != previous:
            return suffix


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
        # The service-account and service-account-profiles modules refuse
        # underscores in deployment_name, so their blocks use this name instead
        # of blueprint_name. The deployment folder keeps blueprint_name.
        self.builder_deployment_name = f"img-{self.image.id}"

    def _operation(self, verb):
        return f"{verb} image {self.image.name}"

    def _cloud(self, verb):
        """The credential and operation of a Terraform or Packer run"""
        return {
            "credential": self.image.cloud_credential,
            "operation": self._operation(verb),
        }

    def _check_credential(self):
        """Stop before any cloud work when the build cannot succeed: an
        impersonate credential (Packer needs a key), or an identity that lacks
        a permission the build needs"""
        credential = self.image.cloud_credential
        try:
            credentials.check_image_build_credential(credential)
            credentials.preflight(
                credential,
                self._operation("build"),
                credentials.permissions_for(
                    "image_build",
                    profiles=self.image.identity_model == "profiles",
                ),
            )
        except credentials.CredentialError as err:
            self.update_image_status("e")
            logger.error("Image %s cannot be built: %s", self.image.id, err)
            raise

    def prepare(self):
        """
        Prepare the image creation process by following these steps:

        1. Create the necessary directory structure for the image.
        2. Generate a Cluster Toolkit blueprint to build the image.
        3. Run the Cluster Toolkit (`gcluster`) to create the image based on the blueprint.
        4. Set up the builder environment on Google Cloud Platform (GCP) using Terraform.
        5. Create the image on GCP using Packer.
        6. Destroy the builder environment after the image creation is complete.

        This method handles the entire image creation process, from setting up the necessary
        directories and configuration files to executing Cluster Toolkit and Packer to build
        and finalize the image. If any step encounters an error, it logs the issue and marks
        the image's status as "error" (status code 'e').

        Note:
        - This method assumes that the necessary tools (Cluster Toolkit, Terraform, and Packer)
          are properly installed and configured on the system.
        - Each tool run gets the image's credential from credentials.py; an
          image whose credential has no key is refused first, because Packer
          cannot impersonate a service account.

        Raises:
            credentials.CredentialError: If the credential cannot build the
                                         image (no key, or a missing permission).
            OSError: If there is an error while creating the image directory.
            subprocess.CalledProcessError: If any of the subprocess calls (gcluster, Terraform, or Packer)
                                           encounter an error during execution.
        """
        self._check_credential()
        self._refresh_role_id_suffix()
        self._create_image_dir()
        self._create_blueprint()
        self._run_ghpc()
        self._create_builder_env()
        self._create_image()
        self._destroy_builder_env()

    def update_image_status(self, new_status):
        self.image.status = new_status
        self.image.save()

    def _refresh_role_id_suffix(self):
        """Give a profiles image new custom role ids when its build needs them.

        A finished build destroys the builder environment, and a failed one may
        have destroyed part of it, which soft-deletes the image-builder custom
        roles. Google keeps a deleted role id for 7 to 44 days, so a build that
        starts after a finished ("r") or failed ("e") one gets a new suffix;
        Terraform then replaces any roles left from the failed build. A build
        that is started again before it reached either state keeps its
        suffix, so Terraform finds the roles it already made. The suffix is
        saved before any cloud work, so the database always holds the one in
        the builder environment's Terraform state. Legacy images have no
        custom roles and are left unchanged.
        """
        if self.image.identity_model != "profiles":
            return
        if self.image.role_id_suffix and self.image.status not in ("e", "r"):
            return
        self.image.role_id_suffix = new_role_id_suffix(self.image.role_id_suffix)
        self.image.save(update_fields=["role_id_suffix"])

    def _create_image_dir(self):
        try:
            self.image_dir.mkdir(parents=True, exist_ok=True)
        except OSError as e:
            self.update_image_status("e")
            logger.error(f"Error occurred while creating the image directory: {e}")

    def _builder_blocks(self, project_id):
        """
        Blueprint text that gives a profiles image its own build VM account.

        Returns the builder-env module blocks, the scripts_for_image settings and
        the custom-image settings to insert, or empty strings for a legacy image,
        whose blueprint keeps the default Compute Engine account.
        """
        identity_model = self.image.identity_model
        if identity_model == "legacy":
            return "", "", ""
        if identity_model != "profiles":
            raise ValueError(f"Unknown identity model {identity_model!r}")
        if not self.image.role_id_suffix:
            raise ValueError(
                f"Image {self.image.id} has no role_id_suffix; a build sets it "
                "before rendering the blueprint"
            )

        # OFE applies builder-env and runs Packer separately and passes only the
        # startup script between them, so Packer gets the account's email as a
        # literal. The account exists, and its grants have propagated, by the
        # time Packer runs: builder-env, including the profile's IAM wait, has
        # been applied first.
        builder_email = (
            f"{self.builder_deployment_name}-{BUILDER_ACCOUNT_NAME}"
            f"@{project_id}.iam.gserviceaccount.com"
        )
        modules = f"""
  - id: builder_sa
    source: modules/project/service-account
    settings:
      deployment_name: {self.builder_deployment_name}
      name: {BUILDER_ACCOUNT_NAME}
      project_roles: []

  - id: builder_profile
    source: community/modules/iam/service-account-profiles
    use: [builder_sa]
    settings:
      deployment_name: {self.builder_deployment_name}
      name: {BUILDER_ACCOUNT_NAME}
      profile: image-builder
      role_id_suffix: "{self.image.role_id_suffix}"
"""
        # The build VM downloads the runners from the startup-script staging
        # bucket as its own account
        scripts_settings = """
      bucket_viewers:
      - serviceAccount:$(builder_profile.service_account_email)"""
        # The custom-image module names the build VM packer-<hex>, the prefix the
        # image-builder profile's scoped setMetadata grant covers
        packer_settings = f"""      service_account_email: {builder_email}
      service_account_scopes:
      - {CLOUD_PLATFORM_SCOPE}
"""
        return modules, scripts_settings, packer_settings

    def _create_blueprint(self):
        """
        Create Cluster Toolkit blueprint that will build the image.
        """
        try:
            blueprint_file = self.image_dir / "image.yaml"
            project_id = self.image.cloud_credential.project_id
            builder_modules, scripts_settings, packer_settings = (
                self._builder_blocks(project_id)
            )
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
{builder_modules}
  - id: scripts_for_image
    source: modules/scripts/startup-script
    settings:{scripts_settings}
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
{packer_settings}"""
            )
        except Exception as e:
            self.update_image_status("e")
            logger.error(f"Error occurred while creating blueprint: {e}")
            raise

    def _run_ghpc(self):
        target_dir = self.image_dir
        try:
            logger.info(f"Invoking gcluster create for the image {self.image.id}")
            log_out_fn = target_dir / "ghpc_create_log.stdout"
            log_err_fn = target_dir / "ghpc_create_log.stderr"

            # The gcluster validators read only a key file
            with credentials.tool_environment(
                self.image.cloud_credential, key_file=True
            ) as env, log_out_fn.open("wb") as log_out, log_err_fn.open(
                "wb"
            ) as log_err:
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
        cross-group `use`: export-outputs from the applied builder
        group, then import-inputs into the packer group.
        """
        target_dir = self.image_dir
        log_out_fn = target_dir / "ghpc_import_log.stdout"
        log_err_fn = target_dir / "ghpc_import_log.stderr"
        # export-outputs reads the builder group's state from the state
        # bucket, so it acts as the credential too
        try:
            with credentials.tool_environment(
                self.image.cloud_credential, key_file=True
            ) as env, log_out_fn.open("ab") as log_out, log_err_fn.open(
                "ab"
            ) as log_err:
                subprocess.run(
                    [self.ghpc_path, "export-outputs",
                     f"{self.blueprint_name}/builder-env"],
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
        cloud = self._cloud("build")
        try:
            logger.info("Invoking Terraform Init for builder env.")
            try:
                terraform_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/builder-env")
                packer_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/packer-image")
            except OSError as e:
                self.update_image_status("e")
                logger.error(f"Error occurred while constructing terraform_dir: {e}")
            # init reads the state bucket, so it acts as the credential too
            utils.run_terraform(terraform_dir, "init", **cloud)
            utils.run_terraform(terraform_dir, "validate", **cloud)
            logger.info("Invoking Terraform Plan for builder env.")
            utils.run_terraform(terraform_dir, "plan", **cloud)
            # Before the first apply: an apply or build that fails leaves
            # the status "e" whether or not it made resources, so a delete
            # without the builder folder needs this to know (see
            # _empty_state_without_builder_folder). Never cleared: set with
            # an update of that column alone, and Image.save() keeps it.
            if not self.image.builder_started:
                self.image.set_builder_started()
            logger.info("Invoking Terraform Apply for builder env.")
            utils.run_terraform(terraform_dir, "apply", **cloud)
            logger.info("Exporting startup script from builder env.")
            utils.run_terraform(terraform_dir, "output", **cloud,
                                arguments=[
                                    "-raw",
                                    "startup_script_scripts_for_image"],
                                    )
            # Wrapped so that the build cannot hang on the Packer plugin's
            # own completion report; see image_startup.py.
            with open(f"{terraform_dir}/terraform_output_log.stdout", encoding="utf-8") as src:
                startup_script = image_startup.wrap_startup_script(src.read())
            with open(f"{packer_dir}/custom-image/startup_script.sh", "w", encoding="utf-8") as dst:
                dst.write(startup_script)
            # The packer module uses the builder VPC (ghpc create rejects
            # unused modules), so its subnetwork input arrives through the
            # toolkit's cross-group bridge. startup_script arrives that way
            # too, but the explicit startup_script_file -var below overrides it.
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
        cloud = self._cloud("build")
        try:
            logger.info("Invoking Packer Init for image.")
            try:
                packer_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/packer-image/custom-image")
            except OSError as e:
                self.update_image_status("e")
                logger.exception(f"Error occurred while constructing packer_dir: {e}")
            utils.run_packer(packer_dir, "init", arguments=["."])
            utils.run_packer(packer_dir, "validate", **cloud,
                             arguments=["-var", "startup_script_file=startup_script.sh", "."])
            logger.info("Invoking Packer build for the image")
            utils.run_packer(packer_dir, "build", **cloud,
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
            # Mark the build failed: the builder env, including the profile's
            # custom role, is destroyed next, so a rebuild needs a new
            # role_id_suffix, which only a failed or finished image gets.
            self.update_image_status("e")
            logger.exception(f"Unhandled error happened during image {self.image.id} creation.")
        
    def _destroy_builder_env(self, mark_failed=True):
        """Destroy builder environment on GCP.

        A failure marks the image failed ("e") when mark_failed is set, as
        for the clean-up at the end of a build. A delete passes False: a
        failed delete keeps the image as it was.
        """
        try:
            logger.info("Invoking Terraform Destroy for builder env.")
            try:
                terraform_dir = os.path.join(self.image_dir, f"{self.blueprint_name}/builder-env")
            except OSError as e:
                if mark_failed:
                    self.update_image_status("e")
                logger.error(f"Error occurred while constructing terraform_dir: {e}")
            logger.info("Invoking Terraform Destroy for builder env.")
            utils.run_terraform(
                terraform_dir, "destroy", **self._cloud("clean up the builder of")
            )
        except subprocess.CalledProcessError as cpe:
            if mark_failed:
                self.update_image_status("e")
            logger.error(f"Terraform exec failed for destroying builder env, image: {self.image.id}", exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
            raise
        
    def delete_image(self):
        """Destroy the image's builder environment, then delete the image.

        Raises, leaving the image to its caller to keep, when the identity
        lacks a permission the delete needs (credentials.CredentialError),
        the builder environment cannot be destroyed, or the delete fails.
        An image that Google Cloud no longer has counts as deleted.

        Without the builder environment folder (a build refused before
        gcluster wrote it, or a server rebuilt with a restored database),
        Terraform cannot run there, so the builder's state in the state
        bucket decides (see _empty_state_without_builder_folder): the
        builder destroy is skipped, or the delete is refused
        (credentials.CredentialError).
        """
        credential = self.image.cloud_credential
        project_id = credential.project_id
        image_name = f"image-{self.image.name}"
        zone = self.image.cloud_zone

        try:
            credentials.preflight(
                credential,
                self._operation("delete"),
                credentials.permissions_for(
                    "image_destroy",
                    profiles=self.image.identity_model == "profiles",
                ),
            )
            # The client acts as the image's credential
            client = compute_v1.ImagesClient(
                credentials=credentials.python_credentials(credential)
            )

            # Make sure that the builder env is destroyed
            builder_dir = self.image_dir / self.blueprint_name / "builder-env"
            if builder_dir.is_dir():
                self._destroy_builder_env(mark_failed=False)
            else:
                nothing_left = self._empty_state_without_builder_folder(
                    builder_dir
                )
                logger.info(
                    "Image %s has no builder environment folder %s and %s, "
                    "so no builder is left to destroy; skipping the builder "
                    "destroy",
                    self.image.id,
                    builder_dir,
                    nothing_left,
                )

            # Delete the image
            try:
                operation = client.delete(project=project_id, image=image_name)
                operation.result()
            except NotFound:
                logger.warning(
                    "Image '%s' was not found in project '%s'; nothing to "
                    "delete",
                    image_name,
                    project_id,
                )
                return
            logger.info(f"Image '{image_name}' deleted successfully from project '{project_id}' in zone '{zone}'")

        except Exception as e:
            logger.error(f"An error occurred while deleting the image {image_name}: {e}")
            raise

    def _empty_state_without_builder_folder(self, builder_dir):
        """For an image whose builder environment folder builder_dir is
        missing, return why no builder is left in the cloud (for the log),
        or refuse.

        The state is read from the state bucket as the image's credential,
        as ClusterInfo does for a cluster without its Terraform folder.
        Terraform runs only in a builder folder gcluster wrote, so state
        objects that hold no resources mean no builder is left in the cloud.
        No state object at all means that only for an image whose builder
        apply never started (builder_started is False): an apply writes the
        builder's state to the bucket, and its destroy leaves it there,
        emptied, so for one that started, the state is somewhere else
        (never migrated to the bucket, a server under another deployment
        name, or a restored database with a new state namespace). Images
        from before builder_started existed hold True, and cannot be told
        apart from one whose apply started, so the refusal names both.

        Refused when the state holds resources, cannot be read, or is
        missing for an image whose builder apply started: raise
        credentials.CredentialError, and the image is kept. The status is
        no evidence: a build that failed ("e") may have made the builder's
        resources, and images record no cloud state.
        """
        credential = self.image.cloud_credential
        location = "the Terraform state bucket"
        try:
            bucket = utils.get_terraform_state_bucket()
            prefix = utils.terraform_state_prefix("image", self.image.id)
            location = f"gs://{bucket}/{prefix.strip('/')}/"
            contents = utils.state_contents(bucket, prefix, credential)
        except Exception as err:  # pylint: disable=broad-except
            # Any failure: without the state there is no knowing whether
            # the builder still has resources in Google Cloud
            reason = (
                f"its Terraform state in {location} could not be read as "
                f"credential {credential.name}: "
                f"{str(err) or type(err).__name__}. Without the state OFE "
                "cannot tell whether the image's builder still has resources "
                "in Google Cloud. Fix the error and delete the image again."
            )
            cause = err
        else:
            cause = None
            started = self.image.builder_started
            if contents.states and not contents.resources:
                return f"its Terraform state in {location} holds no resources"
            if not contents.states and not started:
                return (
                    f"it has no Terraform state in {location} and no "
                    "Terraform apply of its builder ever started"
                )
            if contents.states:
                count = contents.resources
                reason = (
                    f"its Terraform state in {location} still holds {count} "
                    f"resource{'' if count == 1 else 's'}, which Terraform "
                    "can destroy only from that folder. Have an administrator "
                    "destroy the resources from that state, then delete the "
                    "image again."
                )
            else:
                reason = (
                    f"no Terraform state was found under {location}, "
                    "although a Terraform apply of its builder was started, "
                    "or the image predates the started-flag check, so OFE "
                    "treats its builder as deployed. Its builder's "
                    "resources may still exist in Google Cloud. Possible "
                    "causes: its state was never migrated to "
                    "the state bucket, the server was deployed under another "
                    "deployment name (and so uses another state bucket), or "
                    "the database was restored with a new state namespace. "
                    "See \"When a resource's state is not in the bucket\" "
                    "in docs/admin_guide.md to find the state or delete the "
                    "resources, then delete the image again."
                )
        raise credentials.CredentialError(
            f"Cannot {self._operation('delete')}: its builder environment "
            f"folder {builder_dir} is missing, and {reason} The image is kept."
        ) from cause
