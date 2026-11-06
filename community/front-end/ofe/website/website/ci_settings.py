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

"""Settings for running the ghpcfe unit tests in CI. Never use in production.

    cd community/front-end/ofe/website
    python manage.py test ghpcfe --settings=website.ci_settings

They refuse any other manage.py command. On top of website/settings.py they:

  * replace the ghpcfe AppConfig with one whose ready() only registers the
    signal receivers, so no Pub/Sub subscription or Cloud Build log
    subscriber is started;
  * load a placeholder server configuration, with its base directory in a
    temporary directory, instead of reading ofe/configuration.yaml;
  * stand in for the Spack checkout that only a deployed server has, which
    the URL configuration imports;
  * refuse any connection to a host other than this one, and running
    terraform, packer, gcloud, gcluster or anything under /opt;
  * log to the console only, and create ghpcfe's tables directly in the
    test database, because OFE ships no migrations.
"""

# pylint: disable=wildcard-import,unused-wildcard-import,wrong-import-position
import ipaddress
import os
import socket
import sys
import tempfile
import types
from pathlib import Path

from django.apps import AppConfig
from django.core.exceptions import ImproperlyConfigured

if sys.argv[1:2] != ["test"]:
    raise ImproperlyConfigured(
        "website.ci_settings is only for `manage.py test` in CI.")

_REFUSED_PROGRAMS = {"terraform", "packer", "gcloud", "gsutil", "ghpc",
                     "gcluster"}


def _is_local(host):
    if isinstance(host, bytes):
        host = host.decode(errors="replace")
    if host in (None, "localhost", "testserver", ""):
        return True
    try:
        return ipaddress.ip_address(str(host).split("%")[0]).is_loopback
    except ValueError:
        return False


def _refuse(event, args):
    """Audit hook: tests must not reach the network or the cloud tools."""
    if event == "socket.connect":
        sock, address = args
        if sock.family in (socket.AF_INET, socket.AF_INET6) \
                and not _is_local(address[0]):
            raise PermissionError(f"CI tests must not connect to {address}")
    elif event == "socket.getaddrinfo":
        if not _is_local(args[0]):
            raise PermissionError(f"CI tests must not look up {args[0]}")
    elif event == "subprocess.Popen":
        program = os.fspath(args[0]) if args[0] is not None else ""
        if os.path.basename(program) in _REFUSED_PROGRAMS \
                or program.startswith("/opt/"):
            raise PermissionError(f"CI tests must not run {program}")


sys.addaudithook(_refuse)

# cluster_manager/spack.py imports these from the Spack checkout a deployed
# server clones; nothing in the tests calls into them.
for _name in ("spack", "spack.main", "spack.repo", "spack.version"):
    sys.modules.setdefault(_name, types.ModuleType(_name))

from ghpcfe.cluster_manager import utils as _utils

_utils.g_config.update({
    "baseDir": Path(tempfile.mkdtemp(prefix="ofe-ci-")),
    "server": {
        "host_type": "GCP",
        "gcp_project": "ofe-ci-placeholder",
        "gcs_bucket": "ofe-ci-placeholder",
        "c2_topic": "ofe-ci-placeholder",
        "deployment_name": "ofe-ci",
    },
    "loaded": True,
})


class CiGHPCFEConfig(AppConfig):
    """ghpcfe without the C2 start-up of ghpcfe.apps.GHPCFEConfig."""

    name = "ghpcfe"
    default_auto_field = "django.db.models.AutoField"

    def ready(self):
        import ghpcfe.signals  # pylint:disable=unused-import,import-outside-toplevel


from .settings import *  # noqa: E402,F401,F403

INSTALLED_APPS = [
    "website.ci_settings.CiGHPCFEConfig"
    if app == "ghpcfe.apps.GHPCFEConfig" else app
    for app in INSTALLED_APPS  # noqa: F405
]
ALLOWED_HOSTS = ["localhost", "127.0.0.1", "testserver"]
MEDIA_ROOT = str(_utils.g_config["baseDir"] / "media")
MIGRATION_MODULES = {"ghpcfe": None}
LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "handlers": {
        "console": {"level": "WARNING", "class": "logging.StreamHandler"},
    },
    "loggers": {"": {"handlers": ["console"], "level": "INFO"}},
}
