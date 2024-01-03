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
""" urls.py """

# They actually make sense here because we really do want almost all
# pylint: disable=wildcard-import,unused-wildcard-import

from django.urls import path, re_path, include
from django.views.generic import TemplateView
from rest_framework import routers
from . import views
from .views.credentials import *
from .views.images import *
from .views.clusters import *
from .views.applications import *
from .views.jobs import *
from .views.benchmarks import *
from .views.workbench import *
from .views.users import *
from .views.vpc import *
from .views.filesystems import *
from .views.gcpfilestore import *
from .views.grafana import GrafanaProxyView, GrafanaView
from .views.asyncview import RunningTasksViewSet

handler403 = "ghpcfe.views.error_pages.custom_error_403"

urlpatterns = [
    path("", views.index, name="index"),
    path(
        "document/",
        TemplateView.as_view(template_name="document.html"),
        name="document",
    ),
    path("credentials/", CredentialListView.as_view(), name="credentials"),
    path("clusters/", ClusterListView.as_view(), name="clusters"),
    path("vpc/", VPCListView.as_view(), name="vpcs"),
    path("applications/", ApplicationListView.as_view(), name="applications"),
    path("jobs/", JobListView.as_view(), name="jobs"),
    path("benchmarks/", BenchmarkListView.as_view(), name="benchmarks"),
    path(
        "credential/<int:pk>",
        CredentialDetailView.as_view(),
        name="credential-detail",
    ),
    path(
        "cluster/<int:pk>", ClusterDetailView.as_view(), name="cluster-detail"
    ),
    path("vpc/<int:pk>", VPCDetailView.as_view(), name="vpc-detail"),
    path(
        "application/<int:pk>",
        ApplicationDetailView.as_view(),
        name="application-detail",
    ),
    path("job/<int:pk>", JobDetailView.as_view(), name="job-detail"),
    path(
        "benchmark/<int:pk>",
        BenchmarkDetailView.as_view(),
        name="benchmark-detail",
    ),
    path("account/", AccountUpdateView.as_view(), name="account"),
    path("workbench/", WorkbenchListView.as_view(), name="workbench"),
    path(
        "workbench/<int:pk>",
        WorkbenchDetailView.as_view(),
        name="workbench-detail",
    ),
    re_path(
        "^grafana/(?P<path>.*)$",
        GrafanaProxyView.as_view(),
        name="grafana-proxy"
    ),
    path("graphs", GrafanaView.as_view(), name="grafana"),
]

