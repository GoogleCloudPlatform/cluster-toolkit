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
import re
import secrets

from django.template import engines as template_engines
from django.utils import timezone
from google.api_core import exceptions as gcp_exceptions
from google.api_core.exceptions import PermissionDenied as GCPPermissionDenied
from website.settings import SITE_NAME

from . import c2
from . import cloud_info
from . import credentials
from . import utils

from .. import grafana
from ..models import Cluster, ApplicationInstallationLocation, ComputeInstance, ContainerRegistry
from ..models import MAX_SERVICE_ACCOUNT_SLOTS

logger = logging.getLogger(__name__)

# Account names of a profiles cluster. modules/project/service-account names
# an account <deployment_name>-<name> in at most 30 characters, and a cluster's
# deployment name (cloud_id) can be 26, which leaves 3 for each name.
CONTROLLER_ACCOUNT = "ctl"
LOGIN_ACCOUNT = "lgn"
COMPUTE_ACCOUNT = "cmp"


def partition_account_name(slot):
    """c00 to c99: the account of the partition holding slot
    (ClusterPartition.service_account_slot)"""
    if not 0 <= slot < MAX_SERVICE_ACCOUNT_SLOTS:
        raise ValueError(
            f"At most {MAX_SERVICE_ACCOUNT_SLOTS} partitions of a cluster "
            "can have their own service account"
        )
    return f"c{slot:02d}"


