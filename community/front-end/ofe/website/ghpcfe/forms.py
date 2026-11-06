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
""" forms.py """
import dataclasses
import json
import logging
import re

from django import forms
from django.contrib.auth.forms import (
    UserChangeForm as BaseUserChangeForm,
    UserCreationForm as BaseUserCreationForm,
)
from django.db.models import Q
from django.forms import ValidationError
from django.utils.safestring import mark_safe

from crispy_forms.helper import FormHelper
from crispy_forms.layout import Layout, Field, Submit, Div, Hidden
from crispy_forms.bootstrap import InlineCheckboxes

from .cluster_manager import cloud_info
from .cluster_manager import cmek
from .cluster_manager import utils
from .cluster_manager import validate_credential
from .views.view_utils import RegistryDataHelper

# If we have a model, it has a form - pretty much
from .models import *  # pylint: disable=wildcard-import,unused-wildcard-import

logger = logging.getLogger(__name__)


class UserCreationForm(BaseUserCreationForm):
    """Custom UserCreationForm"""

    class Meta(BaseUserCreationForm):
        model = User
        fields = ("email",)


class UserUpdateForm(BaseUserChangeForm):
    """Custom form for updating user account"""

    password = None

    class Meta:
        model = User
        fields = ("email",)
        widgets = {
            "email": forms.TextInput(attrs={"class": "form-control"}),
        }


class UserAdminUpdateForm(forms.ModelForm):
    """Custom form for Admin update of users"""

    class Meta:
        model = User
        fields = (
            "username",
            "email",
            "roles",
            "quota_type",
            "quota_amount",
        )
        widgets = {
            "username": forms.TextInput(attrs={"class": "form-control"}),
            "email": forms.TextInput(attrs={"class": "form-control"}),
            "roles": forms.SelectMultiple(attrs={"class": "form-control"}),
            "quota_type": forms.Select(
                attrs={"class": "form-control", "disabled": False}
            ),
            "quota_amount": forms.NumberInput(attrs={"class": "form-control"}),
        }


class CredentialForm(forms.ModelForm):
    """Custom form for Credential model implementing additional validation"""

    class Meta:
        model = Credential

        fields = ("name", "detail")

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "detail": forms.Textarea(attrs={"class": "form-control"}),
        }

    def clean(self):
        super().clean()

        # validate the credential details with cloud platform
        detail = self.cleaned_data["detail"]
        validated = validate_credential.validate_credential("GCP", detail)
        if not validated:
            raise ValidationError("Credential cannot be validated.")


# The encryption panel, shared by every resource form that offers it.
#
# The fields are model fields (models.CmekResourceMixin), so a form only has to
# list them and call the two hooks below. Declared once because the one thing
# that went wrong before was a field present on the model but missing from a
# form.
CMEK_PANEL_FIELDS = (
    "cmek_enabled",
    "cmek_key_name",
    "cmek_key_project_id",
    "cmek_key_ring_name",
    "cmek_deletion_policy",
    "cmek_protection_level",
    "cmek_rotation_period",
    "cmek_encryption_service_account",
)

CMEK_PANEL_WIDGETS = {
    "cmek_enabled": forms.CheckboxInput(attrs={"class": "form-check-input"}),
    "cmek_key_name": forms.TextInput(
        attrs={"class": "form-control", "placeholder": "e.g. research-a-key"}),
    "cmek_key_project_id": forms.TextInput(attrs={"class": "form-control"}),
    "cmek_key_ring_name": forms.TextInput(attrs={"class": "form-control"}),
    "cmek_deletion_policy": forms.Select(attrs={"class": "form-control"}),
    "cmek_protection_level": forms.Select(attrs={"class": "form-control"}),
    "cmek_rotation_period": forms.TextInput(
        attrs={"class": "form-control", "placeholder": "7776000s"}),
    "cmek_encryption_service_account": forms.TextInput(attrs={
        "class": "form-control",
        "placeholder": "name@project.iam.gserviceaccount.com",
    }),
}


