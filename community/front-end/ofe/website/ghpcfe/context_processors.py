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

"""Template context shared by the pages built on base_generic.html.

manage.py ofe_bootstrap_credentials, run at each server start, records the
errors of a failed run in CREDENTIAL_SETUP_ERRORS_FILE under the backend's
baseDir and removes the file when a run succeeds. The pages built on
base_generic.html show those errors to superusers, so a server whose stored credentials could not be
prepared does not look healthy.
"""

import json
import logging

from ghpcfe.cluster_manager import utils

logger = logging.getLogger(__name__)

CREDENTIAL_SETUP_ERRORS_FILE = "credential-setup-errors.json"

# Shown instead of the errors when the file is there but cannot be read
UNREADABLE_ERRORS = (
    f"the record of the errors ({CREDENTIAL_SETUP_ERRORS_FILE}) could not be "
    "read; see the server's start-up log"
)


def credential_setup_errors_path():
    """Where manage.py ofe_bootstrap_credentials records its errors"""
    return utils.load_config()["baseDir"] / CREDENTIAL_SETUP_ERRORS_FILE


def _read_errors(path):
    """The time and errors recorded at path, or None when there is no file"""
    try:
        text = path.read_text(encoding="utf-8")
    except FileNotFoundError:
        return None
    except (OSError, UnicodeDecodeError) as err:
        logger.warning("Could not read %s: %s", path, err)
        return {"time": "", "errors": [UNREADABLE_ERRORS]}
    try:
        record = json.loads(text)
    except (ValueError, RecursionError) as err:
        logger.warning("Could not parse %s: %s", path, err)
        return {"time": "", "errors": [UNREADABLE_ERRORS]}
    if (
        not isinstance(record, dict)
        or not isinstance(record.get("time", ""), str)
        or not isinstance(record.get("errors"), list)
        or not record["errors"]
        or not all(isinstance(error, str) for error in record["errors"])
    ):
        logger.warning("%s does not hold a list of errors", path)
        return {"time": "", "errors": [UNREADABLE_ERRORS]}
    return {"time": record.get("time", ""), "errors": record["errors"]}


def credential_setup_errors(request):
    """credential_setup_errors, {"time": ..., "errors": [...]}, for a
    superuser while the last credential set-up failed"""
    user = getattr(request, "user", None)
    if user is None or not user.is_authenticated or not user.is_superuser:
        return {}
    try:
        path = credential_setup_errors_path()
    except Exception as err:  # pylint: disable=broad-except
        # Without the backend configuration there is no record to read;
        # pages that do not need it (the Django admin) must still load
        logger.warning("No credential set-up record to read: %s", err)
        return {}
    record = _read_errors(path)
    if record is None:
        return {}
    return {"credential_setup_errors": record}
