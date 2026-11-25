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

"""Service-account identities in the generated Slurm cluster blueprint"""

import contextlib
import io
import itertools
import json
import re
from pathlib import Path
from unittest import mock

import yaml
from django.apps.registry import Apps
from django.core.exceptions import ValidationError
from django.db import connection, models
from django.forms.models import model_to_dict
from django.test import SimpleTestCase, TestCase, TransactionTestCase
from django.urls import reverse

from ghpcfe.cluster_manager import utils
from ghpcfe.cluster_manager.clusterinfo import (
    COMPUTE_ACCOUNT,
    CONTROLLER_ACCOUNT,
    LOGIN_ACCOUNT,
    ClusterInfo,
    new_role_id_suffix,
    partition_account_name,
)
from ghpcfe.cluster_manager.image import BUILDER_ACCOUNT_NAME
from ghpcfe.forms import ClusterForm, ClusterPartitionForm
from ghpcfe.models import (
    MAX_SERVICE_ACCOUNT_SLOTS,
    SERVICE_ACCOUNT_EMAIL_VALIDATOR,
    Cluster,
    ClusterPartition,
    ContainerRegistry,
    Credential,
    Filesystem,
    FilesystemExport,
    FilesystemImpl,
    IdentityModel,
    Image,
    MountPoint,
    User,
    VirtualNetwork,
    VirtualSubnet,
    identity_model_field,
    slurm_cluster_name_for,
)
from website.settings import SITE_NAME

PROJECT = "example-project"
REGION = "us-central1"
ZONE = "us-central1-a"
ROLE_ID_SUFFIX = "a1b2c3"

GOLDEN_DIR = Path(__file__).resolve().parent / "fixtures" / "blueprint_identities"
# The blueprint labels resources with the server's site name, which is the
# host name when the tests run off GCE. The golden files hold this marker in
# its place.
SITE_NAME_MARKER = "@SITE_NAME@"

ACCOUNT_SOURCE = "modules/project/service-account"
PROFILE_SOURCE = "community/modules/iam/service-account-profiles"
CLOUD_PLATFORM_SCOPE = "https://www.googleapis.com/auth/cloud-platform"
BUCKET_SOURCE = "modules/file-system/cloud-storage-bucket"
SLURM_BUCKET_NAME = "$(slurm_bucket.gcs_bucket_name)"
# Read access to the Slurm bucket, granted through each profile so its wait
# covers it. Not objectViewer: the bucket module's authoritative objectViewer
# binding (its viewers) would remove such a grant on the next apply.
SLURM_BUCKET_BINDINGS = {
    "slurm_bucket_object_reader": {
        "bucket": SLURM_BUCKET_NAME,
        "role": "roles/storage.legacyObjectReader",
    },
    "slurm_bucket_reader": {"bucket": SLURM_BUCKET_NAME, "role": "roles/storage.legacyBucketReader"},
}
# The TKFE control bucket: every node reads only its bootstrap file
# (clusters/<cluster id>/bootstrap_<role>.sh) and clusters/ansible_setup, and
# only the controller writes, only under clusters/<cluster id>/ (its logs, job
# output and install logs). The rest of the bucket includes
# webserver/startup.sh, which the server runs as root at every boot, and other
# clusters' files.
CONTROL_BUCKET_ROLES = {
    "slurm-controller": "roles/storage.objectViewer",
    "slurm-login": "roles/storage.objectViewer",
    "slurm-compute": "roles/storage.objectViewer",
}
# Bucket roles that cannot create, change or delete an object.
READ_ONLY_BUCKET_ROLES = {
    "roles/storage.objectViewer",
    "roles/storage.legacyObjectReader",
    "roles/storage.legacyBucketReader",
}
# - service-account-profiles variables.tf (bucket_bindings condition title
#   and description)
CONDITION_TITLE_MAX = 100
CONDITION_DESCRIPTION_MAX = 256


def control_bucket_bindings(profile, bucket, cluster_id):
    """The control-bucket entries of a profile's bucket_bindings"""
    bindings = {
        "ofe_bucket": {
            "bucket": bucket,
            "role": CONTROL_BUCKET_ROLES[profile],
            "condition": {
                "title": f"ofe-cluster-{cluster_id}-node-files",
                "description": (
                    "Lists the TKFE control bucket; reads only clusters/ansible_setup, "
                    f"the objects under it and clusters/{cluster_id}/bootstrap_* in it."
                ),
                "expression": node_files_expression(bucket, cluster_id),
            },
        }
    }
    if profile == "slurm-controller":
        bindings["ofe_bucket_cluster_prefix"] = {
            "bucket": bucket,
            "role": "roles/storage.objectUser",
            "condition": {
                "title": f"ofe-cluster-{cluster_id}-prefix",
                "description": (
                    f"Objects under clusters/{cluster_id}/ in the TKFE control bucket only."
                ),
                "expression": cluster_prefix_expression(bucket, cluster_id),
            },
        }
    return bindings


def cluster_prefix_expression(bucket, cluster_id):
    """The IAM condition that limits a grant to clusters/<cluster_id>/"""
    return (
        f'resource.name.startsWith("projects/_/buckets/{bucket}/objects/clusters/{cluster_id}/")'
    )


def node_files_expression(bucket, cluster_id):
    """The IAM condition that limits a node's control-bucket read grant to
    listing the bucket (gcloud storage cp --recursive and the C2 daemon list
    it), the object clusters/ansible_setup (gcloud storage cp --recursive
    may look that name up first), the objects under clusters/ansible_setup/
    and the cluster's own bootstrap files"""
    objects = f"projects/_/buckets/{bucket}/objects"
    return (
        f'resource.name == "projects/_/buckets/{bucket}"'
        f' || resource.name == "{objects}/clusters/ansible_setup"'
        f' || resource.name.startsWith("{objects}/clusters/ansible_setup/")'
        f' || resource.name.startsWith("{objects}/clusters/{cluster_id}/bootstrap_")'
    )


def node_files_cases(bucket, cluster_id):
    """(allowed, refused): resource names a node's control-bucket read
    condition must allow, and ones it must refuse"""
    objects = f"projects/_/buckets/{bucket}/objects"
    allowed = [
        f"projects/_/buckets/{bucket}",
        f"{objects}/clusters/ansible_setup",
        f"{objects}/clusters/ansible_setup/",
        f"{objects}/clusters/ansible_setup/roles/main.yml",
        f"{objects}/clusters/{cluster_id}/bootstrap_controller.sh",
        f"{objects}/clusters/{cluster_id}/bootstrap_login.sh",
    ]
    refused = [
        f"{objects}/clusters/ansible_setup_other",
        f"{objects}/clusters/ansible_setup_other/file",
        f"{objects}/clusters/ansible_setup.tar",
        f"{objects}/clusters/{cluster_id}/controller_logs",
        f"{objects}/clusters/{cluster_id + 1}/bootstrap_controller.sh",
        f"{objects}/clusters/{cluster_id}0/bootstrap_controller.sh",
        f"{objects}/webserver/startup.sh",
        f"projects/_/buckets/{bucket}-other",
        f"projects/_/buckets/{bucket}-other/objects/clusters/ansible_setup",
    ]
    return allowed, refused


_CEL_TERM = re.compile(
    r'resource\.name == "(?P<equals>[^"\\]*)"'
    r'|resource\.name\.startsWith\("(?P<prefix>[^"\\]*)"\)'
)


def cel_terms(expression):
    """[(operator, literal)] for an IAM condition made only of
    resource.name == "..." and resource.name.startsWith("...") joined by
    ||; operator is "==" or "startsWith". Fails on anything else, so a
    condition outside that form is never judged by a guess."""
    terms = []
    for text in expression.split(" || "):
        match = _CEL_TERM.fullmatch(text)
        if match is None:
            raise AssertionError(f"not a resource.name term: {text!r}")
        if match.group("equals") is not None:
            terms.append(("==", match.group("equals")))
        else:
            terms.append(("startsWith", match.group("prefix")))
    return terms


def cel_allows(expression, name):
    """Whether the IAM condition expression is true for resource name"""
    return any(
        name == literal if operator == "==" else name.startswith(literal)
        for operator, literal in cel_terms(expression)
    )


# Input rules of the modules, which gcluster create does not check because
# Terraform enforces them at plan time:
# - schedmd-slurm-gcp-v6-controller and service-account-profiles variables.tf
SLURM_CLUSTER_NAME_PATTERN = r"[a-z]([-a-z0-9]{0,19})"
# - service-account-profiles variables.tf (role_id_suffix)
ROLE_ID_SUFFIX_PATTERN = r"[a-z0-9]{1,8}"
# - modules/project/service-account metadata.yaml (<deployment_name>-<name>)
ACCOUNT_ID_PATTERN = r"[a-z][-a-z0-9]{4,28}[a-z0-9]"
# - service-account-profiles main.tf binding_role_problems
REFUSED_BINDING_ROLE = r"^roles/(owner|editor|viewer)$|^roles/iam[.]|^roles/resourcemanager[.]"