class CmekPanelMixin:
    """Wires the encryption panel into a resource form."""

    def setup_cmek_panel(self, settings=None, location=None):
        """Gate the panel on enforcement and populate the key picker."""
        # Kept for the key-name uniqueness check in clean_cmek_panel, which
        # needs the resource's location to build the full key path.
        self._cmek_location = location
        self._cmek_config_error = None

        instance = getattr(self, "instance", None)
        self._cmek_frozen = cmek.is_frozen(instance)
        if settings is None:
            try:
                settings = cmek.CmekSettings.from_config(utils.load_config())
            except cmek.CmekConfigError as err:
                # Invalid stored settings must not break every resource form.
                # A deployed resource needs none of them to show its stored
                # choice; any other is refused at clean_cmek_panel, which
                # names the page that fixes it.
                if not self._cmek_frozen:
                    self._cmek_config_error = str(err)
        self.cmek_settings = settings

        if self._cmek_frozen:
            # A deployed resource keeps the encryption it was deployed with,
            # whatever enforcement says now: shown, but not editable. Django
            # ignores a disabled field's POST value and uses the initial one.
            for name in CMEK_PANEL_FIELDS:
                if name in self.fields:
                    self.fields[name].disabled = True
                    self.initial[name] = getattr(
                        instance, name, self.initial.get(name))
            if "cmek_enabled" in self.fields:
                self.fields["cmek_enabled"].help_text = (
                    "Encryption is fixed once a resource is deployed.")
            self._set_cmek_choices()
            return

        if self._cmek_config_error or not self.cmek_settings.offered:
            # enforcement: disabled -- no key is ever emitted, so offering the
            # controls would be a lie. Unreadable settings cannot say what to
            # offer either.
            for name in CMEK_PANEL_FIELDS:
                self.fields.pop(name, None)
            return

        if "cmek_enabled" in self.fields and self.cmek_settings.required:
            # A resource cannot opt out of a deployment that requires CMEK.
            field = self.fields["cmek_enabled"]
            field.initial = True
            # A ModelForm built with an instance takes its starting value from
            # the instance (the model default is False), and that wins over
            # field.initial, so the locked toggle would render off.
            self.initial["cmek_enabled"] = True
            field.disabled = True
            field.help_text = (
                "This deployment requires a customer-managed encryption key, "
                "so encryption cannot be turned off here."
            )

        self._set_cmek_choices()
        if "cmek_deletion_policy" in self.fields \
                and not self.initial.get("cmek_deletion_policy"):
            self.initial["cmek_deletion_policy"] = \
                cmek.DELETION_POLICY_ABANDON

    def _set_cmek_choices(self):
        """Label the protection-level and tear-down choices."""
        if "cmek_protection_level" in self.fields:
            self.fields["cmek_protection_level"].choices = [
                ("", "Use the module default (SOFTWARE)"),
            ] + [(p, p) for p in cmek.PROTECTION_LEVELS]
        if "cmek_deletion_policy" in self.fields:
            # kms-key has no default, so name the one OFE will send, and lead
            # with the value the module receives: an operator choosing a
            # tear-down policy needs to know whether they picked DELETE or
            # ABANDON, not only what the words mean. ABANDON is listed first
            # because it is the default, and the one that cannot destroy data.
            self.fields["cmek_deletion_policy"].choices = [
                (cmek.DELETION_POLICY_ABANDON,
                 "ABANDON — leave the key in place when this resource is "
                 "torn down"),
                (cmek.DELETION_POLICY_DELETE,
                 "DELETE — destroy the key when this resource is torn down; "
                 "anything still encrypted with it becomes unreadable"),
            ]

    def clean_cmek_panel(self, cleaned):
        """Validate the panel, and clear it when encryption is off."""
        if getattr(self, "_cmek_config_error", None):
            raise forms.ValidationError(
                "This deployment's encryption settings are invalid "
                f"({self._cmek_config_error}). An administrator must correct "
                "them on the Encryption settings page before resources can be "
                "saved.")
        if not getattr(self, "cmek_settings", None) \
                or getattr(self, "_cmek_frozen", False) \
                or not self.cmek_settings.offered:
            # a deployed resource's panel is disabled, so cleaned already
            # holds the stored values, and they stand
            return cleaned

        enabled = bool(cleaned.get("cmek_enabled"))
        if self.cmek_settings.required:
            enabled = True
            cleaned["cmek_enabled"] = True

        if not enabled:
            # Nothing below applies, and leaving stale values on the record
            # would make the detail page claim an encryption that is not there.
            for name in CMEK_PANEL_FIELDS:
                if name == "cmek_enabled":
                    continue
                if name in cleaned:
                    cleaned[name] = ""
            return cleaned

        if not cleaned.get("cmek_deletion_policy"):
            cleaned["cmek_deletion_policy"] = cmek.DELETION_POLICY_ABANDON
        for field, label in (("cmek_key_name", "key name"),
                             ("cmek_key_ring_name", "key ring")):
            value = (cleaned.get(field) or "").strip()
            if value and not cmek.KMS_NAME_RE.fullmatch(value):
                self.add_error(field, (
                    f"A Cloud KMS {label} may use letters, digits, hyphen "
                    "and underscore only, up to 63 characters, and can "
                    "never be changed once created."))
        project = (cleaned.get("cmek_key_project_id") or "").strip()
        if project:
            try:
                cleaned["cmek_key_project_id"] = cmek.assert_project_id(
                    project, what="key project")
            except cmek.CmekConfigError as err:
                self.add_error("cmek_key_project_id", str(err))

        for field, check in (
                ("cmek_protection_level", cmek.assert_protection_level),
                ("cmek_rotation_period", cmek.assert_rotation_period)):
            try:
                cleaned[field] = check(cleaned.get(field))
            except cmek.CmekConfigError as err:
                self.add_error(field, str(err))

        self._reject_a_key_another_resource_uses(cleaned)

        account = (cleaned.get("cmek_encryption_service_account") or "").strip()
        if account:
            try:
                cleaned["cmek_encryption_service_account"] = \
                    cmek.assert_service_account_email(account)
            except cmek.CmekConfigError as err:
                self.add_error("cmek_encryption_service_account", str(err))
        return cleaned

    def _cmek_credential_project(self, cleaned):
        """The project of the credential this resource deploys with, or "".

        From the submission on create, where the instance has no credential
        yet; from the instance on update, where the field may be disabled.
        """
        credential = (cleaned.get("cloud_credential")
                      or getattr(getattr(self, "instance", None),
                                 "cloud_credential", None))
        try:
            return json.loads(getattr(credential, "detail", "") or "{}").get(
                "project_id", "") or ""
        except ValueError:
            return ""

    def _reject_a_key_another_resource_uses(self, cleaned):
        """Refuse a key name a live resource already has.

        kms-key-iam's service agents are project-scoped, so two resources on
        one key share a single (member, role) grant that either one's teardown
        destroys, taking the other's access with it. Each resource
        therefore gets its own key. The derived default name cannot collide,
        but a name typed in here can, so it is checked before anything is
        deployed rather than discovered at teardown.

        The location must come from this submission's cleaned data, not
        self._cmek_location: that was captured from self.instance at __init__,
        which on every Create form is a blank, unsaved instance with no
        cloud_zone/cloud_region set yet -- the operator is choosing it in this
        same submission. Reading only self._cmek_location made this check a
        silent no-op on every create, which is exactly where a duplicate name
        first gets typed. self._cmek_location remains a fallback for a form
        that omits both fields.

        The project is resolved by cmek.resolve_created_key, the same function
        the deploy uses, so a blank key project falls back to the credential's
        project here exactly as it does in the blueprint. Every other resource
        is resolved the same way, so one not yet deployed, whose project and
        key ring are still blank, is compared by where its key will live.
        """
        name = (cleaned.get("cmek_key_name") or "").strip()
        if not name:
            # left to OFE, which derives a name unique to this resource
            return
        # Resolve ring and project exactly as the deploy will, including its
        # fallback to the credential's project when neither this resource nor
        # the deployment settings name one -- the default. Resolving it any
        # other way let this check pass a key name the deploy then shared.
        resolved = cmek.resolve_created_key(
            {"key_name": name,
             "key_ring_name": cleaned.get("cmek_key_ring_name"),
             "key_project_id": cleaned.get("cmek_key_project_id")},
            self.cmek_settings, self._cmek_credential_project(cleaned))
        ring = resolved["key_ring_name"]
        project = resolved["key_project_id"]
        location = cmek.normalize_location(
            cleaned.get("cloud_zone") or cleaned.get("cloud_region")
            or self._cmek_location or "")
        if not (ring and project and location):
            # not enough to build a path with, so nothing to compare against
            return
        path = cmek.full_key_path(project, location, ring, name)
        # a plain Form (the render tests) has no instance; on create the
        # instance has no pk, so it excludes nothing, which is correct
        sharers = cmek.resources_sharing_key(
            path, exclude=getattr(self, "instance", None),
            settings=self.cmek_settings)
        if sharers:
            self.add_error("cmek_key_name", (
                f"{', '.join(sharers)} already uses this key. Each resource "
                "needs its own: tearing either one down would revoke the "
                "other's access to a shared key. Leave the name blank to let "
                "OFE choose one."))


