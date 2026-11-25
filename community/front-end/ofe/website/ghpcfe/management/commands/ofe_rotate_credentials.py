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

"""Re-seal every stored key with the newest database key"""

from django.core.management.base import BaseCommand
from django.core.management.base import CommandError

from ghpcfe.cluster_manager import credentials
from ghpcfe.models import Credential


class Command(BaseCommand):
    """Re-seal every stored key with the newest database key.

    Run it after adding a version to the database-key secret: it reads
    every enabled version, decrypts each stored key with whichever version
    sealed it and encrypts it again with the newest. It also does what the
    start-up does to stored keys (credentials.fix_existing_credentials):
    plaintext keys are encrypted and project ids filled in.

    A row is written only if it still holds the text and project id that
    were read, so an edit made meanwhile is never overwritten, and a row
    deleted meanwhile is skipped. Safe to run again: a second run writes
    nothing.

    The database keys are read once, before any row: if they cannot be read
    (the secret is missing, its latest version is disabled, or Secret
    Manager cannot be reached), the command fails with that error and
    touches no row. It also exits non-zero when a stored key cannot be read,
    or when a row changed while it ran and is not sealed with the newest key
    (run it again). Until it exits zero, no older version of the secret may
    be disabled or destroyed: the rows it seals would become unreadable.
    """

    help = (
        "Re-seal every stored key with the newest version of the "
        "database-key secret (safe to run again)"
    )

    def handle(self, *args, **kwargs):
        try:
            credentials.check_db_keys()
        except credentials.CredentialError as err:
            raise CommandError(
                f"The database keys cannot be read, so no stored key was "
                f"read or changed: {err}. Check the database-key secret (the "
                f"{credentials.DB_KEY_SECRET_ATTRIBUTE} instance metadata "
                "value names it): it must exist, its latest version must be "
                "enabled, and the server's service account must be able to "
                "access it. Then run this command again."
            ) from err
        counts = credentials.fix_existing_credentials(Credential)
        self.stdout.write(
            "Stored keys: {resealed} re-sealed with the newest database "
            "key, {unchanged} already sealed with it, {encrypted} encrypted "
            "from plaintext, {changed} changed while this ran and left for "
            "the next run, {unreadable} unreadable".format(
                **counts
            )
        )
        errors = []
        if counts["unreadable"]:
            ids = ", ".join(str(pk) for pk in counts["unreadable_ids"])
            errors.append(
                f"{counts['unreadable']} stored keys could not be read "
                f"(credential ids: {ids}). No enabled version of the "
                "database-key secret decrypts them, or they are damaged. If "
                "a version was disabled, enable it again and re-run this "
                "command; otherwise delete each of these credentials in TKFE "
                "and add it again"
            )
        if counts["changed"]:
            ids = ", ".join(str(pk) for pk in counts["changed_ids"])
            errors.append(
                f"{counts['changed']} stored keys changed while this ran and "
                f"were left as they are (credential ids: {ids}). "
                "Run this command again"
            )
        if errors:
            raise CommandError(
                "; ".join(errors)
                + ". Do not disable or destroy any version of the "
                "database-key secret until this command succeeds."
            )
        self.stdout.write(
            "Every stored key is sealed with the newest database key"
        )