urlpatterns += [
    path(
        "credential/create/",
        CredentialCreateView.as_view(),
        name="credential-create",
    ),
    path("vpc/create/", VPCCreateView1.as_view(), name="vpc-create"),
    path(
        "vpc/create2/?credential=<int:credential>",
        VPCCreateView2.as_view(),
        name="vpc-create2",
    ),
    path("vpc/import/", VPCImportView1.as_view(), name="vpc-import"),
    path(
        "vpc/import2/?credential=<int:credential>",
        VPCImportView2.as_view(),
        name="vpc-import2",
    ),
    path(
        "cluster/create/", ClusterCreateView.as_view(), name="cluster-create"
    ),
    path(
        "application/create1",
        ApplicationCreateSelectView.as_view(),
        name="application-create-select",
    ),
    path(
        "application/create/<int:cluster>",
        ApplicationCreateView.as_view(),
        name="application-create",
    ),
    path(
        "application/create_install/<int:cluster>",
        CustomInstallationApplicationCreateView.as_view(),
        name="application-create-install",
    ),
    path(
        "application/create_spack/<int:cluster>",
        SpackApplicationCreateView.as_view(),
        name="application-create-spack-cluster",
    ),
    path("job/create/<int:app>", JobCreateView.as_view(), name="job-create"),
    path(
        "job/create2/<int:app>/<int:cluster>",
        JobCreateView2.as_view(),
        name="job-create-2",
    ),
    path("job/rerun/<int:job>", JobRerunView.as_view(), name="job-rerun"),
    path(
        "benchmark/create/",
        BenchmarkCreateView.as_view(),
        name="benchmark-create",
    ),
    path(
        "credential/update/<int:pk>",
        CredentialUpdateView.as_view(),
        name="credential-update",
    ),
    path("vpc/update/<int:pk>", VPCUpdateView.as_view(), name="vpc-update"),
    path(
        "cluster/update/<int:pk>",
        ClusterUpdateView.as_view(),
        name="cluster-update",
    ),
    path(
        "application/update/<int:pk>",
        ApplicationUpdateView.as_view(),
        name="application-update",
    ),
    path("job/update/<int:pk>", JobUpdateView.as_view(), name="job-update"),
    path(
        "credential/delete/<int:pk>",
        CredentialDeleteView.as_view(),
        name="credential-delete",
    ),
    path("vpc/delete/<int:pk>", VPCDeleteView.as_view(), name="vpc-delete"),
    path(
        "cluster/delete/<int:pk>",
        ClusterDeleteView.as_view(),
        name="cluster-delete",
    ),
    path(
        "application/delete/<int:pk>",
        ApplicationDeleteView.as_view(),
        name="application-delete",
    ),
    path("job/delete/<int:pk>", JobDeleteView.as_view(), name="job-delete"),
    path(
        "workbench/create/",
        WorkbenchCreateView1.as_view(),
        name="workbench-create",
    ),
    path(
        "workbench/create2/?credential=<int:credential>",
        WorkbenchCreateView2.as_view(),
        name="workbench-create2",
    ),
    path(
        "workbench/update/<int:pk>",
        WorkbenchUpdate.as_view(),
        name="workbench-update",
    ),
    path(
        "workbench/delete/<int:pk>",
        WorkbenchDeleteView.as_view(),
        name="workbench-delete",
    ),
]

urlpatterns += [
    path(
        "application/<int:pk>/logs/",
        ApplicationLogView.as_view(),
        name="application-log",
    ),
    path(
        "application/<int:pk>/logs/<int:logid>",
        ApplicationLogFileView.as_view(),
        name="application-log-file",
    ),
    path("job/<int:pk>/logs/", JobLogView.as_view(), name="job-log"),
    path(
        "job/<int:pk>/logs/<int:logid>",
        JobLogFileView.as_view(),
        name="job-log-file",
    ),
    path(
        "cluster/<int:pk>/logs/", ClusterLogView.as_view(), name="cluster-log"
    ),
    path(
        "cluster/<int:pk>/logs/<int:logid>",
        ClusterLogFileView.as_view(),
        name="cluster-log-file",
    ),
]

urlpatterns += [
    path("vpc/destroy/<int:pk>", VPCDestroyView.as_view(), name="vpc-destroy"),
    path(
        "cluster/destroy/<int:pk>",
        ClusterDestroyView.as_view(),
        name="cluster-destroy",
    ),
    path(
        "workbench/destroy/<int:pk>",
        WorkbenchDestroyView.as_view(),
        name="workbench-destroy",
    ),
    path(
        "cluster/cost/<int:pk>", ClusterCostView.as_view(), name="cluster-cost"
    ),
    path(
        "cluster/costexport/<int:pk>",
        ClusterCostExportView.as_view(),
        name="cluster-cost-export",
    ),
]

urlpatterns += [
    re_path(
        r"^vpc/(?P<vpc_id>\d+)/subnets/$",
        VirtualSubnetView.as_view(),
        name="vpc-subnets",
    ),
]