class ClusterForm(CmekPanelMixin, forms.ModelForm):
    """Custom form for Cluster model implementing option filtering"""

    def _get_creds(self, kwargs):
        # We do this, because on Create views, there isn't an instance, so we
        # set the creds via the 'initial' data field.  On Updates, there is
        # an object, so pull from there
        if "cloud_credential" in kwargs["initial"]:
            creds = kwargs["initial"]["cloud_credential"]
        else:
            creds = self.instance.cloud_credential
        return creds

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

        self.setup_cmek_panel(
            location=(getattr(self.instance, "cloud_zone", None)
                      or getattr(self.instance, "cloud_region", None)))

        # Set default for new creations to True (Native Auth)
        if not self.instance.id:
            self.fields['enable_slurm_auth'].initial = True

        # For machine types, will use JS to get valid types dependent on
        # cloud zone. So bypass cleaning and choices
        def prep_dynamic_select(field, value):
            self.fields[field].widget.choices = [
                ( value, value )
            ]
            self.fields[field].clean = lambda value: value

        prep_dynamic_select(
            "controller_instance_type",
            self.instance.controller_instance_type
        )
        prep_dynamic_select(
            "controller_disk_type",
            self.instance.controller_disk_type
        )
        prep_dynamic_select(
            "login_node_instance_type",
            self.instance.login_node_instance_type
        )
        prep_dynamic_select(
            "login_node_disk_type",
            self.instance.login_node_disk_type
        )

        # If cluster is running make some of form field ready only.
        if self.instance.status == "r":
            logger.info("Cluster is running making some fields ready only")
            # Define a list of field names you want to set as readonly
            fields_to_make_readonly = ['cloud_credential', 'name', 'subnet', 'cloud_region', 'cloud_zone']

            # Loop through the fields and set the 'readonly' attribute.
            # disabled makes Django keep the instance's value whatever a
            # POST sends; readonly alone only stops the browser editing it.
            for field_name in fields_to_make_readonly:
                self.fields[field_name].widget = forms.TextInput(attrs={'class': 'form-control'})
                self.fields[field_name].widget.attrs['readonly'] = True
                self.fields[field_name].disabled = True

    def clean(self):
        return self.clean_cmek_panel(super().clean())

    class Meta:
        model = Cluster

        fields = (
            "cloud_credential",
            "name",
            "subnet",
            "cloud_region",
            "cloud_zone",
            "authorised_users",
            "spackdir",
            "controller_instance_type",
            "controller_disk_type",
            "controller_disk_size",
            "num_login_nodes",
            "login_node_instance_type",
            "login_node_disk_type",
            "login_node_disk_size",
            "login_node_image",
            "controller_node_image",
            "use_cloudsql",
            "use_bigquery",
            "use_containers",
            "enable_slurm_auth",
        ) + CMEK_PANEL_FIELDS

        widgets = {
            **CMEK_PANEL_WIDGETS,
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control"}
            ),
            "subnet": forms.Select(attrs={"class": "form-control"}),
            "cloud_region": forms.Select(attrs={"class": "form-control", "readonly": "readonly"}),
            "cloud_zone": forms.Select(attrs={"class": "form-control"}),
            "authorised_users": forms.SelectMultiple(attrs={"class": "form-control"}),
            "spackdir": forms.TextInput(attrs={"class": "form-control"}),
            "controller_instance_type": forms.Select(
                attrs={"class": "form-control machine_type_select"}
            ),
            "controller_disk_size": forms.NumberInput(
                attrs={"class": "form-control"}
            ),
            "controller_disk_type": forms.Select(
                attrs={"class": "form-control disk_type_select"}
            ),
            "login_node_instance_type": forms.Select(
                attrs={"class": "form-control machine_type_select"}
            ),
            "login_node_disk_size": forms.NumberInput(
                attrs={"class": "form-control"}
            ),
            "login_node_disk_type": forms.Select(
                attrs={"class": "form-control disk_type_select"}
            ),
            "num_login_nodes": forms.NumberInput(
                attrs={"class": "form-control"}
            ),
            "login_node_image": forms.Select(attrs={"class": "form-control",
                                                       "id": "login-node-image",
                                                       "name": "login_node_image",
                                                       "value": "",}),
            "controller_node_image": forms.Select(attrs={"class": "form-control",
                                                       "id": "controller-node-image",
                                                       "name": "controller_node_image",
                                                       "value": "",}),
            "use_cloudsql": forms.CheckboxInput(attrs={"class": "required checkbox"}),
            "use_bigquery": forms.CheckboxInput(attrs={"class": "required checkbox"}),
            "use_containers": forms.CheckboxInput(attrs={"class": "required checkbox"}),
            "enable_slurm_auth": forms.CheckboxInput(attrs={"class": "required checkbox"}),
        }


class ClusterMountPointForm(forms.ModelForm):
    """Form for Cluster Mount points"""

    class Meta:
        model = MountPoint
        fields = ("export", "mount_order", "mount_path", "mount_options")

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        for field in self.fields:
            self.fields[field].widget.attrs.update({"class": "form-control"})


class ClusterPartitionForm(forms.ModelForm):
    """Form for Cluster Partitions"""

    machine_type = forms.ChoiceField(widget=forms.Select())
    GPU_type = forms.ChoiceField(widget=forms.Select()) # pylint: disable=invalid-name

    class Meta:
        model = ClusterPartition
        fields = (
            "name",
            "machine_type",
            "image",
            "dynamic_node_count",
            "static_node_count",
            "reservation_name",
            "exclusive",
            "enable_placement",
            "enable_hyperthreads",
            "enable_tier1_networking",
            "enable_node_reuse",
            "GPU_type",
            "GPU_per_node",
            "boot_disk_type",
            "boot_disk_size",
            "additional_disk_type",
            "additional_disk_count",
            "additional_disk_size",
            "additional_disk_auto_delete"
        )

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

        for field in self.fields:
            self.fields[field].widget.attrs.update({"class": "form-control"})
            if self.fields[field].help_text:
                self.fields[field].widget.attrs.update(
                    {"title": self.fields[field].help_text}
                )
        
        self.fields["boot_disk_type"].widget = forms.Select(attrs={"class": "form-control disk_type_select"})
        self.fields["additional_disk_type"].widget = forms.Select(attrs={"class": "form-control disk_type_select"})

        self.fields["machine_type"].widget.attrs[
            "class"
        ] += " machine_type_select"

        def prep_dynamic_select(field, value):
            self.fields[field].widget.choices = [
                ( value, value )
            ]
            self.fields[field].clean = lambda value: value
        
        prep_dynamic_select(
            "boot_disk_type",
            self.instance.boot_disk_type
        )

        prep_dynamic_select(
            "additional_disk_type",
            self.instance.additional_disk_type
        )

        prep_dynamic_select(
            "machine_type",
            self.instance.machine_type
        )

        prep_dynamic_select(
            "GPU_type",
            self.instance.GPU_type
        )

        # Mark 'reservation_name' as optional
        self.fields["reservation_name"].widget.attrs.update({"placeholder": "Optional"})

    def clean(self):
        cleaned_data = super().clean()
        if cleaned_data["enable_placement"] and cleaned_data[
            "machine_type"
        ].split("-")[0] in ["e2", "t2d", "n1", "t2a", "m1", "m2", "m3"]:
            raise ValidationError(
                "SlurmGCP does not support Placement Groups for selected instance type"  # pylint: disable=line-too-long
            )

        # schedmd-slurm-gcp-v6-partition/outputs.tf
        if cleaned_data["dynamic_node_count"] > 0 and not cleaned_data[
            "exclusive"
        ]:
            raise ValidationError(
                "If any non-static nodesets have enable placement set to true, exclusive must be true."
            )  

        if cleaned_data["static_node_count"] > 0 and cleaned_data[
            "exclusive"
        ]:
            raise ValidationError(
                "Can't use static nodes within partition with exclusive set to true."
            )  

        # schedmd-slurm-gcp-v6-nodeset/outputs.tf
        if cleaned_data["reservation_name"] and cleaned_data[
            "enable_placement"
        ]:
            raise ValidationError("If a reservation is specified, placement must be false.")  

        if cleaned_data["enable_placement"] and cleaned_data[
            "static_node_count"
        ] > 0 and cleaned_data[
            "dynamic_node_count"
        ] > 0:
            raise ValidationError(
                "Cannot use placement with static and auto-scaling nodes in the same node set."
            )  

        # Reservation validation logic
        reservation_name = cleaned_data.get("reservation_name")
        if reservation_name:
            try:
                cluster = cleaned_data.get('cluster')
                cloud_credential = cluster.cloud_credential.detail
                cloud_zone = cluster.cloud_zone

                # logger.info(f"Cluster: {cluster}")
                # logger.info(f"Cloud Credential: {cloud_credential}")
                # logger.info(f"Cloud Zone: {cloud_zone}")

                reservations = cloud_info.get_vm_reservations("GCP", cloud_credential, None, cloud_zone)

                if not reservations:
                    raise ValidationError("No reservations found for the specified zone.")

                matching_reservation = reservations.get(reservation_name)

                if not matching_reservation:
                    raise ValidationError(
                        f"Reservation {reservation_name} does not exist in the specified zone."
                    )

                if matching_reservation[
                    "instanceProperties"
                    ][
                        "machineType"
                        ] != cleaned_data["machine_type"]:
                    raise ValidationError(
                        f"Reservation {reservation_name} does not support the specified machine type. "
                        f"Machine type: {cleaned_data['machine_type']}."
                    )

                total_requested_nodes = cleaned_data["dynamic_node_count"] + cleaned_data["static_node_count"]
                available_nodes = matching_reservation.get("instanceProperties", {}).get("availableCount", 0)

                if total_requested_nodes > available_nodes:
                    raise ValidationError(
                        f"Reservation {reservation_name} does not have enough available nodes."
                        f"Requested: {total_requested_nodes}, Available: {available_nodes}"
                    )

                specific_reservation = matching_reservation.get("specificReservationRequired")
                if specific_reservation == False:
                    raise ValidationError(
                        "You must use a 'specific' reservation type. "
                        "Please read the following URL for more information about setting up reservations: "
                        "https://cloud.google.com/compute/docs/instances/reservations-overview#how-reservations-work"
                    )

            except Exception as e:
                logger.error(f"Error validating reservation: {reservation_name}. Exception: {e}")
                raise ValidationError(
                    f"Error validating reservation: {reservation_name}. Exception: {str(e)}"
                )

        return cleaned_data


