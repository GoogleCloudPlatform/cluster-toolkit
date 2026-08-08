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

"""Blueprint rendering tests for CMEK.

Renders the real cluster and partition templates through Django's
template engine (the .j2 files are DjangoTemplates, not Jinja2), parses
the resulting YAML, and asserts exact values on every module setting.
Covers: controller, login, two partitions (one with two additional
disks), the Slurm bucket key, and rendering without a key.

Run with:
    python manage.py test ghpcfe.tests --settings=website.test_settings
"""

import types

import yaml
from django.template import engines as template_engines
from django.test import SimpleTestCase

from ghpcfe.cluster_manager import cmek

KEY = (
    "projects/key-proj/locations/us-central1"
    "/keyRings/ring/cryptoKeys/ofe-key"
)
DISK_REF = "$(cmek_key_iam.disk_encryption_key)"
BUCKET_REF = "$(cmek_key_iam.slurm_bucket_kms_key)"


def make_cluster():
    """Attribute stub mirroring the fields the templates read."""
    c = types.SimpleNamespace()
    c.id = 42
    c.cloud_id = "testcluster42"
    c.cloud_region = "us-central1"
    c.cloud_zone = "us-central1-a"
    c.project_id = "hpc-proj"
    c.use_bigquery = False
    c.subnet = types.SimpleNamespace(
        cloud_id="test-subnet",
        vpc=types.SimpleNamespace(cloud_id="test-vpc"),
    )
    c.controller_instance_type = "e2-small"
    c.controller_disk_type = "pd-standard"
    c.controller_disk_size = 50
    c.num_login_nodes = 1
    c.login_node_instance_type = "e2-small"
    c.login_node_disk_type = "pd-standard"
    c.login_node_disk_size = 50
    return c


def make_partition(name, additional_disk_count=0):
    p = types.SimpleNamespace()
    p.name = name
    p.exclusive = "False"
    p.enable_tier1_networking = False
    p.enable_hyperthreads = True
    p.enable_placement = False
    p.machine_type = "e2-small"
    p.reservation_name = ""
    p.dynamic_node_count = 2
    p.static_node_count = 0
    p.boot_disk_size = 50
    p.boot_disk_type = "pd-standard"
    p.image = None
    p.additional_disk_count = additional_disk_count
    p.additional_disk_size = 10
    p.additional_disk_type = "pd-standard"
    p.additional_disk_auto_delete = True
    p.GPU_per_node = 0
    return p


def indent(text, level):
    pad = "  " * level
    return "\n".join(pad + l if l else l for l in text.split("\n"))


def render_blueprint(cmek_ctx, partitions=None, slurm_conf_template_yaml="",
                     artifact_registry_yaml=""):
    """Compose the blueprint the same way _prepare_ghpc_yaml does."""
    engine = template_engines["django"]
    cluster = make_cluster()
    partitions = partitions if partitions is not None else [
        make_partition("batch"),
        make_partition("bigmem", additional_disk_count=2),
    ]

    part_template = engine.get_template("blueprint/partition_config.yaml.j2")
    parts_yaml = []
    for i, part in enumerate(partitions):
        parts_yaml.append(indent(part_template.render({
            "part": part,
            "part_id": f"partition_{i}",
            "uses_str": "  - hpc_network",
            "cluster": cluster,
            "disk_range": list(range(part.additional_disk_count)),
            "exclusive": part.exclusive,
            "startup_bucket": "test-bucket",
            "cmek": cmek_ctx,
        }), 1))

    cluster_template = engine.get_template("blueprint/cluster_config.yaml.j2")
    rendered = cluster_template.render({
        "project_id": cluster.project_id,
        "site_name": "test-site",
        "filesystems_yaml": "",
        "partitions_yaml": "\n\n".join(parts_yaml),
        "artifact_registry_yaml": artifact_registry_yaml,
        "cloudsql_yaml": "",
        "cluster": cluster,
        "controller_uses": "    - hpc_network",
        "login_uses": "",
        "controller_sa": "sa",
        "startup_bucket": "test-bucket",
        "cmek": cmek_ctx,
        "slurm_conf_template_yaml": slurm_conf_template_yaml,
    })
    return yaml.safe_load(rendered)


