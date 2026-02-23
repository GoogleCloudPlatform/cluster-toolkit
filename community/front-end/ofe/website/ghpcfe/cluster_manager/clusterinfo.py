#!/usr/bin/env python3
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
# To create a cluster, we need:
# 1) Know which Cloud Provider & region/zone/project
# 2) Know authentication credentials
# 3) Know an "ID Number" or name - for directory to store state info
# 1 - Supplied via commandline
# 2 - Supplied via... Env vars / commandline?
# 3 - Supplied via commandline
"""Cluster specification and management routines"""

import json
import logging
import subprocess
import os
import re

from django.template import engines as template_engines
from google.api_core.exceptions import PermissionDenied as GCPPermissionDenied
from website.settings import SITE_NAME

from . import c2
from . import cloud_info
from . import utils

from .. import grafana
from ..models import Cluster, ApplicationInstallationLocation, ComputeInstance, ContainerRegistry

logger = logging.getLogger(__name__)


class ClusterInfo:
    """Expected process:

    ClusterInfo object - represent a cluster
    - Call prepare()
        - This will create the directory, dump a YAML file for GHPC
    - Call update()
        - This will dump a new YAML file
    - Call start_cluster()
        - Calls ghpc to create Terraform
        - Initializes Terraform
        - Applies Terraform"""

    def __init__(self, cluster):
        self.config = utils.load_config()
        self.ghpc_path = "/opt/gcluster/cluster-toolkit/ghpc"

        self.cluster = cluster
        self.cluster_dir = (
            self.config["baseDir"] / "clusters" / f"cluster_{self.cluster.id}"
        )

        self.env = template_engines["django"]

    def prepare(self, credentials):
        """Prepares the cluster for deployment.

        This method performs the necessary steps to prepare the cluster for deployment.
        It creates the required directory, sets up authentication credentials, and updates
        the cluster configuration. This method must be called before starting the cluster.

        Args:
            credentials (str): The authentication credentials required to access the cloud
            provider's resources. This should be a JSON-formatted string containing the
            necessary authentication details.

        Raises:
            subprocess.CalledProcessError: If there is an error during the preparation
            process, this exception will be raised, indicating that the process failed.

        Note:
            The required credentials can be obtained from the cloud provider's dashboard or
            by following the documentation for obtaining authentication credentials.
        """
        #self._create_cluster_dir()
        #self._set_credentials(credentials)
        #self.update()

    def update(self):
        self._prepare_ghpc_yaml()
        self._prepare_bootstrap_gcs()

    def start_cluster(self, credentials):
        self.cluster.cloud_state = "nm"
        self.cluster.status = "c"
        self.cluster.save()

        self._create_cluster_dir()
        self._set_credentials(credentials)
        self.update()

        try:
            self._run_ghpc()
            self._initialize_terraform()
            self._apply_terraform()

            dash = grafana.create_cluster_dashboard(self.cluster)
            self.cluster.grafana_dashboard_url = dash.get("url", "")
            self.cluster.save()

        # Not a lot we can do if terraform fails, it's on the admin user to
        # investigate and fix the errors shown in the log
        except Exception:  # pylint: disable=broad-except
            self.cluster.status = "e"
            self.cluster.cloud_state = "nm"
            self.cluster.save()
            raise

    def reconfigure_cluster(self):
        try:
            self._run_ghpc()
            self._initialize_terraform()
            self._apply_terraform()
            self.cluster.status = "r"
            self.cluster.cloud_state = "m"
            self.cluster.save()

        # Not a lot we can do if terraform fails, it's on the admin user to
        # investigate and fix the errors shown in the log
        except Exception:  # pylint: disable=broad-except
            self.cluster.status = "e"
            self.cluster.cloud_state = "nm"
            self.cluster.save()
            raise

    def stop_cluster(self):
        self._destroy_terraform()

    def get_cluster_access_key(self):
        return self.cluster.get_access_key()

    def _create_cluster_dir(self):
        try:
            self.cluster_dir.mkdir(parents=True)
        except FileExistsError:
            pass  # Do nothing if the directory already exists

    def _get_credentials_file(self):
        return self.cluster_dir / "cloud_credentials"

    def _set_credentials(self, creds=None):
        credfile = self._get_credentials_file()
        if not creds:
            # pull from DB
            creds = self.cluster.cloud_credential.detail
        with credfile.open("w") as fp:
            fp.write(creds)

        # Create SSH Keys
        self._create_ssh_key(self.cluster_dir)

    def _create_ssh_key(self, target_dir):
        # ssh-keygen -t rsa -f <tgtdir>/.ssh/id_rsa -N ""
        sshdir = target_dir / ".ssh"

        if not sshdir.exists():
            sshdir.mkdir(mode=0o711)

            priv_key_file = sshdir / "id_rsa"

            subprocess.run(
                [
                    "ssh-keygen",
                    "-t",
                    "rsa",
                    "-f",
                    priv_key_file.as_posix(),
                    "-N",
                    "",
                    "-C",
                    "citc@mgmt",
                ],
                check=True,
            )
        else:
            # Directory already exists, no need to create it again
            pass

    def indent_text(self, text, indent_level):
        indent = '  ' * indent_level  # 2 spaces per indent level, adjust as needed
        return '\n'.join(indent + line if line else line for line in text.split('\n'))

    def _prepare_ghpc_filesystems(self):
        filesystems_yaml = []
        refs = []
        template = self.env.get_template('blueprint/filesystem_config.yaml.j2')

        for (count, mp) in enumerate(self.cluster.mount_points.order_by("mount_order")):
            storage_id = f"mount_num_{mp.id}"
            server_ip = "'$controller'" if mp.export in self.cluster.shared_fs.exports.all() else mp.export.server_name
            context = {
                'storage_id': storage_id,
                'server_ip': server_ip,
                'remote_mount': mp.export.export_name,
                'local_mount': mp.mount_path,
                'mount_options': mp.mount_options,
                'fs_type': mp.fstype_name
            }
            rendered_yaml = template.render(context)
            indented_yaml = self.indent_text(rendered_yaml, 1) # Indent as necessary...
            filesystems_yaml.append(indented_yaml)
            refs.append(context['storage_id'])

        return ("\n\n".join(filesystems_yaml), refs)

    def _prepare_ghpc_artifact_registry(self):
        if not getattr(self.cluster, "use_containers", False):
            return "", False

        artifact_registry_yaml = []
        template = self.env.get_template('blueprint/artifact_registry_config.yaml.j2')

        registries = self.cluster.container_registry_relations.exclude(status="d")

        has_registries = registries.exists()  # Check if any registries exist

        for registry in registries:
            # logger.info(f"Processing registry ID: {registry.id}, repo_mode: {registry.repo_mode}")

            registry.status = "i"
            registry.cloud_state = "nm"
            registry.save(update_fields=["status"])

            context = {
                "registry_id": f"registry_{registry.id}",
                "repo_mode": registry.repo_mode,
                "format": registry.format,
                "use_public_repository": registry.use_public_repository,
                "repo_mirror_url": registry.repo_mirror_url,
                "repo_username": registry.repo_username,
                "repo_password": registry.repo_password,
                "use_upstream_credentials": registry.use_upstream_credentials,
            }

            # logger.info(f"Registry Context: {json.dumps(context, indent=2)}")
            rendered_yaml = template.render(context)
            if not rendered_yaml.strip():
                logger.warning(f"Rendered YAML for registry {registry.id} (mode: {registry.repo_mode}) is EMPTY!")

            indented_yaml = self.indent_text(rendered_yaml, 1)
            artifact_registry_yaml.append(indented_yaml)

        return "\n\n".join(artifact_registry_yaml), has_registries

    def _prepare_ghpc_partitions(self, part_uses):
        partitions_yaml = []
        refs = []
        template = self.env.get_template('blueprint/partition_config.yaml.j2')
        uses_str = self._yaml_refs_to_uses(part_uses, indent_level=1)

        for part in self.cluster.partitions.all():
            disk_range = list(range(part.additional_disk_count))
            exclusive = 'True' if part.enable_placement or not part.enable_node_reuse else 'False'
            context = {
                'part': part,
                'part_id': f"partition_{part.id}",
                'uses_str': uses_str,
                'cluster': self.cluster,
                'disk_range': disk_range,
                'exclusive': exclusive,
                "startup_bucket": self.config["server"]["gcs_bucket"],
            }
            rendered_yaml = template.render(context)
            indented_yaml = self.indent_text(rendered_yaml, 1)   # Same here
            partitions_yaml.append(indented_yaml)
            refs.append(context['part_id'])

        return ("\n\n".join(partitions_yaml), refs)

    def _prepare_cloudsql_yaml(self):
        if not self.cluster.use_cloudsql:
            return "", []
        template = self.env.get_template('blueprint/cloudsql_config.yaml.j2')
        context = {
            'cluster_id': self.cluster.cloud_id
        }
        rendered_yaml = template.render(context)
        indented_yaml = self.indent_text(rendered_yaml, 1)  # Adjust indent as necessary

        return indented_yaml, ['slurm-sql']

    def _yaml_refs_to_uses(self, use_list, indent_level=0):
        indent = '  ' * indent_level
        use_lines = [f"{indent}- {item}" for item in use_list]
        return "\n".join(use_lines)

    def _prepare_ghpc_yaml(self):
        try:
            yaml_file = self.cluster_dir / "cluster.yaml"
            project_id = json.loads(self.cluster.cloud_credential.detail)["project_id"]
            filesystems_yaml, filesystems_refs = self._prepare_ghpc_filesystems()
            partitions_yaml, partitions_refs = self._prepare_ghpc_partitions(filesystems_refs)
            artifact_registry_yaml, use_containers = self._prepare_ghpc_artifact_registry()
            cloudsql_yaml, cloudsql_refs = self._prepare_cloudsql_yaml()

            # Use a template to generate the final YAML configuration
            template = self.env.get_template('blueprint/cluster_config.yaml.j2')
            controller_uses_refs = ["hpc_network"] + partitions_refs + filesystems_refs + cloudsql_refs
            context = {
                "project_id": project_id,
                "site_name": SITE_NAME,
                "filesystems_yaml": filesystems_yaml,
                "partitions_yaml": partitions_yaml,
                "artifact_registry_yaml": artifact_registry_yaml,
                "cloudsql_yaml": cloudsql_yaml,
                "cluster": self.cluster,
                "controller_uses": self._yaml_refs_to_uses(controller_uses_refs, indent_level=2),
                "login_uses": self._yaml_refs_to_uses(filesystems_refs, indent_level=2),
                "controller_sa": "sa",
                "startup_bucket": self.config["server"]["gcs_bucket"],
            }
            rendered_yaml = template.render(context)

            # logger.debug("Generated YAML Output:\n" + rendered_yaml)

            if self.cluster.controller_node_image is not None:
                context["controller_image_yaml"] = f"""instance_image:
            family: image-{self.cluster.controller_node_image.family}
            project: {self.cluster.project_id}
            """

            if self.cluster.login_node_image is not None:
                context["login_image_yaml"] = f"""instance_image:
            family: image-{self.cluster.login_node_image.family}
            project: {self.cluster.project_id}
            """

            with yaml_file.open("w") as f:
                f.write(rendered_yaml)
            
            self.use_containers = use_containers

        except Exception as e:
            logger.exception(f"Exception happened creating blueprint for cluster {self.cluster.name} - {e}")

    def _prepare_bootstrap_gcs(self):
        template_dir = (
            self.config["baseDir"]
            / "infrastructure_files"
            / "cluster_startup"
            / "templates"
        )
        engine = template_engines["django"]
        for templ in ["controller", "login", "compute"]:
            template_fn = template_dir / f"bootstrap_{templ}.sh"
            with open(template_fn, "r", encoding="utf-8") as fp:
                tstr = fp.read()
                template = engine.from_string(tstr)
                # TODO: Add to context any other information we may need in the
                # startup script
                rendered_file = template.render(
                    context={
                        "server_bucket": self.config["server"]["gcs_bucket"],
                        "cluster": self.cluster,
                        "spack_dir": self.cluster.spackdir,
                        "fec2_topic": c2.get_topic_path(),
                        "use_containers": self.use_containers,
                        "fec2_subscription": c2.get_cluster_subscription_path(
                            self.cluster.id
                        ),
                    }
                )
                blobpath = f"clusters/{self.cluster.id}/{template_fn.name}"
                cloud_info.gcs_upload_file(
                    self.config["server"]["gcs_bucket"], blobpath, rendered_file
                )

    def _initialize_terraform(self):
        terraform_dir = self.get_terraform_dir()
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }
        try:
            logger.info("Invoking Terraform Init")
            utils.run_terraform(terraform_dir, "init")
            utils.run_terraform(terraform_dir, "validate", extra_env=extra_env)
            logger.info("Invoking Terraform Plan")
            utils.run_terraform(terraform_dir, "plan", extra_env=extra_env)
        except subprocess.CalledProcessError as cpe:
            logger.error("Terraform exec failed", exc_info=cpe)
            if cpe.stdout:
                logger.info("  STDOUT:\n%s\n", cpe.stdout.decode("utf-8"))
            if cpe.stderr:
                logger.info("  STDERR:\n%s\n", cpe.stderr.decode("utf-8"))
            raise

    def _run_ghpc(self):
        target_dir = self.cluster_dir
        try:
            logger.info("Invoking ghpc create")
            log_out_fn = target_dir / "ghpc_create_log.stdout"
            log_err_fn = target_dir / "ghpc_create_log.stderr"

            env = os.environ.copy()
            env['GOOGLE_APPLICATION_CREDENTIALS'] = self._get_credentials_file()

            with log_out_fn.open("wb") as log_out:
                with log_err_fn.open("wb") as log_err:
                    subprocess.run(
                        [self.ghpc_path, "create", "cluster.yaml", "-w", "--validation-level", "WARNING"],
                        cwd=target_dir,
                        stdout=log_out,
                        stderr=log_err,
                        check=True,
                        env=env,
                    )
        except subprocess.CalledProcessError as cpe:
            logger.error("ghpc exec failed", exc_info=cpe)
            # No logs from stdout/err - get dumped to files
            raise

    def _get_tf_state_resource(self, state, filters):
        """Given a Terraform State json file, look for the Resource that matches
        each entry in the supplied filters dictionary.

        Returns each match
        """
        print(state["resources"])
        print(filters)

        def matches(x):
            try:
                for k, v in filters.items():
                    if x[k] != v:
                        return False
                return True
            except KeyError:
                return False

        return list(filter(matches, state["resources"]))

    def _create_model_instances_from_tf_state(self, state, filters):
        tf_resources = self._get_tf_state_resource(state, filters)
        print(tf_resources)

        if not tf_resources:
            logger.error(f"No resources found for filters: {filters}")
            return []

        tf_nodes = tf_resources[0].get("instances", [])
        if not tf_nodes:
            logger.error(f"No instances found for resource with filters: {filters}")
            return []

        def model_from_tf(tf):
            ci_kwargs = {
                "id": None,
                "cloud_credential": self.cluster.cloud_credential,
                "cloud_state": "m",
                "cloud_region": self.cluster.cloud_region,
                "cloud_zone": self.cluster.cloud_zone,
            }

            try:
                ci_kwargs["cloud_id"] = tf["attributes"]["name"]
                ci_kwargs["instance_type"] = tf["attributes"]["machine_type"]
            except KeyError:
                pass

            try:
                nic = tf["attributes"]["network_interface"][0]
                ci_kwargs["internal_ip"] = nic["network_ip"]
                ci_kwargs["public_ip"] = nic["access_config"][0]["nat_ip"]
            except (KeyError, IndexError):
                pass

            try:
                service_acct = tf["attributes"]["service_account"][0]
                ci_kwargs["service_account"] = service_acct["email"]
            except (KeyError, IndexError):
                pass

            # Check if a model with the same attributes exists
            try:
                existing_instance = ComputeInstance.objects.get(
                    internal_ip=ci_kwargs["internal_ip"]
                )
                # If the instance already exists, update its attributes
                for key, value in ci_kwargs.items():
                    setattr(existing_instance, key, value)
                existing_instance.save()
                return existing_instance  # Return the existing instance
            except ComputeInstance.DoesNotExist:
                # If the instance doesn't exist, create a new one
                return ComputeInstance(**ci_kwargs)

        return [model_from_tf(instance) for instance in tf_nodes]

    def _get_service_accounts(self, tf_state):
        # TODO:  Once we're creating service accounts, can pull them from those
        # resources At the moment, pull from controller & login instances. This
        # misses "compute" nodes, but they're going to just be the same as
        # controller & login until we start setting them.
        service_accounts = {}

        controller_filters = {
            "module": "module.slurm_controller",
            "type": "google_compute_instance_from_template",
            "name": "controller",
        }

        controller_resources = self._get_tf_state_resource(tf_state, controller_filters)
        if controller_resources:
            controller_instance = controller_resources[0]["instances"][0]
            service_accounts["controller"] = controller_instance["attributes"]["service_account"][0]["email"]
        else:
            logger.error(f"No resources found for controller filters: {controller_filters}")


        login_filters = {
            "module": 'module.slurm_controller.module.login["slurm-login"].module.instance',
            "type": "google_compute_instance_from_template",
            "name": "slurm_instance",
        }

        login_resources = self._get_tf_state_resource(tf_state, login_filters)
        if login_resources:
            login_instance = login_resources[0]["instances"][0]
            service_accounts["login"] = login_instance["attributes"]["service_account"][0]["email"]
            service_accounts["compute"] = login_instance["attributes"]["service_account"][0]["email"]
        else:
            logger.error(f"No resources found for login filters: {login_filters}")

        return service_accounts

    def _apply_service_account_permissions(self, service_accounts):
        # Need to give permission for all instances to download startup scripts
        # Need to give ability to upload job log files to bucket
        # TODO:  Figure out who exactly will do this.  For now, grant to all.
        all_sas = set(service_accounts.values())
        bucket = self.config["server"]["gcs_bucket"]
        for sa in all_sas:
            cloud_info.gcs_apply_bucket_acl(
                bucket,
                f"serviceAccount:{sa}",
                permission="roles/storage.objectAdmin",
            )

        # Give Command & Control access
        try:
            c2.add_cluster_subscription_service_account(
                self.cluster.id, service_accounts["controller"]
            )
        except GCPPermissionDenied:
            logger.warning(
                "Permission Denied attempting to add IAM permissions for "
                "service account to PubSub Subscription/Topic.  Command and "
                "Control may not work.  Please grant the role of pubsub.admin "
                "to FrontEnd service account."
            )
            if self.cluster.project_id != self.config["server"]["gcp_project"]:
                logger.error(
                    "Cluster project differs from FrontEnd project.  C&C will "
                    "not work."
                )

    def extract_and_update_registry_info(self, tf_state):
        """Extract repository_id and secret_id from terraform state and update existing ContainerRegistry models."""
        filters = {
            "type": "google_artifact_registry_repository",
        }
        tf_resources = self._get_tf_state_resource(tf_state, filters)

        if not tf_resources:
            logger.error("No repository resources found in terraform state.")
            return

        # Track updated registry IDs to prevent redundant updates
        updated_registry_ids = set()

        for resource in tf_resources:
            instances = resource.get("instances", [])
            for instance in instances:
                attributes = instance.get("attributes", {})
                repo_id = attributes.get("repository_id")

                # Construct secret_id directly based on the repo_id pattern
                secret_id = f"{repo_id}-secret" if repo_id else None

                if repo_id:
                    # Extract the numeric ID from the module name in tfstate
                    # Example: "module.registry_5" should extract "5"
                    module_name = resource.get("module", "")
                    match = re.search(r"module\.registry_(\d+)", module_name)
                    if match:
                        django_registry_id = int(match.group(1))
                        logger.info(f"Attempting to match registry ID: {django_registry_id} to repo_id: {repo_id}")

                        # Match by model ID (primary key)
                        registry = self.cluster.container_registry_relations.filter(id=django_registry_id).first()

                        if registry:
                            # Update repository_id if missing or different
                            if not registry.repository_id or registry.repository_id != repo_id:
                                registry.repository_id = repo_id  # Full name with unique identifier
                                registry.cloud_state = "nm"
                                registry.status = "i"
                                registry.save(update_fields=["status"])

                            # Update secret_id if missing or different
                            if secret_id and (not registry.secret_id or registry.secret_id != secret_id):
                                registry.secret_id = secret_id

                            # Set status to ready if both repository_id and secret_id are available
                            if registry.repository_id and registry.secret_id:
                                registry.cloud_state = "m"
                                registry.status = "r"
                                registry.save(update_fields=["status"])

                            # Save updates
                            registry.save()
                            logger.info(f"Updated registry '{registry.get_registry_url()}' with repository_id '{repo_id}' and secret_id '{secret_id}'.")
                            updated_registry_ids.add(registry.id)
                        else:
                            logger.warning(f"No existing ContainerRegistry found for Django registry ID '{django_registry_id}' in cluster {self.cluster.id}")
                    else:
                        logger.warning(f"Could not extract registry ID from module name: {module_name}")

        # Log info about missing registries if any
        if not updated_registry_ids:
            logger.warning("No ContainerRegistry entries were updated with repository or secret information.")

    def _apply_terraform(self):
        terraform_dir = self.get_terraform_dir()

        # Create C&C Subscription
        c2.create_cluster_subscription(self.cluster.id)

        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }
        try:
            logger.info("Invoking Terraform Apply")
            utils.run_terraform(terraform_dir, "apply", extra_env=extra_env)

            # Look for Management and Login Nodes in TF state file
            tf_state_file = terraform_dir / "terraform.tfstate"
            with tf_state_file.open("r") as statefp:
                state = json.load(statefp)

                # Extract and save Artifact Registry repository and secret information (if any)
                self.extract_and_update_registry_info(state)

                # Apply Perms to the service accounts
                try:
                    service_accounts = self._get_service_accounts(state)
                    self._apply_service_account_permissions(service_accounts)
                except Exception as e:
                    # Be nicer to the user and continue creating cluster
                    logger.warning(f"An error occurred while applying permissions to service accounts: {e}")


                # Cluster is now being initialized
                self.cluster.internal_name = self.cluster.name
                self.cluster.cloud_state = "m"

                # Cluster initialization is now running.
                self.cluster.status = "i"
                self.cluster.save()

                # Filters for Management Nodes (Controller)
                mgmt_filters = {
                    "module": "module.slurm_controller",
                    "type": "google_compute_instance_from_template",
                    "name": "controller",
                }
                mgmt_nodes = self._create_model_instances_from_tf_state(
                    state,
                    mgmt_filters,
                )
                if len(mgmt_nodes) != 1:
                    logger.warning(
                        "Found %d controller nodes, there should be only 1",
                        len(mgmt_nodes),
                    )
                if len(mgmt_nodes):
                    node = mgmt_nodes[0]
                    node.save()
                    self.cluster.controller_node = node
                    logger.info(
                        "Created cluster controller node with IP address %s",
                        node.public_ip if node.public_ip else node.internal_ip,
                    )

                # Filters for Login Nodes
                login_filters = {
                    "module": 'module.slurm_controller.module.login["slurm-login"].module.instance',
                    "type": "google_compute_instance_from_template",
                    "name": "slurm_instance",
                }
                login_nodes = self._create_model_instances_from_tf_state(
                    state,
                    login_filters,
                )
                if len(login_nodes) != self.cluster.num_login_nodes:
                    logger.warning(
                        "Found %d login nodes, expected %d from config",
                        len(login_nodes),
                        self.cluster.num_login_nodes,
                    )
                for lnode in login_nodes:
                    lnode.cluster_login = self.cluster
                    lnode.save()
                    logger.info(
                        "Created login node with IP address %s",
                        lnode.public_ip
                        if lnode.public_ip
                        else lnode.internal_ip,
                    )

                # Set up Spack Install location
                self._configure_spack_install_loc()

                self.cluster.save()

        except subprocess.CalledProcessError as err:
            # We can error during provisioning, in which case Terraform
            # doesn't tear things down.
            logger.error("Terraform apply failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            raise

    def _destroy_terraform(self):
        terraform_dir = self.get_terraform_dir()
        extra_env = {
            "GOOGLE_APPLICATION_CREDENTIALS": self._get_credentials_file()
        }
        try:
            logger.info("Invoking Terraform destroy")
            self.cluster.status = "t"
            self.cluster.cloud_state = "dm"
            self.cluster.save()

            utils.run_terraform(terraform_dir, "destroy", extra_env=extra_env)

            # Mark Container Registry objects as deleted
            registries = ContainerRegistry.objects.filter(cluster=self.cluster)
            registry_count = 0
            for registry in registries:
                registry.status = "d"
                registry.save(update_fields=["status"])
                registry_count += 1

            controller_sa = self.cluster.controller_node.service_account

            self.cluster.controller_node.delete()
            self.cluster.login_nodes.all().delete()
            # Refresh so our python object gets the SET_NULL's from the above
            # deletes
            self.cluster = Cluster.objects.get(id=self.cluster.id)

            self.cluster.status = "d"
            self.cluster.cloud_state = "xm"
            self.cluster.save()

            c2.delete_cluster_subscription(self.cluster.id, controller_sa)
            logger.info("Terraform destroy completed")

        except subprocess.CalledProcessError as err:
            logger.error("Terraform destroy failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            raise

    def _configure_spack_install_loc(self):
        """Configures the spack_install field.
        Could point to an existing install, if paths match appropriately,
        otherwise, create a new DB entry.
        """
        cluster_spack_dir = self.cluster.spackdir
        # Find the mount point that best matches our spack dir
        spack_mp = None
        for mp in self.cluster.mount_points.order_by("mount_order"):
            if cluster_spack_dir.startswith(mp.mount_path):
                spack_mp = mp

        if not spack_mp:
            logger.error(
                "Cannot find a mount_point matching configured spack path %s",
                cluster_spack_dir,
            )
            return

        partial_path = cluster_spack_dir[len(spack_mp.mount_path) + 1 :]
        # Now we have a Mount Point, Find app Install locs with that MP's export
        possible_apps = ApplicationInstallationLocation.objects.filter(
            fs_export=spack_mp.export
        ).filter(path=partial_path)
        if possible_apps:
            self.cluster.spack_install = possible_apps[0]
        else:
            # Need to create a new entry
            self.cluster.spack_install = ApplicationInstallationLocation(
                fs_export=spack_mp.export, path=partial_path
            )
            self.cluster.spack_install.save()
        self.cluster.save()

    def get_app_install_loc(self, install_path):
        my_mp = None
        for mp in self.cluster.mount_points.order_by("mount_order"):
            if install_path.startswith(mp.mount_path):
                my_mp = mp

        if not my_mp:
            logger.warning(
                "Unable to find a mount_point matching path %s", install_path
            )
            return None

        partial_path = install_path[len(my_mp.mount_path) + 1 :]
        possible_apps = ApplicationInstallationLocation.objects.filter(
            fs_export=my_mp.export
        ).filter(path=partial_path)
        if possible_apps:
            return possible_apps[0]
        else:
            # Need to create a new entry
            install_loc = ApplicationInstallationLocation(
                fs_export=my_mp.export, path=partial_path
            )
            install_loc.save()
            return install_loc

    def get_terraform_dir(self):
        return self.cluster_dir / self.cluster.cloud_id / "primary"