class WorkbenchForm(forms.ModelForm):
    """Custom form for Workbench model implementing option filtering"""

    def clean(self):
        cleaned_data = super().clean()
        subnet = cleaned_data.get("subnet")

        if subnet.cloud_region not in self.workbench_zones:
            validation_error_message = (
                f"Network {subnet.vpc.cloud_id} has an invalid region & zone "
                "for Vertex AI Workbenches: {subnet.cloud_region}. Please see "
                '<a href="https://cloud.google.com/vertex-ai/docs/general/'
                'locations#vertex-ai-workbench-locations" target="_blank"> '
                "Workbench Documentation</a> for more information on region "
                "availability."
            )
            raise forms.ValidationError(mark_safe(validation_error_message))

        user = cleaned_data.get("trusted_user")
        # check user is associated with a social login account
        try:
            if user.socialaccount_set.first().uid:
                pass
        except:
            raise forms.ValidationError(  # pylint: disable=raise-missing-from
                "User not associated with a required Social ID "
            )

    def __init__(self, user, *args, **kwargs):
        has_creds = "cloud_credential" in kwargs
        if has_creds:
            credential = kwargs.pop("cloud_credential")
            kwargs["initial"]["cloud_credential"] = credential
        super().__init__(*args, **kwargs)
        if not has_creds:
            credential = self.instance.cloud_credential
        zone_choices = None
        if "zone_choices" in kwargs:
            zone_choices = kwargs.pop("zone_choices")

        if self.instance.id:
            for field in self.fields:
                if field != "name":
                    self.fields[field].disabled = True

        self.fields["subnet"].queryset = VirtualSubnet.objects.filter(
            cloud_credential=credential
        ).filter(Q(cloud_state="i") | Q(cloud_state="m"))
        if zone_choices:
            # We set this on the widget, because we will be changing the
            # widget's field in the template via javascript
            self.fields["cloud_zone"].widget.choices = zone_choices

        if "n" not in self.instance.cloud_state:
            # Need to disable certain widgets
            self.fields["subnet"].disabled = True
            self.fields["cloud_zone"].disabled = True
            self.fields["attached_cluster"].disabled = True

        self.workbench_zones = cloud_info.get_gcp_workbench_region_zone_info(
            credential.detail
        )

        self.fields["trusted_user"].queryset = (
            User.objects.exclude(socialaccount__isnull=True)
        )

        # Pull instance types from cloud_info
        instance_types = cloud_info.get_machine_types(
            "GCP", credential.detail, "europe-west4", "europe-west4-a"
        )
        # set variables for retrieving instance types for dropdown menu
        choices_list = []
        instance_list = []
        category = ""
        # Populate dropdown menu with preset instance_types from
        # WorkbenchPresets
        for preset in WorkbenchPreset.objects.order_by("category").values():
            # if category variable has changed from last loop then append
            # instances to overall choices list as tuple and clear instance_list
            if category != preset["category"]:
                if category:
                    choices_list.append((category, tuple(instance_list)))
                instance_list = []
            # set category to current value and append preset to dropdown menu
            # list
            category = preset["category"]
            instance_list.append((preset["machine_type"], preset["name"]))
        # append final preset instance type from loop
        choices_list.append((category, tuple(instance_list)))
        category = ""
        if Role.CLUSTERADMIN in [x.id for x in user.roles.all()]:
            for instance_type in sorted(instance_types):
                # if family variable has changed from last loop then append
                # instances to overall choices list as tuple and clear
                # instance_list
                if category != instance_types[instance_type]["family"]:
                    if category:
                        choices_list.append((category, tuple(instance_list)))
                    instance_list = []
                # save family of current instance
                category = instance_types[instance_type]["family"]
                # create instance string for displaying to user
                instance_string = (
                    instance_types[instance_type]["name"]
                    + " - "
                    + str(instance_types[instance_type]["vCPU"])
                    + "x "
                    + instance_types[instance_type]["arch"]
                    + " vCPUs with "
                    + str(instance_types[instance_type]["memory"])
                    + " Memory"
                )
                # append tuple to instance list
                instance_list.append(
                    (instance_types[instance_type]["name"], instance_string)
                )
            # append final preset instance type from loop
            choices_list.append((category, tuple(instance_list)))
        self.fields["machine_type"].widget.choices = choices_list
        self.fields["attached_cluster"].queryset= Cluster.objects.filter(
                cloud_state="m"
                )

    class Meta:
        model = Workbench

        fields = (
            "name",
            "subnet",
            "cloud_zone",
            "cloud_credential",
            "trusted_user",
            "machine_type",
            "boot_disk_type",
            "boot_disk_capacity",
            "image_family",
            "attached_cluster",
        )

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "subnet": forms.Select(attrs={"class": "form-control"}),
            "machine_type": forms.Select(attrs={"class": "form-control"}),
            "cloud_zone": forms.Select(attrs={"class": "form-control"}),
            "trusted_user": forms.Select(attrs={"class": "form-control"}),
            "attached_cluster": forms.Select(attrs={"class": "form-control"}),
        }


class ApplicationEditForm(forms.ModelForm):
    """Custom form for application model"""

    class Meta:
        model = Application

        fields = ("name", "description")

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "description": forms.Textarea(attrs={"class": "form-control"}),
            "load_command": forms.TextInput(attrs={"class": "form-control"}),
        }


