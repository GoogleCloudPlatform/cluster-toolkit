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

"""Only a superuser sets a cluster's existing service accounts.

A cluster admin who is not a superuser sees the existing-service-account
fields read-only; a form that changes them is refused, and one that leaves
them as they are is accepted. A form built without a user is treated the
same way. Offline: the region lookup is replaced at its
module boundary.
"""

from unittest import mock

from django.test import TestCase
from django.urls import reverse

from ghpcfe.forms import ClusterForm
from ghpcfe.models import Role, User
from ghpcfe.tests.test_blueprint_identities import (
    OWN_ACCOUNTS,
    PROFILE_CASES,
    clear_clusters,
    make_world,
)

FIELDS = ClusterForm.OWN_SERVICE_ACCOUNT_FIELDS
SUPERUSER_ONLY = "Only a superuser can set or change the existing service accounts."


def form_data(cluster, **changes):
    """What the cluster page posts for cluster as it is, with changes"""
    form = ClusterForm(instance=cluster)
    data = {
        name: form[name].value()
        for name in form.fields
        if form[name].value() is not None
    }
    # The page requires at least one authorised user
    data["authorised_users"] = [cluster.owner.pk]
    data.update(changes)
    return data


def own_accounts(cluster):
    return {name: getattr(cluster, name) for name in FIELDS}


class ByoAccountTests(TestCase):
    """The cluster form and page for superusers and other cluster admins"""

    def setUp(self):
        for role_id, _ in Role.ROLE_CHOICES:
            Role.objects.get_or_create(id=role_id)
        self.world = make_world()
        self.superuser = User.objects.create_user(
            username="byo-superuser", password="pass", is_superuser=True
        )
        self.admin = User.objects.create_user(username="byo-admin", password="pass")
        self.admin.roles.set([Role.CLUSTERADMIN])

    def test_superuser_sets_and_changes_the_accounts(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        form = ClusterForm(
            data=form_data(cluster, **OWN_ACCOUNTS),
            instance=cluster,
            user=self.superuser,
        )
        self.assertTrue(form.is_valid(), form.errors)
        for name in FIELDS:
            self.assertNotIn("readonly", form.fields[name].widget.attrs)

    def test_other_cluster_admin_cannot_set_or_change_them(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        form = ClusterForm(
            data=form_data(cluster, **OWN_ACCOUNTS), instance=cluster, user=self.admin
        )
        self.assertFalse(form.is_valid())
        for name in FIELDS:
            self.assertEqual(form.errors[name], [SUPERUSER_ONLY])
            self.assertTrue(form.fields[name].widget.attrs["readonly"])

        clear_clusters()
        cluster = PROFILE_CASES["own-minimal"](self.world)
        changed = "changed@example-project.iam.gserviceaccount.com"
        form = ClusterForm(
            data=form_data(cluster, compute_service_account_email=changed),
            instance=cluster,
            user=self.admin,
        )
        self.assertFalse(form.is_valid())
        self.assertEqual(form.errors["compute_service_account_email"], [SUPERUSER_ONLY])
        self.assertNotIn("controller_service_account_email", form.errors)

    def test_other_cluster_admin_cannot_clear_them(self):
        cluster = PROFILE_CASES["own-minimal"](self.world)
        cleared = {name: "" for name in FIELDS}
        form = ClusterForm(
            data=form_data(cluster, **cleared), instance=cluster, user=self.admin
        )
        self.assertFalse(form.is_valid())
        for name in FIELDS:
            self.assertEqual(form.errors[name], [SUPERUSER_ONLY])

        # A superuser may clear them, so that OFE creates the accounts
        form = ClusterForm(
            data=form_data(cluster, **cleared), instance=cluster, user=self.superuser
        )
        self.assertTrue(form.is_valid(), form.errors)

    def test_without_a_user_they_cannot_be_set_or_changed(self):
        """A form built without the editing user fails closed"""
        cluster = PROFILE_CASES["minimal"](self.world)
        form = ClusterForm(data=form_data(cluster, **OWN_ACCOUNTS), instance=cluster)
        self.assertFalse(form.is_valid())
        for name in FIELDS:
            self.assertEqual(form.errors[name], [SUPERUSER_ONLY])
            self.assertTrue(form.fields[name].widget.attrs["readonly"])

        clear_clusters()
        cluster = PROFILE_CASES["own-minimal"](self.world)
        form = ClusterForm(data=form_data(cluster), instance=cluster)
        self.assertTrue(form.is_valid(), form.errors)
        self.assertEqual(
            {name: form.cleaned_data[name] for name in FIELDS},
            own_accounts(cluster),
        )

    def test_other_cluster_admin_may_leave_them_as_they_are(self):
        for case in ("minimal", "own-minimal"):
            with self.subTest(case):
                clear_clusters()
                cluster = PROFILE_CASES[case](self.world)
                form = ClusterForm(
                    data=form_data(cluster), instance=cluster, user=self.admin
                )
                self.assertTrue(form.is_valid(), form.errors)
                self.assertEqual(
                    {name: form.cleaned_data[name] for name in FIELDS},
                    own_accounts(cluster),
                )

    def update_page(self, user, cluster, data=None):
        self.client.force_login(user)
        url = reverse("cluster-update", kwargs={"pk": cluster.pk})
        with mock.patch(
            "ghpcfe.cluster_manager.cloud_info.get_region_zone_info",
            return_value={},
        ):
            if data is None:
                return self.client.get(url)
            return self.client.post(url, data)

    def test_page_shows_them_read_only_to_other_cluster_admins(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        response = self.update_page(self.admin, cluster)
        self.assertEqual(response.status_code, 200)
        form = response.context["form"]
        for name in FIELDS:
            self.assertIn("readonly", str(form[name]))
        self.assertContains(response, SUPERUSER_ONLY)

        response = self.update_page(self.superuser, cluster)
        self.assertEqual(response.status_code, 200)
        form = response.context["form"]
        for name in FIELDS:
            self.assertNotIn("readonly", str(form[name]))
        self.assertNotContains(response, SUPERUSER_ONLY)

    def test_page_refuses_a_change_by_another_cluster_admin(self):
        cluster = PROFILE_CASES["minimal"](self.world)
        before = own_accounts(cluster)
        response = self.update_page(
            self.admin, cluster, form_data(cluster, **OWN_ACCOUNTS)
        )
        self.assertEqual(response.status_code, 200)
        form = response.context["form"]
        for name in FIELDS:
            self.assertEqual(form.errors[name], [SUPERUSER_ONLY])
        cluster.refresh_from_db()
        self.assertEqual(own_accounts(cluster), before)
