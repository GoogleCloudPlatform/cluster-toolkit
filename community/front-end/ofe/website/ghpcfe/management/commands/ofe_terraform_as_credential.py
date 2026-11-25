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

"""Run Terraform as a resource's stored credential"""

import contextlib
import re
import subprocess
import sys

from django.core.management.base import BaseCommand
from django.core.management.base import CommandError
from google.api_core.exceptions import GoogleAPIError
from google.auth.exceptions import GoogleAuthError

from ghpcfe.cluster_manager import credentials
from ghpcfe.models import Cluster
from ghpcfe.models import Credential
from ghpcfe.models import Filesystem
from ghpcfe.models import Image
from ghpcfe.models import VirtualNetwork
from ghpcfe.models import Workbench

# The resource kinds, named as script/migrate_tf_state_to_gcs.sh names them
# from their run-time directories (clusters/cluster_<id>, fs/fs_<id>,
# vpcs/vpc_<id>, workbenches/workbench_<id>, images/image_<id>).
MODELS = {
    "cluster": Cluster,
    "filesystem": Filesystem,
    "vpc": VirtualNetwork,
    "workbench": Workbench,
    "image": Image,
}

# The only program this command runs
TERRAFORM = "terraform"

# The Terraform subcommands it runs: those the state migration needs, and
# others that read but never change a resource. apply, destroy, import and
# the state subcommands change resources or their state, and go through TKFE.
ALLOWED_SUBCOMMANDS = ("version", "init", "validate", "plan", "show", "output")

# Raised while the stored credential is read: the credential itself is
# unusable (CredentialError), or the database key that decrypts a stored key
# cannot be fetched from Secret Manager.
CREDENTIAL_READ_ERRORS = (
    credentials.CredentialError,
    GoogleAPIError,
    GoogleAuthError,
)


class Command(BaseCommand):
    """Run Terraform with the given arguments as a resource's credential.

    The environment is the one TKFE gives its own Terraform runs
    (credentials.tool_environment, which alone reads the credential): no
    ambient credential and no key file; Terraform impersonates an impersonate
    credential's service account, or gets a key credential's key in memory.
    Terraform's output goes straight to this command's, and the command exits
    with Terraform's exit code, so `plan -detailed-exitcode` can be read as
    usual.

    Runs only the subcommands in ALLOWED_SUBCOMMANDS: the first argument
    that does not start with "-" (global options such as -chdir= come
    before it) must be one of them.

    Refuses, on stderr with exit code 1 and without running Terraform, when
    the subcommand is not allowed, the resource does not exist, has no
    credential, its credential cannot be read (or the database key that
    decrypts it is malformed), or the private credential directory for the
    run cannot be made; also, with exit code 1, when the terraform program
    cannot be started.
    """

    help = (
        "Run terraform as the stored credential of a cluster, filesystem, "
        "vpc, workbench or image: "
        "ofe_terraform_as_credential <kind> <id> -- <terraform arguments>"
    )

    # Terraform's output is the only output; system-check warnings would be
    # mixed into it.
    requires_system_checks = []

    def add_arguments(self, parser):
        parser.add_argument("kind", choices=sorted(MODELS))
        parser.add_argument("id", help="the resource's id")
        parser.add_argument(
            "terraform_args",
            nargs="+",
            metavar="terraform-argument",
            help="arguments for terraform, after --",
        )

    def handle(self, *args, **options):
        kind = options["kind"]
        resource_id = options["id"]
        self._check_subcommand(options["terraform_args"])
        credential = self._credential(kind, resource_id)

        with contextlib.ExitStack() as stack:
            try:
                env = stack.enter_context(
                    credentials.tool_environment(credential)
                )
            except CREDENTIAL_READ_ERRORS as err:
                raise self._unusable(kind, resource_id, credential, err) from err
            except ValueError as err:
                # A stored key TKFE did not write (a malformed database key
                # arrives as a CredentialError, above). The error's text may
                # quote part of the key, so only its type is shown.
                raise self._unusable(
                    kind,
                    resource_id,
                    credential,
                    "its stored key was not written by TKFE and cannot be "
                    f"read ({type(err).__name__})",
                ) from err
            except OSError as err:
                raise self._unusable(
                    kind,
                    resource_id,
                    credential,
                    f"its private credential directory cannot be made: {err}",
                ) from err
            # Anything this process wrote must come before Terraform's output
            sys.stdout.flush()
            sys.stderr.flush()
            try:
                returncode = subprocess.run(
                    [TERRAFORM, *options["terraform_args"]],
                    env=env,
                    check=False,
                ).returncode
            except OSError as err:
                raise CommandError(
                    f"{kind} {resource_id}: {TERRAFORM} cannot be run: {err}. "
                    "Check that Terraform is installed and on PATH."
                ) from err
        if returncode:
            raise SystemExit(returncode)

    @staticmethod
    def _unusable(kind, resource_id, credential, reason):
        """The CommandError refusing to run Terraform as credential"""
        return CommandError(
            f"{kind} {resource_id}: its stored credential "
            f"{credential.name} (id {credential.id}) cannot be used: {reason}"
        )

    @staticmethod
    def _check_subcommand(terraform_args):
        """CommandError unless the Terraform subcommand is allowed"""
        subcommand = next(
            (arg for arg in terraform_args if not arg.startswith("-")), None
        )
        if subcommand in ALLOWED_SUBCOMMANDS:
            return
        if subcommand:
            refused = f"Terraform '{subcommand}' is not run by this command"
        else:
            refused = "No Terraform subcommand was given"
        raise CommandError(
            f"{refused}; this command runs only "
            f"{', '.join(ALLOWED_SUBCOMMANDS)}. apply and destroy need the "
            "key file that the Slurm compute-node cleanup (cleanup_compute) "
            "reads, which this command never writes, so they must go "
            "through TKFE, as must anything else that changes a resource or "
            "its state (state, import and the rest)."
        )

    @staticmethod
    def _credential(kind, resource_id):
        """The Credential of resource kind resource_id, or CommandError"""
        record = None
        if re.fullmatch(r"[0-9]+", resource_id):
            try:
                record = MODELS[kind].objects.filter(
                    pk=int(resource_id)
                ).first()
            except OverflowError:
                # Larger than the database's integers: no such record
                record = None
        if record is None:
            raise CommandError(f"There is no {kind} with id {resource_id}")
        if record.cloud_credential_id is None:
            raise CommandError(
                f"{kind} {resource_id} has no credential to run Terraform as"
            )
        try:
            return record.cloud_credential
        except Credential.DoesNotExist as err:
            raise CommandError(
                f"{kind} {resource_id}: its credential (id "
                f"{record.cloud_credential_id}) no longer exists"
            ) from err
