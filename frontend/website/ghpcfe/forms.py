# Copyright 2022 Google LLC
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

from django import forms
from django.forms import ValidationError, modelformset_factory, inlineformset_factory
from django.db.models import Q
from django.contrib.auth.forms import UserCreationForm, UserChangeForm
from django.utils.safestring import mark_safe
from .cluster_manager import validate_credential, cloud_info
from .models import *
import json

import logging
logger = logging.getLogger(__name__)

class UserCreationForm(UserCreationForm):
    """ Custom UserCreationForm """

    class Meta(UserCreationForm):
        model = User
        fields = ('email',)


class UserUpdateForm(UserChangeForm):
    """ Custom form for updating user account """

    password = None

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields['ssh_key'].label = "SSH key"

    class Meta:
        model = User
        fields = ('email','ssh_key',)
        widgets = {
            'email': forms.TextInput(attrs={'class': 'form-control'}),
            'ssh_key': forms.Textarea(attrs={'class': 'form-control'}),
        }


class CredentialForm(forms.ModelForm):
    """ Custom form for Credential model implementing additional validation """

    class Meta:
        model = Credential

        fields = ('name', 'detail')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'detail': forms.Textarea(attrs={'class': 'form-control'}),
        }

    def clean(self):
        super().clean()

        # validate the credential details with cloud platform
        detail = self.cleaned_data['detail']
        validated = validate_credential.validate_credential("GCP", detail)
        if not validated:
            raise ValidationError('Credential cannot be validated.')


class ClusterForm(forms.ModelForm):
    """ Custom form for Cluster model implementing option filtering """

    def clean(self):
        super().clean()
        # TODO - validate 'region' and 'zone'

    def _get_creds(self, kwargs):
        # We do this, because on Create views, there isn't an instance, so we
        # set the creds via the 'initial' data field.  On Updates, there is 
        # an object, so pull from there
        if 'cloud_credential' in kwargs['initial']:
            creds = kwargs['initial']['cloud_credential']
        else:
            creds = self.instance.cloud_credential
        return creds

    def __init__(self, *args, **kwargs):

        zone_choices = None
        if 'zone_choices' in kwargs:
            zone_choices = kwargs.pop('zone_choices')

        super().__init__(*args, **kwargs)
        credential = self._get_creds(kwargs)

        self.fields['subnet'].queryset = VirtualSubnet.objects.filter(cloud_credential=credential).filter(Q(cloud_state="i")|Q(cloud_state="m"))
        if zone_choices:
            # We set this on the widget, because we will be changing the
            # widget's field in the template via javascript
            self.fields['cloud_zone'].widget.choices = zone_choices

        if 'n' not in self.instance.cloud_state:
            # Need to disable certain widgets
            self.fields['subnet'].disabled = True
            self.fields['cloud_zone'].disabled = True
            self.fields['spackdir'].disabled = True

    class Meta:
        model = Cluster

        fields = ('name', 'subnet', 'cloud_zone', 'cloud_credential', 'authorised_users', 'spackdir', 'num_login_nodes')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_credential': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
            'subnet': forms.Select(attrs={'class': 'form-control'}), 
            'cloud_zone': forms.Select(attrs={'class': 'form-control'}),
        }




class ClusterMountPointForm(forms.ModelForm):
    """ Form for Cluster Mount points """
    class Meta:
        model = MountPoint
        fields = ('export', 'mount_order', 'mount_path', 'mount_options')

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        for field in self.fields:
            self.fields[field].widget.attrs.update({'class': 'form-control'})

class ClusterPartitionForm(forms.ModelForm):
    """ Form for Cluster Paritions """
    class Meta:
        model = ClusterPartition
        fields = ('name', 'machine_type', 'image', 'max_node_count', 'enable_placement', 'enable_hyperthreads', 'enable_node_reuse')

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        for field in self.fields:
            self.fields[field].widget.attrs.update({'class': 'form-control'})
            if self.fields[field].help_text:
                self.fields[field].widget.attrs.update({'title': self.fields[field].help_text})

    def clean(self):
        cleaned_data = super().clean()
        if cleaned_data['enable_placement'] and cleaned_data['machine_type'].family.name not in ['c2', 'c2d']:
            raise ValidationError('Placement Groups are only valid for C2 and C2D instance types')
        return cleaned_data


