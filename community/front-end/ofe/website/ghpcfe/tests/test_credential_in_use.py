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

"""A credential that cloud resources use keeps its kind, project and
account.

Its resources were made, and are torn down, as that account in that
project, so the credential page refuses to change them; it can still be
renamed, and its key replaced by another key of the same account and
project. Offline: keys are generated here and the impersonation check is
replaced at its module boundary.
"""

import json
from unittest import mock

from django.test import TestCase
from django.urls import reverse

from ghpcfe.cluster_manager import credentials
from ghpcfe.forms import CredentialForm
from ghpcfe.models import Credential, User, VirtualNetwork
from ghpcfe.tests.test_credentials import make_key_json

PROJECT = "in-use-project"
ACCOUNT = f"provisioner@{PROJECT}.iam.gserviceaccount.com"
OTHER_PROJECT = "other-project"
OTHER_ACCOUNT = f"other@{PROJECT}.iam.gserviceaccount.com"
VALIDATE_IMPERSONATION = (
    "ghpcfe.cluster_manager.validate_credential.validate_impersonation"
)
IN_USE = "This credential is used by 1 cloud resource"


def owner():
    user = User.objects.filter(username="in-use-owner").first()
    return user or User.objects.create_user(
        username="in-use-owner", password="pass", is_superuser=True
    )


def use(credential, cloud_state="m"):
    """A network made with credential"""
    return VirtualNetwork.objects.create(
        name="net-a",
        cloud_id="net-a",
        cloud_credential=credential,
        cloud_region="europe-west4",
        cloud_state=cloud_state,
    )


def fresh(credential):
    return Credential.objects.get(pk=credential.pk)


def stored(credential):
    return Credential.objects.values_list(
        "name", "kind", "project_id", "detail"
    ).get(pk=credential.pk)


def errors(form, field):
    return " ".join(form.errors.get(field, []))


class InUseImpersonateTests(TestCase):
    """An impersonate credential in use"""

    def setUp(self):
        self.credential = Credential.objects.create(
            name="keyless",
            owner=owner(),
            kind=credentials.KIND_IMPERSONATE,
            detail=ACCOUNT,
            project_id=PROJECT,
        )
        patcher = mock.patch(VALIDATE_IMPERSONATION, return_value=None)
        self.addCleanup(patcher.stop)
        self.check = patcher.start()

    def form(self, **changes):
        data = {
            "name": self.credential.name,
            "kind": credentials.KIND_IMPERSONATE,
            "service_account_email": ACCOUNT,
            "project_id": PROJECT,
            **changes,
        }
        # As the edit page does, a form for the row as stored
        return CredentialForm(instance=fresh(self.credential), data=data)

    def test_account_and_project_are_locked_while_in_use(self):
        before = stored(self.credential)
        for cloud_state in ("m", "xm"):
            network = use(self.credential, cloud_state)
            with self.subTest(cloud_state=cloud_state):
                form = self.form(service_account_email=OTHER_ACCOUNT)
                self.assertFalse(form.is_valid())
                self.assertIn(IN_USE, errors(form, "service_account_email"))
                self.assertIn(
                    "the service account it impersonates cannot be changed",
                    errors(form, "service_account_email"),
                )
                form = self.form(project_id=OTHER_PROJECT)
                self.assertFalse(form.is_valid())
                self.assertIn(
                    "its project cannot be changed", errors(form, "project_id")
                )
                # Refused before Google is asked
                self.check.assert_not_called()
            network.delete()
        self.assertEqual(stored(self.credential), before)

        # With no resource left, the account can change
        form = self.form(service_account_email=OTHER_ACCOUNT)
        self.assertTrue(form.is_valid(), form.errors)
        form.save()
        self.assertEqual(
            credentials.impersonation_target(fresh(self.credential)), OTHER_ACCOUNT
        )

    def test_rename_is_allowed_while_in_use_but_kind_is_locked(self):
        use(self.credential)
        form = self.form(
            kind=credentials.KIND_KEY,
            detail=make_key_json(PROJECT, ACCOUNT),
        )
        self.assertFalse(form.is_valid())
        self.assertIn("its type cannot be changed", errors(form, "kind"))
        self.assertEqual(
            Credential.objects.get(pk=self.credential.pk).kind,
            credentials.KIND_IMPERSONATE,
        )

        form = self.form(name="renamed")
        self.assertTrue(form.is_valid(), form.errors)
        form.save()
        self.check.assert_called_once_with("GCP", ACCOUNT, PROJECT)
        self.assertEqual(
            stored(self.credential),
            ("renamed", credentials.KIND_IMPERSONATE, PROJECT, ACCOUNT),
        )


