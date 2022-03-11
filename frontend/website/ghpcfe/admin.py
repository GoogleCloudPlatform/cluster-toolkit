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

""" admin.py """

from django.contrib import admin
from django.contrib.auth.admin import UserAdmin
from .models import *
from .forms import UserCreationForm, UserChangeForm

class UserAdmin(UserAdmin):
    """ Custom UserAdmin """
    add_form = UserCreationForm
    form = UserChangeForm
    model = User
    list_display = ('username', 'first_name', 'last_name', 'email', 'is_staff', 'is_active',)
    list_filter = ('email', 'is_staff', 'is_active',)
    ordering = ('username',)
    readonly_fields = ('last_login', 'date_joined', 'unix_id',)

    fieldsets = (
        (None, {'fields': ('username', 'password',)}),
        ('Personal info', {'fields': ('first_name', 'last_name', 'email',)}),
        ('Permissions', {'fields': ('is_active', 'is_staff', 'is_superuser', 'roles',)}),
        ('Important dates', {'fields': ('last_login', 'date_joined')}),
        ('Identity', {'fields': ('unix_id', 'ssh_key',)}),
    )



class MountPointInline(admin.TabularInline):
    """ To enable inline editing of instance types on cluster admin page """
    model = MountPoint
    extra = 1

class ClusterPartitionInline(admin.TabularInline):
    """ To enable inline editing of instance types on cluster admin page """
    model = ClusterPartition
    extra = 1


class FilesystemExportInline(admin.TabularInline):
    """ To enable inline editing of instance types on cluster admin page """
    model = FilesystemExport
    extra = 1


class VirtualNetworkAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for VirtualNetwork model """
    list_display = ('id', 'name', 'cloud_region', 'cloud_id', 'cloud_state')



class VirtualSubnetAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for VirtualSubnet model """
    list_display = ('id', 'name', 'cloud_zone', 'cloud_id', '_vpc_name', 'cloud_state')


    def _vpc_name(self, obj):
        return obj.vpc.name
    _vpc_name.short_description = 'VPC'


class FilesystemAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Filesystem model """
    inlines = (FilesystemExportInline,)

    list_display = ('id', 'name', 'impl_type', 'cloud_zone', 'cloud_id', 'subnet', 'cloud_state')



class ClusterAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Cluster model """
    inlines = (MountPointInline,ClusterPartitionInline)
    list_display = ('id', 'name', 'cloud_zone', '_controller_node', 'status')

    def _controller_node(self, obj):
        if obj.controller_node:
            return obj.controller_node.public_ip if obj.controller_node.public_ip else obj.controller_node.internal_ip
        else:
            return "<none>"
    _controller_node.short_description = "Controller Node IP"



class ApplicationAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Application model """
    list_display = ('id', 'name', 'install_loc', 'compiler', 'mpi', 'status')

class SpackApplicationAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Application model """
    list_display = ('id', 'name', 'spack_spec', 'install_loc', 'compiler', 'mpi', 'status')



class JobAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Job model """
    list_display = ('id', 'get_name',  'partition', 'number_of_nodes', 'ranks_per_node', 'threads_per_rank', 'status')

    def get_name(self, obj):
        return obj.application.name
    get_name.short_description = 'Aplication'  #Renames column head

# Register your models here.
admin.site.register(Application, ApplicationAdmin)
admin.site.register(SpackApplication, SpackApplicationAdmin)
admin.site.register(CustomInstallationApplication)
admin.site.register(ApplicationInstallationLocation)
admin.site.register(VirtualNetwork, VirtualNetworkAdmin)
admin.site.register(VirtualSubnet, VirtualSubnetAdmin)
admin.site.register(Cluster, ClusterAdmin)
admin.site.register(ClusterPartition)
admin.site.register(ComputeInstance)
admin.site.register(Credential)
admin.site.register(Job, JobAdmin)
admin.site.register(MachineType)
admin.site.register(InstanceType)
admin.site.register(Benchmark)
admin.site.register(Role)
admin.site.register(User, UserAdmin)
admin.site.register(Filesystem, FilesystemAdmin)
admin.site.register(GCPFilestoreFilesystem)
admin.site.register(FilesystemExport)
admin.site.register(MountPoint)
admin.site.register(Workbench)
admin.site.register(WorkbenchPreset)