class WorkbenchForm(forms.ModelForm):
    """ Custom form for Workbench model implementing option filtering """

    def clean(self):
        cleaned_data = super().clean()
        subnet = cleaned_data.get("subnet")

        # if subnet.cloud_zone not in self.workbench_zones:
        #     validation_error_message = "Network " + subnet.vpc.cloud_id + " has an invalid region & zone for Vertex AI Workbenches: " + subnet.cloud_zone + ". Please see <a href=\"https://cloud.google.com/vertex-ai/docs/general/locations#vertex-ai-workbench-locations\" target=\"_blank\"> Workbench Documentation</a> for more infromation on region availability, try"
        #     raise forms.ValidationError(mark_safe(validation_error_message))

        #validate user has an email address that we can pass to GCP 
        user = cleaned_data.get("trusted_users")
        if not user.email:
            raise forms.ValidationError("User has no email address")

        #check user is associated with a social login account
        try:
            if user.socialaccount_set.first().uid:
                pass
        except:
            raise forms.ValidationError("User not associated with a required Social ID ")

    def __init__(self, user, *args, **kwargs):
        has_creds = 'cloud_credential' in kwargs
        if has_creds:
            credential = kwargs.pop('cloud_credential')
            kwargs['initial']['cloud_credential'] = credential
        super().__init__(*args, **kwargs)
        if not has_creds:
            credential = self.instance.cloud_credential
        zone_choices = None
        if 'zone_choices' in kwargs:
            zone_choices = kwargs.pop('zone_choices')

        self.fields['subnet'].queryset = VirtualSubnet.objects.filter(cloud_credential=credential).filter(Q(cloud_state="i")|Q(cloud_state="m"))
        if zone_choices:
            # We set this on the widget, because we will be changing the
            # widget's field in the template via javascript
            self.fields['cloud_zone'].widget.choices = zone_choices

        if 'n' not in self.instance.cloud_state:
            # Need to disable certain widgets
            self.fields['subnet'].disabled = True
            self.fields['cloud_zone'].disabled = True

        self.workbench_zones = cloud_info.get_gcp_workbench_region_zone_info(credential.detail)
        if 'n' not in self.instance.cloud_state:
            #Need to disable certain widgets
            self.fields['subnet'].disabled = True
        #Pull instance types from cloud_info
        instance_types = cloud_info.get_machine_types("GCP", credential.detail, "europe-west4", "europe-west4-a")
        #set variables for retrieving instance types for dropdown menu
        choices_list = []
        instance_list = []
        category = ""
        #Populate dropdown menu with preset instance_types from WorkbenchPresets
        for preset in WorkbenchPreset.objects.order_by('category').values():
            #if category variable has changed from last loop then append instances to overall choices list as tuple and clear instance_list
            if category != preset['category']:
                if category:
                    choices_list.append((category,tuple(instance_list)))
                instance_list = []
            #set category to current value and append preset to dropdown menu list
            category = preset['category']
            instance_list.append((preset['machine_type'],preset['name']))
        #append final preset instance type from loop
        choices_list.append((category,tuple(instance_list)))
        category = ""
        if Role.CLUSTERADMIN in [x.id for x in user.roles.all()]:
            for instance_type in sorted(instance_types):
                #if family variable has changed from last loop then append instances to overall choices list as tuple and clear instance_list
                if category != instance_types[instance_type]['family']:
                    if category:
                        choices_list.append((category,tuple(instance_list)))
                    instance_list = []
                #save family of current instance
                category = instance_types[instance_type]['family']
                #create instance string for displaying to user
                instance_string = instance_types[instance_type]['name'] + " - " + str(instance_types[instance_type]['vCPU']) + "x " + instance_types[instance_type]['arch'] + " vCPUs with " + str(instance_types[instance_type]['memory']) + " Memory"
                #append tuple to instance list
                instance_list.append((instance_types[instance_type]['name'],instance_string))
            #append final preset instance type from loop
            choices_list.append((category,tuple(instance_list)))
        self.fields['machine_type'].widget.choices = choices_list

    class Meta:
        model = Workbench

        fields = ('name', 'subnet', 'cloud_zone', 'cloud_credential', 'trusted_users', 'machine_type', 'boot_disk_type', 'boot_disk_capacity', 'image_family')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_credential': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
            'subnet': forms.Select(attrs={'class': 'form-control'}),
            'machine_type': forms.Select(attrs={'class': 'form-control'}),
            'cloud_zone': forms.Select(attrs={'class': 'form-control'}),
            'trusted_users': forms.Select(attrs={'class': 'form-control'}),
        }