# Capability flags that grant unscoped or unused project-wide permissions,
# and inputs for features the profiles do not cover (Slurm repair, compute
# performMaintenance, controller_network_attachment). OFE renders none.
NEVER_RENDERED = (
    "enable_mig",
    "enable_mig_flex",
    "enable_tpu",
    "enable_tpu_gce",
    "enable_public_ips",
    "controller_network_attachment",
    "enable_opportunistic_maintenance",
    "enable_maintenance_reservation",
    "maintenance_interval",
    "repair",
)

OWN_ACCOUNTS = {
    "controller_service_account_email": f"own-ctl@{PROJECT}.iam.gserviceaccount.com",
    "login_service_account_email": f"own-lgn@{PROJECT}.iam.gserviceaccount.com",
    "compute_service_account_email": f"own-cmp@{PROJECT}.iam.gserviceaccount.com",
}


def make_world(project=PROJECT):
    """An owner, a credential for project and a managed subnet"""
    owner = User.objects.filter(username="blueprint-owner").first()
    if owner is None:
        # The first user gets no default role, so no Role rows are needed
        owner = User.objects.create_user(
            username="blueprint-owner", password="blueprint-pass"
        )
    credential = Credential.objects.create(
        name=f"credential-{project}",
        owner=owner,
        detail=json.dumps({"type": "service_account", "project_id": project}),
    )
    vpc = VirtualNetwork.objects.create(
        name="net-a",
        cloud_id="net-a",
        cloud_credential=credential,
        cloud_region=REGION,
        cloud_state="m",
    )
    subnet = VirtualSubnet.objects.create(
        name="subnet-a",
        cloud_id="subnet-a",
        vpc=vpc,
        cidr="10.10.0.0/24",
        cloud_credential=credential,
        cloud_region=REGION,
        cloud_state="m",
    )
    return owner, credential, subnet


def as_superuser(user):
    """user, made a superuser: only a superuser may set a cluster's
    existing service accounts in the cluster form"""
    user.is_superuser = True
    user.save(update_fields=["is_superuser"])
    return user


def make_cluster(world, pk, name, suffix, identity_model=None, **fields):
    """A cluster in status "n" whose cloud_id is <name>-<suffix>.

    identity_model None leaves the field alone, which is how the legacy golden
    files were captured from models that had no such field.
    """
    owner, credential, subnet = world
    cluster = Cluster(
        id=pk,
        name=name,
        cloud_id=f"{name}-{suffix}",
        cloud_credential=credential,
        subnet=subnet,
        owner=owner,
        cloud_region=REGION,
        cloud_zone=ZONE,
        status="n",
        **fields,
    )
    if identity_model is not None:
        cluster.identity_model = identity_model
    cluster.save()
    return cluster


def make_partition(cluster, pk, name, **fields):
    fields.setdefault("machine_type", "c2-standard-60")
    return ClusterPartition.objects.create(
        id=pk, cluster=cluster, name=name, **fields
    )


def make_image(world, pk, name, source_image_family):
    owner, credential, _ = world
    return Image.objects.create(
        id=pk,
        name=name,
        family=f"{name}-family",
        source_image_family=source_image_family,
        owner=owner,
        cloud_credential=credential,
        cloud_region=REGION,
        cloud_zone=ZONE,
    )


def scenario_minimal(world, identity_model=None, **fields):
    """The cluster the create page starts from: one default partition"""
    cluster = make_cluster(
        world, 101, "alpha", "0a1b2c3d", identity_model, **fields
    )
    make_partition(cluster, 201, "batch", dynamic_node_count=4)
    return cluster


def scenario_full(world, identity_model=None, **fields):
    """Every option the cluster and partition forms render"""
    _, credential, subnet = world
    fields.setdefault("use_bigquery", True)
    fields.setdefault("use_cloudsql", True)
    fields.setdefault("use_containers", True)
    cluster = make_cluster(
        world,
        102,
        "researchgroupa",
        "9f8e7d6c",
        identity_model,
        enable_slurm_auth=False,
        num_login_nodes=2,
        **fields,
    )

    shared_fs = Filesystem.objects.create(
        name="researchgroupa-sharedfs",
        cloud_credential=credential,
        cloud_region=REGION,
        cloud_zone=ZONE,
        subnet=subnet,
        fstype="n",
        impl_type=FilesystemImpl.BUILT_IN,
    )
    home = FilesystemExport.objects.create(
        filesystem=shared_fs, export_name="/home"
    )
    tools_fs = Filesystem.objects.create(
        name="tools",
        cloud_credential=credential,
        cloud_region=REGION,
        cloud_zone=ZONE,
        subnet=subnet,
        fstype="n",
        impl_type=FilesystemImpl.IMPORTED,
        hostname_or_ip="10.10.0.5",
    )
    tools = FilesystemExport.objects.create(
        filesystem=tools_fs, export_name="/tools"
    )
    cluster.shared_fs = shared_fs
    cluster.save()
    MountPoint.objects.create(
        id=401,
        export=home,
        cluster=cluster,
        mount_order=0,
        mount_path="/home",
        mount_options="defaults,nofail,nosuid",
    )
    MountPoint.objects.create(
        id=402,
        export=tools,
        cluster=cluster,
        mount_order=1,
        mount_path="/opt/cluster",
        mount_options="defaults,hard",
    )
    ContainerRegistry.objects.create(
        id=301, cluster=cluster, use_public_repository=True
    )

    imported = make_image(world, 501, "imported", "Imported")
    built = make_image(world, 502, "built", "rocky-linux-8")
    make_partition(
        cluster,
        211,
        "gpu",
        machine_type="n1-standard-8",
        GPU_type="nvidia-tesla-t4",
        GPU_per_node=1,
        enable_hyperthreads=True,
        dynamic_node_count=2,
        image=imported,
    )
    make_partition(
        cluster,
        212,
        "hpc",
        enable_placement=True,
        enable_node_reuse=False,
        enable_tier1_networking=True,
        boot_disk_type="pd-ssd",
        boot_disk_size=100,
        additional_disk_count=2,
        additional_disk_size=100,
        additional_disk_type="pd-ssd",
        additional_disk_auto_delete=False,
        image=built,
    )
    make_partition(
        cluster,
        213,
        "small",
        machine_type="e2-standard-8",
        static_node_count=1,
        dynamic_node_count=0,
        exclusive=False,
        reservation_name="res-a",
    )
    return cluster


def scenario_bare(world, identity_model=None, **fields):
    """No partitions and no login nodes"""
    return make_cluster(
        world, 103, "bare", "00ff00ff", identity_model, num_login_nodes=0, **fields
    )


# Each scenario's legacy render, captured from the templates as they were
# before identity profiles, is stored as legacy_<name>.yaml.golden. The
# extension keeps the YAML linters and license tooling off these files, which
# must stay byte for byte as rendered.
GOLDEN_SCENARIOS = {
    "minimal": scenario_minimal,
    "full": scenario_full,
    "bare": scenario_bare,
}


def make_profiles_cluster(scenario, world, separate=(), **fields):
    """scenario as a profiles cluster, as the create view and a start leave
    it; the partitions named in separate get their own account"""
    cluster = scenario(world, IdentityModel.PROFILES, **fields)
    cluster.slurm_cluster_name = slurm_cluster_name_for(
        cluster.name, cluster.cloud_id
    )
    cluster.role_id_suffix = ROLE_ID_SUFFIX
    cluster.save()
    cluster.partitions.filter(name__in=separate).update(
        separate_service_account=True
    )
    return cluster


# Profiles renders that every check below, and gcluster create, runs on
PROFILE_CASES = {
    "minimal": lambda world: make_profiles_cluster(scenario_minimal, world),
    "full": lambda world: make_profiles_cluster(
        scenario_full, world, separate=("gpu", "hpc")
    ),
    "bare": lambda world: make_profiles_cluster(scenario_bare, world),
    "all-separate": lambda world: make_profiles_cluster(
        scenario_minimal, world, separate=("batch",)
    ),
    "own-minimal": lambda world: make_profiles_cluster(
        scenario_minimal, world, **OWN_ACCOUNTS
    ),
    "own-full": lambda world: make_profiles_cluster(
        scenario_full, world, separate=("gpu",), **OWN_ACCOUNTS
    ),
}


def clear_clusters():
    """Remove what a case made, so the next case can reuse its ids"""
    Cluster.objects.all().delete()
    Image.objects.all().delete()


def render_blueprint(cluster):
    """The cluster.yaml that ClusterInfo writes for cluster"""
    info = ClusterInfo(cluster)
    info._create_cluster_dir()  # pylint: disable=protected-access
    blueprint = info.cluster_dir / "cluster.yaml"
    blueprint.unlink(missing_ok=True)
    info._prepare_ghpc_yaml()  # pylint: disable=protected-access
    return blueprint.read_text()


def blueprint_modules(text):
    blueprint = yaml.safe_load(text)
    (group,) = blueprint["deployment_groups"]
    return {module["id"]: module for module in group["modules"]}


def modules_from(modules, source):
    return {
        module_id: module
        for module_id, module in modules.items()
        if module["source"] == source
    }


def nodesets(modules):
    return modules_from(
        modules, "community/modules/compute/schedmd-slurm-gcp-v6-nodeset"
    )


# The blueprint's Terraform state prefix starts with this database's state
# namespace, which is random, and ends with the cluster's id. The golden files
# hold this marker in its place.
STATE_PREFIX_MARKER = "@STATE_PREFIX@"


