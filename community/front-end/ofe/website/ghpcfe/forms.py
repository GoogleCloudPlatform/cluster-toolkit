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
import logging

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


class ClusterForm(forms.ModelForm):
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

            # Loop through the fields and set the 'readonly' attribute
            for field_name in fields_to_make_readonly:
                self.fields[field_name].widget = forms.TextInput(attrs={'class': 'form-control'})
                self.fields[field_name].widget.attrs['readonly'] = True

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
        )

        widgets = {
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
                        f"You must use a 'specific' reservation type."
                        f"Please read the following URL for more information about setting up reservations:"
                        f"https://cloud.google.com/compute/docs/instances/reservations-overview#how-reservations-work"
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


class FilestoreForm(forms.ModelForm):
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

        if "n" not in self.instance.cloud_state:
            # Need to disable certain widgets
            self.fields["vpc"].disabled = True
            self.fields["cloud_zone"].disabled = True
            self.fields["share_name"].disabled = True
            self.fields["performance_tier"].disabled = True

    class Meta:
        model = GCPFilestoreFilesystem

        fields = (
            "name",
            "vpc",
            "cloud_zone",
            "cloud_credential",
            "capacity",
            "performance_tier",
        )

        widgets = {
            "name": forms.TextInput(attrs={"class": "form-control"}),
            "cloud_credential": forms.Select(
                attrs={"class": "form-control", "disabled": True}
            ),
            "vpc": forms.Select(attrs={"class": "form-control"}),
            "capacity": forms.NumberInput(attrs={"min": 2660, "default": 2660}),
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

class ImageForm(forms.ModelForm):
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
            "authorised_users"
        )

        widgets = {
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
                        f"repo_mirror_url is required for remote repositories unless using a public repository."
                    )

            if cleaned_data.get("use_upstream_credentials") and not cleaned_data.get("repo_username"):
                self.add_error("repo_username", "Username is required when using upstream credentials.")
                raise ValidationError(
                        f"repo_username is required when using upstream credentials."
                    )

            # Allow repo_password to exist in cleaned_data even if not saved to the model
            if cleaned_data.get("use_upstream_credentials") and not cleaned_data.get("repo_password"):
                self.add_error("repo_password", "Password is required when using upstream credentials.")
                raise ValidationError(
                        f"repo_password is required when using upstream credentials."
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
