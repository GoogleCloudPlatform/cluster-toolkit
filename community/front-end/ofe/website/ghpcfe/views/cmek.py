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
"""The deployment-wide encryption defaults, and a read-only key overview.

Superusers only, and deliberately not in the sidebar: encryption is part of
creating a resource, not a section of its own. Both are reached from the
Advanced link on a resource form's encryption panel.
"""
import logging
import re

from django.contrib import messages
from django.http import Http404, HttpResponseRedirect
from django.urls import reverse
from django.views.generic import TemplateView
from django.views.generic.edit import FormView

from ..cluster_manager import cloud_info, cmek, utils
from ..forms import CmekSettingsForm
from ..permissions import SuperUserRequiredMixin

logger = logging.getLogger(__name__)


class CmekSettingsView(SuperUserRequiredMixin, FormView):
    """Edit the `cmek:` section of the server configuration.

    Admin only, like the key pages: the settings decide where every new
    resource's key lives, and the file they are written to is the server's
    own configuration.
    """

    template_name = "cmek/settings.html"
    form_class = CmekSettingsForm

    def _current(self):
        """The settings as they stand, as the form's initial values.

        A configuration that fails to parse must still render the page: a
        setting the operator cannot reach is a setting they cannot fix. The
        stored values are shown as they are, so the one at fault can be seen
        and corrected; only an enforcement mode the form cannot offer falls
        back to the default.
        """
        config = utils.load_config()
        try:
            current = cmek.CmekSettings.from_config(config)
        except cmek.CmekConfigError as err:
            messages.warning(
                self.request,
                f"The current encryption settings could not be read ({err}). "
                "Saving this form will replace them.")
            defaults = cmek.CmekSettings()
            section = (config or {}).get("cmek") or {}
            if not isinstance(section, dict):
                # A section that is not a mapping at all (a bare string, a
                # list) has no stored values to show; the warning above says
                # why it could not be read.
                section = {}
            enforcement = section.get("enforcement", defaults.enforcement)
            if enforcement not in cmek.ENFORCEMENT_MODES:
                enforcement = defaults.enforcement
            return {
                "enforcement": enforcement,
                "key_project_id": str(section.get(
                    "key_project_id") or defaults.key_project_id),
                "key_ring_name": str(section.get(
                    "key_ring_name") or defaults.key_ring_name),
            }
        return {
            "enforcement": current.enforcement,
            "key_project_id": current.key_project_id,
            "key_ring_name": current.key_ring_name,
        }

    def get_initial(self):
        return self._current()

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = ""
        context["config_path"] = str(utils.config_file_path())
        return context

    def form_valid(self, form):
        settings = form.settings
        try:
            utils.update_config_file(
                lambda section: section.__setitem__(
                    "cmek", settings.to_config_section()))
        except OSError as err:
            logger.exception("Could not write the encryption settings")
            form.add_error(
                None, f"Could not write {utils.config_file_path()}: {err}")
            return self.form_invalid(form)
        messages.success(
            self.request,
            "Encryption defaults saved. They apply to resources created from "
            "now on; existing resources keep the key they were created with.")
        return super().form_valid(form)

    def get_success_url(self):
        return reverse("cmek-settings")


def _cmek_kinds():
    """The resource kinds that carry a key, by the slug their URLs use.

    Each maps to its model, the label shown to the admin, and the route to the
    resource's own page.
    """
    from ..models import (  # pylint: disable=import-outside-toplevel
        Cluster, GCPFilestoreFilesystem, Image,
    )

    return {
        "cluster": (Cluster, "Cluster", "cluster-detail"),
        "filestore": (GCPFilestoreFilesystem, "Filestore", "fs-detail"),
        "image": (Image, "Image", "image-view"),
    }


def _live_rows(kind):
    """The live, encrypted resources of one kind.

    Torn-down resources are excluded: their key is no longer this deployment's
    concern, and listing one would invite destroying a key nothing uses.
    """
    model = _cmek_kinds()[kind][0]
    rows = model.objects.filter(cmek_enabled=True).exclude(cloud_state="xm")
    if kind == "cluster":
        rows = rows.exclude(status="d")
    return rows


def _live_cmek_rows():
    """Every live, encrypted resource, with its kind and detail route."""
    for kind, (_model, label, route) in _cmek_kinds().items():
        for row in _live_rows(kind):
            yield kind, row, label, route


