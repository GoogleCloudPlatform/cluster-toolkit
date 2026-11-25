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

"""The optional condition of the service-account-profiles module's
bucket_bindings, evaluated by Terraform.

A bucket binding gets an IAM condition only when its input sets one, and a
binding without one is encoded in local.grants_fingerprint exactly as before
conditions were accepted ({ bucket, role }), so an existing deployment's
propagation wait does not re-run.

The module is the root of a scratch configuration, so `terraform console`
can read its locals. Offline: `terraform init -plugin-dir` installs the
providers from a local copy, and `terraform console` configures no provider
and calls no API. The local copy is OFE_TEST_TF_PLUGIN_DIR when it is set;
otherwise the first of Terraform's plugin cache (TF_PLUGIN_CACHE_DIR) and
the module's own .terraform/providers that holds both providers. When
OFE_TEST_TF_PLUGIN_DIR is unset, the tests are skipped when terraform is not
on PATH, or when no copy of the providers is found (a fresh checkout); the
skip reason says how to make one. When it is set, terraform missing from
PATH, or a copy that is missing or incomplete, fails the tests, so a run
that asks for them never skips them.

`terraform console` has no state, so it shows a resource as
"(known after apply)"; the condition block is checked by evaluating the
resource's own dynamic block for_each expression, read from main.tf, for
each binding.
"""

import json
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

from django.test import SimpleTestCase

from ghpcfe.cluster_manager import credentials
from ghpcfe.tests.test_blueprint_identities import (
    cel_terms,
    node_files_cases,
    node_files_expression,
)

OFE_DIR = Path(__file__).resolve().parents[3]
REPO_ROOT = OFE_DIR.parents[2]
MODULE = REPO_ROOT / "community/modules/iam/service-account-profiles"
MODULE_FILES = ("main.tf", "variables.tf", "outputs.tf", "versions.tf")
TERRAFORM = shutil.which("terraform")
# A hang guard, not a wait: each call is one offline init or console
# (measured 2026-11-18 on a 6-core developer machine: init 0.18 s, as it
# links the local providers rather than copying them; console, which loads
# the 134 MB google provider, 0.8 s), and when it expires
# subprocess.run raises TimeoutExpired, which fails the test
TERRAFORM_TIMEOUT_SECONDS = 60
PROVIDERS = ("google", "time")


def _missing_providers(directory):
    """The module's providers that directory holds no copy of"""
    return [
        name
        for name in PROVIDERS
        if not (directory / "registry.terraform.io" / "hashicorp" / name).is_dir()
    ]


def _plugin_dir():
    """(directory, None) for the local copy of the module's providers, or
    (None, reason) naming every place looked in and what it lacks"""
    explicit = os.environ.get("OFE_TEST_TF_PLUGIN_DIR")
    if explicit:
        # A copy named on purpose is used or reported, never passed over
        candidates = [("OFE_TEST_TF_PLUGIN_DIR", Path(explicit))]
    else:
        candidates = [
            ("the module's .terraform", MODULE / ".terraform" / "providers")
        ]
        cache = os.environ.get("TF_PLUGIN_CACHE_DIR")
        if cache:
            candidates.insert(0, ("TF_PLUGIN_CACHE_DIR", Path(cache)))
    looked = []
    for source, directory in candidates:
        missing = _missing_providers(directory)
        if not missing:
            return directory, None
        looked.append(
            f"{source} ({directory}): no hashicorp/{' or hashicorp/'.join(missing)}"
        )
    return None, (
        "no local copy of the hashicorp/google and hashicorp/time providers. "
        "Looked in: " + "; ".join(looked) + ". Make one with "
        f"`terraform -chdir={MODULE} init -backend=false` (downloads them "
        "once; Git ignores the result), or set OFE_TEST_TF_PLUGIN_DIR to a "
        "directory holding registry.terraform.io/hashicorp/{google,time}."
    )

BUCKET = "ofe-test-control-bucket"
PLAIN = {"bucket": BUCKET, "role": "roles/storage.objectViewer"}
CONDITION = {
    "title": "ofe-cluster-7-prefix",
    "description": "Objects under clusters/7/ in the TKFE control bucket only.",
    "expression": (
        f'resource.name.startsWith("projects/_/buckets/{BUCKET}/objects/clusters/7/")'
    ),
}
SCOPED = {
    "bucket": BUCKET,
    "role": "roles/storage.objectAdmin",
    "condition": CONDITION,
}
# The cluster blueprint template, whose node read grants carry the condition
CLUSTER_TEMPLATE = (
    OFE_DIR / "website/ghpcfe/templates/blueprint/cluster_config.yaml.j2"
)


def _template_node_files_expression(bucket, cluster_id):
    """The node read grant's condition expression as the cluster blueprint
    template writes it (one line for each node profile, all the same), for
    bucket and cluster_id"""
    lines = {
        line.strip()
        for line in CLUSTER_TEMPLATE.read_text(encoding="utf-8").splitlines()
        if line.strip().startswith("expression:") and "ansible_setup" in line
    }
    if len(lines) != 1:
        raise AssertionError(f"expected one node-files expression, found {lines}")
    (line,) = lines
    return (
        line.removeprefix("expression:")
        .strip()
        .replace("{{ identity.bucket }}", bucket)
        .replace("{{ cluster.id }}", str(cluster_id))
    )