class ApplicationEditForm(forms.ModelForm):
    """ Custom form for application model """

    class Meta:
        model = Application

        fields = ('name', 'description')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'description': forms.Textarea(attrs={'class': 'form-control'}),
            'load_command': forms.TextInput(attrs={'class': 'form-control'}),
        }

    def clean(self):
        super().clean()

        # Validate selected instance types with each other, and 'install instance'
        common_arch = cloud_info.get_common_arch([x.cpu_arch for x in self.cleaned_data['instance_type']])
        if not common_arch:
            raise ValidationError('Selected Instance Types have incompatible architectures.')

        if self.instance.installed_architecture:
            install_arch = self.instance.installed_architecture
            for t in self.cleaned_data['instance_type']:
                common = cloud_info.get_common_arch([install_arch, t.cpu_arch])
                if not common or common != install_arch:
                    raise ValidationError(f'Application installed for {install_arch} not valid on {t.cpu_arch} ({t.name})')


class ApplicationForm(forms.ModelForm):
    """ Custom form for application model """

    class Meta:
        model = Application

        fields = ('install_loc', 'name', 'version', 'description', 'load_command')

        widgets = {
            'install_loc': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'version': forms.TextInput(attrs={'class': 'form-control'}),
            'description': forms.Textarea(attrs={'class': 'form-control'}),
            'load_command': forms.TextInput(attrs={'class': 'form-control'}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    def clean(self):
        super().clean()

        # Validate selected instance types with each other, and 'install instance'
        common_arch = cloud_info.get_common_arch([x.cpu_arch for x in self.cleaned_data['instance_type']])
        if not common_arch:
            raise ValidationError('Instance Types have incompatible architectures.')

        if self.cleaned_data['installed_architecture']:
            tgt = self.cleaned_data['installed_architecture']
            if cloud_info.get_common_arch([tgt, common_arch]) != tgt:
                raise ValidationError('Install instance architecture does not match run instance types')


class SpackApplicationForm(forms.ModelForm):
    """ Custom form for application model """

    class Meta:
        model = Application

        fields = ('cluster', 'spack_name', 'name', 'version', 'spack_spec', 'description', 'install_partition')

        widgets = {
            'cluster': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
            'spack_name': forms.TextInput(attrs={'class': 'form-control'}),
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'spack_spec': forms.TextInput(attrs={'class': 'form-control'}),
            'version': forms.Select(attrs={'class': 'form-control'}),
            'description': forms.Textarea(attrs={'class': 'form-control'}),
            'install_partition': forms.Select(attrs={'class': 'form-control'}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        cluster = kwargs['initial']['cluster']
        self.fields['install_partition'].queryset = cluster.partitions



class JobForm(forms.ModelForm):
    """ Custom form for job model """

    class Meta:
        model = Job

        fields = ('name', 'application', 'cluster', 'partition', 'number_of_nodes', 'ranks_per_node', \
            'threads_per_rank', 'wall_clock_time_limit', 'run_script', 'cleanup_choice', \
            'input_data', 'result_data', 'benchmark')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'application': forms.HiddenInput(),
            'cluster': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
            'partition': forms.Select(attrs={'class': 'form-control'}),
            'number_of_nodes': forms.NumberInput(attrs={'class': 'form-control', 'min': '1'}),
            'ranks_per_node': forms.NumberInput(attrs={'class': 'form-control', 'min': '1'}),
            'threads_per_rank': forms.NumberInput(attrs={'class': 'form-control', 'min': '1', 'readonly': True}),
            'wall_clock_time_limit': forms.NumberInput(attrs={'class': 'form-control', 'min': '0'}),
            'run_script': forms.URLInput(attrs={'class': 'form-control'}),
            'input_data': forms.URLInput(attrs={'class': 'form-control'}),
            'result_data': forms.URLInput(attrs={'class': 'form-control'}),
            'cleanup_choice': forms.Select(attrs={'class': 'form-control'}),
            'benchmark': forms.Select(attrs={'class': 'form-control'}),
        }

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        cluster = kwargs['initial']['cluster']
        self.fields['partition'].queryset = cluster.partitions


class BenchmarkForm(forms.ModelForm):
    """ Custom form for benchmark model """

    class Meta:
        model = Benchmark

        fields = ('name', 'description')
        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'description': forms.Textarea(attrs={'class': 'form-control'}),
        }


class VPCForm(forms.ModelForm):
    """ Custom form for VPC model implementing option filtering """
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields['cloud_region'].widget.choices = [(x, x) for x in kwargs['initial']['regions']]

    class Meta:
        model = VirtualNetwork

        fields = ('name', 'cloud_credential', 'cloud_region')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_region': forms.Select(attrs={'class': 'form-control'}),
            'cloud_credential': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
        }


class VPCImportForm(forms.ModelForm):

    subnets = forms.MultipleChoiceField(widget=forms.SelectMultiple(attrs={'class': 'form-control', 'disabled': True}))
    vpc = forms.ChoiceField(widget=forms.Select(attrs={'class': 'form-control', 'onchange': 'vpcSelected()'}))

    def clean(self):
        super().clean()

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields['subnets'].choices = kwargs['initial']['subnets']
        self.fields['vpc'].choices = kwargs['initial']['vpc']

    class Meta:
        model = VirtualNetwork

        fields = ('name', 'cloud_credential')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_credential': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
        }