def _describe_duration(duration):
    """A Cloud KMS duration such as "2592000s" in words, or None."""
    match = re.fullmatch(r"(\d+)s", duration or "")
    if not match:
        return None
    seconds = int(match[1])
    if seconds % 86400 == 0:
        days = seconds // 86400
        return f"{days} day{'' if days == 1 else 's'}"
    return f"{seconds / 3600:g} hours"


class CmekKeysView(SuperUserRequiredMixin, TemplateView):
    """Every key OFE has created, and the resource each one belongs to.

    Admin only, because destroying a key is offered from here. Everything shown
    is read from what the deploys recorded, with links out to the Cloud Console;
    OFE creates and edits nothing on this page. Destruction is the one action it
    does offer, since revoking a key is the reason a customer wants CMEK in the
    first place and it cannot be done from a resource's own page.
    """

    template_name = "cmek/keys.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = ""

        keys = []
        for kind, row, label, route in _live_cmek_rows():
            path = cmek.resource_key_path(row)
            if not path:
                # no key name and ring yet: they are typed on the form or
                # written back by the deploy
                continue
            try:
                parts = cmek.parse_key_path(path)
            except cmek.CmekConfigError:
                continue
            keys.append({
                "key": parts["key"],
                "key_ring_name": parts["ring"],
                "location": parts["location"],
                "project": parts["project"],
                "path": path,
                "key_url": cmek.console_key_url(
                    parts["project"], parts["location"], parts["ring"],
                    parts["key"]),
                "key_ring_url": cmek.console_keyring_url(
                    parts["project"], parts["location"], parts["ring"]),
                "kind": kind,
                "resource_kind": label,
                "resource_name": row.name,
                "resource_route": route,
                "resource_pk": row.pk,
                "teardown": row.cmek_deletion_policy
                            or cmek.DELETION_POLICY_ABANDON,
            })
        context["keys"] = sorted(
            keys, key=lambda k: (k["key_ring_name"], k["key"]))
        context["console_root"] = cmek.CONSOLE_KMS_ROOT
        return context