def modules_by_id(blueprint):
    mods = {}
    for group in blueprint["deployment_groups"]:
        for mod in group["modules"]:
            mods[mod["id"]] = mod
    return mods


class BlueprintCmekRenderingTests(SimpleTestCase):
    """Parsed-YAML assertions on the real templates."""

    def setUp(self):
        self.ctx = cmek.blueprint_context(
            KEY, region="us-central1", zone="us-central1-a",
            service_agents={"compute", "storage"})

    def test_controller_gets_disk_and_bucket_keys(self):
        mods = modules_by_id(render_blueprint(self.ctx))
        settings = mods["slurm_controller"]["settings"]
        self.assertEqual(settings["disk_encryption_key"], DISK_REF)
        self.assertEqual(settings["slurm_bucket_kms_key"], BUCKET_REF)
        self.assertNotIn(
            "disk_encryption_key_service_account", settings)

    def test_login_gets_disk_key(self):
        mods = modules_by_id(render_blueprint(self.ctx))
        settings = mods["slurm_login"]["settings"]
        self.assertEqual(settings["disk_encryption_key"], DISK_REF)
        self.assertNotIn("slurm_bucket_kms_key", settings)

    def test_every_nodeset_gets_disk_key(self):
        mods = modules_by_id(render_blueprint(self.ctx))
        nodesets = [m for i, m in mods.items() if i.endswith("-nodeset")]
        self.assertEqual(len(nodesets), 2)
        for mod in nodesets:
            self.assertEqual(
                mod["settings"]["disk_encryption_key"], DISK_REF)

    def test_every_additional_disk_gets_key(self):
        mods = modules_by_id(render_blueprint(self.ctx))
        with_disks = mods["partition_1-nodeset"]["settings"]
        disks = with_disks["additional_disks"]
        self.assertEqual(len(disks), 2)
        for disk in disks:
            self.assertEqual(disk["disk_encryption_key"], DISK_REF)
        # the no-disk partition must not render an additional_disks list
        self.assertNotIn(
            "additional_disks", mods["partition_0-nodeset"]["settings"])

    def test_service_account_email_reference_preserved(self):
        # The blueprint reference must survive YAML parsing verbatim so
        # ghpc can resolve it against the hpc_service_account module.
        mods = modules_by_id(render_blueprint(self.ctx))
        self.assertEqual(
            mods["slurm_controller"]["settings"]["service_account_email"],
            "$(hpc_service_account.service_account_email)")

    def test_key_and_iam_modules_are_emitted(self):
        mods = modules_by_id(render_blueprint(self.ctx))
        key = mods["cmek_key"]
        self.assertEqual(
            key["source"], "community/modules/security/pre-existing-kms-key")
        self.assertEqual(key["settings"]["project_id"], "key-proj")
        self.assertEqual(key["settings"]["location"], "us-central1")
        self.assertEqual(key["settings"]["key_ring_name"], "ring")
        self.assertEqual(key["settings"]["key_name"], "ofe-key")

        iam = mods["cmek_key_iam"]
        self.assertEqual(
            iam["source"], "community/modules/security/kms-key-iam")
        self.assertEqual(iam["use"], ["cmek_key"])
        # compute for the disks, storage for the Slurm bucket. Filestore is
        # absent on purpose: this blueprint mounts filesystems, it does not
        # create them.
        self.assertEqual(
            sorted(iam["settings"]["service_agents"]), ["compute", "storage"])

    def test_cmek_enables_the_cloudkms_api(self):
        mods = modules_by_id(render_blueprint(self.ctx))
        services = mods["services-api"]["settings"]["gcp_service_list"]
        self.assertIn("cloudkms.googleapis.com", services)

    def test_one_services_api_module_when_cmek_and_registry_combine(self):
        # A second `- id: services-api` block would be a duplicate module
        # id and fail blueprint expansion, so the two features must share
        # one block rather than each emitting their own.
        registry_yaml = "\n".join([
            "  - id: some_registry",
            "    source: community/modules/container/artifact-registry",
            "    settings:",
            "      repository_id: r",
        ])
        blueprint = render_blueprint(self.ctx, artifact_registry_yaml=registry_yaml)
        ids = [m["id"] for g in blueprint["deployment_groups"]
               for m in g["modules"]]
        self.assertEqual(ids.count("services-api"), 1)
        services = modules_by_id(blueprint)["services-api"]["settings"][
            "gcp_service_list"]
        self.assertIn("cloudkms.googleapis.com", services)
        self.assertIn("artifactregistry.googleapis.com", services)

    def test_no_key_emits_neither_module(self):
        mods = modules_by_id(render_blueprint(None))
        self.assertNotIn("cmek_key", mods)
        self.assertNotIn("cmek_key_iam", mods)

    def test_no_key_renders_no_cmek_settings(self):
        """Without a key the rendering is unchanged."""
        blueprint = render_blueprint(None)
        text = yaml.safe_dump(blueprint)
        self.assertNotIn("disk_encryption_key", text)
        self.assertNotIn("slurm_bucket_kms_key", text)
        self.assertNotIn("kms", text)