class InUseKeyTests(TestCase):
    """A key credential in use"""

    def setUp(self):
        self.credential = Credential.objects.create(
            name="with-key", owner=owner(), detail=make_key_json(PROJECT, ACCOUNT)
        )
        use(self.credential)

    def form(self, **changes):
        data = {"name": self.credential.name, "kind": credentials.KIND_KEY, **changes}
        return CredentialForm(instance=fresh(self.credential), data=data)

    def test_key_of_another_account_or_project_is_refused(self):
        before = stored(self.credential)
        for case, key in {
            "another account": make_key_json(PROJECT, OTHER_ACCOUNT),
            "another project": make_key_json(OTHER_PROJECT, ACCOUNT),
        }.items():
            with self.subTest(case):
                form = self.form(detail=key)
                self.assertFalse(form.is_valid())
                self.assertIn(IN_USE, errors(form, "detail"))
                self.assertIn(
                    f"a new key must be for the same service account "
                    f"({ACCOUNT}) and project ({PROJECT})",
                    errors(form, "detail"),
                )
        self.assertEqual(stored(self.credential), before)

        # Another key of the same account and project replaces it, with a
        # new name too
        new_key = make_key_json(PROJECT, ACCOUNT)
        form = self.form(name="renamed", detail=new_key)
        self.assertTrue(form.is_valid(), form.errors)
        form.save()
        credential = Credential.objects.get(pk=self.credential.pk)
        self.assertEqual(credential.name, "renamed")
        self.assertEqual(credentials.key_json(credential), new_key)
        self.assertTrue(credentials.is_sealed(credential.detail))

    def test_rename_with_a_blank_key_keeps_the_stored_key(self):
        before = stored(self.credential)
        with mock.patch(
            "ghpcfe.cluster_manager.validate_credential.validate_credential"
        ) as validate:
            # A project given with a blank key is ignored: the key's own
            # project stays
            form = self.form(name="renamed", detail="", project_id=OTHER_PROJECT)
            self.assertTrue(form.is_valid(), form.errors)
        validate.assert_not_called()
        form.save()
        self.assertEqual(stored(self.credential), ("renamed", *before[1:]))

    def test_a_row_with_no_project_id_takes_it_from_the_stored_key(self):
        # As a row saved before project ids were kept, until the start-up
        # fills it
        Credential.objects.filter(pk=self.credential.pk).update(project_id="")
        form = self.form(detail=make_key_json(OTHER_PROJECT, ACCOUNT))
        self.assertFalse(form.is_valid())
        self.assertIn(
            f"a new key must be for the same service account ({ACCOUNT}) "
            f"and project ({PROJECT})",
            errors(form, "detail"),
        )
        form = self.form(detail=make_key_json(PROJECT, OTHER_ACCOUNT))
        self.assertFalse(form.is_valid())
        self.assertIn(IN_USE, errors(form, "detail"))

        new_key = make_key_json(PROJECT, ACCOUNT)
        form = self.form(detail=new_key)
        self.assertTrue(form.is_valid(), form.errors)
        form.save()
        credential = fresh(self.credential)
        self.assertEqual(credentials.key_json(credential), new_key)
        self.assertEqual(credential.project_id, PROJECT)

    def test_with_no_project_anywhere_only_the_account_is_compared(self):
        info = json.loads(make_key_json(PROJECT, ACCOUNT))
        del info["project_id"]
        no_project = Credential.objects.create(
            name="no-project", owner=owner(), detail=json.dumps(info)
        )
        Credential.objects.filter(pk=no_project.pk).update(project_id="")
        VirtualNetwork.objects.create(
            name="net-b",
            cloud_id="net-b",
            cloud_credential=no_project,
            cloud_region="europe-west4",
            cloud_state="m",
        )
        data = {"name": "no-project", "kind": credentials.KIND_KEY}
        form = CredentialForm(
            instance=fresh(no_project),
            data={**data, "detail": make_key_json(PROJECT, OTHER_ACCOUNT)},
        )
        self.assertFalse(form.is_valid())
        self.assertIn(
            f"a new key must be for the same service account ({ACCOUNT}). ",
            errors(form, "detail"),
        )
        form = CredentialForm(
            instance=fresh(no_project),
            data={**data, "detail": make_key_json(PROJECT, ACCOUNT)},
        )
        self.assertTrue(form.is_valid(), form.errors)

    def test_a_stored_key_that_cannot_be_read_refuses_a_new_key(self):
        lost = credentials.Fernet(credentials.Fernet.generate_key())
        cases = {
            # Sealed with a database key the server no longer has
            "lost key": credentials.ENCRYPTED_PREFIX + lost.encrypt(
                make_key_json(PROJECT, ACCOUNT).encode("utf-8")
            ).decode("ascii"),
            # Not a token at all: decoding it raises a ValueError
            "not a token": credentials.ENCRYPTED_PREFIX + "gAAAAéé",
            # One non-ASCII character: the error's own text quotes it
            "not a token, one character": credentials.ENCRYPTED_PREFIX + "gAAAAé",
        }
        for case, unreadable in cases.items():
            with self.subTest(case):
                Credential.objects.filter(pk=self.credential.pk).update(
                    detail=unreadable
                )
                before = stored(self.credential)
                with self.assertLogs("ghpcfe.forms", "WARNING") as logs:
                    form = self.form(detail=make_key_json(PROJECT, ACCOUNT))
                    self.assertFalse(form.is_valid())
                self.assertIn(IN_USE, errors(form, "detail"))
                self.assertIn(
                    "the stored key cannot be read to check that. "
                    "See the server log.",
                    errors(form, "detail"),
                )
                output = "\n".join(logs.output)
                self.assertIn(
                    f"Credential {self.credential.pk}: stored key cannot be "
                    "read",
                    output,
                )
                if case.startswith("not a token"):
                    self.assertIn(
                        "stored text is not a valid token "
                        "(UnicodeEncodeError)",
                        output,
                    )
                self.assertNotIn(unreadable, output)
                self.assertNotIn("é", output)
                self.assertNotIn("\\xe9", output)
                self.assertEqual(stored(self.credential), before)

    def test_kind_is_locked(self):
        with mock.patch(VALIDATE_IMPERSONATION, return_value=None) as check:
            form = self.form(
                kind=credentials.KIND_IMPERSONATE,
                service_account_email=ACCOUNT,
                project_id=PROJECT,
            )
            self.assertFalse(form.is_valid())
        check.assert_not_called()
        self.assertIn("its type cannot be changed", errors(form, "kind"))
        self.assertEqual(
            Credential.objects.get(pk=self.credential.pk).kind, credentials.KIND_KEY
        )