urlpatterns += [
    path("filesystem/", FilesystemListView.as_view(), name="filesystems"),
    path(
        "filesystem/create/", FilesystemCreateView1.as_view(), name="fs-create"
    ),
    path(
        "filesystem/create2/?credential=<int:credential>",
        FilesystemCreateView2.as_view(),
        name="fs-create2",
    ),
    path(
        "filesystem/detail/<int:pk>",
        FilesystemRedirectView.as_view(target="detail"),
        name="fs-detail",
    ),
    path(
        "filesystem/edit/<int:pk>",
        FilesystemRedirectView.as_view(target="update"),
        name="fs-update",
    ),
    path(
        "filesystem/destroy/<int:pk>",
        FilesystemDestroyView.as_view(),
        name="fs-destroy",
    ),
    path(
        "filesystem/delete/<int:pk>",
        FilesystemDeleteView.as_view(),
        name="fs-delete",
    ),
    path(
        "filesystem/log/<int:pk>", FilesystemTFLogView.as_view(), name="fs-log"
    ),
    path(
        "filesystem/import/?credential=<int:credential>",
        FilesystemImportView.as_view(),
        name="import-fs-create",
    ),
    path(
        "filesystem/import/<int:pk>/detail",
        FilesystemImportDetailView.as_view(),
        name="import-fs-detail",
    ),
    path(
        "filesystem/import/<int:pk>/edit",
        FilesystemImportUpdateView.as_view(),
        name="import-fs-update",
    ),
    path(
        "backend/filesystem/create-files/<int:pk>",
        BackendCreateFilesystem.as_view(),
        name="backend-filesystem-create-files",
    ),
    path(
        "backend/filesystem/update-files/<int:pk>",
        BackendUpdateFilesystem.as_view(),
        name="backend-filesystem-update-files",
    ),
    path(
        "backend/filesystem/start/<int:pk>",
        BackendStartFilesystem.as_view(),
        name="backend-filesystem-start",
    ),
    path(
        "backend/filesystem/destroy/<int:pk>",
        BackendDestroyFilesystem.as_view(),
        name="backend-filesystem-destroy",
    ),
    path(
        "filesystem/filestore/create/?credential=<int:credential>",
        GCPFilestoreFilesystemCreateView.as_view(),
        name="filestore-create",
    ),
    path(
        "filesystem/filestore/detail/<int:pk>",
        GCPFilestoreFilesystemDetailView.as_view(),
        name="filestore-detail",
    ),
    path(
        "filesystem/filestore/edit/<int:pk>",
        GCPFilestoreFilesystemUpdateView.as_view(),
        name="filestore-update",
    ),
]

urlpatterns += [
    path("users/", UserListView.as_view(), name="users"),
    path("user/detail/<int:pk>", UserDetailView.as_view(), name="user-detail"),
    path(
        "user/admin/<int:pk>", UserAdminUpdateView.as_view(), name="user-admin"
    ),
]

# For APIs

router = routers.DefaultRouter()
router.register(
    r"api/applications", ApplicationViewSet, basename="api-application"
)
router.register(r"api/clusters", ClusterViewSet, basename="api-cluster")
router.register(
    r"api/credentials", CredentialViewSet, basename="api-credential"
)
router.register(r"api/jobs", JobViewSet, basename="api-job")
router.register(r"api/users", UserViewSet, basename="api-user")
router.register(
    r"api/spack_packages", SpackPackageViewSet, basename="api-spack"
)
router.register(r"api/tasks", RunningTasksViewSet, basename="api-tasks")
router.register(
    r"api/instance_pricing", InstancePricingViewSet, basename="api-pricing"
)  # Specify pk=ClusterPartition
router.register(
    r"api/instance_available",
    InstanceAvailabilityViewSet,
    basename="api-instancetype",
)  # Specify pk=ClusterID, zone, region
router.register(
    r"api/disks_available",
    DiskAvailabilityViewSet,
    basename="api-disktype",
)  # Specify pk=ClusterID, zone, region
router.register(r"api/vpcs", VPCViewSet, basename="api-vpcs")
router.register(r"api/subnets", VirtualSubnetViewSet, basename="api-subnets")

urlpatterns += [
    path("", include(router.urls)),
    path(
        "api-auth/", include("rest_framework.urls", namespace="rest_framework")
    ),
    path(r"api/credential-validate", CredentialValidateAPIView.as_view()),
]