class SlurmAccountingOverrideTests(SimpleTestCase):
    """OFE disables Slurm accounting when no Cloud SQL is
    attached, so slurmctld does not hang on a database the module's
    boot scripts never correctly provision."""

    def test_helper_empty_when_cloudsql_attached(self):
        from ghpcfe.cluster_manager.clusterinfo import (
            slurm_conf_template_setting,
        )
        self.assertEqual(slurm_conf_template_setting(True), "")

    def test_helper_disables_accounting_without_cloudsql(self):
        from ghpcfe.cluster_manager.clusterinfo import (
            slurm_conf_template_setting,
        )
        block = slurm_conf_template_setting(False)
        self.assertIn("slurm_conf_template: |", block)
        self.assertIn("AccountingStorageType=accounting_storage/none", block)
        self.assertNotIn("accounting_storage/slurmdbd", block)
        # The runtime str.format placeholders must survive untouched so
        # slurm-gcp's install_slurm_conf() can fill them on the controller.
        self.assertIn("{control_host}", block)
        self.assertIn("ClusterName={name}", block)

    def test_rendered_controller_gets_no_accounting_template(self):
        from ghpcfe.cluster_manager.clusterinfo import (
            slurm_conf_template_setting,
        )
        mods = modules_by_id(render_blueprint(
            None,
            slurm_conf_template_yaml=slurm_conf_template_setting(False),
        ))
        tpl = mods["slurm_controller"]["settings"]["slurm_conf_template"]
        self.assertIn(
            "AccountingStorageType=accounting_storage/none", tpl)
        self.assertNotIn("accounting_storage/slurmdbd", tpl)
        # cloud.conf include must be preserved so cloud nodes still work.
        self.assertIn("include cloud.conf", tpl)

    def test_rendered_controller_omits_template_with_cloudsql(self):
        from ghpcfe.cluster_manager.clusterinfo import (
            slurm_conf_template_setting,
        )
        mods = modules_by_id(render_blueprint(
            None,
            slurm_conf_template_yaml=slurm_conf_template_setting(True),
        ))
        self.assertNotIn(
            "slurm_conf_template", mods["slurm_controller"]["settings"])