def new_role_id_suffix(previous=""):
    """6 hex characters for the role_id_suffix of the profile modules,
    never previous"""
    while True:
        suffix = secrets.token_hex(3)
        if suffix != previous:
            return suffix


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

    def prepare(self):
        """Prepares the cluster for deployment.

        Nothing is prepared here any more: start_cluster() creates the
        cluster directory and its SSH key, and every cloud run gets the
        cluster's credential from cluster_manager/credentials.py.
        """
        #self._create_cluster_dir()
        #self.update()

    def update(self):
        self._prepare_ghpc_yaml()
        self._prepare_bootstrap_gcs()

    def _refresh_role_id_suffix(self):
        """Give a profiles cluster new custom role ids when it needs them.

        A destroy soft-deletes the cluster's custom roles, and Google keeps a
        deleted role id for 7 to 44 days, so every start after one needs new
        role ids. A retry after a failed create keeps the suffix, so Terraform
        finds the roles it already made. Call before the status changes.
        """
        if self.cluster.uses_service_account_profiles and (
            not self.cluster.role_id_suffix
            or self.cluster.status == "d"
            or self.cluster.cloud_state == "xm"
        ):
            self.cluster.role_id_suffix = new_role_id_suffix(
                self.cluster.role_id_suffix
            )

    def start_cluster(self):
        self._refresh_role_id_suffix()
        # A Retry of a cluster whose earlier apply succeeded, or whose
        # destroy started, must not forget that its resources exist.
        if self.cluster.cloud_state not in ("m", "dm"):
            self.cluster.cloud_state = "nm"
        self.cluster.status = "c"
        self.cluster.save()

        # Everything after the status change is inside the try, so a cluster
        # whose blueprint cannot be prepared ends failed, not "Being created"
        try:
            self._create_cluster_dir()
            self._create_ssh_key(self.cluster_dir)
            self._regenerate("create")
            self._preflight("create")
            self._run_ghpc()
            self._initialize_terraform()
            self._apply_terraform()

            dash = grafana.create_cluster_dashboard(self.cluster)
            self.cluster.grafana_dashboard_url = dash.get("url", "")
            self.cluster.save()

        # Not a lot we can do if terraform fails, it's on the admin user to
        # investigate and fix the errors shown in the log.
        # cloud_state is left alone: it becomes "m" as soon as the apply
        # succeeds, so a later failure keeps the cluster managed.
        except Exception:  # pylint: disable=broad-except
            self.cluster.status = "e"
            self.cluster.save()
            raise

    def reconfigure_cluster(self):
        """Regenerate the blueprint and apply it to the running cluster"""
        try:
            self._regenerate("update")
            self._preflight("update")
            self._run_ghpc()
            self._initialize_terraform()
            self._apply_terraform()
            self.cluster.status = "r"
            self.cluster.cloud_state = "m"
            self.cluster.save()

        # Not a lot we can do if terraform fails, it's on the admin user to
        # investigate and fix the errors shown in the log.
        # The cluster was running before this reconfigure and still exists,
        # so it stays managed.
        except Exception:  # pylint: disable=broad-except
            self.cluster.status = "e"
            self.cluster.save()
            raise

    def _regenerate(self, verb):
        """update(), adding why it failed to the cluster's log page.

        A blueprint that cannot be rendered (a template error, or an identity
        the cluster cannot have) or startup scripts that cannot be uploaded
        stop the operation `verb` (create or update) before gcluster runs.
        The error is raised again, so the caller marks the cluster failed.
        """
        try:
            self.update()
        except Exception as err:  # pylint: disable=broad-except
            reason = str(err) or type(err).__name__
            credentials.record_refusal(
                self.get_terraform_dir(),
                f"Cannot {self._operation(verb)}: preparing its blueprint or "
                f"startup scripts failed: {reason}",
                "apply",
            )
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

    def _operation(self, verb):
        return f"{verb} cluster {self.cluster.name} ({self.cluster.cloud_id})"

    def _preflight(self, verb):
        """Stop before gcluster or Terraform runs when the cluster's
        credential lacks a permission that `verb` (create, update or destroy)
        needs.

        A profiles cluster that creates its own accounts also needs to
        create service accounts, custom roles and project bindings. When its
        credential's project is not the server's, its blueprint binds its
        accounts on the server's bucket, C2 topic and C2 subscription, so
        their IAM policies are checked too.
        """
        credential = self.cluster.cloud_credential
        creates_accounts = (
            self.cluster.uses_service_account_profiles
            and not self.cluster.own_service_accounts
        )
        key = "cluster_destroy" if verb == "destroy" else "cluster_create"
        resources = []
        server = self.config["server"]
        if (
            verb != "destroy"
            and creates_accounts
            and credential.project_id != server["gcp_project"]
        ):
            project = server["gcp_project"]
            topic = server["c2_topic"]
            subscription = f"{topic}-{c2.get_cluster_sub_id(self.cluster.id)}"
            resources = [
                credentials.ResourceCheck(
                    "bucket",
                    server["gcs_bucket"],
                    credentials.BUCKET_IAM_PERMISSIONS,
                ),
                credentials.ResourceCheck(
                    "topic",
                    f"projects/{project}/topics/{topic}",
                    credentials.TOPIC_IAM_PERMISSIONS,
                ),
                credentials.ResourceCheck(
                    "subscription",
                    f"projects/{project}/subscriptions/{subscription}",
                    credentials.SUBSCRIPTION_IAM_PERMISSIONS,
                    fallback_project=project,
                ),
            ]
        try:
            credentials.preflight(
                credential,
                self._operation(verb),
                credentials.permissions_for(key, profiles=creates_accounts),
                resources=resources,
            )
        except credentials.CredentialError as err:
            # Also on the cluster's Terraform log page
            credentials.record_refusal(
                self.get_terraform_dir(),
                err,
                "destroy" if verb == "destroy" else "apply",
            )
            raise

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
        """Blueprint modules for the cluster's registries.

        Returns the YAML, whether there are any registries, and their module
        ids.
        """
        if not getattr(self.cluster, "use_containers", False):
            return "", False, []

        artifact_registry_yaml = []
        refs = []
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
            refs.append(context["registry_id"])

        return "\n\n".join(artifact_registry_yaml), has_registries, refs

    def _prepare_ghpc_partitions(self, part_uses, identity):
        partitions_yaml = []
        refs = []
        template = self.env.get_template('blueprint/partition_config.yaml.j2')

        for part in self.cluster.partitions.all():
            part_identity = identity["partitions"].get(part.id, {})
            # A nodeset gets its account from the profile module it uses,
            # which also makes it wait until the account's grants propagate
            part_refs = part_uses + (
                [part_identity["profile"]] if "profile" in part_identity else []
            )
            uses_str = self._yaml_refs_to_uses(part_refs, indent_level=1)
            disk_range = list(range(part.additional_disk_count))
            exclusive = 'True' if part.enable_placement or not part.enable_node_reuse else 'False'
            # The nodeset module defaults on_host_maintenance to TERMINATE, which
            # GCP rejects for non-preemptible e2 instances. Emit MIGRATE for e2
            # partitions instead. GPU nodes must keep TERMINATE (GPUs cannot
            # live-migrate). The partition form (forms.py ClusterPartitionForm.clean)
            # already refuses enable_placement for e2, so MIGRATE here can never
            # silently deactivate a placement group.
            machine_family = part.machine_type.split("-")[0]
            use_migrate_maintenance = (
                machine_family == "e2" and part.GPU_per_node == 0
            )
            context = {
                'part': part,
                'part_id': f"partition_{part.id}",
                'uses_str': uses_str,
                'cluster': self.cluster,
                'disk_range': disk_range,
                'exclusive': exclusive,
                'use_migrate_maintenance': use_migrate_maintenance,
                "startup_bucket": self.config["server"]["gcs_bucket"],
                "identity": identity,
                "part_identity": part_identity,
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

    def _identity_context(self):
        """Which service accounts the blueprint creates and grants.

        A legacy cluster gets {"profiles": False, "partitions": {}}, and its
        templates render as they did before service-account profiles.

        A profiles cluster gets separate controller, login and compute
        accounts (modules/project/service-account, with no project roles),
        each granted by a service-account-profiles block. A partition with
        separate_service_account gets its own compute account. "partitions"
        maps each partition id to the profile its nodeset uses.

        With bring-your-own accounts ("own"), no account or profile block is
        rendered: the nodes use the given e-mails, and OFE grants nothing.
        """
        cluster = self.cluster
        if not cluster.uses_service_account_profiles:
            return {"profiles": False, "partitions": {}}

        if not cluster.slurm_cluster_name:
            raise ValueError(
                f"Cluster {cluster.id} uses service-account profiles but has "
                "no slurm_cluster_name"
            )
        partitions = list(cluster.partitions.order_by("id"))
        identity = {
            "profiles": True,
            "slurm_cluster_name": cluster.slurm_cluster_name,
            "own": cluster.own_service_accounts,
        }
        if identity["own"]:
            if not all(identity["own"].values()):
                raise ValueError(
                    f"Cluster {cluster.id} sets only some of its own service "
                    "accounts; set all three or none"
                )
            identity["partitions"] = {
                part.id: {"email": identity["own"]["compute"]}
                for part in partitions
            }
            return identity

        account_of = {}
        for part in partitions:
            if part.separate_service_account:
                # A stored slot, so the name never moves to another partition
                if part.service_account_slot is None:
                    part.save()  # assigns the slot
                account_of[part.id] = partition_account_name(
                    part.service_account_slot
                )
            else:
                account_of[part.id] = COMPUTE_ACCOUNT
        # The shared compute account first, then c00, c01, ...; an account no
        # partition uses is not created.
        compute_accounts = sorted(
            set(account_of.values()),
            key=lambda name: (name != COMPUTE_ACCOUNT, name),
        )

        server = self.config["server"]
        identity.update({
            "role_id_suffix": cluster.role_id_suffix,
            "controller": CONTROLLER_ACCOUNT,
            "login": LOGIN_ACCOUNT,
            "compute": compute_accounts,
            "partitions": {
                part_id: {"profile": f"{name}_profile"}
                for part_id, name in account_of.items()
            },
            # The controller profile turns placement on when unset, so it is
            # always rendered: true only if some partition uses placement
            "enable_placement_policies": any(
                part.enable_placement for part in partitions
            ),
            "enable_bigquery_load": cluster.use_bigquery,
            # Grants that OFE made in Python for legacy clusters: every node
            # reads its bootstrap script from the server's bucket and writes
            # logs there, and the controller's command-and-control daemon
            # pulls from the cluster's subscription and replies on the topic.
            "bucket": server["gcs_bucket"],
            "server_project": server["gcp_project"],
            "c2_topic": server["c2_topic"],
            # c2.get_cluster_subscription_path() without the project, which
            # needs the running C2 client
            "c2_subscription": (
                f"{server['c2_topic']}-{c2.get_cluster_sub_id(cluster.id)}"
            ),
            "registries": [],
        })
        return identity

    def _prepare_ghpc_yaml(self):
        try:
            yaml_file = self.cluster_dir / "cluster.yaml"
            project_id = self.cluster.cloud_credential.project_id
            identity = self._identity_context()
            filesystems_yaml, filesystems_refs = self._prepare_ghpc_filesystems()
            partitions_yaml, partitions_refs = self._prepare_ghpc_partitions(
                filesystems_refs, identity
            )
            artifact_registry_yaml, use_containers, registry_refs = (
                self._prepare_ghpc_artifact_registry()
            )
            cloudsql_yaml, cloudsql_refs = self._prepare_cloudsql_yaml()
            if identity["profiles"]:
                # Container jobs pull with the controller's access token
                identity["registries"] = registry_refs

            # Use a template to generate the final YAML configuration
            template = self.env.get_template('blueprint/cluster_config.yaml.j2')
            controller_uses_refs = ["hpc_network"] + partitions_refs + filesystems_refs + cloudsql_refs
            if identity["profiles"] and not identity["own"]:
                controller_uses_refs.append(f"{CONTROLLER_ACCOUNT}_profile")
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
                "identity": identity,
                "state_bucket": utils.get_terraform_state_bucket(),
                "state_prefix": utils.terraform_state_prefix(
                    "cluster", self.cluster.id
                ),
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
            raise

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
        cloud = {
            "credential": self.cluster.cloud_credential,
            "operation": self._operation("deploy"),
        }
        try:
            logger.info("Invoking Terraform Init")
            # init reads the state bucket, so it acts as the credential too
            utils.run_terraform(terraform_dir, "init", **cloud)
            utils.run_terraform(terraform_dir, "validate", **cloud)
            logger.info("Invoking Terraform Plan")
            utils.run_terraform(terraform_dir, "plan", **cloud)
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

            # The gcluster validators read only a key file. With an
            # impersonate credential they run as the server's own account,
            # at WARNING level.
            with credentials.tool_environment(
                self.cluster.cloud_credential, key_file=True
            ) as env, log_out_fn.open("wb") as log_out, log_err_fn.open(
                "wb"
            ) as log_err:
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

    def _get_profile_service_accounts(self, tf_state):
        """A profiles cluster's account e-mails, by role.

        Keys are "controller", "login" and "compute", plus the c00-c99 names
        of partition accounts. Bring-your-own accounts come from the cluster;
        the others from the service-account modules in tf_state.
        """
        own = self.cluster.own_service_accounts
        if own:
            return dict(own)

        roles = {
            CONTROLLER_ACCOUNT: "controller",
            LOGIN_ACCOUNT: "login",
            COMPUTE_ACCOUNT: "compute",
        }
        service_accounts = {}
        for resource in self._get_tf_state_resource(
            tf_state, {"type": "google_service_account"}
        ):
            match = re.fullmatch(
                r"module\.([a-z0-9]+)_sa\.module\.service_account",
                resource.get("module", ""),
            )
            if not match:
                continue
            for instance in resource.get("instances", []):
                email = instance.get("attributes", {}).get("email")
                if email:
                    name = match.group(1)
                    service_accounts[roles.get(name, name)] = email
        return service_accounts

    def _get_service_accounts(self, tf_state):
        if self.cluster.uses_service_account_profiles:
            return self._get_profile_service_accounts(tf_state)

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
        if self.cluster.uses_service_account_profiles:
            # The blueprint's profile blocks grant these (bucket_bindings and
            # the Pub/Sub bindings), and bring-your-own accounts get nothing
            # from OFE, so a profiles cluster makes no IAM call here.
            logger.info(
                "Cluster %s runs as %s; its grants are in its blueprint",
                self.cluster.cloud_id,
                service_accounts,
            )
            return

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

        # Before the first apply: an apply that fails part way can leave
        # resources while the cloud state stays "nm", and a destroy without
        # the Terraform folder must then not take a missing state to mean
        # nothing was deployed (_ever_applied). Never cleared: set with an
        # update of that column alone, and Cluster.save() keeps it.
        if not self.cluster.terraform_started:
            self.cluster.set_terraform_started()

        try:
            logger.info("Invoking Terraform Apply")
            utils.run_terraform(
                terraform_dir,
                "apply",
                credential=self.cluster.cloud_credential,
                operation=self._operation("deploy"),
            )

            # The apply succeeded, so the cluster's resources exist. Record
            # that before anything below can fail (reading the state back,
            # registry or node bookkeeping), so the cluster stays managed.
            self.cluster.internal_name = self.cluster.name
            self.cluster.cloud_state = "m"
            self.cluster.save()

            # Look for Management and Login Nodes in the Terraform state
            state = utils.get_terraform_state(
                terraform_dir,
                credential=self.cluster.cloud_credential,
                operation=self._operation("deploy"),
            )

            # Extract and save Artifact Registry repository and secret information (if any)
            self.extract_and_update_registry_info(state)

            # Apply Perms to the service accounts
            try:
                service_accounts = self._get_service_accounts(state)
                self._apply_service_account_permissions(service_accounts)
            except Exception as e:
                # Be nicer to the user and continue creating cluster
                logger.warning(f"An error occurred while applying permissions to service accounts: {e}")


            # Cluster initialization is now running. Stamp the start time
            # so a bootstrap that stalls (controller never reports ready)
            # can be surfaced instead of spinning on "initialising".
            self.cluster.status = "i"
            self.cluster.initialization_started = timezone.now()
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
        # Before the status changes: a cluster that cannot be destroyed keeps
        # the status it has
        self._preflight("destroy")
        # Terraform destroys from the cluster's Terraform folder (the
        # deployment folder's "primary" group), which gcluster writes and
        # never removes (a rewrite only moves its groups aside). Without it
        # (a create that stopped before gcluster ran, or a server rebuilt
        # with a restored database), the state in the bucket decides, also
        # before the status changes.
        nothing_left = None
        if not terraform_dir.is_dir():
            nothing_left = self._empty_state_without_terraform_folder(
                terraform_dir
            )
        # Read before the cloud state becomes "dm" below
        ever_applied = self._ever_applied()
        try:
            self.cluster.status = "t"
            self.cluster.cloud_state = "dm"
            self.cluster.save()

            if nothing_left:
                logger.info(
                    "Cluster %s (%s) has no Terraform folder %s and %s, so "
                    "nothing is left to destroy; removing it without running "
                    "Terraform",
                    self.cluster.id,
                    self.cluster.cloud_id,
                    terraform_dir,
                    nothing_left,
                )
                # Its state holds no resources, but the compute nodes
                # slurm-gcp made were never in the state. Only a cluster
                # that was deployed can have them.
                if ever_applied:
                    self._sweep_orphaned_compute_nodes()
            else:
                logger.info("Invoking Terraform destroy")
                try:
                    self._run_destroy(terraform_dir)
                except subprocess.CalledProcessError as err:
                    # Everything but the cluster's custom roles (and what
                    # they keep) is destroyed: the teardown finishes and the
                    # log page says what is left and how to delete it. Any
                    # other failure stops the teardown.
                    left = self._custom_roles_left(terraform_dir)
                    if left is None:
                        raise
                    self._report_custom_roles_left(terraform_dir, left, err)

                # Dynamic compute nodes are created directly via the GCP API
                # by slurm-gcp at job-schedule time, so they are not in
                # Terraform state and can survive the destroy above (still
                # running and billing). Sweep any that carry this cluster's
                # Slurm label.
                self._sweep_orphaned_compute_nodes()

            # Mark Container Registry objects as deleted
            registries = ContainerRegistry.objects.filter(cluster=self.cluster)
            registry_count = 0
            for registry in registries:
                registry.status = "d"
                registry.save(update_fields=["status"])
                registry_count += 1

            # A cluster whose start_cluster() failed before recording its
            # controller has no controller_node; tearing it down must not
            # crash on that, or the status and cleanup below are skipped
            # and the cluster stays in "t".
            controller_sa = None
            if self.cluster.controller_node is not None:
                controller_sa = self.cluster.controller_node.service_account
                self.cluster.controller_node.delete()
            self.cluster.login_nodes.all().delete()
            # Refresh so our python object gets the SET_NULL's from the above
            # deletes
            self.cluster = Cluster.objects.get(id=self.cluster.id)

            self.cluster.status = "d"
            self.cluster.cloud_state = "xm"
            self.cluster.save()

            if nothing_left:
                self._delete_subscription_without_terraform_folder(
                    controller_sa
                )
                logger.info("Cluster %s removed", self.cluster.id)
            else:
                c2.delete_cluster_subscription(self.cluster.id, controller_sa)
                logger.info("Terraform destroy completed")

        except subprocess.CalledProcessError as err:
            logger.error("Terraform destroy failed", exc_info=err)
            if err.stdout:
                logger.info("TF stdout:\n%s\n", err.stdout.decode("utf-8"))
            if err.stderr:
                logger.info("TF stderr:\n%s\n", err.stderr.decode("utf-8"))
            raise

    def _delete_subscription_without_terraform_folder(self, controller_sa):
        """Delete the C2 subscription of a cluster removed without Terraform
        because its Terraform folder is missing.

        The subscription is made just before the first apply, so a create
        that stopped before gcluster ran has none (NotFound, logged as
        already gone); a server rebuilt with a restored database can have
        lost the folder of a cluster that has one. Any other error is
        logged and raised, as a failed delete on the normal path is: the
        cluster is already deleted, and the subscription is left.
        """
        try:
            c2.delete_cluster_subscription(self.cluster.id, controller_sa)
        except gcp_exceptions.NotFound:
            logger.info(
                "Cluster %s has no C2 subscription to delete",
                self.cluster.id,
            )
        except Exception:
            logger.error(
                "Could not delete the C2 subscription of cluster %s",
                self.cluster.id,
                exc_info=True,
            )
            raise

    def _ever_applied(self):
        """Why the cluster may have resources from a Terraform apply (for a
        message), or None: its cloud state says so ("m", or "dm" for a
        destroy that started), it has a controller recorded, which only a
        finished apply records, or a Terraform apply was started for it
        (terraform_started, set before the first apply and never cleared, so
        also after an apply that failed part way; True for clusters from
        before the flag existed, which a set flag cannot be told apart
        from, so the reason names both)"""
        if self.cluster.cloud_state in ("m", "dm"):
            return f'its cloud state is "{self.cluster.cloud_state}"'
        if self.cluster.controller_node_id is not None:
            return "it has a controller recorded"
        if self.cluster.terraform_started:
            return (
                "a Terraform apply was started for it, or the cluster "
                "predates the started-flag check, so OFE treats it as "
                "deployed"
            )
        return None

    def _empty_state_without_terraform_folder(self, terraform_dir):
        """For a cluster whose Terraform folder terraform_dir is missing (or
        not a directory), return why nothing is left in the cloud (for the
        log), or refuse.

        The state is read from the state bucket as the cluster's credential.
        Terraform runs only in a group folder gcluster wrote, so state
        objects that hold no resources mean nothing is left in the cloud. No
        state object at all means that only for a cluster that never started
        an apply (_ever_applied() is None): for one that did, its state is
        somewhere else (never migrated to the bucket, a server
        under another deployment name, or a restored database with a new
        state namespace), and its resources may still exist.

        Refused when the state holds resources, cannot be read, or is
        missing for a cluster that was deployed: raise
        credentials.CredentialError, also added to the cluster's log page,
        and nothing about the cluster changes.
        """
        credential = self.cluster.cloud_credential
        location = "the Terraform state bucket"
        try:
            bucket = utils.get_terraform_state_bucket()
            prefix = utils.terraform_state_prefix("cluster", self.cluster.id)
            location = f"gs://{bucket}/{prefix.strip('/')}/"
            contents = utils.state_contents(bucket, prefix, credential)
        except Exception as err:  # pylint: disable=broad-except
            # Any failure: without the state there is no knowing whether
            # the cluster still has resources in Google Cloud
            reason = (
                f"its Terraform state in {location} could not be read as "
                f"credential {credential.name}: "
                f"{str(err) or type(err).__name__}. Without the state OFE "
                "cannot tell whether the cluster still has resources in "
                "Google Cloud. Fix the error and destroy the cluster again, "
                "or restore the deployment folder."
            )
            cause = err
        else:
            cause = None
            deployed = self._ever_applied()
            if contents.states and not contents.resources:
                return f"its Terraform state in {location} holds no resources"
            if not contents.states and not deployed:
                return (
                    f"it has no Terraform state in {location} and never "
                    "started a Terraform apply"
                )
            if contents.states:
                count = contents.resources
                reason = (
                    f"its Terraform state in {location} still holds {count} "
                    f"resource{'' if count == 1 else 's'}, which Terraform "
                    "can destroy only from that folder. Restore the "
                    "deployment folder and destroy the cluster again, or "
                    "have an administrator destroy the resources from that "
                    "state."
                )
            else:
                reason = (
                    f"no Terraform state was found under {location}, "
                    f"although the cluster may have been deployed "
                    f"({deployed}), so "
                    "its resources may still exist in Google Cloud. Possible "
                    "causes: its state was never migrated to the state "
                    "bucket, the server was deployed under another "
                    "deployment name (and so uses another state bucket), or "
                    "the database was restored with a new state namespace. "
                    "See \"When a resource's state is not in the bucket\" "
                    "in docs/admin_guide.md to find the state or delete the "
                    "resources, then destroy the cluster again."
                )
        missing = "is not a folder" if terraform_dir.exists() else "is missing"
        refusal = credentials.CredentialError(
            f"Cannot {self._operation('destroy')}: its Terraform folder "
            f"{terraform_dir} {missing}, and {reason} The cluster keeps its "
            "status."
        )
        logger.error("%s", refusal)
        # Also on the cluster's Terraform log page
        credentials.record_refusal(terraform_dir, refusal, "destroy")
        raise refusal from cause

    def _run_destroy(self, terraform_dir):
        utils.run_terraform(
            terraform_dir,
            "destroy",
            credential=self.cluster.cloud_credential,
            operation=self._operation("destroy"),
        )

    @staticmethod
    def _destroy_errors(terraform_dir):
        try:
            return (terraform_dir / "terraform_destroy_log.stderr").read_text(
                encoding="utf-8", errors="replace"
            )
        except OSError as err:
            logger.warning("Cannot read the Terraform destroy log: %s", err)
            return None

    def _custom_roles_left(self, terraform_dir):
        """The role ids of a destroy that failed only on custom IAM roles.

        Google Cloud can fail to delete a profiles cluster's custom roles (an
        IAM API error, a deny policy) after Terraform has destroyed
        everything else; the roles depend on nothing else the cluster has.
        That is the case when every error in the destroy's stderr is the
        Google provider's role-delete error, about a
        google_project_iam_custom_role, and the state Terraform wrote
        afterwards holds those roles, all in the credential's project, and
        nothing else that exists in Google Cloud apart from what the roles
        keep: the Slurm files bucket and the cluster's container registry
        repositories with their password secrets. Then a dict of sorted
        lists: "role_ids", "buckets" (names), "repositories" ((repository
        id, location)) and "secrets" (secret ids); for any other failure, or
        a log or state that cannot be read, None.
        """
        role_type = "google_project_iam_custom_role"
        # Types that make nothing in Google Cloud. A role that Terraform could
        # not delete keeps what it depends on, such as its module's
        # terraform_data input checks.
        no_cloud_types = ("terraform_data", "null_", "random_", "time_")
        # A role also keeps the Slurm files bucket the blueprint creates: the
        # profiles' input checks, which the roles depend on, read the
        # bucket_bindings that name it.
        slurm_bucket = ("module.slurm_bucket", "google_storage_bucket")
        buckets = set()
        # Likewise the cluster's container registries (registry_<id> in the
        # blueprint), which the controller profile's
        # artifact_registry_bindings name, and a registry's password secret,
        # which its repository depends on.
        registry_module = re.compile(r"module\.registry_[0-9]+")
        repositories = set()
        secrets_left = set()
        registry_projects = set()
        errors = self._destroy_errors(terraform_dir)
        if errors is None:
            return None
        # The state is in the state bucket, so it is read as the credential
        try:
            state = utils.get_terraform_state(
                terraform_dir,
                credential=self.cluster.cloud_credential,
                operation=self._operation("destroy"),
            )
        except (OSError, RuntimeError, subprocess.CalledProcessError) as err:
            logger.warning("Cannot read the state Terraform destroy left: %s", err)
            return None

        # Terraform -no-color prints each error as "Error: <summary>" at the
        # start of a line, then "  with <address>," naming its resource.
        blocks = re.split(r"^(?=Error: )", errors, flags=re.MULTILINE)
        if blocks[0].strip():
            return None
        failed = set()
        for block in blocks[1:]:
            address = re.search(r"^  with (\S+),$", block, flags=re.MULTILINE)
            if not (
                address
                and block.startswith(
                    "Error: Error deleting the custom project role "
                )
            ):
                return None
            failed.add(address.group(1))
        if not failed or not isinstance(state, dict):
            return None

        roles = {}
        for resource in state.get("resources") or []:
            if resource.get("mode") != "managed":
                continue
            resource_type = resource.get("type") or ""
            base = f"{resource_type}.{resource.get('name')}"
            if resource.get("module"):
                base = f"{resource['module']}.{base}"
            for instance in resource.get("instances") or []:
                key = instance.get("index_key")
                if isinstance(key, int):
                    address = f"{base}[{key}]"
                elif isinstance(key, str):
                    address = f"{base}[{json.dumps(key)}]"
                else:
                    address = base
                attributes = instance.get("attributes") or {}
                if resource_type == role_type and address in failed:
                    roles[address] = attributes
                elif (resource.get("module"), resource_type) == slurm_bucket:
                    name = attributes.get("name")
                    if not name:
                        return None
                    buckets.add(name)
                elif registry_module.fullmatch(resource.get("module") or ""):
                    if resource_type == "google_artifact_registry_repository":
                        repository_id = attributes.get("repository_id")
                        location = attributes.get("location")
                        if not (repository_id and location):
                            return None
                        repositories.add((repository_id, location))
                    elif resource_type == "google_secret_manager_secret":
                        secret_id = attributes.get("secret_id")
                        if not secret_id:
                            return None
                        secrets_left.add(secret_id)
                    elif resource_type == "google_secret_manager_secret_version":
                        # Deleted with its secret
                        continue
                    elif not resource_type.startswith(no_cloud_types):
                        return None
                    registry_projects.add(attributes.get("project"))
                elif not resource_type.startswith(no_cloud_types):
                    return None
        if set(roles) != failed:
            return None

        # The gcloud command names the credential's project
        project = self.cluster.cloud_credential.project_id
        if not project:
            return None
        role_ids = set()
        for attributes in roles.values():
            role_id = attributes.get("role_id")
            if not role_id or attributes.get("project") not in (None, project):
                return None
            role_ids.add(role_id)
        # The registry commands name the credential's project too
        if not registry_projects <= {None, project}:
            return None
        return {
            "role_ids": sorted(role_ids),
            "buckets": sorted(buckets),
            "repositories": sorted(repositories),
            "secrets": sorted(secrets_left),
        }

    def _report_custom_roles_left(self, terraform_dir, left, err):
        """Say which custom roles (and what they kept) a finished teardown
        left, and how to delete them, in the Django log and at the end of
        the destroy log that the cluster's Terraform log page shows."""
        project = self.cluster.cloud_credential.project_id
        role_ids = left["role_ids"]
        buckets = left["buckets"]
        commands = "; ".join(
            f"gcloud iam roles delete {role_id} --project={project}"
            for role_id in role_ids
        )
        message = (
            f"Cluster {self.cluster.name} ({self.cluster.cloud_id}) is "
            "destroyed except for custom IAM role(s) "
            f"{', '.join(role_ids)} in project {project}, which Google Cloud "
            "did not delete (the errors are above). The "
            "roles have no members: the cluster's service accounts and IAM "
            "bindings are destroyed. Delete them as an identity with "
            f"iam.roles.delete in {project}: {commands}"
        )
        if buckets:
            bucket_commands = "; ".join(
                f"gcloud storage rm --recursive gs://{bucket}" for bucket in buckets
            )
            message += (
                f". Terraform also kept the cluster's Slurm files bucket(s) "
                f"{', '.join(buckets)}, which hold only its Slurm "
                "configuration, because the roles depend on them. Delete "
                f"them too: {bucket_commands}"
            )
        if left["repositories"] or left["secrets"]:
            registry_commands = [
                f"gcloud artifacts repositories delete {repository_id} "
                f"--location={location} --project={project} --quiet"
                for repository_id, location in left["repositories"]
            ] + [
                f"gcloud secrets delete {secret_id} --project={project} --quiet"
                for secret_id in left["secrets"]
            ]
            kept = [
                f"container registry repository {repository_id} ({location})"
                for repository_id, location in left["repositories"]
            ] + [f"registry password secret {secret_id}" for secret_id in left["secrets"]]
            message += (
                f". Terraform also kept the cluster's {', '.join(kept)}, "
                "because the roles depend on them; a repository holds only "
                "the cluster's container images. Delete them too: "
                f"{'; '.join(registry_commands)}"
            )
        logger.warning("%s (%s)", message, err)
        try:
            with (terraform_dir / "terraform_destroy_log.stderr").open(
                "a", encoding="utf-8"
            ) as logfp:
                logfp.write(f"\nOFE: {message}\n")
        except OSError as log_err:
            logger.warning(
                "Could not add the custom role message to the Terraform log: %s",
                log_err,
            )

    def _sweep_orphaned_compute_nodes(self):
        """Delete any leftover compute instances still labelled for this cluster.

        Best-effort safety net: slurm-gcp creates burst compute nodes directly
        through the GCP API, so they are not tracked by Terraform and a destroy
        (or the controller dying mid-teardown) can leave them running. Failures
        here must never break teardown, which has already completed.
        """
        cluster_name = None
        try:
            from google.cloud import compute_v1

            project_id = self.cluster.project_id
            # slurm-gcp labels every node it creates with its Slurm cluster
            # name. A profiles cluster sets that name explicitly. A legacy
            # cluster leaves it to the controller, which derives it from the
            # deployment name, so cloud_id is not it; that is unchanged here.
            if self.cluster.uses_service_account_profiles:
                cluster_name = self.cluster.slurm_cluster_name
            else:
                cluster_name = self.cluster.cloud_id
            if not project_id or not cluster_name:
                return

            client = compute_v1.InstancesClient(
                credentials=credentials.python_credentials(
                    self.cluster.cloud_credential
                )
            )
            # No return_partial_success: google-cloud-compute 1.55 and later,
            # including the pinned version, reject it as an unknown field.
            request = compute_v1.AggregatedListInstancesRequest(
                project=project_id,
                filter=f"labels.slurm_cluster_name={cluster_name}",
            )

            deleted = 0
            for zone_scope, scoped in client.aggregated_list(request=request):
                for instance in getattr(scoped, "instances", []) or []:
                    # Skip nodes GCP is already tearing down.
                    if instance.status in ("STOPPING", "TERMINATED"):
                        continue
                    zone = zone_scope.split("/")[-1]
                    logger.warning(
                        "Deleting orphaned compute node %s in %s "
                        "(cluster %s left it running after destroy)",
                        instance.name, zone, cluster_name,
                    )
                    # One failed delete (transient API error, node already
                    # going away) must not abandon the rest of the sweep.
                    try:
                        client.delete(
                            project=project_id, zone=zone, instance=instance.name
                        )
                        deleted += 1
                    except Exception as delete_err:  # noqa: BLE001
                        logger.warning(
                            "Failed to delete orphaned node %s in %s: %s",
                            instance.name, zone, delete_err,
                        )

            if deleted:
                logger.warning(
                    "Swept %d orphaned compute node(s) for cluster %s",
                    deleted, cluster_name,
                )
        except Exception as err:  # noqa: BLE001 - best-effort cleanup
            # Teardown has finished, but compute nodes may still be running
            # and billing; nothing retries the sweep, so say how to find them.
            label = (
                f"instances labelled slurm_cluster_name={cluster_name}"
                if cluster_name
                else "the cluster's instances"
            )
            logger.error(
                "Orphaned compute node sweep failed for cluster %s (%s): %s. "
                "Leftover compute nodes may still be running; find %s in "
                "project %s and delete them by hand.",
                self.cluster.name, self.cluster.cloud_id, err, label,
                self.cluster.project_id,
            )

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
