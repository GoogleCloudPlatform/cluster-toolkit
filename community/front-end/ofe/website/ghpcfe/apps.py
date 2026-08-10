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

"""Top level Django app definitions"""

import sys

from django.apps import AppConfig
from .cluster_manager import c2

class GHPCFEConfig(AppConfig):
    name = "ghpcfe"
    default_auto_field = "django.db.models.AutoField"

    def ready(self):
        # Has side effect of registering various receiver callbacks
        import ghpcfe.signals # pylint:disable=unused-import,import-outside-toplevel

        # C2 startup reads the server configuration file and connects to
        # Pub/Sub, neither of which exists when running unit tests from a
        # source checkout.
        #
        # settings.TESTING is the reliable signal: it survives pytest,
        # coverage and any other runner that does not put "test" in argv.
        # The argv check is kept as a fallback for `manage.py test` run
        # without --settings=website.test_settings, which would otherwise
        # fail here on a missing configuration.yaml instead of skipping.
        from django.conf import settings  # pylint:disable=import-outside-toplevel
        if getattr(settings, "TESTING", False) or sys.argv[1:2] == ["test"]:
            return

        c2.startup()
        c2.start_cloud_build_log_subscriber()
