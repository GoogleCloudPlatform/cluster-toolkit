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

"""Which processes start the C2 Pub/Sub subscribers.

The server (uvicorn, or manage.py runserver) starts them in
GHPCFEConfig.ready(); any other manage.py command must not, or it would
pull C2 replies meant for the server. The test settings use
TestGHPCFEConfig, so the real class is imported and called here, with the
two start functions replaced.
"""

from unittest import mock

from django.test import SimpleTestCase

import ghpcfe
from ghpcfe.apps import GHPCFEConfig


class C2StartupTests(SimpleTestCase):
    """GHPCFEConfig.ready() for each kind of process"""

    def ready_with_argv(self, argv):
        """Run ready() as if the process had argv; return the mocks of
        c2.startup and c2.start_cloud_build_log_subscriber"""
        config = GHPCFEConfig("ghpcfe", ghpcfe)
        with mock.patch("sys.argv", argv), mock.patch(
            "ghpcfe.cluster_manager.c2.startup"
        ) as startup, mock.patch(
            "ghpcfe.cluster_manager.c2.start_cloud_build_log_subscriber"
        ) as build_logs:
            config.ready()
        return startup, build_logs

    def assert_started(self, argv):
        startup, build_logs = self.ready_with_argv(argv)
        startup.assert_called_once_with()
        build_logs.assert_called_once_with()

    def assert_not_started(self, argv):
        startup, build_logs = self.ready_with_argv(argv)
        startup.assert_not_called()
        build_logs.assert_not_called()

    def test_uvicorn_starts_both(self):
        self.assert_started(
            ["/opt/gcluster/django-env/bin/uvicorn", "website.asgi:application"]
        )

    def test_manage_py_runserver_starts_both(self):
        self.assert_started(["manage.py", "runserver", "0.0.0.0:8000"])

    def test_manage_py_migrate_starts_neither(self):
        self.assert_not_started(["manage.py", "migrate"])

    def test_manage_py_terraform_as_credential_starts_neither(self):
        self.assert_not_started(
            ["/opt/gcluster/website/manage.py", "ofe_terraform_as_credential"]
        )

    def test_bare_manage_py_starts_neither(self):
        self.assert_not_started(["manage.py"])