# A node's control-bucket read grant, as the cluster blueprint writes it
NODE_CLUSTER_ID = 7
NODE_CONDITION = {
    "title": f"ofe-cluster-{NODE_CLUSTER_ID}-node-files",
    "description": (
        "Lists the TKFE control bucket; reads only clusters/ansible_setup, "
        f"the objects under it and clusters/{NODE_CLUSTER_ID}/bootstrap_* in it."
    ),
    "expression": _template_node_files_expression(BUCKET, NODE_CLUSTER_ID),
}
NODE_FILES = {
    "bucket": BUCKET,
    "role": "roles/storage.objectViewer",
    "condition": NODE_CONDITION,
}
INPUTS = {
    "project_id": "example-project",
    "deployment_name": "dep",
    "name": "ctl",
    "service_account_email": "ctl@example-project.iam.gserviceaccount.com",
    "profile": "slurm-controller",
}
VAR_FILES = {
    "unconditional": {**INPUTS, "bucket_bindings": {"plain": PLAIN}},
    "mixed": {**INPUTS, "bucket_bindings": {"plain": PLAIN, "scoped": SCOPED}},
    "node_files": {**INPUTS, "bucket_bindings": {"ofe_bucket": NODE_FILES}},
}


def _terraform_env():
    """The environment for Terraform: no update check, and no ambient
    credentials or TF_* settings (TF_VAR_*, TF_CLI_ARGS*) that could change
    what is evaluated"""
    env = {
        key: value
        for key, value in os.environ.items()
        if key not in credentials.AMBIENT_CREDENTIAL_VARIABLES
        and not key.startswith("TF_")
    }
    env["CHECKPOINT_DISABLE"] = "1"
    env["TF_IN_AUTOMATION"] = "1"
    return env


def _hcl_literal(value):
    """value as an HCL expression (jsondecode of its JSON text)"""
    text = json.dumps(value).replace("${", "$${").replace("%{", "%%{")
    return f"jsondecode({json.dumps(text)})"


# Before conditions were accepted, the wait re-ran on jsonencode(local.grants)
# with each bucket binding { bucket, role }: this is that encoding for the
# "unconditional" inputs
BEFORE_CONDITIONS = (
    "jsonencode(merge(local.grants, {"
    "bindings = merge(local.grants.bindings, "
    f"{{bucket = {_hcl_literal({'plain': PLAIN})}}})"
    "}))"
)


def _wait_grants_trigger():
    """The grants trigger expression of time_sleep.wait_for_iam in the
    module's main.tf"""
    text = (MODULE / "main.tf").read_text()
    start = re.search(
        r'^resource\s+"time_sleep"\s+"wait_for_iam"\s*\{', text, re.M
    )
    if start is None:
        raise AssertionError("main.tf has no time_sleep.wait_for_iam")
    resource = text[start.end():text.index("\n}\n", start.end())]
    triggers = re.search(r"\btriggers\s*=\s*\{(.*?)\n\s*\}", resource, re.S)
    grants = triggers and re.search(
        r"(?m)^\s*grants\s*=\s*(.+?)\s*$", triggers.group(1)
    )
    if not grants:
        raise AssertionError("time_sleep.wait_for_iam has no grants trigger")
    return grants.group(1)


def _bucket_condition_block():
    """(for_each expression, content body) of the dynamic condition block
    of google_storage_bucket_iam_member.this in the module's main.tf"""
    text = (MODULE / "main.tf").read_text()
    start = re.search(
        r'^resource\s+"google_storage_bucket_iam_member"\s+"this"\s*\{',
        text,
        re.M,
    )
    resource = text[start.end():text.index("\n}\n", start.end())]
    dynamic = re.search(
        r'dynamic\s+"condition"\s*\{\s*for_each\s*=\s*(.+?)\n\s*content\s*\{(.*?)\}',
        resource,
        re.S,
    )
    return dynamic.group(1).strip(), dynamic.group(2)


