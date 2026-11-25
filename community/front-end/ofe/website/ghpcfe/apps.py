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
from pathlib import Path

from django.apps import AppConfig
from .cluster_manager import c2


def _starts_c2():
    """Whether this process should start the C2 Pub/Sub subscribers.

    A manage.py command (migrate, ofe_terraform_as_credential and the rest)
    must not: it would open streaming pulls on the C2 response and build-log
    subscriptions alongside the running server, and could take a cluster's
    reply from the server and lose it when the command exits. runserver is a
    server, so it starts them.
    """
    return not (
        Path(sys.argv[0]).name == "manage.py" and sys.argv[1:2] != ["runserver"]
    )


class GHPCFEConfig(AppConfig):
    name = "ghpcfe"
    default_auto_field = "django.db.models.AutoField"

    def ready(self):
        # Has side effect of registering various receiver callbacks
        import ghpcfe.signals # pylint:disable=unused-import,import-outside-toplevel

        if _starts_c2():
            c2.startup()
            c2.start_cloud_build_log_subscriber()
