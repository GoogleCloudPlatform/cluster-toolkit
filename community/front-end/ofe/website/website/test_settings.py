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

"""Test-only Django settings.

The production settings write logs to <install>/run/django.log, a path
that only exists on a deployed OFE server. Unit tests run from a source
checkout, so this module reuses every production setting but logs to
the console and uses an in-memory database.

Run the suite with:

    python manage.py test ghpcfe.tests --settings=website.test_settings
"""

from .settings import *  # noqa: F401,F403  pylint: disable=wildcard-import,unused-wildcard-import

# ghpcfe.views.clusters reads the server configuration file at import
# time (ClusterLogFileView class body), and the URLconf imports every
# view module. Unit tests exercise application logic, not routing, so
# use an empty URLconf to keep the checkout free of deployment state.
ROOT_URLCONF = "website.test_urls"

DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.sqlite3",
        "NAME": ":memory:",
    }
}

# Migrations are generated during OFE installation and are intentionally not
# versioned in this directory. Ignore any developer-local generated files so
# the test database is synchronized directly from the current model schema.
MIGRATION_MODULES = {"ghpcfe": None}

LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "handlers": {
        "console": {"level": "WARNING", "class": "logging.StreamHandler"},
    },
    "loggers": {
        "": {
            "handlers": ["console"],
            "level": "WARNING",
        },
    },
}