def _state_prefix_line(prefix):
    return f"    prefix: {prefix}\n"


def to_golden(text, cluster):
    text = text.replace(
        _state_prefix_line(utils.terraform_state_prefix("cluster", cluster.id)),
        _state_prefix_line(STATE_PREFIX_MARKER),
    )
    return text.replace(f"created_by: {SITE_NAME}\n", f"created_by: {SITE_NAME_MARKER}\n")


def from_golden(text, cluster):
    text = text.replace(
        _state_prefix_line(STATE_PREFIX_MARKER),
        _state_prefix_line(utils.terraform_state_prefix("cluster", cluster.id)),
    )
    return text.replace(f"created_by: {SITE_NAME_MARKER}\n", f"created_by: {SITE_NAME}\n")


class LegacyGoldenTests(TestCase):
    """A legacy cluster renders byte for byte as before identity profiles"""

    def test_legacy_renders_match_the_base_templates(self):
        world = make_world()
        for name, scenario in GOLDEN_SCENARIOS.items():
            with self.subTest(scenario=name):
                cluster = scenario(world, IdentityModel.LEGACY)
                expected = from_golden(
                    (GOLDEN_DIR / f"legacy_{name}.yaml.golden").read_text(), cluster
                )
                self.assertEqual(render_blueprint(cluster), expected)


class RenderErrorTests(TestCase):
    """A failed render stops the create instead of leaving no blueprint"""

    def test_render_error_is_raised(self):
        cluster = PROFILE_CASES["minimal"](make_world())
        info = ClusterInfo(cluster)
        info._create_cluster_dir()  # pylint: disable=protected-access
        with mock.patch.object(
            info.env, "get_template", side_effect=RuntimeError("template failed")
        ), self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", level="ERROR"
        ), self.assertRaisesMessage(RuntimeError, "template failed"):
            info._prepare_ghpc_yaml()  # pylint: disable=protected-access