class ClusterKeyWiringTests(SimpleTestCase):
    """The cluster's own key reaches the blueprint context."""

    def _clusterinfo(self, key):
        from ghpcfe.cluster_manager.clusterinfo import ClusterInfo
        ci = object.__new__(ClusterInfo)
        ci.cluster = types.SimpleNamespace(
            id=7, project_id="hpc-proj",
            cloud_region="us-central1", cloud_zone="us-central1-a",
            cmek_key=key)
        return ci

    def test_key_parts_are_parsed_for_the_lookup_module(self):
        ctx = self._clusterinfo(KEY)._cmek_context()
        self.assertEqual(ctx["key_project"], "key-proj")
        self.assertEqual(ctx["key_location"], "us-central1")
        self.assertEqual(ctx["key_ring"], "ring")
        self.assertEqual(ctx["key_name"], "ofe-key")

    def test_every_template_field_is_a_reference_not_the_key(self):
        # A literal key name would encrypt the resources just the same and
        # be unordered with respect to the grants; the reference is what
        # makes gcluster emit the dependency.
        ctx = self._clusterinfo(KEY)._cmek_context()
        for field in ("disk_key", "bucket_key", "artifact_registry_key",
                      "secret_key", "cloudsql_key"):
            with self.subTest(field=field):
                self.assertTrue(ctx[field].startswith("$(cmek_key_iam."))
                self.assertNotIn(KEY, ctx[field])

    def test_blank_key_means_no_cmek(self):
        self.assertIsNone(self._clusterinfo("")._cmek_context())

    def test_key_in_another_region_is_rejected(self):
        wrong = (
            "projects/key-proj/locations/europe-west4"
            "/keyRings/ring/cryptoKeys/ofe-key")
        with self.assertRaises(cmek.CmekConfigError):
            self._clusterinfo(wrong)._cmek_context()


class CmekKeyFieldReachabilityTests(SimpleTestCase):
    """The CMEK key field must be reachable from the browser.

    It previously existed on the model and in ClusterForm.Meta.fields but
    was rendered by no template, so it could not be set by anyone --
    including a superuser. A field nothing renders is not a feature, so
    these assert both halves: the form offers it, and the template that
    the view actually renders puts it on the page.
    """

    FORMS_AND_TEMPLATES = [
        ("ClusterForm", "cluster/update_form.html"),
        ("FilestoreForm", "filesystem/filestore_create_form.html"),
        ("FilestoreForm", "filesystem/filestore_update_form.html"),
    ]

    def test_every_cmek_form_declares_the_field(self):
        # FilestoreForm.__init__ makes a live region lookup, so the
        # declared field list is what can be asserted here without a
        # network or a database.
        #
        # Workbench is deliberately absent: it is a Terraform root copied
        # per workbench rather than a generated blueprint, so wiring it
        # for CMEK needs changes to workbench_tf that are not part of
        # this change. A form field nothing renders and nothing consumes
        # would be worse than no field at all.
        from ghpcfe import forms as ofe_forms
        for name in ("ClusterForm", "FilestoreForm", "ImageForm"):
            with self.subTest(form=name):
                fields = getattr(ofe_forms, name).Meta.fields
                self.assertIn("cmek_key", fields)

    def test_ordinary_users_see_the_cluster_field(self):
        # ClusterForm is instantiated rather than inspected because this
        # is exactly where the field used to disappear: __init__ popped
        # it for anyone who was not staff or a superuser.
        from ghpcfe.forms import ClusterForm
        self.assertIn("cmek_key", ClusterForm().fields)

    def test_every_explicit_template_renders_the_field(self):
        engine = template_engines["django"]
        for _, template_name in self.FORMS_AND_TEMPLATES:
            with self.subTest(template=template_name):
                source = engine.get_template(template_name).template.source
                self.assertIn("form.cmek_key", source)

    def test_image_template_auto_renders_all_fields(self):
        # image-create.html loops over form.visible_fields rather than
        # naming each one, so cmek_key needs no block there. Assert the
        # loop, so that a future rewrite to explicit fields fails here
        # instead of silently dropping the field.
        engine = template_engines["django"]
        source = engine.get_template("image/image-create.html").template.source
        self.assertIn("form.visible_fields", source)

