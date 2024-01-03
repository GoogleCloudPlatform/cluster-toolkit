#!/usr/bin/env python3
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

from django.template import engines as template_engines
from google.api_core.exceptions import PermissionDenied as GCPPermissionDenied
from website.settings import SITE_NAME

from . import c2
from . import cloud_info
from . import utils

from .. import grafana
from ..models import Cluster, ApplicationInstallationLocation, ComputeInstance

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
        self.ghpc_path = "/opt/gcluster/hpc-toolkit/ghpc"

        self.cluster = cluster
        self.cluster_dir = (
            self.config["baseDir"] / "clusters" / f"cluster_{self.cluster.id}"
        )

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

    def _prepare_ghpc_filesystems(self):
        yaml = []
        refs = []
        for (count, mp) in enumerate(
            self.cluster.mount_points.order_by("mount_order")
        ):
            storage_id = f"mount_num_{count}"
            ip = (
                "'$controller'"
                if mp.export in self.cluster.shared_fs.exports.all()
                else mp.export.server_name
            )
            yaml.append(
                f"""
  - source: modules/file-system/pre-existing-network-storage
    kind: terraform
    id: {storage_id}
    settings:
      server_ip: {ip}
      remote_mount: {mp.export.export_name}
      local_mount: {mp.mount_path}
      mount_options: {mp.mount_options}
      fs_type: {mp.fstype_name}
"""
            )
            refs.append(storage_id)

        return ("\n\n".join(yaml), refs)

    def _prepare_ghpc_partitions(self, part_uses):
        yaml = []
        refs = []
        uses_str = self._yaml_refs_to_uses(part_uses)
        for (count, part) in enumerate(self.cluster.partitions.all()):
            part_id = f"partition_{count}"
            if part.image is not None:
                instance_image_yaml = f"""instance_image:
        family: image-{part.image.family}
        project: {self.cluster.project_id}"""
            else:
                instance_image_yaml = ""

            if part.additional_disk_count > 0 and part.additional_disk_count is not None:
                additional_disks_yaml = f"""additional_disks:
"""
                for disk in range(part.additional_disk_count):
                    additional_disks_yaml += f"""      - device_name: disk{disk}
        disk_name: null
        disk_size_gb: {part.additional_disk_size}
        disk_type: {part.additional_disk_type}
        disk_labels: {{}}
        auto_delete: {part.additional_disk_auto_delete}
        boot: false\n"""
            else:
                additional_disks_yaml = ""

            yaml.append(
                f"""
  - source: community/modules/compute/schedmd-slurm-gcp-v5-partition
    kind: terraform
    id: {part_id}
    use:
    - {part_id}-group
{uses_str}
    settings:
      partition_name: {part.name}
      subnetwork_self_link: {self.cluster.subnet.cloud_id}
      enable_placement: {part.enable_placement}
      exclusive: {part.enable_placement or not part.enable_node_reuse}
  - source: community/modules/compute/schedmd-slurm-gcp-v5-node-group
    id: {part_id}-group
    use:
{uses_str}
    settings:
      enable_smt: {part.enable_hyperthreads}
      machine_type: {part.machine_type}
      node_count_dynamic_max: {part.dynamic_node_count}
      node_count_static: {part.static_node_count}
      disk_size_gb: {part.boot_disk_size}
      disk_type: {part.boot_disk_type}
      {instance_image_yaml}
      {additional_disks_yaml}
"""
            )

            if part.image:
                yaml[-1] += (
                    f"""\
"""
                )

            # Temporarily hack in some A100 support
            if part.GPU_per_node > 0:
                yaml[-1] += (
                    f"""\
      guest_accelerator:
        - type: {part.GPU_type}
          count: {part.GPU_per_node}
"""
                )
            refs.append(part_id)

        return ("\n\n".join(yaml), refs)

    def _yaml_refs_to_uses(self, use_list):
        return "\n".join([f"    - {x}" for x in use_list])
    

    def _prepare_ghpc_yaml(self):
        try:
            yaml_file = self.cluster_dir / "cluster.yaml"
            project_id = json.loads(self.cluster.cloud_credential.detail)[
                "project_id"
            ]
            (
                filesystems_yaml,
                filesystems_references,
            ) = self._prepare_ghpc_filesystems()
            (
                partitions_yaml,
                partitions_references,
            ) = self._prepare_ghpc_partitions(
                ["hpc_network"] + filesystems_references
            )
            controller_uses = self._yaml_refs_to_uses(
                ["hpc_network"] + partitions_references + filesystems_references
            )
            login_uses = self._yaml_refs_to_uses(
                ["hpc_network"] + filesystems_references
            )
            controller_sa = "sa"
            # TODO: Determine if these all should be different, and if so, add to
            # resource to be created. NOTE though, that at the moment, GHPC won't
            # let us unpack output variables, so we can't index properly.
            # for now, just use the singular access, and only create a single acct
            # compute_sa = controller_sa
            # login_sa = controller_sa
            
            # pylint: disable=line-too-long
            startup_bucket = self.config["server"]["gcs_bucket"]
            if self.cluster.login_node_image is not None:
                login_image_yaml = f"""instance_image:
            family: image-{self.cluster.login_node_image.family}
            project: {self.cluster.project_id}"""
            else:
                login_image_yaml = ""
            
            if self.cluster.controller_node_image is not None:
                controller_image_yaml = f"""instance_image:
            family: image-{self.cluster.controller_node_image.family}
            project: {self.cluster.project_id}
            """
            else:
                controller_image_yaml = ""

            if self.cluster.use_cloudsql:
                controller_uses = self._yaml_refs_to_uses(
                ["hpc_network"] + partitions_references + filesystems_references + ["slurm-sql"]
                )
                slurmdbd_cloudsql_yaml = f"""  - source: community/modules/database/slurm-cloudsql-federation
    kind: terraform
    id: slurm-sql
    use: [hpc_network]
    settings:
      sql_instance_name: sql-{self.cluster.cloud_id}
      tier: "db-g1-small"
    """
            else:
                slurmdbd_cloudsql_yaml = ""

            with yaml_file.open("w") as f:
                f.write(
            f"""
blueprint_name: {self.cluster.cloud_id}

vars:
  project_id: {project_id}
  deployment_name: {self.cluster.cloud_id}
  region: {self.cluster.cloud_region}
  zone: {self.cluster.cloud_zone}
  enable_reconfigure: True
  enable_cleanup_compute: False
  enable_cleanup_subscriptions: True
  enable_bigquery_load: {self.cluster.use_bigquery}
  instance_image_custom: True
  labels:
    created_by: {SITE_NAME}

deployment_groups:
- group: primary
  modules:
  - source: modules/network/pre-existing-vpc
    kind: terraform
    settings:
      network_name: {self.cluster.subnet.vpc.cloud_id}
      subnetwork_name: {self.cluster.subnet.cloud_id}
    id: hpc_network

{filesystems_yaml}

  - source: community/modules/project/service-account
    kind: terraform
    id: hpc_service_account
    settings:
      project_id: {project_id}
      name: {controller_sa}
      project_roles:
      - compute.instanceAdmin.v1
      - iam.serviceAccountUser
      - monitoring.metricWriter
      - logging.logWriter
      - storage.objectAdmin
      - pubsub.admin
      - compute.securityAdmin
      - iam.serviceAccountAdmin
      - resourcemanager.projectIamAdmin
      - compute.networkAdmin

{partitions_yaml}

{slurmdbd_cloudsql_yaml}

  - source: community/modules/scheduler/schedmd-slurm-gcp-v5-controller
    kind: terraform
    id: slurm_controller
    settings:
      cloud_parameters:
        resume_rate: 0
        resume_timeout: 500
        suspend_rate: 0
        suspend_timeout: 300
        no_comma_params: false
      machine_type: {self.cluster.controller_instance_type}
      disk_type: {self.cluster.controller_disk_type}
      disk_size_gb: {self.cluster.controller_disk_size}
      {controller_image_yaml}
      service_account:
        email: $(hpc_service_account.service_account_email)
        scopes:
        - https://www.googleapis.com/auth/cloud-platform
        - https://www.googleapis.com/auth/monitoring.write
        - https://www.googleapis.com/auth/logging.write
        - https://www.googleapis.com/auth/devstorage.read_write
        - https://www.googleapis.com/auth/pubsub
      controller_startup_script: |
        #!/bin/bash
        echo "******************************************** CALLING CONTROLLER STARTUP"
        gsutil cp gs://{startup_bucket}/clusters/{self.cluster.id}/bootstrap_controller.sh - | bash
      compute_startup_script: |
        #!/bin/bash
        gsutil cp gs://{startup_bucket}/clusters/{self.cluster.id}/bootstrap_compute.sh - | bash
    use:
{controller_uses}

  - source: community/modules/scheduler/schedmd-slurm-gcp-v5-login
    kind: terraform
    id: slurm_login
    settings:
      num_instances: {self.cluster.num_login_nodes}
      subnetwork_self_link: {self.cluster.subnet.cloud_id}
      machine_type: {self.cluster.login_node_instance_type}
      disk_type: {self.cluster.login_node_disk_type}
      disk_size_gb: {self.cluster.login_node_disk_size}
      {login_image_yaml}
      service_account:
        email: $(hpc_service_account.service_account_email)
        scopes:
        - https://www.googleapis.com/auth/cloud-platform
        - https://www.googleapis.com/auth/monitoring.write
        - https://www.googleapis.com/auth/logging.write
        - https://www.googleapis.com/auth/devstorage.read_write
      startup_script: |
        #!/bin/bash
        echo "******************************************** CALLING LOGIN STARTUP"
        gsutil cp gs://{startup_bucket}/clusters/{self.cluster.id}/bootstrap_login.sh - | bash
    use:
    - slurm_controller
{login_uses}

    """
                )
        # pylint: enable=line-too-long
        
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
            with log_out_fn.open("wb") as log_out:
                with log_err_fn.open("wb") as log_err:
                    subprocess.run(
                        [self.ghpc_path, "create", "cluster.yaml","-w"],
                        cwd=target_dir,
                        stdout=log_out,
                        stderr=log_err,
                        check=True,
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
        tf_nodes = self._get_tf_state_resource(state, filters)[0]["instances"]

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

        filters = {
            "module": "module.slurm_controller.module.slurm_controller_instance.module.slurm_controller_instance",  #pylint:disable=line-too-long
            "name": "slurm_instance",
        }
        tf_node = self._get_tf_state_resource(tf_state, filters)[0]["instances"][0]  #pylint:disable=line-too-long
        ctrl_sa = tf_node["attributes"]["service_account"][0]["email"]

        filters = {
            "module": "module.slurm_login.module.slurm_login_instance.module.slurm_login_instance",  #pylint:disable=line-too-long
            "name": "slurm_instance",
        }
        tf_node = self._get_tf_state_resource(tf_state, filters)[0]["instances"][0]  #pylint:disable=line-too-long
        login_sa = tf_node["attributes"]["service_account"][0]["email"]

        return {"controller": ctrl_sa, "login": login_sa, "compute": login_sa}

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

                mgmt_nodes = self._create_model_instances_from_tf_state(
                    state,
                    {
                        "module": "module.slurm_controller.module.slurm_controller_instance.module.slurm_controller_instance",  # pylint: disable=line-too-long
                        "name": "slurm_instance",
                    },
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

                login_nodes = self._create_model_instances_from_tf_state(
                    state,
                    {
                        "module": "module.slurm_login.module.slurm_login_instance.module.slurm_login_instance",   # pylint: disable=line-too-long
                        "name": "slurm_instance",
                    },
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