class InUseUpdatePageTests(TestCase):
    """The credential edit page shows the refusal and saves nothing"""

    def test_changed_account_is_refused_on_the_page(self):
        self.client.force_login(owner())
        credential = Credential.objects.create(
            name="keyless",
            owner=owner(),
            kind=credentials.KIND_IMPERSONATE,
            detail=ACCOUNT,
            project_id=PROJECT,
        )
        use(credential)
        before = stored(credential)
        with mock.patch(VALIDATE_IMPERSONATION, return_value=None), mock.patch(
            "ghpcfe.cluster_manager.validate_credential.check_state_bucket_access",
            return_value=None,
        ):
            response = self.client.post(
                reverse("credential-update", args=[credential.pk]),
                {
                    "name": "keyless",
                    "kind": credentials.KIND_IMPERSONATE,
                    "service_account_email": OTHER_ACCOUNT,
                    "project_id": PROJECT,
                },
            )
        self.assertEqual(response.status_code, 200)
        self.assertContains(
            response, "the service account it impersonates cannot be changed"
        )
        self.assertEqual(stored(credential), before)

    def test_detail_page_of_a_damaged_stored_key_says_it_cannot_be_read(self):
        self.client.force_login(owner())
        credential = Credential.objects.create(
            name="damaged", owner=owner(), project_id=PROJECT
        )
        # Not a token at all: decoding it raises a ValueError, whose own
        # text quotes the character when there is only one
        for damaged in (
            credentials.ENCRYPTED_PREFIX + "gAAAAéé",
            credentials.ENCRYPTED_PREFIX + "gAAAAé",
        ):
            with self.subTest(damaged=damaged):
                Credential.objects.filter(pk=credential.pk).update(detail=damaged)
                with self.assertLogs("ghpcfe.views.credentials", "WARNING") as logs:
                    response = self.client.get(
                        reverse("credential-detail", args=[credential.pk])
                    )
                self.assertEqual(response.status_code, 200)
                self.assertContains(response, "(cannot be read)")
                self.assertIn(
                    f"Credential {credential.pk} cannot be read", logs.output[0]
                )
                output = "\n".join(logs.output)
                self.assertIn(
                    "stored text is not a valid token (UnicodeEncodeError)",
                    output,
                )
                self.assertNotIn(damaged, output)
                self.assertNotIn("é", output)
                self.assertNotIn("\\xe9", output)