class VirtualSubnetForm(forms.ModelForm):
    """ Form for VirtualSubnet model to be embedded """

    class Meta:
        model = VirtualSubnet

        fields = ('name', 'cidr', 'cloud_region')
        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'cidr': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_region': forms.Select(attrs={'class': 'form-control'}),
        }


class FilestoreForm(forms.ModelForm):
    """ Custom form for GCP Filestoremodel implementing option filtering """

    share_name = forms.CharField(label='Export Name',
                    max_length=16,
                    validators=[
                        RegexValidator(
                            regex='^/[-a-zA-Z0-9_]{1,16}',
                            message="Share must start with a '/' and be no more than 16 characters long, with no spaces"),
                        ]
                    ) 

    def _get_creds(self, kwargs):
        # We do this, because on Create views, there isn't an instance, so we
        # set the creds via the 'initial' data field.  On Updates, there is 
        # an object, so pull from there
        if 'cloud_credential' in kwargs['initial']:
            creds = kwargs['initial']['cloud_credential']
        else:
            creds = self.instance.cloud_credential
        return creds

    def __init__(self, *args, **kwargs):

        zone_choices = None
        if 'zone_choices' in kwargs:
            zone_choices = kwargs.pop('zone_choices')

        super().__init__(*args, **kwargs)

        creds = self._get_creds(kwargs)

        self.fields['subnet'].queryset = VirtualSubnet.objects.filter(cloud_credential=creds).filter(Q(cloud_state="i")|Q(cloud_state="m"))
        if zone_choices:
            # We set this on the widget, because we will be changing the
            # widget's field in the template via javascript
            self.fields['cloud_zone'].widget.choices = zone_choices

        if 'n' not in self.instance.cloud_state:
            # Need to disable certain widgets
            self.fields['subnet'].disabled = True
            self.fields['cloud_zone'].disabled = True
            self.fields['share_name'].disabled = True
            self.fields['performance_tier'].disabled = True

    class Meta:
        model = GCPFilestoreFilesystem

        fields = ('name', 'subnet', 'cloud_zone', 'cloud_credential', 'capacity', 'performance_tier')

        widgets = {
            'name': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_credential': forms.Select(attrs={'class': 'form-control', 'disabled': True}),
            'subnet': forms.Select(attrs={'class': 'form-control'}), 
            'capacity': forms.NumberInput(attrs={'min': 1024, 'default':1024}),
            'share_name': forms.TextInput(attrs={'class': 'form-control'}),
            'cloud_zone': forms.Select(attrs={'class': 'form-control'}),
        }