class CmekKeyDestroyView(SuperUserRequiredMixin, TemplateView):
    """Schedule a key's versions for destruction, deliberately breaking its
    resource.

    This is revocation, not tidy-up. A CryptoKey cannot be deleted -- Cloud KMS
    keeps the name for ever, which is why a name can never be reused --
    so "deleting a key" means scheduling every live version for destruction.
    Cloud KMS holds the versions in DESTROY_SCHEDULED for the key's
    destroy_scheduled_duration before the material goes -- 30 days for a key
    OFE created, since it leaves kms-key's default alone -- which is the only
    window in which this can be undone, and only in the Cloud Console.

    The resource encrypted with the key stops working. That is the intended
    outcome and matches what kms-key already does on a DELETE teardown; it is
    the reason a customer wants CMEK at all. Because it is irreversible after
    the scheduled window, GET only ever shows what will break and POST is the
    only thing that acts.
    """

    template_name = "cmek/key_destroy.html"

    def _target(self):
        """The live resource whose key this is, or 404.

        Looked up by resource rather than by key name: the short name is unique
        only within one key ring, and two projects or key rings can each hold a
        key of the same name. Only the rows the key listing shows can be
        reached, so a key with no live resource is not offered for destruction
        at all.
        """
        kind, pk = self.kwargs["kind"], self.kwargs["pk"]
        if kind not in _cmek_kinds():
            raise Http404(f"No encrypted resource kind {kind!r}")
        _model, label, route = _cmek_kinds()[kind]
        row = _live_rows(kind).filter(pk=pk).first()
        path = cmek.resource_key_path(row) if row else ""
        if not path:
            raise Http404(f"No live encrypted {kind} {pk}")
        try:
            parts = cmek.parse_key_path(path)
        except cmek.CmekConfigError as err:
            raise Http404(str(err)) from err
        return row, label, route, path, parts

    def get_context_data(self, **kwargs):
        row, label, route, path, parts = self._target()
        context = super().get_context_data(**kwargs)
        context["navtab"] = ""
        context["key_name"] = parts["key"]
        context["key_path"] = path
        context["key_ring_name"] = parts["ring"]
        context["resource_kind"] = label
        context["resource_name"] = row.name
        context["resource_route"] = route
        context["resource_pk"] = row.pk
        # Only a cluster's teardown reads objects encrypted with the key (its
        # Slurm bucket), so only its page explains how it is destroyed after.
        context["is_cluster"] = self.kwargs["kind"] == "cluster"
        context["key_url"] = cmek.console_key_url(
            parts["project"], parts["location"], parts["ring"], parts["key"])
        # Read from the key itself, since a key adopted rather than created
        # may have its own; the page falls back to OFE's default without it.
        context["destroy_window"] = _describe_duration(
            cloud_info.crypto_key_destroy_scheduled_duration(
                row.cloud_credential.detail, path))
        return context

    def post(self, request, *args, **kwargs):
        row, label, _route, path, parts = self._target()
        # The page names the key it was rendered for. If the resource has
        # since been redeployed onto another key, confirming the old page must
        # not destroy the new one.
        if request.POST.get("path") != path:
            messages.error(
                request,
                f"{label} {row.name!r} now uses key {path}, not the one you "
                "confirmed. Nothing was scheduled for destruction; check the "
                "key and confirm again.")
            return HttpResponseRedirect(reverse("cmek-keys"))
        # Every outcome from here on records the revocation on the resource:
        # once the rotation schedule is cleared the key is no longer a valid,
        # rotating key, and OFE must not start, change or re-apply it again.
        abandon = (f"Destroy this {label.lower()} and create a new one: OFE "
                   "will no longer start, change or re-apply it.")
        try:
            destroyed = cloud_info.destroy_crypto_key_versions(
                row.cloud_credential.detail, path)
        except cloud_info.KeyDestructionError as err:
            logger.exception("Could not destroy key versions for %s", path)
            # Rotation is cleared and some versions may be scheduled, so the
            # resource may already be failing; its own page has to say which.
            if err.scheduled:
                progress = (
                    "Rotation was cleared. Scheduled for destruction: "
                    f"{', '.join(err.scheduled)}. Failed: {err.failed}. "
                    "Versions after it were not attempted. Revoke the key "
                    "again to finish, or restore the scheduled versions in the "
                    "Cloud Console.")
            else:
                progress = (
                    "Rotation was cleared but no version was scheduled for "
                    f"destruction; failure: {err.failed}; revoke again to "
                    "finish.")
            self._record_revoked(
                row, f"Revoking encryption key {parts['key']} stopped "
                     f"part-way. {progress} {abandon}")
            messages.error(
                request,
                f"Could not schedule {parts['key']} for destruction: {err}")
            return HttpResponseRedirect(reverse("cmek-keys"))
        except Exception as err:  # pylint: disable=broad-except
            # The rotation schedule could not be cleared, so nothing changed
            # and the key still protects a working resource.
            logger.exception("Could not destroy key versions for %s", path)
            messages.error(
                request,
                f"Could not schedule {parts['key']} for destruction: {err}")
            return HttpResponseRedirect(reverse("cmek-keys"))

        if not destroyed:
            self._record_revoked(
                row, f"Encryption key {parts['key']} was revoked by an "
                     "administrator: its rotation was cleared and it had no "
                     f"live versions left to destroy. {abandon}")
            messages.warning(
                request,
                f"{parts['key']} had no live versions; nothing to destroy. "
                f"Its rotation was cleared. {abandon}")
            return HttpResponseRedirect(reverse("cmek-keys"))

        # Say so on the resource's own page: from here on it is expected to
        # fail, and an operator looking at it needs to know why.
        self._record_revoked(
            row, f"Encryption key {parts['key']} was scheduled for destruction "
                 f"by an administrator. This {label.lower()} can no longer be "
                 "used, and data encrypted with the key becomes unreadable "
                 f"once Cloud KMS completes the destruction. {abandon}")

        messages.success(
            request,
            f"{parts['key']} scheduled for destruction "
            f"({len(destroyed)} version(s)). {label} {row.name!r} can no longer "
            "be used. Cloud KMS holds the versions for the key's scheduled "
            "duration before the material is destroyed; only the Cloud Console "
            f"can restore them within that window. {abandon}")
        return HttpResponseRedirect(reverse("cmek-keys"))

    @staticmethod
    def _record_revoked(row, message):
        """Mark the resource's key as revoked, and say why on its page."""
        row.status_message = message
        row.cmek_revoked = True
        row.save(update_fields=["status_message", "cmek_revoked"])