class ApplicationForm(forms.ModelForm):
    """Custom form for application model"""

    installation_path = forms.CharField(
        widget=forms.TextInput(attrs={"class": "form-control"}),
        help_text="Path where application was installed.",
    )

    class Meta:
        model = Application

        fields = (
            "cluster",
            "name",
            "version",
            "description",
            "load_command",
            "installed_architecture",
            "compiler",
            "mpi",
        )

        widgets = {
            "cluster": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "version": forms.TextInput(attrs={"class": "form-control"}),
            "description": forms.Textarea(attrs={"class": "form-control"}),
            "load_command": forms.TextInput(attrs={"class": "form-control"}),
            "installed_architecture": forms.TextInput(
                attrs={"class": "form-control"}
            ),
            "compiler": forms.TextInput(attrs={"class": "form-control"}),
            "mpi": forms.TextInput(attrs={"class": "form-control"}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)


class CustomInstallationApplicationForm(forms.ModelForm):
    """Form to collect custom app installation details"""

    install_loc = forms.CharField(
        widget=forms.TextInput(attrs={"class": "form-control"}),
        help_text="Path where application will be installed.",
    )

    class Meta:
        model = CustomInstallationApplication

        fields = (
            "cluster",
            "name",
            "version",
            "description",
            "install_partition",
            "install_script",
            "module_name",
            "module_script",
        )

        widgets = {
            "cluster": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "version": forms.TextInput(attrs={"class": "form-control"}),
            "description": forms.Textarea(attrs={"class": "form-control"}),
            "install_partition": forms.Select(attrs={"class": "form-control"}),
            "install_script": forms.URLInput(attrs={"class": "form-control"}),
            "module_name": forms.TextInput(attrs={"class": "form-control"}),
            "module_script": forms.Textarea(attrs={"class": "form-control"}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        cluster = kwargs["initial"]["cluster"]
        self.fields["install_partition"].queryset = cluster.partitions


class RegistrySelectWidget(forms.Select):
    def create_option(self, name, value, label, selected, index, subindex=None, attrs=None):
        option = super().create_option(name, value, label, selected, index, subindex=subindex, attrs=attrs)
        # Look up the repo_mode from the mapping provided on the widget.
        if hasattr(self, 'repo_mode_mapping') and value:
            mode = self.repo_mode_mapping.get(str(value))
            if mode:
                option['attrs']['data-repo-mode'] = mode
        return option
class ContainerApplicationForm(forms.ModelForm):
    """Form to collect container installation details"""

    container_image_remote = forms.CharField(
        required=False,
        widget=forms.TextInput(attrs={"class": "form-control"}),
        label="Container Image",
    )

    registry = forms.ModelChoiceField(
        queryset=ContainerRegistry.objects.all(),
        required=True,
        widget=RegistrySelectWidget(attrs={"class": "form-control"}),
    )

    def __init__(self, *args, **kwargs):
        cluster = kwargs.pop("cluster", None)
        image_choices = kwargs.pop("image_choices", [])
        super().__init__(*args, **kwargs)

        if cluster:
            self.fields["cluster"].initial = cluster.pk
            self.fields["cluster"].widget = forms.HiddenInput()
            qs = cluster.container_registry_relations.all()
            self.fields["registry"].queryset = qs

            # Build mapping of registry ID to repo_mode.
            mapping = {str(reg.id): reg.repo_mode for reg in qs}
            self.fields["registry"].widget.repo_mode_mapping = mapping

        if image_choices:
            self.fields["container_image"].choices = image_choices
        else:
            self.fields["container_image"].choices = [("", "--- Select a registry first ---")]
        self.fields["container_image"].widget = forms.Select(attrs={"class": "form-control"})

        self.helper = FormHelper()
        self.helper.form_method = "post"
        self.helper.label_class = "font-weight-bold"
        self.helper.field_class = ""
        self.helper.layout = Layout(
            # Handling bootstrap layout in forms instead of the template
            Div(
                Div(Field("name", css_class="form-control mb-3"), css_class="col-md-6"),
                Div(Field("registry", css_class="form-control mb-3"), css_class="col-md-6"),
                css_class="row"
            ),
            Div(
                Div(Field("container_image", css_class="form-control"), css_class="image-dropdown mb-3"),
                Div(Field("container_image_remote", css_class="form-control"), css_class="image-remote mb-3", attrs={"style": "display:none;"}),
                css_class="mb-3",
                css_id="container-image-wrapper"
            ),
            Div(
                Div(Field("container_mounts", css_class="form-control mb-3"), css_class="col-md-4"),
                Div(Field("container_envvars", css_class="form-control mb-3"), css_class="col-md-4"),
                Div(Field("container_workdir", css_class="form-control mb-3"), css_class="col-md-4"),
                css_class="row"
            ),
            Div(
                Div(Field("container_writable"), css_class="col-md-3 mb-3"),
                Div(Field("container_use_entrypoint"), css_class="col-md-3 mb-3"),
                Div(Field("container_mount_home"), css_class="col-md-3 mb-3"),
                Div(Field("container_remap_root"), css_class="col-md-3 mb-3"),
                css_class="row"
            ),
            Div(
                Submit("submit", "Save", css_class="btn btn-primary"),
                css_class="mb-3"
            )
        )

    class Meta:
        model = ContainerApplication
        fields = (
            "cluster",
            "name",
            "registry",
            "container_image",
            "container_mounts",
            "container_envvars",
            "container_workdir",
            "container_writable",
            "container_use_entrypoint",
            "container_mount_home",
            "container_remap_root",
        )

    def clean(self):
        cleaned_data = super().clean()
        registry = cleaned_data.get("registry")
        image = cleaned_data.get("container_image")
        remote_image = self.data.get("container_image_remote", "").strip()

        # If the registry is a remote repository, use the remote image input.
        if registry and registry.repo_mode == "REMOTE_REPOSITORY":
            if not remote_image:
                self.add_error("container_image", "Please specify an image name for the remote repository.")
            else:
                cleaned_data["container_image"] = remote_image
        elif not image:
            self.add_error("container_image", "Please select a container image from the list.")
        return cleaned_data


class SpackApplicationForm(forms.ModelForm):
    """Custom form for application model"""

    class Meta:
        model = SpackApplication

        fields = (
            "cluster",
            "spack_name",
            "name",
            "version",
            "spack_spec",
            "description",
            "install_partition",
        )

        widgets = {
            "cluster": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "spack_name": forms.TextInput(attrs={"class": "form-control"}),
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "spack_spec": forms.TextInput(attrs={"class": "form-control"}),
            "version": forms.Select(attrs={"class": "form-control"}),
            "description": forms.Textarea(attrs={"class": "form-control"}),
            "install_partition": forms.Select(attrs={"class": "form-control"}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        cluster = kwargs["initial"]["cluster"]
        self.fields["install_partition"].queryset = cluster.partitions


class JobForm(forms.ModelForm):
    """Custom form for job model"""

    class Meta:
        model = Job

        fields = (
            "name",
            "application",
            "cluster",
            "partition",
            "number_of_nodes",
            "ranks_per_node",
            "threads_per_rank",
            "wall_clock_time_limit",
            "run_script",
            "cleanup_choice",
            "input_data",
            "result_data",
            "benchmark",
        )

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "application": forms.HiddenInput(),
            "cluster": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "partition": forms.Select(attrs={"class": "form-control"}),
            "number_of_nodes": forms.NumberInput(
                attrs={"class": "form-control", "min": "1"}
            ),
            "ranks_per_node": forms.NumberInput(
                attrs={"class": "form-control", "min": "1"}
            ),
            "threads_per_rank": forms.NumberInput(
                attrs={"class": "form-control", "min": "1", "readonly": True}
            ),
            "wall_clock_time_limit": forms.NumberInput(
                attrs={"class": "form-control", "min": "1"}
            ),
            "run_script": forms.URLInput(attrs={"class": "form-control"}),
            "input_data": forms.URLInput(attrs={"class": "form-control"}),
            "result_data": forms.URLInput(attrs={"class": "form-control"}),
            "cleanup_choice": forms.Select(attrs={"class": "form-control"}),
            "benchmark": forms.Select(attrs={"class": "form-control"}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        cluster = kwargs["initial"]["cluster"]
        self.fields["partition"].queryset = cluster.partitions


class ContainerJobForm(JobForm):
    """Custom form for containerized jobs"""

    class Meta(JobForm.Meta):
        model = ContainerJob
        fields = JobForm.Meta.fields + (
            "container_image_uri",
            "container_mounts",
            "container_envvars",
            "container_workdir",
            "container_writable",
            "container_use_entrypoint",
            "container_mount_home",
            "container_remap_root",
        )

        widgets = {
            "container_image_uri": forms.TextInput(attrs={"class": "form-control", "readonly": True}),
            "container_mounts": forms.TextInput(attrs={"class": "form-control", "readonly": False}),
            "container_envvars": forms.TextInput(attrs={"class": "form-control", "readonly": False}),
            "container_workdir": forms.TextInput(attrs={"class": "form-control", "readonly": False}),
            "container_writable": forms.CheckboxInput(attrs={"class": "form-check-input", "disabled": False}),
            "container_use_entrypoint": forms.CheckboxInput(attrs={"class": "form-check-input", "disabled": False}),
            "container_mount_home": forms.CheckboxInput(attrs={"class": "form-check-input", "disabled": False}),
            "container_remap_root": forms.CheckboxInput(attrs={"class": "form-check-input", "disabled": False}),
        }

    def __init__(self, *args, **kwargs):
        """Initialize form for containerized jobs"""
        application = kwargs.get("initial", {}).get("application")
        super().__init__(*args, **kwargs)

        # Modify labels for ranks_per_node field
        self.fields["ranks_per_node"].label = "CPUs per Node"
        self.fields["ranks_per_node"].help_text = "The number of CPUs per node"

        # Set and hide threads_per_rank field
        self.fields["threads_per_rank"].required = False
        self.fields["threads_per_rank"].initial = "N/A"
        self.fields["threads_per_rank"].widget = forms.HiddenInput()

        # Disable field in UI
        self.fields["ranks_per_node"].widget.attrs.pop("readonly", True)

        # Pre-fill fields from application form
        if application and hasattr(application, "containerapplication"):
            self.fields["container_image_uri"].initial = application.containerapplication.container_image
            self.fields["container_mounts"].initial = application.containerapplication.container_mounts
            self.fields["container_envvars"].initial = application.containerapplication.container_envvars
            self.fields["container_workdir"].initial = application.containerapplication.container_workdir
            self.fields["container_writable"].initial = application.containerapplication.container_writable
            self.fields["container_use_entrypoint"].initial = application.containerapplication.container_use_entrypoint
            self.fields["container_mount_home"].initial = application.containerapplication.container_mount_home
            self.fields["container_remap_root"].initial = application.containerapplication.container_remap_root


class BenchmarkForm(forms.ModelForm):
    """Custom form for benchmark model"""

    class Meta:
        model = Benchmark

        fields = ("name", "description")
        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "description": forms.Textarea(attrs={"class": "form-control"}),
        }


class VPCForm(forms.ModelForm):
    """Custom form for VPC model implementing option filtering"""

    subnets = forms.MultipleChoiceField(
        widget=forms.SelectMultiple(attrs={"class": "form-control"}),
        help_text="Available Subnets",
        required=False,
    )

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields["cloud_region"].widget.choices = [
            (x, x) for x in kwargs["initial"]["regions"]
        ]
        self.fields["subnets"].choices = kwargs["initial"].get(
            "available_subnets", []
        )

    class Meta:
        model = VirtualNetwork

        fields = ("name", "cloud_credential", "cloud_region")

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_region": forms.Select(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
        }


class VPCImportForm(forms.ModelForm):
    """Form for importing externally created VPCs"""

    subnets = forms.MultipleChoiceField(
        widget=forms.SelectMultiple(
            attrs={"class": "form-control", "disabled": True}
        )
    )
    vpc = forms.ChoiceField(
        widget=forms.Select(
            attrs={"class": "form-control", "onchange": "vpcSelected()"}
        )
    )

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields["subnets"].choices = kwargs["initial"]["subnets"]
        self.fields["vpc"].choices = kwargs["initial"]["vpc"]

    class Meta:
        model = VirtualNetwork

        fields = ("name", "cloud_credential")

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
        }


class VirtualSubnetForm(forms.ModelForm):
    """Form for VirtualSubnet model to be embedded"""

    class Meta:
        model = VirtualSubnet

        fields = ("name", "cidr", "cloud_region")
        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cidr": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_region": forms.Select(attrs={"class": "form-control"}),
            "private_google_access_enabled": forms.CheckboxInput(attrs={"class": "form-control"}),
        }


class FilesystemImportForm(forms.ModelForm):
    """Form to import externally managed filesystems"""

    share_name = forms.CharField(
        label="Export Name",
        help_text="Mount point from this filesystem (ie:  /shared)",
        widget=forms.TextInput(attrs={"class": "form-control"}),
        validators=[
            RegexValidator(
                regex="^/[-a-zA-Z0-9_]{1,63}",
                message=(
                    "Share name must start with a '/' and be no more than 64 "
                    "characters long, with no spaces"
                ),
            ),
        ],
    )

    class Meta:
        model = Filesystem
        fields = ("name", "vpc", "cloud_zone", "hostname_or_ip", "fstype")

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "vpc": forms.Select(attrs={"class": "form-control"}),
            "cloud_zone": forms.Select(attrs={"class": "form-control"}),
            "hostname_or_ip": forms.TextInput(attrs={"class": "form-control"}),
            "fstype": forms.Select(attrs={"class": "form-control"}),
        }

    def _get_creds(self, kwargs):
        # We do this, because on Create views, there isn't an instance, so we
        # set the creds via the 'initial' data field.  On Updates, there is
        # an object, so pull from there
        if "cloud_credential" in kwargs["initial"]:
            creds = kwargs["initial"]["cloud_credential"]
        else:
            creds = self.instance.cloud_credential
        return creds

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

        creds = self._get_creds(kwargs)
        self.fields["vpc"].queryset = VirtualNetwork.objects.filter(
            cloud_credential=creds
        ).filter(Q(cloud_state="i") | Q(cloud_state="m"))
        region_info = cloud_info.get_region_zone_info("GCP", creds.detail)
        self.fields["cloud_zone"].widget.choices = [
            (r, [(z, z) for z in rz]) for r, rz in region_info.items()
        ]


# per-tier capacity rules, keyed by the model's stored tier code
# (see GCPFilestoreFilesystem.FILESTORE_TIER). "step" is the required
# capacity increment in GiB (None = any 1 GiB step allowed). Values are from
# Google's Filestore service-tiers documentation. Without this the form only
# enforced a single hard-coded HTML min=2660, which both blocked the real
# ENTERPRISE minimum (1024) and permitted values GCP rejects at apply time
# (e.g. 2660 is not a multiple of 256 GiB).
# Per-tier capacity limits, in GiB, from Filestore's own service tiers doc.
# HIGH_SCALE_SSD is the legacy API name for the *high-scale* zonal band, not
# zonal in general: GCP enforces a 10 TiB floor in 2.5 TiB steps for it and
# rejects anything smaller at apply time with "capacity must be at least
# 10240 GiB". (Zonal's 1 TiB / 256 GiB band is the separate ZONAL tier, which
# OFE does not offer.) This previously read min 1024 / step 256, so OFE
# accepted capacities the API always refused.
FILESTORE_CAPACITY_RULES = {
    "bh": {"min": 1024, "max": 65433, "step": None},     # BASIC_HDD
    "bs": {"min": 2560, "max": 65433, "step": None},     # BASIC_SSD
    "hs": {"min": 10240, "max": 102400, "step": 2560},   # HIGH_SCALE_SSD
    "en": {"min": 1024, "max": 10240, "step": 256},      # ENTERPRISE
}


# Filestore tiers that support CMEK, as OFE tier codes: "hs" is
# HIGH_SCALE_SSD and "en" is ENTERPRISE. The filestore module enforces this
# with a Terraform precondition (modules/file-system/filestore/main.tf), but
# only at apply time -- by then the resource is saved and the operator has sat
# through an init and a plan to be told the tier and the key are incompatible.
# The basic tiers genuinely cannot do CMEK, so this is checked on the form.
CMEK_CAPABLE_FILESTORE_TIERS = ("hs", "en")


def filestore_cmek_tier_error(tier, cmek_enabled):
    """Why this tier cannot carry a customer-managed key, or None."""
    if not cmek_enabled or not tier:
        return None
    if tier in CMEK_CAPABLE_FILESTORE_TIERS:
        return None
    label = dict(GCPFilestoreFilesystem.FILESTORE_TIER).get(tier, tier)
    allowed = ", ".join(
        dict(GCPFilestoreFilesystem.FILESTORE_TIER)[t]
        for t in CMEK_CAPABLE_FILESTORE_TIERS)
    return (f"{label} Filestore does not support customer-managed encryption "
            f"keys. Choose {allowed}, or turn encryption off.")


def filestore_capacity_error(tier, capacity):
    """Return a human-readable error string if `capacity` (GiB) is invalid
    for the Filestore `tier` (a FILESTORE_TIER code), else None."""
    rules = FILESTORE_CAPACITY_RULES.get(tier)
    if capacity is None or rules is None:
        return None
    label = dict(GCPFilestoreFilesystem.FILESTORE_TIER).get(tier, tier)
    if capacity < rules["min"]:
        return f"{label} Filestore requires at least {rules['min']} GiB."
    if capacity > rules["max"]:
        return f"{label} Filestore allows at most {rules['max']} GiB."
    if rules["step"] and capacity % rules["step"] != 0:
        return (f"{label} Filestore capacity must be a multiple of "
                f"{rules['step']} GiB.")
    return None


class FilestoreForm(CmekPanelMixin, forms.ModelForm):
    """Custom form for GCP Filestoremodel implementing option filtering"""

    share_name = forms.CharField(
        label="Export Name",
        max_length=16,
        validators=[
            RegexValidator(
                regex="^/[-a-zA-Z0-9_]{1,16}",
                message=(
                    "Share name must start with a '/' and be no more than 16 "
                    "characters long, with no spaces"
                ),
            ),
        ],
    )

    def _get_creds(self, kwargs):
        # We do this, because on Create views, there isn't an instance, so we
        # set the creds via the 'initial' data field.  On Updates, there is
        # an object, so pull from there
        if "cloud_credential" in kwargs["initial"]:
            creds = kwargs["initial"]["cloud_credential"]
        else:
            creds = self.instance.cloud_credential
        return creds

    def __init__(self, *args, **kwargs):
        zone_choices = None
        if "zone_choices" in kwargs:
            zone_choices = kwargs.pop("zone_choices")

        super().__init__(*args, **kwargs)

        creds = self._get_creds(kwargs)
        self.fields["vpc"].queryset = VirtualNetwork.objects.filter(
            cloud_credential=creds
        ).filter(Q(cloud_state="i") | Q(cloud_state="m"))
        region_info = cloud_info.get_region_zone_info("GCP", creds.detail)
        self.fields["cloud_zone"].widget.choices = [
            (r, [(z, z) for z in rz]) for r, rz in region_info.items()
        ]

        if zone_choices:
            # We set this on the widget, because we will be changing the
            # widget's field in the template via javascript
            self.fields["cloud_zone"].widget.choices = zone_choices

        self.setup_cmek_panel(
            location=getattr(self.instance, "cloud_zone", None))

        # Deployed, or deployed once with its key: the key's location
        # follows the zone, so neither can change any more.
        if "n" not in self.instance.cloud_state \
                or getattr(self.instance, "cmek_applied", False) is True:
            # Need to disable certain widgets
            self.fields["vpc"].disabled = True
            self.fields["cloud_zone"].disabled = True
            self.fields["share_name"].disabled = True
            self.fields["performance_tier"].disabled = True


    def clean(self):
        # reject capacities GCP will refuse at apply time, per tier.
        cleaned = super().clean()
        error = filestore_capacity_error(
            cleaned.get("performance_tier"), cleaned.get("capacity"))
        if error:
            self.add_error("capacity", error)
        # after the panel, so a deployment that forces encryption on is judged
        # on the encryption it will actually get rather than the box submitted
        cleaned = self.clean_cmek_panel(cleaned)
        error = filestore_cmek_tier_error(
            cleaned.get("performance_tier"), cleaned.get("cmek_enabled"))
        if error:
            self.add_error("performance_tier", error)
        return cleaned

    class Meta:
        model = GCPFilestoreFilesystem

        fields = (
            "name",
            "vpc",
            "cloud_zone",
            "cloud_credential",
            "capacity",
            "performance_tier",
        ) + CMEK_PANEL_FIELDS

        widgets = {
            **CMEK_PANEL_WIDGETS,
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "vpc": forms.Select(attrs={"class": "form-control"}),
            # 1024 is the smallest valid tier minimum (ENTERPRISE /
            # BASIC_HDD / HIGH_SCALE_SSD); precise per-tier limits and the
            # 256 GiB increment are enforced in clean().
            "capacity": forms.NumberInput(attrs={"min": 1024, "default": 1024}),
            "share_name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_zone": forms.Select(attrs={"class": "form-control"}),
        }


class WorkbenchMountPointForm(forms.ModelForm):
    """Form for Cluster Mount points"""

    class Meta:
        model = WorkbenchMountPoint
        fields = ("workbench", "export", "mount_order", "mount_path")

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        for field in self.fields:
            self.fields[field].widget.attrs.update({"class": "form-control"})

class StartupScriptForm(forms.ModelForm):
    """
    Custom form for handling data input and validation for the StartupScript model.

    This form class extends the `forms.ModelForm` class and is designed to work with the
    `StartupScript` model, which represents a script executed during
    the startup phase of a node.

    Form Fields:
        - "name": A text input field for providing a name for the startup script.
        - "description": A textarea input field for adding a description of the script.
        - "type": A select input field for choosing the type or category of the script.
        - "content": A file input field for uploading the content of the startup script.

    Form Validation:
        The form automatically validates the input data based on the model field definitions
        and any additional constraints defined in the model.
    """

    class Meta:
        model = StartupScript

        fields = (
            "name", 
            "description", 
            "type", 
            "content",
        )

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "description": forms.Textarea(attrs={"class": "form-control"}),
            "type": forms.Select(attrs={"class": "form-control"}),
            "content": forms.ClearableFileInput(attrs={"class": "form-control"}),
        }

