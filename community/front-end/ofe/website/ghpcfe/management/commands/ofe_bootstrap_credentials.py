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

# Import errors are expected from pylint here due to Django behaviour
# pylint: disable=import-error

"""Prepare stored credentials when the web server starts"""

import contextlib
import datetime
import json
import logging
import os
import tempfile

from django.core.management.base import BaseCommand
from django.core.management.base import CommandError

from ghpcfe.cluster_manager import credentials
from ghpcfe.context_processors import credential_setup_errors_path
from ghpcfe.models import Credential
from ghpcfe.models import User

logger = logging.getLogger(__name__)


def record_errors(errors):
    """Record the errors of this run for the pages to show to superusers
    (see ghpcfe.context_processors), or remove the record when there are
    none. The record is replaced atomically, so a page never reads half of
    it. The error texts hold no key material.
    """
    path = credential_setup_errors_path()
    if not errors:
        path.unlink(missing_ok=True)
        return
    record = {
        "time": datetime.datetime.now(datetime.timezone.utc).isoformat(
            timespec="seconds"
        ),
        "errors": list(errors),
    }
    fd, tmp = tempfile.mkstemp(
        dir=path.parent, prefix=f".{path.name}.", suffix=".tmp"
    )
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(record, f)
        os.replace(tmp, path)
    except BaseException:
        with contextlib.suppress(FileNotFoundError):
            os.unlink(tmp)
        raise


class Command(BaseCommand):
    """Prepare stored credentials when the web server starts.

    Reads the instance metadata keys ofe-db-key-secret and
    ofe-provisioning-service-account, then:

    1. deletes the plaintext key copies older code left in the working
       directories of clusters, filesystems, networks, workbenches and
       images (credentials.LEGACY_KEY_FILE files and
       credentials.LEGACY_GCLOUD_CONFIG directories);
    2. gives the database-key secret a first version if it has none;
    3. encrypts stored keys saved before encryption and fills in their
       project id. A stored key that cannot be read is skipped, the others
       are still done, and the command then fails with how many were
       skipped, their credential ids and how to fix or delete them;
    4. registers an impersonate credential for the provisioning account,
       owned by the first admin user, unless one already impersonates it.

    Every step is safe to run again. The command exits non-zero, after
    running every step it can, when a step fails. The errors are also
    recorded in credential-setup-errors.json under the backend's baseDir,
    which the pages built on base_generic.html show to superusers until a
    run succeeds and removes it. An unexpected error is recorded too, by
    its type only, and the command then fails.
    """

    help = (
        "Set up the database key, encrypt stored keys and register the "
        "provisioning account's credential (safe to run on every start)"
    )

    def handle(self, *args, **kwargs):
        try:
            errors = credentials.bootstrap(
                Credential, User, report=self.stdout.write
            )
        except Exception as err:  # pylint: disable=broad-except
            # bootstrap() reports each step's failure itself, so this is an
            # unexpected one. It is still recorded, so the pages show that
            # the set-up failed; only its type is shown there, and the full
            # error goes to the log, in case its text quotes stored data.
            logger.exception("Preparing stored credentials failed")
            errors = [
                "Preparing stored credentials stopped with an unexpected "
                f"error ({type(err).__name__}); see the server log "
                "(/opt/gcluster/run/django.log) or the start-up output"
            ]
        try:
            record_errors(errors)
        except OSError as err:
            # The run's own result still counts; only the pages miss it.
            logger.error("Could not record the credential set-up result: %s", err)
            self.stderr.write(
                f"Could not record the credential set-up result: {err}"
            )
        if errors:
            raise CommandError("; ".join(errors))
        self.stdout.write("Credentials are ready")