class ProfilesModuleConditionTests(SimpleTestCase):
    """bucket_bindings with and without a condition"""

    @classmethod
    def setUpClass(cls):
        if not TERRAFORM:
            if os.environ.get("OFE_TEST_TF_PLUGIN_DIR"):
                # A run that names a provider copy asks for these tests
                raise AssertionError(
                    "OFE_TEST_TF_PLUGIN_DIR is set but terraform is not on PATH"
                )
            raise unittest.SkipTest("needs terraform on PATH")
        plugin_dir, problem = _plugin_dir()
        if problem:
            if os.environ.get("OFE_TEST_TF_PLUGIN_DIR"):
                # A copy named on purpose must work
                raise AssertionError(f"OFE_TEST_TF_PLUGIN_DIR is set but there is {problem}")
            # A fresh checkout has no provider copy: skip, saying why and how
            # to enable the tests, rather than fail the whole suite
            raise unittest.SkipTest(f"terraform is on PATH but there is {problem}")
        super().setUpClass()
        cls.root = Path(tempfile.mkdtemp(prefix="ofe-profiles-condition-"))
        cls.addClassCleanup(shutil.rmtree, cls.root, ignore_errors=True)
        for name in MODULE_FILES:
            (cls.root / name).symlink_to(MODULE / name)
        for name, values in VAR_FILES.items():
            (cls.root / f"{name}.tfvars.json").write_text(json.dumps(values))
        init = cls.terraform(
            "init", "-backend=false", "-input=false", f"-plugin-dir={plugin_dir}"
        )
        if init.returncode != 0:
            raise AssertionError(f"terraform init failed:\n{init.stderr}")

    @classmethod
    def terraform(cls, *args, stdin=None):
        return subprocess.run(
            [TERRAFORM, *args],
            cwd=cls.root,
            env=_terraform_env(),
            input=stdin,
            capture_output=True,
            text=True,
            timeout=TERRAFORM_TIMEOUT_SECONDS,
            check=False,
        )

    def evaluate(self, var_file, expression):
        """The value of expression in the module with var_file's inputs"""
        result = self.terraform(
            "console",
            f"-var-file={var_file}.tfvars.json",
            stdin=f"jsonencode({expression})\n",
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        # The console prints the JSON text as an HCL string literal
        text = json.loads(result.stdout.strip())
        return json.loads(text.replace("$${", "${").replace("%%{", "%{"))

    def test_only_a_binding_with_a_condition_gets_a_condition_block(self):
        for_each, content = _bucket_condition_block()
        for key, expected in (("plain", []), ("scoped", [CONDITION])):
            with self.subTest(binding=key):
                found = self.evaluate(
                    "mixed",
                    for_each.replace(
                        "each.value", f'local.grants.bindings.bucket["{key}"]'
                    ),
                )
                self.assertEqual(found, expected)
        # The block's attributes are the condition's own
        for attribute in ("title", "description", "expression"):
            self.assertRegex(
                content,
                rf"(?m)^\s*{attribute}\s*=\s*condition\.value\.{attribute}\s*$",
            )

    def test_fingerprint_without_a_condition_is_the_encoding_before_conditions(self):
        # Before conditions were accepted, the wait re-ran on
        # jsonencode(local.grants) with each bucket binding { bucket, role }
        found = self.evaluate(
            "unconditional",
            "{"
            "fingerprint = local.grants_fingerprint, "
            f"before = {BEFORE_CONDITIONS}, "
            "raw = jsonencode(local.grants)"
            "}",
        )
        self.assertEqual(found["fingerprint"], found["before"])
        self.assertEqual(
            json.loads(found["fingerprint"])["bindings"]["bucket"],
            {"plain": PLAIN},
        )
        # Without the fingerprint's rule the optional attribute would show
        # as null and change the encoding
        self.assertEqual(
            json.loads(found["raw"])["bindings"]["bucket"]["plain"]["condition"],
            None,
        )
        self.assertNotEqual(found["raw"], found["fingerprint"])

    def test_wait_trigger_without_a_condition_is_the_encoding_before_conditions(self):
        # The propagation wait's grants trigger, as main.tf writes it, keeps
        # its pre-condition value, so upgrading an existing deployment neither
        # replaces the wait nor makes service_account_email unknown
        found = self.evaluate(
            "unconditional",
            f"{{trigger = {_wait_grants_trigger()}, before = {BEFORE_CONDITIONS}}}",
        )
        self.assertEqual(found["trigger"], found["before"])

    def test_fingerprint_holds_the_condition_of_a_conditional_binding(self):
        fingerprint = self.evaluate("mixed", "local.grants_fingerprint")
        self.assertEqual(
            json.loads(fingerprint)["bindings"]["bucket"],
            {"plain": PLAIN, "scoped": SCOPED},
        )

    def test_node_files_condition_allows_only_the_node_files(self):
        # The condition reaches the binding's condition block unchanged
        for_each, _ = _bucket_condition_block()
        found = self.evaluate(
            "node_files",
            for_each.replace(
                "each.value", 'local.grants.bindings.bucket["ofe_bucket"]'
            ),
        )
        self.assertEqual(found, [NODE_CONDITION])
        # Its expression, each term rewritten as the same Terraform string
        # test, is true for the node's files only: clusters/ansible_setup_other
        # and other clusters' files are refused
        expression = " || ".join(
            f"name == {json.dumps(literal)}"
            if operator == "=="
            else f"startswith(name, {json.dumps(literal)})"
            for operator, literal in cel_terms(found[0]["expression"])
        )
        allowed, refused = node_files_cases(BUCKET, NODE_CLUSTER_ID)
        names = allowed + refused
        results = self.evaluate(
            "node_files",
            f"[for name in {_hcl_literal(names)} : {expression}]",
        )
        self.assertEqual(
            dict(zip(names, results)),
            {name: name in allowed for name in names},
        )
        # It is the expression the blueprint tests expect
        self.assertEqual(
            NODE_CONDITION["expression"],
            node_files_expression(BUCKET, NODE_CLUSTER_ID),
        )