class ClusterInfoApiSurfaceTests(SimpleTestCase):
    """Every self._method() ClusterInfo calls must exist.

    Removing the CMEK policy engine meant deleting a contiguous run of
    methods, and the run also contained five that had nothing to do with
    CMEK -- _prepare_ghpc_filesystems and friends. Nothing caught it: no
    unit test calls _prepare_ghpc_yaml, so the missing attributes would
    only have surfaced as an AttributeError when a user created a
    cluster.
    """

    def test_no_method_is_called_but_undefined(self):
        import ast
        import inspect
        from ghpcfe.cluster_manager import clusterinfo

        source = inspect.getsource(clusterinfo)
        tree = ast.parse(source)
        cls = next(
            n for n in tree.body
            if isinstance(n, ast.ClassDef) and n.name == "ClusterInfo"
        )
        defined = {
            n.name for n in cls.body
            if isinstance(n, (ast.FunctionDef, ast.AsyncFunctionDef))
        }
        called = {
            n.func.attr
            for n in ast.walk(cls)
            if isinstance(n, ast.Call)
            and isinstance(n.func, ast.Attribute)
            and isinstance(n.func.value, ast.Name)
            and n.func.value.id == "self"
        }
        inherited = {m for m in dir(clusterinfo.ClusterInfo)}
        missing = sorted(called - defined - inherited)
        self.assertEqual(missing, [], f"called but never defined: {missing}")

class FilestoreBlueprintCmekTests(SimpleTestCase):
    """The standalone Filestore blueprint must grant the Filestore agent
    itself: the cluster blueprint only mounts filesystems, so nothing else
    grants on their behalf."""

    def _render(self, key):
        import json
        import tempfile
        import pathlib
        from ghpcfe.cluster_manager import filesystem

        fs = types.SimpleNamespace(
            name="fs1", cloud_region="us-central1", cloud_zone="us-central1-a",
            capacity=1024, cmek_key=key,
            cloud_credential=types.SimpleNamespace(
                detail=json.dumps({"project_id": "hpc-proj"})),
            vpc=types.SimpleNamespace(cloud_id="test-vpc"),
            exports=types.SimpleNamespace(
                first=lambda: types.SimpleNamespace(export_name="/share")),
            get_performance_tier_display=lambda: "ZONAL",
        )
        with tempfile.TemporaryDirectory() as tmp:
            filesystem.write_filestore_yaml(fs, pathlib.Path(tmp))
            text = (pathlib.Path(tmp) / "filesystem.yaml").read_text()
        return yaml.safe_load(text), text

    def test_key_and_iam_modules_emitted_and_referenced(self):
        blueprint, _ = self._render(KEY)
        mods = modules_by_id(blueprint)
        self.assertEqual(
            mods["cmek_key"]["source"],
            "community/modules/security/pre-existing-kms-key")
        self.assertEqual(
            mods["cmek_key_iam"]["settings"]["service_agents"], ["filestore"])
        self.assertEqual(
            mods["fs1"]["settings"]["kms_key_name"],
            "$(cmek_key_iam.kms_key_name)")

    def test_cloudkms_api_is_enabled(self):
        blueprint, _ = self._render(KEY)
        services = modules_by_id(blueprint)["services-api"][
            "settings"]["gcp_service_list"]
        self.assertIn("cloudkms.googleapis.com", services)

    def test_no_key_emits_no_cmek(self):
        blueprint, text = self._render("")
        self.assertNotIn("cmek_key", text)
        self.assertNotIn("kms_key_name", text)

