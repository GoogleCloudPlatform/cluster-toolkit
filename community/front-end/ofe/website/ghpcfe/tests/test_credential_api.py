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

"""The credential API lists, shows and creates credentials only.

A cluster admin who is not a superuser cannot change, re-own or delete a
credential through the API, and only a superuser can create one, with the
checks of the credential page. Offline: keys are generated here and the
impersonation check is replaced at its module boundary.
"""

import json
from unittest import mock

from django.test import TestCase
from rest_framework.authtoken.models import Token

from ghpcfe.cluster_manager import credentials
from ghpcfe.models import Credential, Role, User
from ghpcfe.serializers import CredentialSerializer
from ghpcfe.tests.test_credentials import database_key_unreadable, make_key_json

PROJECT = "api-project"
ACCOUNT = f"provisioner@{PROJECT}.iam.gserviceaccount.com"
OTHER_PROJECT = "other-project"
OTHER_ACCOUNT = f"intruder@{OTHER_PROJECT}.iam.gserviceaccount.com"
LIST_URL = "/api/credentials/"
VALIDATE_IMPERSONATION = (
    "ghpcfe.cluster_manager.validate_credential.validate_impersonation"
)


def detail_url(credential):
    return f"{LIST_URL}{credential.pk}/"


class CredentialApiTests(TestCase):
    """The credential API as the command-line client and other users call it"""

    def setUp(self):
        for role_id, _ in Role.ROLE_CHOICES:
            Role.objects.get_or_create(id=role_id)
        self.superuser = User.objects.create_user(
            username="api-superuser", password="pass", is_superuser=True
        )
        self.superuser.roles.set([Role.CLUSTERADMIN])
        self.admin = User.objects.create_user(username="api-admin", password="pass")
        self.admin.roles.set([Role.CLUSTERADMIN])
        self.normal = User.objects.create_user(username="api-normal", password="pass")
        self.normal.roles.set([Role.NORMALUSER])
        self.impersonate = Credential.objects.create(
            name="keyless",
            owner=self.superuser,
            kind=credentials.KIND_IMPERSONATE,
            detail=ACCOUNT,
            project_id=PROJECT,
        )
        self.key = Credential.objects.create(
            name="with-key",
            owner=self.superuser,
            detail=make_key_json(PROJECT, ACCOUNT),
        )

    def auth(self, user):
        return {"HTTP_AUTHORIZATION": f"Token {Token.objects.get(user=user).key}"}

    def stored(self):
        """Every credential row as stored"""
        return list(
            Credential.objects.order_by("pk").values_list(
                "pk", "name", "owner_id", "kind", "project_id", "detail"
            )
        )

    def test_cluster_admin_cannot_change_a_credential(self):
        before = self.stored()
        changes = {
            "impersonated account": (self.impersonate, {"detail": OTHER_ACCOUNT}),
            "key JSON into an impersonate row": (
                self.impersonate,
                {"detail": make_key_json(OTHER_PROJECT, OTHER_ACCOUNT)},
            ),
            "another key": (
                self.key,
                {"detail": make_key_json(OTHER_PROJECT, OTHER_ACCOUNT)},
            ),
            "owner": (self.key, {"owner": self.admin.pk}),
            "name": (self.key, {"name": "renamed"}),
        }
        for case, (credential, body) in changes.items():
            for method in ("patch", "put"):
                with self.subTest(case=case, method=method):
                    response = getattr(self.client, method)(
                        detail_url(credential),
                        data=json.dumps(body),
                        content_type="application/json",
                        **self.auth(self.admin),
                    )
                    self.assertEqual(response.status_code, 405)
        self.assertEqual(self.stored(), before)

    def test_no_one_can_change_or_delete_a_credential_through_the_api(self):
        before = self.stored()
        for user in (self.admin, self.superuser):
            with self.subTest(user=user.username):
                response = self.client.delete(
                    detail_url(self.key), **self.auth(user)
                )
                self.assertEqual(response.status_code, 405)
                response = self.client.patch(
                    detail_url(self.impersonate),
                    data=json.dumps({"detail": OTHER_ACCOUNT}),
                    content_type="application/json",
                    **self.auth(user),
                )
                self.assertEqual(response.status_code, 405)
        self.assertEqual(self.stored(), before)

    def test_only_a_superuser_can_create_a_credential(self):
        before = self.stored()
        for user in (self.admin, self.normal):
            with self.subTest(user=user.username):
                response = self.client.post(
                    LIST_URL,
                    {"name": "new-key", "detail": make_key_json()},
                    **self.auth(user),
                )
                self.assertEqual(response.status_code, 403)
        self.assertEqual(self.stored(), before)

    def test_superuser_without_the_cluster_admin_role_can_create(self):
        # As on the credential pages: the superuser flag is enough. Not the
        # first user, who gets no default role, so this one has only the
        # ordinary user role
        superuser = User.objects.create_user(
            username="api-plain-superuser", password="pass", is_superuser=True
        )
        self.assertGreater(superuser.pk, 1)
        self.assertNotIn(
            Role.CLUSTERADMIN, superuser.roles.values_list("id", flat=True)
        )
        response = self.client.post(
            LIST_URL,
            {"name": "plain-superuser-key", "detail": make_key_json()},
            **self.auth(superuser),
        )
        self.assertEqual(response.status_code, 200, response.content)
        credential = Credential.objects.get(name="plain-superuser-key")
        self.assertEqual(credential.owner, superuser)
        self.assertTrue(credentials.is_sealed(credential.detail))
        # The form still checks what it is given
        response = self.client.post(
            LIST_URL,
            {"name": "not-a-key", "detail": "{}"},
            **self.auth(superuser),
        )
        self.assertEqual(response.status_code, 400)
        self.assertFalse(Credential.objects.filter(name="not-a-key").exists())
        # Listing and showing still need the cluster-admin role
        self.assertEqual(
            self.client.get(LIST_URL, **self.auth(superuser)).status_code, 403
        )
        self.assertEqual(
            self.client.get(
                detail_url(self.key), **self.auth(superuser)
            ).status_code,
            403,
        )

    def test_cli_upload_is_checked_like_the_page_and_owned_by_the_caller(self):
        # As cli/ghpcfe.py "credential add" posts it: form-encoded, no kind
        before = self.stored()
        response = self.client.post(
            LIST_URL,
            {"cloud_provider": "GCP", "name": "not-a-key", "detail": "{}"},
            **self.auth(self.superuser),
        )
        self.assertEqual(response.status_code, 400)
        self.assertIn(
            "Credential cannot be validated.",
            response.json()["non_field_errors"],
        )
        self.assertEqual(self.stored(), before)

        key = make_key_json(OTHER_PROJECT, OTHER_ACCOUNT)
        response = self.client.post(
            LIST_URL,
            {
                "cloud_provider": "GCP",
                "name": "cli-key",
                "detail": key,
                # Ignored: the caller owns what it creates
                "owner": self.admin.pk,
            },
            **self.auth(self.superuser),
        )
        self.assertEqual(response.status_code, 200, response.content)
        credential = Credential.objects.get(name="cli-key")
        self.assertEqual(
            response.json(),
            {"id": credential.pk, "name": "cli-key", "owner": self.superuser.pk},
        )
        self.assertEqual(credential.owner, self.superuser)
        self.assertEqual(credential.kind, credentials.KIND_KEY)
        self.assertEqual(credential.project_id, OTHER_PROJECT)
        self.assertTrue(credentials.is_sealed(credential.detail))
        self.assertEqual(credentials.identity_email(credential), OTHER_ACCOUNT)

    def test_impersonate_create_is_checked_through_impersonation(self):
        body = {
            "name": "api-keyless",
            "kind": credentials.KIND_IMPERSONATE,
            "service_account_email": OTHER_ACCOUNT,
            "project_id": OTHER_PROJECT,
        }
        with mock.patch(
            VALIDATE_IMPERSONATION, return_value="the account lacks permission"
        ) as check:
            response = self.client.post(LIST_URL, body, **self.auth(self.superuser))
        check.assert_called_once_with("GCP", OTHER_ACCOUNT, OTHER_PROJECT)
        self.assertEqual(response.status_code, 400)
        self.assertIn(
            "Credential cannot be validated: the account lacks permission",
            response.json()["non_field_errors"],
        )
        self.assertFalse(Credential.objects.filter(name="api-keyless").exists())

        with mock.patch(VALIDATE_IMPERSONATION, return_value=None):
            response = self.client.post(LIST_URL, body, **self.auth(self.superuser))
        self.assertEqual(response.status_code, 200, response.content)
        credential = Credential.objects.get(name="api-keyless")
        self.assertEqual(credential.kind, credentials.KIND_IMPERSONATE)
        self.assertEqual(credential.project_id, OTHER_PROJECT)
        self.assertEqual(credentials.impersonation_target(credential), OTHER_ACCOUNT)
        self.assertEqual(credential.owner, self.superuser)

    def test_unreadable_database_key_saves_nothing(self):
        before = self.stored()
        with database_key_unreadable():
            response = self.client.post(
                LIST_URL,
                {"name": "cli-key", "detail": make_key_json()},
                **self.auth(self.superuser),
            )
        self.assertEqual(response.status_code, 503)
        self.assertIn(
            "TKFE cannot read its database key",
            " ".join(response.json()["non_field_errors"]),
        )
        self.assertEqual(self.stored(), before)

    def test_a_body_that_is_not_an_object_is_refused(self):
        before = self.stored()
        for body in ("[1, 2]", '"text"', "5", "null"):
            with self.subTest(body=body):
                response = self.client.post(
                    LIST_URL,
                    data=body,
                    content_type="application/json",
                    **self.auth(self.superuser),
                )
                self.assertEqual(response.status_code, 400)
                self.assertIn(
                    "The request body must be a JSON object",
                    " ".join(response.json()["non_field_errors"]),
                )
        self.assertEqual(self.stored(), before)

    def test_list_and_show_never_return_the_key_and_owner_is_read_only(self):
        response = self.client.get(LIST_URL, **self.auth(self.admin))
        self.assertEqual(response.status_code, 200)
        self.assertEqual(
            sorted(item["name"] for item in response.json()),
            ["keyless", "with-key"],
        )
        response = self.client.get(detail_url(self.key), **self.auth(self.admin))
        self.assertEqual(response.status_code, 200)
        self.assertNotIn("detail", response.json())
        self.assertNotIn("PRIVATE KEY", response.content.decode())
        # Shown, never written
        self.assertTrue(CredentialSerializer().fields["owner"].read_only)
