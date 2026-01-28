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
""" admin.py """

from django.contrib import admin
from django.contrib.auth.admin import UserAdmin as BaseUserAdmin
from .models import *  #pylint: disable=wildcard-import,unused-wildcard-import
from .forms import UserCreationForm, UserUpdateForm
import json
from django.utils.safestring import mark_safe


class UserAdmin(BaseUserAdmin):
    """ Custom UserAdmin """
    add_form = UserCreationForm
    form = UserUpdateForm
    model = User
    list_display = (
        "username",
        "first_name",
        "last_name",
        "email",
        "is_staff",
        "is_active",
    )
    list_filter = (
        "email",
        "is_staff",
        "is_active",
    )
    ordering = ("username",)
    readonly_fields = (
        "last_login",
        "date_joined",
    )

    fieldsets = (
        (None, {
            "fields": (
                "username",
                "password",
            )
        }),
        ("Personal info", {
            "fields": (
                "first_name",
                "last_name",
                "email",
            )
        }),
        ("Permissions", {
            "fields": (
                "is_active",
                "is_staff",
                "is_superuser",
                "roles",
            )
        }),
        ("Important dates", {
            "fields": ("last_login", "date_joined")
        }),
    )


class ContainerRegistryInline(admin.TabularInline):
    model = ContainerRegistry
    extra = 1


class ContainerRegistryAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for ContainerRegistry model """
    list_display = (
        "id", 
        "cluster", 
        "repository_id", 
        "repo_mode", 
        "format", 
        "status",
        "get_registry_url",
        "get_secret_url"
    )
    list_filter = (
        "status",
        "repo_mode",
        "format",
    )
    search_fields = (
        "repository_id",
        "repo_mirror_url",
    )
    readonly_fields = (
        "get_registry_url",
        "get_secret_url",
        "get_registry_console_url",
        "get_build_url",
        "display_build_info",
    )
    fieldsets = (
        (None, {
            "fields": (
                "cluster",
                "repository_id",
                "secret_id",
                "repo_mode",
                "format",
                "status",
                "display_build_info",
                "get_registry_url",
                "get_secret_url",
            )
        }),
        ("Authentication", {
            "fields": (
                "repo_username",
                "use_upstream_credentials",
            )
        }),
        ("Mirroring", {
            "fields": (
                "use_public_repository",
                "repo_mirror_url",
            )
        }),
    )

    def display_build_info(self, obj):
        """Return pretty-printed JSON for build_info."""
        # Safe to mark as safe because we’re controlling the <pre> block ourselves.
        pretty_json = json.dumps(obj.build_info, indent=2)
        return mark_safe(f"<pre>{pretty_json}</pre>")
    display_build_info.short_description = "Build Info (pretty-printed)"

    def get_registry_url(self, obj):
        return obj.get_registry_url()
    get_registry_url.short_description = "Registry URL"

    def get_secret_url(self, obj):
        return obj.get_secret_url()
    get_secret_url.short_description = "Secret URL"


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
    list_display = ("id", "name", "cloud_region", "cloud_id", "cloud_state")


class VirtualSubnetAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for VirtualSubnet model """
    list_display = ("id", "name", "cloud_zone", "cloud_id", "_vpc_name",
                    "cloud_state")

    def _vpc_name(self, obj):
        return obj.vpc.name

    _vpc_name.short_description = "VPC"


class FilesystemAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Filesystem model """
    inlines = (FilesystemExportInline,)

    list_display = ("id", "name", "impl_type", "cloud_zone", "cloud_id",
                    "subnet", "cloud_state")


class ClusterAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Cluster model """
    inlines = (MountPointInline, ClusterPartitionInline, ContainerRegistryInline)
    list_display = ("id", "name", "cloud_zone", "_controller_node", "status")

    def _controller_node(self, obj):
        if obj.controller_node:
            return (obj.controller_node.public_ip
                    if obj.controller_node.public_ip else
                    obj.controller_node.internal_ip)
        else:
            return "<none>"

    _controller_node.short_description = "Controller Node IP"


class ApplicationAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Application model """
    list_display = ("id", "name", "install_loc", "compiler", "mpi", "status")


class SpackApplicationAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Application model """
    list_display = ("id", "name", "spack_spec", "install_loc", "compiler",
                    "mpi", "status")


class JobAdmin(admin.ModelAdmin):
    """ Custom ModelAdmin for Job model """
    list_display = ("id", "get_name", "partition", "number_of_nodes",
                    "ranks_per_node", "threads_per_rank", "status")

    def get_name(self, obj):
        return obj.application.name

    get_name.short_description = "Application"  #Renames column head


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
admin.site.register(Benchmark)
admin.site.register(Role)
admin.site.register(User, UserAdmin)
admin.site.register(Filesystem, FilesystemAdmin)
admin.site.register(GCPFilestoreFilesystem)
admin.site.register(FilesystemExport)
admin.site.register(MountPoint)
admin.site.register(Workbench)
admin.site.register(WorkbenchPreset)
admin.site.register(AuthorisedUser)
admin.site.register(WorkbenchMountPoint)
admin.site.register(StartupScript)
admin.site.register(Image)
admin.site.register(ContainerRegistry, ContainerRegistryAdmin)
admin.site.register(ContainerApplication)
