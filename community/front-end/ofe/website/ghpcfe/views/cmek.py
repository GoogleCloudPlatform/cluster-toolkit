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
import hashlib
import json
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

STALE_SETTINGS_MESSAGE = ("The settings were changed since you loaded this "
                          "page; reload and try again")


class StaleSettingsError(Exception):
    """The `cmek` section changed after the page being saved was loaded."""


def _section_digest(config):
    """A digest of the `cmek` section of the `config` mapping, as stored.

    The section is hashed as read from the file, not as the form shows it, so
    a change to it -- a value the form offers or one edited by hand -- marks
    a page loaded before the change as stale. A missing section has a digest
    of its own, so adding a section, even an empty one, counts as a change; a
    bare `cmek:` line reads as no section. JSON writes keys and dates as text,
    so a hand edit that only turns a key into the same text (1 to "1", when
    no key of another type sits beside it) or a date into the same text is
    not seen; a value that changes type (1 to "1") is.
    """
    section = (config or {}).get("cmek")
    try:
        canonical = json.dumps(section, sort_keys=True, default=str,
                               separators=(",", ":"))
    except (TypeError, ValueError, RecursionError):
        # A hand-edited file can hold what JSON cannot sort or hold: keys of
        # mixed types, or a section that contains itself. The digest must
        # still exist, or the page could not be shown to fix the section.
        canonical = json.dumps(_canonical_form(section), default=str,
                               separators=(",", ":"))
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def _canonical_form(value, _active=()):
    """`value` as plain lists and strings that JSON can always write.

    A mapping becomes its entries, each key and value in canonical form,
    ordered by the key's type and text, so the same section gives the same
    form in every process and sections that differ give different ones
    (a key 1 and a key "1" stay apart). Only used when json.dumps cannot
    take the value as it is.
    """
    if isinstance(value, (dict, list, tuple, set, frozenset)):
        if id(value) in _active:
            return ["cycle"]
        _active = _active + (id(value),)
    if isinstance(value, dict):
        entries = [[_canonical_form(key, _active),
                    _canonical_form(item, _active)]
                   for key, item in value.items()]
        entries.sort(key=lambda entry: json.dumps(
            entry[0], sort_keys=True, default=str))
        return ["map", entries]
    if isinstance(value, (list, tuple)):
        return ["seq", [_canonical_form(item, _active) for item in value]]
    if isinstance(value, (set, frozenset)):
        return ["set", sorted(
            json.dumps(_canonical_form(item, _active), default=str)
            for item in value)]
    return [type(value).__name__, repr(value)]


class CmekSettingsView(SuperUserRequiredMixin, FormView):
    """Edit the `cmek:` section of the server configuration.

    Admin only, like the key pages: the settings decide where every new
    resource's key lives, and the file they are written to is the server's
    own configuration.

    A save replaces the whole section, so a page loaded before another
    administrator's save would silently undo that save with the values it
    still shows. The page therefore carries a digest of the section it was
    rendered from (LOADED_FIELD), and a save is refused, with the file left
    as it is, when the section read under the config file's lock no longer
    matches it. The digest travels with the form rather than in the session,
    so each of an administrator's tabs is checked against what it showed.
    """

    template_name = "cmek/settings.html"
    form_class = CmekSettingsForm
    LOADED_FIELD = "loaded_digest"

    def _current(self):
        """The settings as they stand, as the form's initial values.

        A configuration that fails to parse must still render the page: a
        setting the operator cannot reach is a setting they cannot fix. The
        stored values are shown as they are, so the one at fault can be seen
        and corrected; only an enforcement mode the form cannot offer falls
        back to the default.
        """
        config = utils.load_config()
        # what this page shows, from the same read as the values themselves
        self._loaded_digest = _section_digest(config)
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
        if self.request.method == "POST":
            # A page shown again after a refused or invalid save still stands
            # for the settings it was first loaded with. Handing it a fresh
            # digest would let a resubmission overwrite a change its form
            # never showed; only a reload brings the page up to date.
            context["loaded_digest"] = self.request.POST.get(
                self.LOADED_FIELD, "")
        else:
            context["loaded_digest"] = getattr(self, "_loaded_digest", None) \
                or _section_digest(utils.load_config())
        return context

    def form_valid(self, form):
        settings = form.settings
        # A missing or altered value matches no digest, so it is refused as
        # stale rather than trusted.
        loaded = self.request.POST.get(self.LOADED_FIELD, "")

        def replace_section(section):
            # Runs under update_config_file's lock, against the section as
            # read there: raising aborts the write and leaves the file as it is.
            if _section_digest(section) != loaded:
                raise StaleSettingsError()
            section["cmek"] = settings.to_config_section()

        try:
            utils.update_config_file(replace_section)
        except StaleSettingsError:
            logger.info("Refused a save of the encryption settings from a "
                        "page loaded before they last changed")
            # The page's values may have come from a copy of the file this
            # process read before someone changed it; re-read it so the
            # reload the message asks for shows the current settings.
            utils.invalidate_config_cache()
            form.add_error(None, STALE_SETTINGS_MESSAGE)
            return self.form_invalid(form)
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


def _live_rows(kind, pk=None):
    """The live, encrypted resources of one kind whose key OFE has applied.

    Torn-down resources are excluded: their key is no longer this deployment's
    concern, and listing one would invite destroying a key nothing uses.

    So are resources no apply has been started for (cmek.is_frozen). Their key
    fields are only what was typed on the form, and a typed name can match a
    key OFE never created; offering that key for destruction could destroy a
    key that belongs to something else. A failed first apply counts as
    applied, since it may already have created the key.

    With `pk`, only that resource is considered.
    """
    model = _cmek_kinds()[kind][0]
    rows = model.objects.filter(cmek_enabled=True).exclude(cloud_state="xm")
    if kind == "cluster":
        rows = rows.exclude(status="d")
    if pk is not None:
        rows = rows.filter(pk=pk)
    return [row for row in rows if cmek.is_frozen(row)]


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
        row = next(iter(_live_rows(kind, pk=pk)), None)
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