class ImageBlueprintCmekTests(SimpleTestCase):
    """A custom image must be ordered behind its grant like anything else.

    The image builds with Packer in a second deployment group, so the key
    and IAM modules go in the terraform group and the packer module `use`s
    the IAM module across groups. gcluster bridges that with an output on
    the terraform group and an import into the packer one.
    """

    def _render(self, key):
        import json
        import pathlib
        import tempfile
        from unittest import mock
        from ghpcfe.cluster_manager.image import ImageBackend

        b = object.__new__(ImageBackend)
        b.blueprint_name = "img-bp"
        b.image = types.SimpleNamespace(
            id=1, name="img", family="fam", cloud_region="us-central1",
            cloud_zone="us-central1-a", source_image_project="p",
            source_image_family="f", enable_os_login=True,
            block_project_ssh_keys=False, cmek_key=key,
            cloud_credential=types.SimpleNamespace(
                detail=json.dumps({"project_id": "hpc-proj"})),
            startup_script=types.SimpleNamespace(all=lambda: []),
            save=lambda **kw: None, status="c",
        )
        with tempfile.TemporaryDirectory() as tmp:
            b.image_dir = pathlib.Path(tmp)
            with mock.patch.object(ImageBackend, "_get_credentials_file",
                                   return_value=pathlib.Path("/tmp/creds")):
                b._create_blueprint()
            text = (b.image_dir / "image.yaml").read_text()
        return yaml.safe_load(text), text

    def test_key_and_iam_modules_are_emitted_and_used(self):
        blueprint, _ = self._render(KEY)
        mods = modules_by_id(blueprint)
        self.assertEqual(
            mods["cmek_key"]["source"],
            "community/modules/security/pre-existing-kms-key")
        # Packer builds a Compute Engine disk and then an image from it.
        self.assertEqual(
            mods["cmek_key_iam"]["settings"]["service_agents"], ["compute"])
        # The image must depend on the grant, which is what `use` gives it.
        self.assertIn("cmek_key_iam", mods["custom-image"]["use"])

    def test_cloudkms_api_is_enabled(self):
        blueprint, _ = self._render(KEY)
        self.assertIn(
            "cloudkms.googleapis.com",
            modules_by_id(blueprint)["services-api"]["settings"][
                "gcp_service_list"])

    def test_no_key_emits_no_cmek(self):
        _, text = self._render("")
        self.assertNotIn("cmek_key", text)
        self.assertNotIn("cloudkms", text)


class CmekKeySuggestionTests(SimpleTestCase):
    """The key field offers suggestions without constraining the input.

    get_kms_keys existed but was called from nowhere, so OFE never
    actually read the key list from the API. These assert the whole
    path -- widget attribute, datalist element, endpoint -- because the
    failure mode is silent: a datalist whose id nothing matches simply
    renders an ordinary text box, and no error is raised anywhere.
    """

    CMEK_TEMPLATES = [
        "cluster/update_form.html",
        "filesystem/filestore_create_form.html",
        "filesystem/filestore_update_form.html",
        "image/image-create.html",
    ]

    def test_every_cmek_widget_points_at_the_datalist(self):
        from ghpcfe import forms as ofe_forms
        for name in ("ClusterForm", "FilestoreForm", "ImageForm"):
            with self.subTest(form=name):
                widget = getattr(ofe_forms, name).Meta.widgets["cmek_key"]
                self.assertEqual(
                    widget.attrs.get("list"), "cmek-key-options")

    def test_every_cmek_template_includes_the_datalist(self):
        engine = template_engines["django"]
        for name in self.CMEK_TEMPLATES:
            with self.subTest(template=name):
                source = engine.get_template(name).template.source
                self.assertIn("_cmek_key_datalist.html", source)

    def test_datalist_id_matches_the_widget_attribute(self):
        # The two halves are in different files and nothing links them
        # but this string, so a rename in one place must fail here.
        engine = template_engines["django"]
        source = engine.get_template(
            "_cmek_key_datalist.html").template.source
        self.assertIn('<datalist id="cmek-key-options">', source)

    def test_key_field_stays_free_text(self):
        # A select would forbid a key in a project these credentials
        # cannot list, which is exactly what pre-existing-kms-key is for.
        from ghpcfe import forms as ofe_forms
        from django import forms as django_forms
        for name in ("ClusterForm", "FilestoreForm", "ImageForm"):
            with self.subTest(form=name):
                widget = getattr(ofe_forms, name).Meta.widgets["cmek_key"]
                self.assertIsInstance(widget, django_forms.TextInput)

    def test_url_name_matches_the_route_registration(self):
        # test_settings uses an empty URLconf, so reverse() cannot be
        # used here. The real risk is a rename on one side only, so
        # assert the template's {% url %} name against the basename the
        # router registers.
        import pathlib
        engine = template_engines["django"]
        source = engine.get_template(
            "_cmek_key_datalist.html").template.source
        self.assertIn("{% url 'api-kmskeys-list' %}", source)
        urls = pathlib.Path(
            __file__).resolve().parent.parent / "urls.py"
        self.assertIn('basename="api-kmskeys"', urls.read_text())