class ImageForm(CmekPanelMixin, forms.ModelForm):
    """Custom form for Image model"""

    class Meta:
        model = Image

        fields = (
            "cloud_credential",
            "name", 
            "family",
            "cloud_region",
            "cloud_zone",
            "source_image_project",
            "source_image_family",
            "startup_script",
            "enable_os_login",
            "block_project_ssh_keys",
            "authorised_users",
        ) + CMEK_PANEL_FIELDS

        widgets = {
            **CMEK_PANEL_WIDGETS,
            "cloud_credential": forms.Select(attrs={"class": "form-control"}),
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "family": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_region": forms.Select(attrs={"class": "form-control"}),
            "cloud_zone": forms.Select(attrs={"class": "form-control"}),
            "source_image_project": forms.TextInput(attrs={"class": "form-control"}),
            "source_image_family": forms.TextInput(attrs={"class": "form-control"}),
            "startup_script": forms.SelectMultiple(attrs={"class": "form-control"}),
            "enable_os_login": forms.RadioSelect(),
            "block_project_ssh_keys": forms.RadioSelect(),
            "authorised_users": forms.SelectMultiple(attrs={"class": "form-control"}),
        }

    def __init__(self, *args, **kwargs):
        user = kwargs.pop("user", None)
        super().__init__(*args, **kwargs)
        self.fields["startup_script"].queryset = self.get_startup_scripts(user)
        self.setup_cmek_panel(
            location=(getattr(self.instance, "cloud_zone", None)
                      or getattr(self.instance, "cloud_region", None)))

    def clean(self):
        return self.clean_cmek_panel(super().clean())

    def get_startup_scripts(self, user):
        # Retrieve startup scripts owned by the user
        owned_scripts = StartupScript.objects.filter(owner=user)

        # Retrieve startup scripts authorized for the user
        authorized_scripts = StartupScript.objects.filter(authorised_users=user)

        # Combine the owned and authorized scripts
        startup_scripts = owned_scripts | authorized_scripts

        return startup_scripts