# Views for backend functions

urlpatterns += [
    path(
        "backend/vpc-create/<int:pk>",
        BackendCreateVPC.as_view(),
        name="backend-create-vpc",
    ),
    path(
        "backend/vpc-start/<int:pk>",
        BackendStartVPC.as_view(),
        name="backend-start-vpc",
    ),
    path(
        "backend/vpc-destroy/<int:pk>",
        BackendDestroyVPC.as_view(),
        name="backend-destroy-vpc",
    ),
    path(
        "backend/cluster-create/<int:pk>",
        BackendCreateCluster.as_view(),
        name="backend-create-cluster",
    ),
    path(
        "backend/cluster-reconfigure/<int:pk>",
        BackendReconfigureCluster.as_view(),
        name="backend-reconfigure-cluster",
    ),
    path(
        "backend/cluster-start/<int:pk>",
        BackendStartCluster.as_view(),
        name="backend-start-cluster",
    ),
    path(
        "backend/cluster-status/<int:pk>",
        BackendClusterStatus.as_view(),
        name="backend-cluster-status",
    ),
    path(
        "backend/cluster_destroy/<int:pk>",
        BackendDestroyCluster.as_view(),
        name="backend-destroy-cluster",
    ),
    path(
        "backend/cluster-sync/<int:pk>",
        BackendSyncCluster.as_view(),
        name="backend-sync-cluster",
    ),
    path(
        "backend/spack-install/<int:pk>",
        BackendSpackInstall.as_view(),
        name="backend-spack-install",
    ),
    path(
        "backend/custom-app-install/<int:pk>",
        BackendCustomAppInstall.as_view(),
        name="backend-custom-app-install",
    ),
    path(
        "backend/job-run/<int:pk>",
        BackendJobRun.as_view(),
        name="backend-job-run",
    ),
    path(
        "backend/user-gcp-auth/<int:pk>",
        BackendAuthUserGCP.as_view(),
        name="backend-user-gcp-auth",
    ),
    path(
        "clusters/gcp-auth/<int:pk>",
        AuthUserGCP.as_view(),
        name="user-gcp-auth",
    ),
    path(
        "backend/workbench-create/<int:pk>",
        BackendCreateWorkbench.as_view(),
        name="backend-create-workbench",
    ),
    path(
        "backend/workbench-start/<int:pk>",
        BackendStartWorkbench.as_view(),
        name="backend-start-workbench",
    ),
    path(
        "backend/workbench-destroy/<int:pk>",
        BackendDestroyWorkbench.as_view(),
        name="backend-destroy-workbench",
    ),
    path(
        "backend/workbench-update/<int:pk>",
        BackendUpdateWorkbench.as_view(),
        name="backend-update-workbench",
    ),
    path(
        "backend/image-create/<int:pk>",
        BackendCreateImage.as_view(),
        name="backend-create-image",
    ),
    path(
        "backend/get-regions/<int:pk>",
        BackendListRegions.as_view(),
        name="backend-list-regions",
    ),
]

# Url paths that handle custom image views
urlpatterns += [
    path("images/", ImagesListView.as_view(), name="images"),
    path(
        "images/create/startup-script", StartupScriptCreateView.as_view(), name="startup-script-create"
    ),
    path(
        "images/startup-script-view/<int:pk>", StartupScriptDetailView.as_view(), name="startup-script-view"
        ),
    path(
        "images/startup-script-delete/<int:pk>", StartupScriptDeleteView.as_view(), name="startup-script-delete"
        ),
    path(
        "images/create/image", ImageCreateView.as_view(), name="image-create"
    ),
    path(
        "images/image-view/<int:pk>", ImageDetailView.as_view(), name="image-view"
        ),
    path(
        "images/image-delete/<int:pk>", ImageDeleteView.as_view(), name="image-delete"
        ),
    path(
        "images/image-status/<int:pk>", ImageStatusView.as_view(), name="image-status"
        ),
]