class FilestoreCmekTierTests(SimpleTestCase):
    """OFE must reject a CMEK tier the Filestore module will reject.

    OFE used to deny only BASIC* while the module allows only
    ZONAL/REGIONAL/ENTERPRISE, so HIGH_SCALE_SSD passed here and failed at
    terraform plan -- an error naming the module rather than the tier the
    user picked. These pin the two rules to each other.
    """

    KEY = ("projects/key-proj/locations/us-central1"
           "/keyRings/ring/cryptoKeys/k")

    def _check(self, tier):
        from ghpcfe.cluster_manager import filesystem
        fs = types.SimpleNamespace(
            cmek_key=self.KEY, cloud_region="us-central1",
            cloud_zone="us-central1-a")
        return filesystem._filestore_cmek_key(fs, tier)

    def test_allowed_tiers_pass(self):
        for tier in ("ZONAL", "REGIONAL", "ENTERPRISE"):
            with self.subTest(tier=tier):
                self.assertIn("cryptoKeys/k", self._check(tier))

    def test_rejected_tiers_raise(self):
        # HIGH_SCALE_SSD is the case that regressed: OFE offers it, and it
        # is not in the module's allow-list.
        from ghpcfe.cluster_manager import cmek
        for tier in ("HIGH_SCALE_SSD", "BASIC_HDD", "BASIC_SSD"):
            with self.subTest(tier=tier):
                with self.assertRaises(cmek.CmekConfigError):
                    self._check(tier)

    def test_ofe_list_matches_the_module_precondition(self):
        # The module is the authority; if its allow-list changes, this
        # fails rather than OFE silently drifting out of step again.
        import pathlib, re
        from ghpcfe.cluster_manager import filesystem
        main_tf = (pathlib.Path(__file__).resolve()
                   .parents[6] / "modules" / "file-system" / "filestore"
                   / "main.tf")
        text = main_tf.read_text()
        # Anchor on the kms_key_name precondition specifically: main.tf
        # has several contains(...) over filestore_tier, and the first one
        # (is_high_capacity_tier) is not the CMEK rule.
        m = re.search(
            r'var\.kms_key_name == null \|\| contains\(\[([^\]]*)\],'
            r'\s*var\.filestore_tier\)', text)
        self.assertIsNotNone(m, "CMEK precondition not found in main.tf")
        module_tiers = set(re.findall(r'"([A-Z_]+)"', m.group(1)))
        self.assertEqual(set(filesystem.CMEK_FILESTORE_TIERS), module_tiers)

    def test_no_key_returns_none_whatever_the_tier(self):
        fs = types.SimpleNamespace(
            cmek_key="", cloud_region="us-central1",
            cloud_zone="us-central1-a")
        from ghpcfe.cluster_manager import filesystem
        self.assertIsNone(filesystem._filestore_cmek_key(fs, "BASIC_HDD"))