class ImageImportForm(forms.ModelForm):
    """Custom form for Image model when importing an image"""

    class Meta:
        model = Image

        fields = (
            "cloud_credential",
            #"name", 
            #"family",
            "startup_script",
            "enable_os_login",
            "block_project_ssh_keys",
            "authorised_users"
        )

        widgets = {
            "cloud_credential": forms.Select(attrs={"class": "form-control"}),
            #"name": forms.HiddenInput,#TextInput(attrs={"class": "form-control"}),
            #"family": forms.HiddenInput,#(attrs={"class": "form-control"}),
            "startup_script": forms.SelectMultiple(attrs={"class": "form-control"}),
            "enable_os_login": forms.RadioSelect(),
            "block_project_ssh_keys": forms.RadioSelect(),
            "authorised_users": forms.SelectMultiple(attrs={"class": "form-control"}),
        }

        help_texts = {
            #"name": "Enter the existing image name",
            #"family": "Enter the family of the existing image"
        }

    def __init__(self, *args, **kwargs):
        user = kwargs.pop("user", None)
        super().__init__(*args, **kwargs)
        self.fields["startup_script"].queryset = self.get_startup_scripts(user)

    def get_startup_scripts(self, user):
        # Retrieve startup scripts owned by the user
        owned_scripts = StartupScript.objects.filter(owner=user)

        # Retrieve startup scripts authorized for the user
        authorized_scripts = StartupScript.objects.filter(authorised_users=user)

        # Combine the owned and authorized scripts
        startup_scripts = owned_scripts | authorized_scripts

        return startup_scripts


