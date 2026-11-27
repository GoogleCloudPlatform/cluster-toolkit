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

"""The control-bucket grant of a cluster created before service-account
profiles, on a bucket whose policy holds conditional bindings.

A profiles cluster's controller writes the control bucket through a binding
limited by an IAM condition to its own prefix. Once the bucket holds one,
Google refuses to return the policy at a version below 3, and refuses to
write a policy holding a condition at a version below 3. The grant must
still reach the legacy cluster's account, and the conditional bindings
must be written back unchanged.

Offline: the Cloud Storage client is a fake at the google.cloud.storage.Client
boundary that behaves as Google does for these versions; the policy is the
library's own Policy class.
"""

import copy
from unittest import mock

from django.test import SimpleTestCase
from google.api_core.exceptions import BadRequest
from google.api_core.iam import Policy

from ghpcfe.cluster_manager import cloud_info

BUCKET = "ofe-test-control-bucket"
LEGACY_ACCOUNT = "serviceAccount:alpha-0a1b2c3d-sa@example-project.iam.gserviceaccount.com"
OTHER_LEGACY_ACCOUNT = "serviceAccount:beta-1a2b3c4d-sa@example-project.iam.gserviceaccount.com"
CONTROLLER = "serviceAccount:gamma-2b3c4d5e-ctl@example-project.iam.gserviceaccount.com"
ROLE = "roles/storage.objectAdmin"

PREFIX_CONDITION = {
    "title": "ofe-cluster-7-prefix",
    "description": "Objects under clusters/7/ in the TKFE control bucket only.",
    "expression": (
        f'resource.name.startsWith("projects/_/buckets/{BUCKET}/objects/clusters/7/")'
    ),
}
# A conditional binding of the same role the legacy grant uses: it must not
# be widened to the whole bucket by adding the legacy account to it
SAME_ROLE_CONDITION = {
    "title": "ofe-cluster-8-prefix",
    "expression": (
        f'resource.name.startsWith("projects/_/buckets/{BUCKET}/objects/clusters/8/")'
    ),
}


class FakeBucket:
    """A bucket whose IAM policy acts as Google's does for policy versions"""

    def __init__(self, resource):
        self.resource = resource
        self.requested_versions = []
        self.written = []

    def _has_condition(self, resource):
        return any(b.get("condition") for b in resource.get("bindings", []))

    def get_iam_policy(self, requested_policy_version=None):
        self.requested_versions.append(requested_policy_version)
        if self._has_condition(self.resource) and (requested_policy_version or 1) < 3:
            raise BadRequest(
                "Policy version 1 cannot be used for a policy with conditions"
            )
        return Policy.from_api_repr(copy.deepcopy(self.resource))

    def set_iam_policy(self, policy):
        resource = policy.to_api_repr()
        if self._has_condition(resource) and (resource.get("version") or 1) < 3:
            raise BadRequest("A policy with conditions needs version 3")
        self.written.append(resource)
        self.resource = resource
        return policy


class FakeClient:
    def __init__(self, bucket):
        self.bucket = bucket
        self.opened = []

    def get_bucket(self, name):
        self.opened.append(name)
        return self.bucket


def binding(role, members, condition=None):
    found = {"role": role, "members": sorted(members)}
    if condition:
        found["condition"] = condition
    return found


class LegacyBucketGrantTests(SimpleTestCase):
    """gcs_apply_bucket_acl on a policy with conditional bindings"""

    def grant(self, resource):
        bucket = FakeBucket(resource)
        client = FakeClient(bucket)
        with mock.patch.object(cloud_info.gcs, "Client", return_value=client), \
                self.assertLogs("ghpcfe.cluster_manager.cloud_info", "INFO") as logs:
            cloud_info.gcs_apply_bucket_acl(BUCKET, LEGACY_ACCOUNT, permission=ROLE)
        self.assertEqual(client.opened, [BUCKET])
        self.assertNotIn(
            "Failed to apply GCS Policy", "\n".join(logs.output)
        )
        self.assertEqual(bucket.requested_versions, [3])
        self.assertEqual(len(bucket.written), 1)
        written = bucket.written[0]
        self.assertEqual(written["version"], 3)
        return written["bindings"]

    def test_extends_the_unconditional_binding_and_keeps_conditional_ones(self):
        bindings = self.grant({
            "version": 3,
            "etag": "BwX1",
            "bindings": [
                binding(ROLE, [OTHER_LEGACY_ACCOUNT]),
                binding(ROLE, [CONTROLLER], SAME_ROLE_CONDITION),
                binding("roles/storage.objectUser", [CONTROLLER], PREFIX_CONDITION),
            ],
        })
        self.assertCountEqual(bindings, [
            binding(ROLE, [OTHER_LEGACY_ACCOUNT, LEGACY_ACCOUNT]),
            binding(ROLE, [CONTROLLER], SAME_ROLE_CONDITION),
            binding("roles/storage.objectUser", [CONTROLLER], PREFIX_CONDITION),
        ])

    def test_skips_a_conditional_binding_listed_before_the_unconditional_one(self):
        # The first binding of the role is the conditional one: it is left
        # as it was, and the unconditional binding after it is extended
        bindings = self.grant({
            "version": 3,
            "etag": "BwX4",
            "bindings": [
                binding(ROLE, [CONTROLLER], SAME_ROLE_CONDITION),
                binding(ROLE, [OTHER_LEGACY_ACCOUNT]),
                binding("roles/storage.objectUser", [CONTROLLER], PREFIX_CONDITION),
            ],
        })
        self.assertCountEqual(bindings, [
            binding(ROLE, [CONTROLLER], SAME_ROLE_CONDITION),
            binding(ROLE, [OTHER_LEGACY_ACCOUNT, LEGACY_ACCOUNT]),
            binding("roles/storage.objectUser", [CONTROLLER], PREFIX_CONDITION),
        ])

    def test_adds_an_unconditional_binding_when_only_a_conditional_one_has_the_role(self):
        bindings = self.grant({
            "version": 3,
            "etag": "BwX2",
            "bindings": [
                binding(ROLE, [CONTROLLER], SAME_ROLE_CONDITION),
                binding("roles/storage.objectUser", [CONTROLLER], PREFIX_CONDITION),
            ],
        })
        self.assertCountEqual(bindings, [
            binding(ROLE, [CONTROLLER], SAME_ROLE_CONDITION),
            binding(ROLE, [LEGACY_ACCOUNT]),
            binding("roles/storage.objectUser", [CONTROLLER], PREFIX_CONDITION),
        ])

    def test_policy_without_conditions(self):
        # A server whose clusters all predate profiles: the grant works as
        # before, and the policy is written as version 3
        bindings = self.grant({
            "version": 1,
            "etag": "BwX3",
            "bindings": [binding("roles/storage.admin", [OTHER_LEGACY_ACCOUNT])],
        })
        self.assertCountEqual(bindings, [
            binding("roles/storage.admin", [OTHER_LEGACY_ACCOUNT]),
            binding(ROLE, [LEGACY_ACCOUNT]),
        ])
