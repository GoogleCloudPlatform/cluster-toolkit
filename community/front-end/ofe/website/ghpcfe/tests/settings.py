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

"""Django settings for the ghpcfe test suite.

Usage, from the website directory:

    python manage.py test ghpcfe.tests --settings=ghpcfe.tests.settings

These settings build on the production settings but run offline: no metadata
server lookups, no C2 Pub/Sub startup, an in-memory database, console-only
logging, and a fake backend configuration instead of the deployment's
configuration.yaml.
"""

# pylint: disable=wildcard-import,unused-wildcard-import,undefined-variable

import atexit
import shutil
import tempfile
from pathlib import Path
from unittest import mock

from cryptography.fernet import Fernet

# Installs the spack stub and the Pub/Sub guards before anything else loads
from ghpcfe.tests import support  # pylint: disable=unused-import

# The production settings query the GCE metadata server at import time to
# find the allowed hosts and site name. Fail those lookups so they fall back
# to localhost straight away.
with mock.patch("requests.get", side_effect=OSError):
    from website.settings import *

# Load the backend configuration from the test fixture before any view module
# is imported, so later load_config() calls never read the deployment's
# configuration.yaml. The explicit access key keeps the
# C398_API_AUTHENTICATION_TOKEN environment variable out of the tests.
from ghpcfe.cluster_manager import utils as _utils  # pylint: disable=wrong-import-position,ungrouped-imports

_FIXTURES_DIR = Path(__file__).resolve().parent / "fixtures"
_utils.load_config(
    config_file=_FIXTURES_DIR / "test_settings_config.yaml",
    access_key="test-access-key",
)

# Anything the code under test writes below baseDir goes to a scratch directory
_TEST_BASE_DIR = tempfile.mkdtemp(prefix="ofe-tests-")
atexit.register(shutil.rmtree, _TEST_BASE_DIR, ignore_errors=True)
_utils.g_config["baseDir"] = Path(_TEST_BASE_DIR)

# Same app, without the C2 startup in GHPCFEConfig.ready()
INSTALLED_APPS = [
    "ghpcfe.tests.apps.TestGHPCFEConfig"
    if app == "ghpcfe.apps.GHPCFEConfig"
    else app
    for app in INSTALLED_APPS
]

DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.sqlite3",
        "NAME": ":memory:",
    }
}

# ghpcfe migrations are generated at deploy time and not kept in the source
# tree, so the test database creates the ghpcfe tables directly from the models.
MIGRATION_MODULES = {"ghpcfe": None}

# Stored keys are encrypted. A deployed server reads the key from Secret
# Manager; the tests use a new random key for each run instead.
OFE_CREDENTIAL_ENCRYPTION_KEY = Fernet.generate_key()

# The production log file lives outside the source tree
LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "handlers": {
        "console": {"level": "WARNING", "class": "logging.StreamHandler"},
    },
    "loggers": {
        "": {
            "handlers": ["console"],
            "level": "INFO",
        },
    },
}