class ContainerRegistryForm(forms.ModelForm):
    class Meta:
        model = ContainerRegistry
        fields = [
            'format',
            'repo_mode',
            'repo_mirror_url',
            'repo_username',
            'repo_password',
            'use_public_repository',
            'use_upstream_credentials',
        ]
        widgets = {
            'repo_password': forms.PasswordInput(attrs={'class': 'form-control'}),
            'format': forms.Select(attrs={'class': 'form-control'}),
            'repo_mode': forms.Select(attrs={'class': 'form-control'}),
            'use_public_repository': forms.CheckboxInput(),
            'repo_mirror_url': forms.URLInput(attrs={'class': 'form-control'}),
            'use_upstream_credentials': forms.CheckboxInput(),
            'repo_username': forms.TextInput(attrs={'class': 'form-control'}),
        }

    def clean_use_public_repository(self):
        """Ensure checkbox is always returned as True/False"""
        return self.cleaned_data.get("use_public_repository", False)

    def clean_use_upstream_credentials(self):
        """Ensure checkbox is always returned as True/False"""
        return self.cleaned_data.get("use_upstream_credentials", False)

    def clean(self):
        cleaned_data = super().clean()
        repo_mode = cleaned_data.get("repo_mode")
        use_public_repository = cleaned_data.get("use_public_repository")

        if repo_mode == "REMOTE_REPOSITORY":
            if not use_public_repository and not cleaned_data.get("repo_mirror_url"):
                self.add_error("repo_mirror_url", "Mirror URL is required for remote repositories unless using a public repository.")
                raise ValidationError(
                        "repo_mirror_url is required for remote repositories unless using a public repository."
                    )

            if cleaned_data.get("use_upstream_credentials") and not cleaned_data.get("repo_username"):
                self.add_error("repo_username", "Username is required when using upstream credentials.")
                raise ValidationError(
                        "repo_username is required when using upstream credentials."
                    )

            # Allow repo_password to exist in cleaned_data even if not saved to the model
            if cleaned_data.get("use_upstream_credentials") and not cleaned_data.get("repo_password"):
                self.add_error("repo_password", "Password is required when using upstream credentials.")
                raise ValidationError(
                        "repo_password is required when using upstream credentials."
                    )

            # If use_public_repository is set, enforce defaults
            if use_public_repository:
                cleaned_data["repo_mirror_url"] = None  # Ensure no mirror URL is stored

        elif repo_mode == "STANDARD_REPOSITORY":
            cleaned_data["repo_mirror_url"] = None
            cleaned_data["repo_username"] = None
            cleaned_data["repo_password"] = None

        return cleaned_data


class PullContainerForm(forms.Form):
    source_uri = forms.CharField(
        required=True,
        label="Remote Container URL",
        widget=forms.TextInput(attrs={
            "class": "form-control",
            "placeholder": "Enter remote container URL (e.g., docker.io/pytorch/pytorch:latest)"
        })
    )
    container_tag = forms.CharField(
        required=False,
        label="Container Tag",
        widget=forms.TextInput(attrs={
            "class": "form-control",
            "placeholder": "Enter tag for built container (e.g., latest)"
        })
    )
    repo_username = forms.CharField(
        required=False,
        label="Repo Username",
        widget=forms.TextInput(attrs={
            "class": "form-control",
            "placeholder": "Repo username (if required)"
        })
    )
    repo_password = forms.CharField(
        required=False,
        label="Repo Password",
        widget=forms.PasswordInput(attrs={
            "class": "form-control",
            "placeholder": "Repo password (if required)"
        })
    )

class CmekSettingsForm(forms.Form):
    """The deployment-wide encryption defaults.

    Three settings, not the twelve the previous implementation had: everything
    else is now either a per-resource choice or a kms-key default, and a control
    that duplicates a module default is a control that can disagree with it.
    """

    enforcement = forms.ChoiceField(
        choices=[
            (cmek.ENFORCEMENT_REQUIRED,
             "Required — every new resource must use a customer-managed key"),
            (cmek.ENFORCEMENT_PREFERRED,
             "Preferred — offered on each resource, off unless the user "
             "turns it on"),
            (cmek.ENFORCEMENT_DISABLED,
             "Disabled — the encryption panel is not shown at all"),
        ],
        widget=forms.Select(attrs={"class": "form-control"}),
        help_text=(
            "Whether a resource may be created without a customer-managed key. "
            "Applies to resources created from now on."
        ),
    )
    key_project_id = forms.CharField(
        required=False,
        widget=forms.TextInput(attrs={"class": "form-control"}),
        help_text=(
            "Project the keys OFE creates are placed in. Leave empty to use "
            "each resource's own project."
        ),
    )
    key_ring_name = forms.CharField(
        required=False,
        widget=forms.TextInput(attrs={"class": "form-control"}),
        help_text=(
            "Cloud KMS key ring the keys OFE creates are placed in. Permanent: "
            "a key ring can never be renamed, reused or deleted."
        ),
    )

    def clean(self):
        cleaned = super().clean()
        try:
            self.settings = cmek.CmekSettings.from_config({"cmek": {
                "enforcement": cleaned.get("enforcement"),
                "key_project_id": (cleaned.get("key_project_id") or "").strip(),
                "key_ring_name": (cleaned.get("key_ring_name") or "").strip(),
            }})
        except cmek.CmekConfigError as err:
            # Validated by the same code that reads the file, so the form cannot
            # accept something the engine would later reject.
            field = next((name for name in ("key_ring_name", "key_project_id")
                          if f"cmek.{name}" in str(err)), None)
            self.add_error(field, str(err))
        return cleaned