class ProfilesAccountTests(TestCase):
    """A profiles cluster gets its own least-privilege accounts"""

    def setUp(self):
        self.world = make_world()
        self.server = utils.load_config()["server"]

    def render(self, case):
        clear_clusters()
        return blueprint_modules(render_blueprint(PROFILE_CASES[case](self.world)))

    def test_three_accounts_without_project_roles_and_three_profiles(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        modules = blueprint_modules(render_blueprint(cluster))

        accounts = modules_from(modules, ACCOUNT_SOURCE)
        self.assertEqual(set(accounts), {"ctl_sa", "lgn_sa", "cmp_sa"})
        for module_id, account in accounts.items():
            with self.subTest(account=module_id):
                self.assertEqual(account["settings"]["project_roles"], [])
                self.assertEqual(
                    account["settings"]["name"], module_id.removesuffix("_sa")
                )
                self.assertRegex(
                    f"{cluster.cloud_id}-{account['settings']['name']}",
                    f"^{ACCOUNT_ID_PATTERN}$",
                )

        profiles = modules_from(modules, PROFILE_SOURCE)
        self.assertEqual(
            {module_id: p["settings"]["profile"] for module_id, p in profiles.items()},
            {
                "ctl_profile": "slurm-controller",
                "lgn_profile": "slurm-login",
                "cmp_profile": "slurm-compute",
            },
        )
        for module_id, profile in profiles.items():
            with self.subTest(profile=module_id):
                name = module_id.removesuffix("_profile")
                self.assertEqual(profile["use"], [f"{name}_sa"])
                self.assertEqual(profile["settings"]["name"], name)
        self.assertNotIn("hpc_service_account", modules)

    def test_nodes_use_their_profiles(self):
        modules = self.render("minimal")
        controller = modules["slurm_controller"]
        login = modules["slurm_login"]
        (nodeset,) = nodesets(modules).values()

        self.assertIn("ctl_profile", controller["use"])
        self.assertEqual(login["use"], ["lgn_profile"])
        self.assertIn("cmp_profile", nodeset["use"])
        for module in (controller, login, nodeset):
            with self.subTest(module=module["id"]):
                # The account comes from the profile, after its grants
                self.assertNotIn("service_account_email", module["settings"])
                self.assertEqual(
                    module["settings"]["service_account_scopes"],
                    [CLOUD_PLATFORM_SCOPE],
                )

    def test_controller_acts_as_only_its_own_compute_accounts(self):
        for case, compute in (
            ("minimal", ["cmp"]),
            ("full", ["cmp", "c00", "c01"]),
            ("all-separate", ["c00"]),
        ):
            with self.subTest(case=case):
                modules = self.render(case)
                profiles = modules_from(modules, PROFILE_SOURCE)
                self.assertEqual(
                    profiles["ctl_profile"]["settings"]["act_as_service_accounts"],
                    [f"$({name}_profile.service_account_email)" for name in compute],
                )
                for module_id, profile in profiles.items():
                    if module_id != "ctl_profile":
                        self.assertNotIn(
                            "act_as_service_accounts", profile["settings"]
                        )
                # Each target is a compute profile that some nodeset uses
                used = {
                    ref
                    for nodeset in nodesets(modules).values()
                    for ref in nodeset["use"]
                    if ref.endswith("_profile")
                }
                self.assertEqual(used, {f"{name}_profile" for name in compute})
                for name in compute:
                    self.assertEqual(
                        profiles[f"{name}_profile"]["settings"]["profile"],
                        "slurm-compute",
                    )

    def test_controller_and_its_profile_share_the_slurm_cluster_name(self):
        for case in ("minimal", "full", "bare", "all-separate"):
            with self.subTest(case=case):
                clear_clusters()
                cluster = PROFILE_CASES[case](self.world)
                modules = blueprint_modules(render_blueprint(cluster))
                on_controller = modules["slurm_controller"]["settings"][
                    "slurm_cluster_name"
                ]
                on_profile = modules["ctl_profile"]["settings"]["slurm_cluster_name"]
                self.assertEqual(on_controller, cluster.slurm_cluster_name)
                self.assertEqual(on_profile, on_controller)
                self.assertRegex(on_controller, f"^{SLURM_CLUSTER_NAME_PATTERN}$")

    def test_grants_are_bindings_on_named_resources(self):
        cluster = PROFILE_CASES["full"](self.world)
        modules = blueprint_modules(render_blueprint(cluster))
        profiles = modules_from(modules, PROFILE_SOURCE)
        for module_id, profile in profiles.items():
            with self.subTest(profile=module_id):
                settings = profile["settings"]
                bucket_binding = {
                    **control_bucket_bindings(
                        settings["profile"], self.server["gcs_bucket"], cluster.id
                    ),
                    **SLURM_BUCKET_BINDINGS,
                }
                self.assertEqual(settings["bucket_bindings"], bucket_binding)
                self.assertEqual(settings["role_id_suffix"], ROLE_ID_SUFFIX)
                self.assertRegex(settings["role_id_suffix"], f"^{ROLE_ID_SUFFIX_PATTERN}$")
                roles = [
                    binding["role"]
                    for key, bindings in settings.items()
                    if key.endswith("_bindings")
                    for binding in bindings.values()
                ]
                for role in roles:
                    self.assertNotRegex(role, REFUSED_BINDING_ROLE)
                # Role id <deployment>.<name>.<suffix>.scoped is <= 64 bytes
                self.assertLessEqual(
                    len(f"{cluster.cloud_id}.{settings['name']}.{ROLE_ID_SUFFIX}.scoped"),
                    64,
                )

        controller = profiles["ctl_profile"]["settings"]
        self.assertEqual(
            controller["pubsub_subscription_bindings"],
            {
                "c2_subscription": {
                    "project": self.server["gcp_project"],
                    "subscription": f"{self.server['c2_topic']}-cluster_{cluster.id}",
                    "role": "roles/pubsub.subscriber",
                }
            },
        )
        self.assertEqual(
            controller["pubsub_topic_bindings"],
            {
                "c2_topic": {
                    "project": self.server["gcp_project"],
                    "topic": self.server["c2_topic"],
                    "role": "roles/pubsub.publisher",
                }
            },
        )
        self.assertEqual(
            controller["artifact_registry_bindings"],
            {
                "registry_301": {
                    "location": REGION,
                    "repository_id": "$(registry_301.repository_id)",
                    "role": "roles/artifactregistry.reader",
                }
            },
        )
        for module_id in ("lgn_profile", "cmp_profile", "c00_profile"):
            settings = profiles[module_id]["settings"]
            self.assertNotIn("pubsub_subscription_bindings", settings)
            self.assertNotIn("pubsub_topic_bindings", settings)
            self.assertNotIn("artifact_registry_bindings", settings)

    def test_no_registry_binding_without_registries(self):
        controller = self.render("minimal")["ctl_profile"]["settings"]
        self.assertNotIn("artifact_registry_bindings", controller)


class ProfilesFlagTests(TestCase):
    """The controller profile's capability flags follow the forms"""

    def setUp(self):
        self.world = make_world()

    def controller_profile(self, cluster):
        return blueprint_modules(render_blueprint(cluster))["ctl_profile"]["settings"]

    def test_placement_policies_follow_the_partitions(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        part = cluster.partitions.get()
        for enabled in (True, False):
            with self.subTest(enable_placement=enabled):
                part.enable_placement = enabled
                part.save()
                self.assertIs(
                    self.controller_profile(cluster)["enable_placement_policies"],
                    enabled,
                )

    def test_placement_policies_on_when_any_partition_uses_placement(self):
        cluster = PROFILE_CASES["full"](self.world)
        self.assertIs(
            self.controller_profile(cluster)["enable_placement_policies"], True
        )
        cluster.partitions.update(enable_placement=False)
        self.assertIs(
            self.controller_profile(cluster)["enable_placement_policies"], False
        )

    def test_bigquery_load_follows_the_cluster(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        for enabled in (True, False):
            with self.subTest(use_bigquery=enabled):
                cluster.use_bigquery = enabled
                cluster.save()
                self.assertIs(
                    self.controller_profile(cluster)["enable_bigquery_load"],
                    enabled,
                )

    def test_unsupported_flags_and_features_are_never_rendered(self):
        cluster_flags = ("use_bigquery", "use_cloudsql", "use_containers", "enable_slurm_auth")
        partition_flags = (
            "exclusive",
            "enable_placement",
            "enable_hyperthreads",
            "enable_tier1_networking",
            "enable_node_reuse",
            "additional_disk_auto_delete",
            "separate_service_account",
        )
        cluster = PROFILE_CASES["minimal"](self.world)
        ContainerRegistry.objects.create(
            id=302, cluster=cluster, use_public_repository=True
        )
        part = cluster.partitions.get()
        part.additional_disk_count = 1
        part.save()
        never = re.compile(rf"\b({'|'.join(NEVER_RENDERED)})\b")
        checked = 0
        for own in (False, True):
            for values in itertools.product(
                (False, True), repeat=len(cluster_flags) + len(partition_flags)
            ):
                cluster_values = dict(zip(cluster_flags, values))
                partition_values = dict(zip(partition_flags, values[len(cluster_flags):]))
                Cluster.objects.filter(id=cluster.id).update(
                    **cluster_values,
                    **(OWN_ACCOUNTS if own else {k: "" for k in OWN_ACCOUNTS}),
                )
                ClusterPartition.objects.filter(id=part.id).update(**partition_values)
                cluster.refresh_from_db()
                text = render_blueprint(cluster)
                match = never.search(text)
                if match:
                    self.fail(
                        f"{match.group(0)} rendered with own accounts={own}, "
                        f"{cluster_values}, {partition_values}"
                    )
                checked += 1
        self.assertEqual(checked, 2 ** (len(cluster_flags) + len(partition_flags) + 1))

    def test_unsupported_features_never_rendered_for_any_case(self):
        never = re.compile(rf"\b({'|'.join(NEVER_RENDERED)})\b")
        for case, build in PROFILE_CASES.items():
            with self.subTest(case=case):
                clear_clusters()
                self.assertIsNone(never.search(render_blueprint(build(self.world))))
        for name, scenario in GOLDEN_SCENARIOS.items():
            with self.subTest(legacy=name):
                clear_clusters()
                cluster = scenario(self.world, IdentityModel.LEGACY)
                self.assertIsNone(never.search(render_blueprint(cluster)))


class PartitionAccountTests(TestCase):
    """Partitions can run as their own compute account"""

    def setUp(self):
        self.world = make_world()

    def test_two_partitions_on_separate_accounts(self):
        cluster = PROFILE_CASES["full"](self.world)
        modules = blueprint_modules(render_blueprint(cluster))

        self.assertEqual(
            set(modules_from(modules, ACCOUNT_SOURCE)),
            {"ctl_sa", "lgn_sa", "cmp_sa", "c00_sa", "c01_sa"},
        )
        # In partition id order: gpu (211) is c00 and hpc (212) is c01; small
        # stays on the shared account
        uses = {
            module_id: [ref for ref in nodeset["use"] if ref.endswith("_profile")]
            for module_id, nodeset in nodesets(modules).items()
        }
        self.assertEqual(
            uses,
            {
                "partition_211-nodeset": ["c00_profile"],
                "partition_212-nodeset": ["c01_profile"],
                "partition_213-nodeset": ["cmp_profile"],
            },
        )
        for name in ("c00", "c01"):
            self.assertEqual(modules[f"{name}_sa"]["settings"]["name"], name)
            self.assertEqual(modules[f"{name}_sa"]["settings"]["project_roles"], [])
            self.assertEqual(modules[f"{name}_profile"]["use"], [f"{name}_sa"])
            self.assertEqual(
                modules[f"{name}_profile"]["settings"]["profile"], "slurm-compute"
            )

    def test_unused_shared_compute_account_is_not_created(self):
        modules = blueprint_modules(
            render_blueprint(PROFILE_CASES["all-separate"](self.world))
        )
        self.assertNotIn("cmp_sa", modules)
        self.assertNotIn("cmp_profile", modules)
        self.assertIn("c00_profile", next(iter(nodesets(modules).values()))["use"])

    def test_account_names_fit_the_longest_cloud_id(self):
        # Cluster names are at most 17 characters, then -<8 hex>
        cloud_id = "a" * 17 + "-0123abcd"
        for name in ("ctl", "lgn", "cmp", partition_account_name(0), partition_account_name(99)):
            with self.subTest(name=name):
                self.assertRegex(f"{cloud_id}-{name}", f"^{ACCOUNT_ID_PATTERN}$")
                self.assertLessEqual(
                    len(f"{cloud_id}.{name}.{ROLE_ID_SUFFIX}.scoped"), 64
                )
        self.assertEqual(partition_account_name(7), "c07")
        with self.assertRaises(ValueError):
            partition_account_name(100)


class SlurmBucketTests(TestCase):
    """A profiles cluster's VMs start only after they can read the Slurm bucket"""

    def setUp(self):
        self.world = make_world()

    def render(self, case):
        clear_clusters()
        return blueprint_modules(render_blueprint(PROFILE_CASES[case](self.world)))

    def test_profiles_cluster_creates_the_bucket_and_grants_it_through_profiles(self):
        for case in ("minimal", "full", "bare", "all-separate"):
            with self.subTest(case=case):
                modules = self.render(case)
                buckets = modules_from(modules, BUCKET_SOURCE)
                self.assertEqual(list(buckets), ["slurm_bucket"])
                # The authoritative objectViewer list names every cluster
                # account, as the controller module's own grants do
                accounts = list(modules_from(modules, ACCOUNT_SOURCE))
                profiles = modules_from(modules, PROFILE_SOURCE)
                kinds = {
                    module_id.removesuffix("_profile"): profile["settings"]["profile"]
                    for module_id, profile in profiles.items()
                }
                ordered = (
                    [name for name, kind in kinds.items() if kind == "slurm-controller"]
                    + [name for name, kind in kinds.items() if kind == "slurm-login"]
                    + [name for name, kind in kinds.items() if kind == "slurm-compute"]
                )
                self.assertEqual(sorted(f"{name}_sa" for name in ordered), sorted(accounts))
                self.assertEqual(
                    buckets["slurm_bucket"]["settings"],
                    {
                        "name_prefix": "slurm",
                        "random_suffix": True,
                        "force_destroy": True,
                        "viewers": [
                            f"serviceAccount:$({name}_sa.service_account_email)"
                            for name in ordered
                        ],
                    },
                )
                # Never mounted: no module uses it.
                for module_id, module in modules.items():
                    self.assertNotIn("slurm_bucket", module.get("use", []), module_id)

                self.assertTrue(profiles)
                for module_id, profile in profiles.items():
                    bindings = profile["settings"]["bucket_bindings"]
                    for key, binding in SLURM_BUCKET_BINDINGS.items():
                        self.assertEqual(bindings.get(key), binding, module_id)
                    # No objectViewer member grant for the authoritative
                    # binding to remove
                    for binding in bindings.values():
                        if binding["bucket"] == SLURM_BUCKET_NAME:
                            self.assertNotEqual(
                                binding["role"], "roles/storage.objectViewer", module_id
                            )

    def test_only_the_controller_can_write_the_control_bucket(self):
        for case in ("minimal", "full", "bare", "all-separate"):
            with self.subTest(case=case):
                modules = self.render(case)
                for module_id, profile in modules_from(modules, PROFILE_SOURCE).items():
                    settings = profile["settings"]
                    self.assertEqual(
                        settings["bucket_bindings"]["ofe_bucket"]["role"],
                        CONTROL_BUCKET_ROLES[settings["profile"]],
                        module_id,
                    )

                controller = modules["slurm_controller"]["settings"]
                self.assertIs(controller["create_bucket"], False)
                self.assertEqual(controller["bucket_name"], SLURM_BUCKET_NAME)

    def test_controller_writes_only_under_its_own_cluster_prefix(self):
        """No profile can write the control bucket outside clusters/<own id>/,
        which keeps webserver/startup.sh and other clusters' files out of a
        cluster's reach"""
        control_bucket = utils.load_config()["server"]["gcs_bucket"]
        for case in ("minimal", "full", "bare", "all-separate"):
            with self.subTest(case=case):
                clear_clusters()
                cluster = PROFILE_CASES[case](self.world)
                modules = blueprint_modules(render_blueprint(cluster))
                profiles = modules_from(modules, PROFILE_SOURCE)
                self.assertTrue(profiles)
                writes = []
                for module_id, profile in profiles.items():
                    for key, binding in profile["settings"]["bucket_bindings"].items():
                        if binding["bucket"] != control_bucket:
                            continue
                        if binding["role"] in READ_ONLY_BUCKET_ROLES:
                            # A read grant needs no condition
                            continue
                        condition = binding.get("condition")
                        self.assertIsNotNone(
                            condition, f"{module_id}: {key} writes the whole control bucket"
                        )
                        writes.append((profile["settings"]["profile"], key, binding))

                # Exactly one write grant, the controller's, limited to the
                # objects under this cluster's own prefix. The trailing "/"
                # keeps cluster 1 out of clusters/10/.
                self.assertEqual(len(writes), 1, writes)
                ((kind, key, binding),) = writes
                self.assertEqual(kind, "slurm-controller")
                self.assertEqual(key, "ofe_bucket_cluster_prefix")
                self.assertEqual(binding["role"], "roles/storage.objectUser")
                condition = binding["condition"]
                self.assertEqual(
                    condition["expression"],
                    cluster_prefix_expression(control_bucket, cluster.id),
                )
                self.assertRegex(
                    condition["expression"],
                    r'^resource\.name\.startsWith\("projects/_/buckets/[^"/]+/objects/'
                    rf'clusters/{cluster.id}/"\)$',
                )
                self.assertTrue(condition["title"].strip())
                self.assertLessEqual(len(condition["title"]), CONDITION_TITLE_MAX)
                self.assertLessEqual(
                    len(condition.get("description") or ""),
                    CONDITION_DESCRIPTION_MAX,
                )

    def test_nodes_read_only_their_own_files_in_the_control_bucket(self):
        """No profile has an unconditioned grant on the control bucket: each
        node's read grant covers listing the bucket, clusters/ansible_setup
        and this cluster's bootstrap files, and not webserver/ or other
        clusters' files"""
        control_bucket = utils.load_config()["server"]["gcs_bucket"]
        for case in ("minimal", "full", "bare", "all-separate"):
            with self.subTest(case=case):
                clear_clusters()
                cluster = PROFILE_CASES[case](self.world)
                modules = blueprint_modules(render_blueprint(cluster))
                profiles = modules_from(modules, PROFILE_SOURCE)
                kinds = set()
                for module_id, profile in profiles.items():
                    kind = profile["settings"]["profile"]
                    kinds.add(kind)
                    on_control_bucket = {
                        key: binding
                        for key, binding in profile["settings"]["bucket_bindings"].items()
                        if binding["bucket"] == control_bucket
                    }
                    for key, binding in on_control_bucket.items():
                        self.assertIn(
                            "condition", binding,
                            f"{module_id}: {key} covers the whole control bucket",
                        )
                    viewer = on_control_bucket["ofe_bucket"]
                    self.assertEqual(viewer["role"], "roles/storage.objectViewer", module_id)
                    condition = viewer["condition"]
                    self.assertEqual(
                        condition["expression"],
                        node_files_expression(control_bucket, cluster.id),
                        module_id,
                    )
                    self.assertEqual(condition["expression"].count(" || "), 3, module_id)
                    allowed, refused = node_files_cases(control_bucket, cluster.id)
                    for name in allowed:
                        self.assertTrue(
                            cel_allows(condition["expression"], name),
                            f"{module_id}: {name} refused",
                        )
                    for name in refused:
                        self.assertFalse(
                            cel_allows(condition["expression"], name),
                            f"{module_id}: {name} allowed",
                        )
                    self.assertIn(
                        f"/objects/clusters/{cluster.id}/bootstrap_",
                        condition["expression"],
                    )
                    self.assertTrue(condition["title"].strip())
                    self.assertLessEqual(len(condition["title"]), CONDITION_TITLE_MAX)
                    self.assertLessEqual(
                        len(condition["description"]), CONDITION_DESCRIPTION_MAX
                    )
                    # Only the controller has more on the control bucket: its
                    # write grant, limited to its own prefix
                    expected = {"ofe_bucket"}
                    if kind == "slurm-controller":
                        expected.add("ofe_bucket_cluster_prefix")
                    self.assertEqual(set(on_control_bucket), expected, module_id)
                # bare has no partitions, so no compute profile
                expected_kinds = {"slurm-controller", "slurm-login"}
                if case != "bare":
                    expected_kinds.add("slurm-compute")
                self.assertEqual(kinds, expected_kinds)

    def test_each_cluster_gets_its_own_prefix(self):
        clear_clusters()
        expressions = {}
        for scenario in (scenario_minimal, scenario_bare):
            cluster = make_profiles_cluster(scenario, self.world)
            controller = modules_from(
                blueprint_modules(render_blueprint(cluster)), PROFILE_SOURCE
            )["ctl_profile"]["settings"]
            condition = controller["bucket_bindings"]["ofe_bucket_cluster_prefix"]["condition"]
            self.assertIn(f"/objects/clusters/{cluster.id}/", condition["expression"])
            expressions[cluster.id] = condition["expression"]
        self.assertEqual(len(set(expressions.values())), 2, expressions)

    def test_own_accounts_keep_the_controller_bucket(self):
        for case in ("own-minimal", "own-full"):
            with self.subTest(case=case):
                modules = self.render(case)
                self.assertEqual(modules_from(modules, BUCKET_SOURCE), {})
                controller = modules["slurm_controller"]["settings"]
                self.assertNotIn("create_bucket", controller)
                self.assertNotIn("bucket_name", controller)


class OwnAccountTests(TestCase):
    """Bring-your-own accounts are used as they are"""

    def setUp(self):
        self.world = make_world()

    def test_own_accounts_render_no_account_or_profile_blocks(self):
        for case in ("own-minimal", "own-full"):
            with self.subTest(case=case):
                clear_clusters()
                cluster = PROFILE_CASES[case](self.world)
                text = render_blueprint(cluster)
                modules = blueprint_modules(text)
                self.assertEqual(modules_from(modules, ACCOUNT_SOURCE), {})
                self.assertEqual(modules_from(modules, PROFILE_SOURCE), {})
                self.assertNotIn("_profile", text)
                self.assertNotIn("hpc_service_account", text)

                controller = modules["slurm_controller"]["settings"]
                login = modules["slurm_login"]["settings"]
                self.assertEqual(
                    controller["service_account_email"],
                    OWN_ACCOUNTS["controller_service_account_email"],
                )
                self.assertEqual(
                    login["service_account_email"],
                    OWN_ACCOUNTS["login_service_account_email"],
                )
                self.assertEqual(
                    controller["slurm_cluster_name"], cluster.slurm_cluster_name
                )
                self.assertNotIn("use", modules["slurm_login"])
                for nodeset in nodesets(modules).values():
                    self.assertEqual(
                        nodeset["settings"]["service_account_email"],
                        OWN_ACCOUNTS["compute_service_account_email"],
                    )
                for module in [controller, login] + [
                    n["settings"] for n in nodesets(modules).values()
                ]:
                    self.assertEqual(
                        module["service_account_scopes"], [CLOUD_PLATFORM_SCOPE]
                    )

    def test_some_own_accounts_are_refused(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        cluster.controller_service_account_email = OWN_ACCOUNTS[
            "controller_service_account_email"
        ]
        cluster.save()
        with self.assertRaisesRegex(ValueError, "set all three or none"):
            ClusterInfo(cluster)._identity_context()  # pylint: disable=protected-access


class SlurmClusterNameTests(SimpleTestCase):
    """slurm_cluster_name_for gives each cluster its own node-name prefix"""

    def test_name_letters_then_cloud_id_suffix(self):
        self.assertEqual(
            slurm_cluster_name_for("researchgroupa", "researchgroupa-9f8e7d6c"),
            "researchgro9f8e7d6c",
        )
        self.assertEqual(slurm_cluster_name_for("a-b", "a-b-0011aabb"), "ab0011aabb")

    def test_clusters_sharing_a_prefix_get_different_names(self):
        # The controller's own default would give both "researchgr"
        self.assertNotEqual(
            slurm_cluster_name_for("researchgroupa", "researchgroupa-0123abcd"),
            slurm_cluster_name_for("researchgroupb", "researchgroupb-4567ef01"),
        )

    def test_longest_name_matches_the_controller_rule(self):
        name = slurm_cluster_name_for("a" * 17, "a" * 17 + "-0123abcd")
        self.assertEqual(len(name), 19)
        self.assertRegex(name, f"^{SLURM_CLUSTER_NAME_PATTERN}$")

    def test_bad_input_is_refused(self):
        with self.assertRaises(ValueError):
            slurm_cluster_name_for("alpha", "alpha-not-hex")
        with self.assertRaises(ValueError):
            slurm_cluster_name_for("123", "123-0123abcd")


def fake_state(controller_email, login_email, accounts=None):
    """Terraform state with a controller, a login node and accounts"""

    def instance(name, ip, email):
        return {
            "attributes": {
                "name": name,
                "machine_type": "n2-standard-2",
                "network_interface": [{"network_ip": ip, "access_config": []}],
                "service_account": [{"email": email}],
            }
        }

    resources = [
        {
            "module": "module.slurm_controller",
            "type": "google_compute_instance_from_template",
            "name": "controller",
            "instances": [instance("ctl-node", "10.10.0.2", controller_email)],
        },
        {
            "module": 'module.slurm_controller.module.login["slurm-login"].module.instance',
            "type": "google_compute_instance_from_template",
            "name": "slurm_instance",
            "instances": [instance("lgn-node", "10.10.0.3", login_email)],
        },
    ]
    for name, email in (accounts or {}).items():
        resources.append({
            "module": f"module.{name}_sa.module.service_account",
            "type": "google_service_account",
            "name": "service_accounts",
            "instances": [{"index_key": name, "attributes": {"email": email}}],
        })
    return {"resources": resources}


class ApplyTerraformTests(TestCase):
    """After apply, OFE makes IAM calls only for legacy clusters"""

    def setUp(self):
        self.world = make_world()

    def apply(self, cluster, state):
        info = ClusterInfo(cluster)
        terraform_dir = info.get_terraform_dir()
        terraform_dir.mkdir(parents=True, exist_ok=True)
        # The state is read back from the state bucket, as the credential
        with mock.patch(
            "ghpcfe.cluster_manager.utils.run_terraform"
        ) as run_terraform, mock.patch(
            "ghpcfe.cluster_manager.utils.get_terraform_state", return_value=state
        ) as get_terraform_state, mock.patch(
            "ghpcfe.cluster_manager.c2.create_cluster_subscription"
        ), mock.patch(
            "ghpcfe.cluster_manager.cloud_info.gcs_apply_bucket_acl"
        ) as bucket_iam, mock.patch(
            "ghpcfe.cluster_manager.c2.add_cluster_subscription_service_account"
        ) as subscription_iam, self.assertLogs(
            "ghpcfe.cluster_manager.clusterinfo", level="INFO"
        ) as logs, contextlib.redirect_stdout(io.StringIO()):
            info._apply_terraform()  # pylint: disable=protected-access
        run_terraform.assert_called_once()
        get_terraform_state.assert_called_once_with(
            terraform_dir,
            credential=cluster.cloud_credential,
            operation=info._operation("deploy"),  # pylint: disable=protected-access
        )
        return bucket_iam, subscription_iam, info, logs

    def test_profiles_cluster_makes_no_iam_call(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        emails = {
            name: f"{cluster.cloud_id}-{name}@{PROJECT}.iam.gserviceaccount.com"
            for name in ("ctl", "lgn", "cmp")
        }
        state = fake_state(emails["ctl"], emails["lgn"], emails)
        bucket_iam, subscription_iam, info, logs = self.apply(cluster, state)
        bucket_iam.assert_not_called()
        subscription_iam.assert_not_called()
        self.assertTrue(
            any("grants are in its blueprint" in line for line in logs.output)
        )
        with contextlib.redirect_stdout(io.StringIO()):
            accounts = info._get_service_accounts(state)  # pylint: disable=protected-access
        self.assertEqual(
            accounts,
            {
                "controller": emails["ctl"],
                "login": emails["lgn"],
                "compute": emails["cmp"],
            },
        )

    def test_own_accounts_cluster_makes_no_iam_call(self):
        cluster = PROFILE_CASES["own-minimal"](self.world)
        state = fake_state(
            OWN_ACCOUNTS["controller_service_account_email"],
            OWN_ACCOUNTS["login_service_account_email"],
        )
        bucket_iam, subscription_iam, info, _ = self.apply(cluster, state)
        bucket_iam.assert_not_called()
        subscription_iam.assert_not_called()
        with contextlib.redirect_stdout(io.StringIO()):
            accounts = info._get_service_accounts(state)  # pylint: disable=protected-access
        self.assertEqual(
            accounts,
            {
                "controller": OWN_ACCOUNTS["controller_service_account_email"],
                "login": OWN_ACCOUNTS["login_service_account_email"],
                "compute": OWN_ACCOUNTS["compute_service_account_email"],
            },
        )

    def test_legacy_cluster_still_grants_in_python(self):
        cluster = scenario_minimal(self.world, IdentityModel.LEGACY)
        email = f"{cluster.cloud_id}-sa@{PROJECT}.iam.gserviceaccount.com"
        bucket_iam, subscription_iam, _, _ = self.apply(
            cluster, fake_state(email, email)
        )
        bucket_iam.assert_called_once_with(
            utils.load_config()["server"]["gcs_bucket"],
            f"serviceAccount:{email}",
            permission="roles/storage.objectAdmin",
        )
        subscription_iam.assert_called_once_with(cluster.id, email)


class OrphanSweepTests(TestCase):
    """The orphaned-node sweep looks for the cluster's Slurm label"""

    def setUp(self):
        self.world = make_world()

    def sweep(self, cluster, list_error=None):
        """Run the sweep with a stand-in client but the installed library's
        real request type. Returns the client mock."""
        with mock.patch(
            "google.oauth2.service_account.Credentials.from_service_account_info"
        ), mock.patch("google.cloud.compute_v1.InstancesClient") as client:
            client.return_value.aggregated_list.return_value = []
            client.return_value.aggregated_list.side_effect = list_error
            ClusterInfo(cluster)._sweep_orphaned_compute_nodes()  # pylint: disable=protected-access
        return client

    def sweep_filter(self, cluster):
        client = self.sweep(cluster)
        return client.return_value.aggregated_list.call_args.kwargs["request"].filter

    def test_request_builds_with_the_installed_library(self):
        """The request sets only fields the installed google-cloud-compute
        defines (1.55 and later have no return_partial_success), so the sweep
        reaches the list call instead of failing before it"""
        from google.cloud import compute_v1

        cluster = PROFILE_CASES["minimal"](self.world)
        with self.assertNoLogs("ghpcfe.cluster_manager.clusterinfo", "ERROR"):
            client = self.sweep(cluster)
        client.return_value.aggregated_list.assert_called_once()
        request = client.return_value.aggregated_list.call_args.kwargs["request"]
        self.assertIsInstance(request, compute_v1.AggregatedListInstancesRequest)
        self.assertEqual(request.project, cluster.project_id)

    def test_failed_sweep_is_an_error_saying_nodes_may_be_left(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        with self.assertLogs("ghpcfe.cluster_manager.clusterinfo", "ERROR") as logs:
            self.sweep(cluster, list_error=RuntimeError("compute API unavailable"))
        (record,) = [r for r in logs.records if r.levelname == "ERROR"]
        message = record.getMessage()
        self.assertIn(f"cluster {cluster.name} ({cluster.cloud_id})", message)
        self.assertIn("compute API unavailable", message)
        self.assertIn(
            f"labelled slurm_cluster_name={cluster.slurm_cluster_name}", message
        )
        self.assertIn(f"project {cluster.project_id}", message)
        self.assertIn("delete them by hand", message)

    def test_profiles_cluster_uses_its_slurm_cluster_name(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        self.assertEqual(
            self.sweep_filter(cluster),
            f"labels.slurm_cluster_name={cluster.slurm_cluster_name}",
        )

    def test_legacy_cluster_filter_is_unchanged(self):
        cluster = scenario_minimal(self.world, IdentityModel.LEGACY)
        self.assertEqual(
            self.sweep_filter(cluster),
            f"labels.slurm_cluster_name={cluster.cloud_id}",
        )


class RoleIdSuffixTests(TestCase):
    """Custom role ids change for each start after a destroy"""

    def setUp(self):
        self.world = make_world()

    def refreshed(self, cluster):
        ClusterInfo(cluster)._refresh_role_id_suffix()  # pylint: disable=protected-access
        return cluster.role_id_suffix

    def test_first_start_gets_a_suffix(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        cluster.role_id_suffix = ""
        suffix = self.refreshed(cluster)
        self.assertRegex(suffix, r"^[0-9a-f]{6}$")
        self.assertRegex(suffix, f"^{ROLE_ID_SUFFIX_PATTERN}$")

    def test_start_after_destroy_gets_a_new_suffix(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        cluster.status = "d"
        cluster.cloud_state = "xm"
        with mock.patch("secrets.token_hex", return_value="d4e5f6"):
            self.assertEqual(self.refreshed(cluster), "d4e5f6")

    def test_retry_after_failed_create_keeps_the_suffix(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        cluster.status = "e"
        cluster.cloud_state = "nm"
        self.assertEqual(self.refreshed(cluster), ROLE_ID_SUFFIX)

    def test_legacy_cluster_gets_no_suffix(self):
        cluster = scenario_minimal(self.world, IdentityModel.LEGACY)
        self.assertEqual(self.refreshed(cluster), "")

    def test_new_suffix_is_never_the_previous_one(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        cluster.status = "d"
        cluster.cloud_state = "xm"
        with mock.patch(
            "secrets.token_hex", side_effect=[ROLE_ID_SUFFIX, "d4e5f6"]
        ) as token_hex:
            self.assertEqual(self.refreshed(cluster), "d4e5f6")
        self.assertEqual(token_hex.call_count, 2)
        with mock.patch("secrets.token_hex", side_effect=["0a0b0c", "0a0b0c", "1a1b1c"]):
            self.assertEqual(new_role_id_suffix("0a0b0c"), "1a1b1c")


class PartitionSlotTests(TestCase):
    """Each partition keeps its own account for the life of the cluster"""

    def setUp(self):
        self.world = make_world()

    def running(self, *separate):
        cluster = make_profiles_cluster(scenario_full, self.world, separate=separate)
        render_blueprint(cluster)  # assigns the slots on first render
        cluster.status = "r"
        cluster.cloud_state = "m"
        cluster.save()
        return cluster

    @staticmethod
    def accounts(cluster):
        """Partition name -> the profiles its nodeset uses, and the accounts"""
        modules = blueprint_modules(render_blueprint(cluster))
        names = {f"partition_{part.id}-nodeset": part.name for part in cluster.partitions.all()}
        uses = {
            names[module_id]: [ref for ref in nodeset["use"] if ref.endswith("_profile")]
            for module_id, nodeset in nodesets(modules).items()
        }
        return uses, set(modules_from(modules, ACCOUNT_SOURCE))

    def test_slots_are_assigned_in_id_order_on_first_render(self):
        cluster = self.running("gpu", "hpc", "small")
        uses, accounts = self.accounts(cluster)
        self.assertEqual(
            uses,
            {"gpu": ["c00_profile"], "hpc": ["c01_profile"], "small": ["c02_profile"]},
        )
        self.assertEqual(
            dict(cluster.partitions.values_list("name", "service_account_slot")),
            {"gpu": 0, "hpc": 1, "small": 2},
        )
        self.assertNotIn("cmp_sa", accounts)

    def test_deleting_a_partition_moves_no_other_partition(self):
        # gpu, hpc and small on c00, c01, c02; gpu is deleted and the
        # cluster reconfigured
        cluster = self.running("gpu", "hpc", "small")
        cluster.partitions.get(name="gpu").delete()
        uses, accounts = self.accounts(cluster)
        self.assertEqual(uses, {"hpc": ["c01_profile"], "small": ["c02_profile"]})
        self.assertEqual(accounts, {"ctl_sa", "lgn_sa", "c01_sa", "c02_sa"})

    def test_unticking_on_a_running_cluster_is_refused(self):
        cluster = self.running("gpu", "hpc", "small")
        gpu = cluster.partitions.get(name="gpu")
        gpu.separate_service_account = False
        with self.assertRaisesRegex(ValidationError, "cannot be changed"):
            gpu.full_clean()
        uses, _ = self.accounts(cluster)
        self.assertEqual(
            uses,
            {"gpu": ["c00_profile"], "hpc": ["c01_profile"], "small": ["c02_profile"]},
        )

    def test_ticking_the_last_shared_partition_is_refused(self):
        # small is the only partition on the shared account
        cluster = self.running("gpu", "hpc")
        small = cluster.partitions.get(name="small")
        small.separate_service_account = True
        with self.assertRaisesRegex(ValidationError, "cannot be changed"):
            small.full_clean()
        uses, accounts = self.accounts(cluster)
        self.assertEqual(
            uses,
            {"gpu": ["c00_profile"], "hpc": ["c01_profile"], "small": ["cmp_profile"]},
        )
        self.assertIn("cmp_sa", accounts)

    def test_change_is_allowed_before_the_cluster_is_created(self):
        cluster = make_profiles_cluster(scenario_full, self.world, separate=("gpu",))
        small = cluster.partitions.get(name="small")
        small.separate_service_account = True
        small.full_clean()

    def test_a_deleted_partitions_slot_is_reused(self):
        cluster = self.running("gpu", "hpc", "small")
        cluster.partitions.get(name="gpu").delete()
        new = make_partition(cluster, 214, "new", separate_service_account=True)
        self.assertEqual(new.service_account_slot, 0)
        # A new partition on a running cluster may have its own account
        new.full_clean()
        uses, _ = self.accounts(cluster)
        self.assertEqual(
            uses,
            {"hpc": ["c01_profile"], "small": ["c02_profile"], "new": ["c00_profile"]},
        )

    def test_at_most_100_partitions_have_their_own_account(self):
        cluster = scenario_bare(self.world, IdentityModel.PROFILES)
        for slot in range(100):
            part = make_partition(
                cluster, 1000 + slot, f"p{slot}", separate_service_account=True
            )
            self.assertEqual(part.service_account_slot, slot)
        extra = ClusterPartition(
            cluster=cluster,
            name="extra",
            machine_type="c2-standard-60",
            separate_service_account=True,
        )
        with self.assertRaisesRegex(ValidationError, "At most 100"):
            extra.full_clean()
        with self.assertRaisesRegex(ValueError, "At most 100"):
            extra.save()
        # A partition on the shared account is still fine
        extra.separate_service_account = False
        extra.save()
        self.assertIsNone(extra.service_account_slot)

    @staticmethod
    def form_data(part, **changes):
        data = {
            name: value
            for name, value in model_to_dict(
                part, fields=ClusterPartitionForm._meta.fields  # pylint: disable=protected-access
            ).items()
            if value is not None
        }
        data.update(changes)
        return data

    def test_form_locks_the_choice_for_existing_partitions_of_a_created_cluster(self):
        cluster = self.running("gpu", "hpc")
        small = cluster.partitions.get(name="small")
        form = ClusterPartitionForm(
            # No reservation, so validation makes no reservation lookup
            data=self.form_data(
                small, separate_service_account="on", reservation_name=""
            ),
            instance=small,
        )
        self.assertTrue(form.fields["separate_service_account"].disabled)
        form.is_valid()
        # A disabled field keeps the stored value, whatever is posted
        self.assertIs(form.cleaned_data["separate_service_account"], False)

        new = ClusterPartition(cluster=cluster)
        self.assertFalse(
            ClusterPartitionForm(instance=new).fields["separate_service_account"].disabled
        )

    def test_form_allows_the_choice_before_the_cluster_is_created(self):
        cluster = make_profiles_cluster(scenario_full, self.world)
        small = cluster.partitions.get(name="small")
        self.assertFalse(
            ClusterPartitionForm(instance=small).fields["separate_service_account"].disabled
        )


class ServiceAccountEmailValidatorTests(SimpleTestCase):
    """Bring-your-own accounts must be user-managed service accounts"""

    def test_user_managed_accounts_are_accepted(self):
        for email in (
            f"own-ctl@{PROJECT}.iam.gserviceaccount.com",
            f"service-acct@{PROJECT}.iam.gserviceaccount.com",
            "cluster-sa@my-proj.example.com.iam.gserviceaccount.com",
        ):
            with self.subTest(email=email):
                SERVICE_ACCOUNT_EMAIL_VALIDATOR(email)

    def test_default_and_google_managed_accounts_are_refused(self):
        for email in (
            "123456789012-compute@developer.gserviceaccount.com",
            f"{PROJECT}@appspot.gserviceaccount.com",
            "service-123456789012@compute-system.iam.gserviceaccount.com",
            "service-123456789012@gcp-sa-pubsub.iam.gserviceaccount.com",
            "service-123456789012@container-engine-robot.iam.gserviceaccount.com",
            "some-agent@gcp-sa-cloudbuild.iam.gserviceaccount.com",
        ):
            with self.subTest(email=email):
                with self.assertRaises(ValidationError):
                    SERVICE_ACCOUNT_EMAIL_VALIDATOR(email)


def sa_email(local_part, project=PROJECT):
    return f"{local_part}@{project}.iam.gserviceaccount.com"


class OfeManagedAccountTests(SimpleTestCase):
    """Accounts that OFE creates are never a cluster's existing accounts"""

    def setUp(self):
        self.deployment = utils.load_config()["server"]["deployment_name"]

    def errors(self, email):
        """The messages of each existing-account field for email"""
        found = {}
        for field_name in ClusterForm.OWN_SERVICE_ACCOUNT_FIELDS:
            field = Cluster._meta.get_field(field_name)  # pylint: disable=protected-access
            try:
                field.run_validators(email)
            except ValidationError as err:
                found[field_name] = " ".join(err.messages)
            else:
                found[field_name] = ""
        return found

    def assert_refused(self, email, words):
        for field_name, message in self.errors(email).items():
            with self.subTest(email=email, field=field_name):
                self.assertIn(words, message)
                self.assertIn(email, message)

    def assert_accepted(self, email):
        for field_name, message in self.errors(email).items():
            with self.subTest(email=email, field=field_name):
                self.assertEqual(message, "")

    def test_this_servers_provisioning_account_is_refused(self):
        # Its domain does not matter: the name is this server's
        for project in (PROJECT, "other-project"):
            self.assert_refused(
                sa_email(f"{self.deployment}-prov-sa", project),
                "provisioning account, which can change the project's IAM policy",
            )

    def test_this_servers_web_server_account_is_refused(self):
        self.assert_refused(
            sa_email(f"{self.deployment}-fe-sa"), "web server account"
        )

    def test_image_builder_accounts_are_refused(self):
        for image_id in (1, 7, 12345):
            self.assert_refused(
                sa_email(f"img-{image_id}-{BUILDER_ACCOUNT_NAME}"),
                "the account OFE creates to build an image",
            )

    def test_cluster_node_accounts_are_refused(self):
        for cloud_id in ("alpha-0a1b2c3d", "research-group-a-9f8e7d6c", "b-12345678"):
            for name in (
                CONTROLLER_ACCOUNT,
                LOGIN_ACCOUNT,
                COMPUTE_ACCOUNT,
                partition_account_name(0),
                partition_account_name(MAX_SERVICE_ACCOUNT_SLOTS - 1),
            ):
                self.assert_refused(
                    sa_email(f"{cloud_id}-{name}"),
                    "named like the accounts OFE creates for a cluster's nodes",
                )

    def test_earlier_provisioning_account_is_refused(self):
        self.assert_refused(
            sa_email(f"{self.deployment}-tkfe-sa"),
            "provisioning account from an earlier version, which can change "
            "the project's IAM policy",
        )

    def test_other_deployments_server_accounts_are_refused(self):
        # Another TKFE in the same project: its accounts hold the same roles
        for suffix, words in (
            ("prov-sa", "an OFE server's provisioning account, which can change"),
            ("fe-sa", "an OFE server's web server account"),
            ("tkfe-sa", "an OFE server's provisioning account from an earlier version"),
        ):
            for deployment in ("otherdep", f"other{self.deployment}"):
                self.assert_refused(sa_email(f"{deployment}-{suffix}"), words)

    def test_workbench_accounts_are_refused(self):
        for local_part in ("sa-p-notebook-1a2b", "sa-p-notebook-0f3e9c"):
            self.assert_refused(
                sa_email(local_part), "the account OFE creates for a workbench"
            )

    def test_legacy_cluster_accounts_are_refused(self):
        for cloud_id in ("alpha-0a1b2c3d", "research-group-a-9f8e7d6c", "b-12345678"):
            self.assert_refused(
                sa_email(f"{cloud_id}-sa"),
                "named like the shared account OFE created for a cluster before "
                "this version",
            )

    def test_other_accounts_are_accepted(self):
        for local_part in (
            "own-ctl",
            f"{self.deployment}-sa",
            f"{self.deployment}-prov-sa2",
            f"{self.deployment}-tkfe-sa2",
            "prov-sa",
            "myprov-sa2",
            "dep-fe-sa-x",
            "sa-p-notebook",
            "sa-p-notebook-xyz1",
            "sa-p-notebooks-1a2b",
            "abc-sa",
            "alpha-0a1b2c3-sa",
            "alpha-0a1b2c3g-sa",
            "alpha-0a1b2c3d-sa2",
            "img-builder",
            "img-7-build",
            "alpha-ctl",
            "alpha-0a1b2c3d-login",
            "alpha-0a1b2c3g-ctl",
            "alpha-0a1b2c3d-c100",
        ):
            self.assert_accepted(sa_email(local_part))

    def test_without_a_deployment_name_every_account_is_refused(self):
        server = {
            key: value
            for key, value in utils.load_config()["server"].items()
            if key != "deployment_name"
        }
        with mock.patch.dict(utils.g_config, {"server": server}):
            self.assert_refused(
                sa_email("own-ctl"), "configuration has no deployment name"
            )


class OfeManagedAccountFormTests(TestCase):
    """The cluster form refuses an OFE account with a field error"""

    def test_provisioning_account_is_a_field_error(self):
        cluster = PROFILE_CASES["minimal"](make_world())
        deployment = utils.load_config()["server"]["deployment_name"]
        form = ClusterForm(
            data={
                **OWN_ACCOUNTS,
                "controller_service_account_email": sa_email(f"{deployment}-prov-sa"),
            },
            instance=cluster,
            user=as_superuser(cluster.owner),
        )
        self.assertFalse(form.is_valid())
        self.assertIn(
            "provisioning account",
            " ".join(form.errors["controller_service_account_email"]),
        )
        self.assertNotIn("login_service_account_email", form.errors)
        self.assertNotIn("compute_service_account_email", form.errors)


class IdentityModelTests(TestCase):
    """New clusters and images use profiles; existing rows stay legacy"""

    def test_new_rows_use_profiles(self):
        world = make_world()
        self.assertEqual(scenario_bare(world).identity_model, IdentityModel.PROFILES)
        self.assertEqual(
            make_image(world, 503, "fresh", "rocky-linux-8").identity_model,
            IdentityModel.PROFILES,
        )

    def test_cluster_and_image_share_the_field_definition(self):
        expected = identity_model_field().deconstruct()[1:]
        for model in (Cluster, Image):
            with self.subTest(model=model.__name__):
                field = model._meta.get_field("identity_model")  # pylint: disable=protected-access
                self.assertEqual(field.deconstruct()[1:], expected)

    def test_create_view_makes_a_profiles_cluster(self):
        owner, _, _ = make_world()
        self.client.force_login(owner)
        response = self.client.get(reverse("cluster-create"))
        cluster = Cluster.objects.get()
        self.assertRedirects(
            response,
            reverse("backend-create-cluster", kwargs={"pk": cluster.pk}),
            fetch_redirect_response=False,
        )
        self.assertEqual(cluster.identity_model, IdentityModel.PROFILES)


class IdentityModelBackfillTests(TransactionTestCase):
    """Adding the field to an existing table fills the rows with legacy"""

    def test_existing_rows_become_legacy(self):
        # The table before and after the upgrade, as two models in registries
        # of their own, so the app's models are untouched
        table = "ghpcfe_test_identity_backfill"

        class Before(models.Model):
            class Meta:
                apps = Apps([])
                app_label = "identity_backfill"
                db_table = table

        class After(models.Model):
            identity_model = identity_model_field()

            class Meta:
                apps = Apps([])
                app_label = "identity_backfill"
                db_table = table

        with connection.schema_editor() as editor:
            editor.create_model(Before)
        try:
            with connection.cursor() as cursor:
                cursor.execute(f"INSERT INTO {table} (id) VALUES (1)")
            # What the AddField operation of the migration the server
            # generates at start does: the old model and the new field
            with connection.schema_editor() as editor:
                editor.add_field(Before, After._meta.get_field("identity_model"))  # pylint: disable=protected-access
            with connection.cursor() as cursor:
                cursor.execute(f"SELECT id, identity_model FROM {table}")
                self.assertEqual(cursor.fetchall(), [(1, IdentityModel.LEGACY)])
            # A row created after the upgrade
            After.objects.create(id=2)
            self.assertEqual(
                After.objects.get(id=2).identity_model, IdentityModel.PROFILES
            )
        finally:
            with connection.schema_editor() as editor:
                editor.delete_model(After)


class OwnAccountFormTests(TestCase):
    """The cluster form's existing-service-account section"""

    def setUp(self):
        self.world = make_world()

    def test_some_accounts_only_is_an_error(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        form = ClusterForm(
            data={
                "controller_service_account_email": OWN_ACCOUNTS[
                    "controller_service_account_email"
                ]
            },
            instance=cluster,
            user=as_superuser(cluster.owner),
        )
        self.assertFalse(form.is_valid())
        self.assertIn(
            "Enter all three existing service accounts",
            " ".join(form.non_field_errors()),
        )

    def test_default_compute_account_is_refused(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        form = ClusterForm(
            data={
                **OWN_ACCOUNTS,
                "compute_service_account_email": "123456789012-compute@developer.gserviceaccount.com",
            },
            instance=cluster,
            user=as_superuser(cluster.owner),
        )
        self.assertFalse(form.is_valid())
        self.assertIn("compute_service_account_email", form.errors)
        self.assertNotIn("controller_service_account_email", form.errors)
        self.assertNotIn(
            "Enter all three existing service accounts",
            " ".join(form.non_field_errors()),
        )

    def update_page(self, cluster):
        cluster.owner.is_superuser = True
        cluster.owner.save(update_fields=["is_superuser"])
        self.client.force_login(cluster.owner)
        with mock.patch(
            "ghpcfe.cluster_manager.cloud_info.get_region_zone_info",
            return_value={},
        ):
            response = self.client.get(
                reverse("cluster-update", kwargs={"pk": cluster.pk})
            )
        self.assertEqual(response.status_code, 200)
        return response

    def test_section_shown_for_profiles_clusters_only(self):
        response = self.update_page(PROFILE_CASES["minimal"](self.world))
        self.assertContains(response, "Existing service accounts (optional)")
        for field_name in ClusterForm.OWN_SERVICE_ACCOUNT_FIELDS:
            self.assertContains(response, f'name="{field_name}"')

        clear_clusters()
        response = self.update_page(scenario_minimal(self.world, IdentityModel.LEGACY))
        self.assertNotContains(response, "Existing service accounts")
        for field_name in ClusterForm.OWN_SERVICE_ACCOUNT_FIELDS:
            self.assertNotContains(response, f'name="{field_name}"')

    def test_fields_are_locked_after_create_and_for_legacy(self):
        new_profiles = PROFILE_CASES["minimal"](self.world)
        for field_name in ClusterForm.OWN_SERVICE_ACCOUNT_FIELDS:
            self.assertFalse(ClusterForm(instance=new_profiles).fields[field_name].disabled)
        new_profiles.status = "r"
        legacy = scenario_bare(self.world, IdentityModel.LEGACY)
        for cluster in (new_profiles, legacy):
            for field_name in ClusterForm.OWN_SERVICE_ACCOUNT_FIELDS:
                with self.subTest(cluster=cluster.name, field=field_name):
                    self.assertTrue(
                        ClusterForm(instance=cluster).fields[field_name].disabled
                    )
