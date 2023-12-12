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

""" clusters.py """

import csv
import json
from asgiref.sync import sync_to_async
from rest_framework import viewsets
from rest_framework.authentication import (
    SessionAuthentication,
    TokenAuthentication,
)
from rest_framework.permissions import IsAuthenticated
from rest_framework.decorators import action
from rest_framework.response import Response
from django.shortcuts import render, get_object_or_404, redirect
from django.db import transaction
from django.db.models import Q
from django.contrib.auth.mixins import LoginRequiredMixin
from django.contrib.auth.views import redirect_to_login
from django.core.exceptions import ValidationError
from django.http import (
    HttpResponse,
    HttpResponseRedirect,
    JsonResponse,
    HttpResponseNotFound,
)
from django.urls import reverse
from django.forms import inlineformset_factory
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from ..models import (
    Application,
    Cluster,
    Credential,
    Job,
    Filesystem,
    FilesystemExport,
    MountPoint,
    FilesystemImpl,
    Role,
    ClusterPartition,
    VirtualSubnet,
    Task,
    User,
)
from ..serializers import ClusterSerializer
from ..forms import ClusterForm, ClusterMountPointForm, ClusterPartitionForm
from ..cluster_manager import cloud_info, c2, utils
from ..cluster_manager.clusterinfo import ClusterInfo
from ..views.asyncview import BackendAsyncView

from .view_utils import TerraformLogFile, GCSFile, StreamingFileView

import logging
import secrets

logger = logging.getLogger(__name__)


class ClusterListView(LoginRequiredMixin, generic.ListView):
    """Custom ListView for Cluster model"""

    model = Cluster
    template_name = "cluster/list.html"

    def get_queryset(self):
        qs = super().get_queryset()
        if self.request.user.has_admin_role():
            return qs
        wanted_items = set()
        for cluster in qs:
            if (
                self.request.user in cluster.authorised_users.all()
                and cluster.status == "r"
            ):
                wanted_items.add(cluster.pk)
        return qs.filter(pk__in=wanted_items)

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for cluster in self.get_queryset():
            if cluster.status in ["c", "i", "t"]:
                loading = 1
                break
        admin_view = 0
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(*args, **kwargs)
        context["loading"] = loading
        context["admin_view"] = admin_view
        context["navtab"] = "cluster"
        return context


class ClusterDetailView(LoginRequiredMixin, generic.DetailView):
    """Custom DetailView for Cluster model"""

    model = Cluster
    template_name = "cluster/detail.html"

    def get_context_data(self, **kwargs):
        admin_view = 0
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(**kwargs)
        context["navtab"] = "cluster"
        context["admin_view"] = admin_view
        # Perform extra query to populate instance types data
        # context['cluster_instance_types'] = \
        #     ClusterInstanceType.objects.filter(cluster=self.kwargs['pk'])
        return context

class ClusterCreateView(LoginRequiredMixin, CreateView):
    """Custom CreateView for Cluster model"""

    def get(self, request, *args, **kwargs):
        # Check if there are any credentials available
        credentials = Credential.objects.filter(owner=self.request.user)

        if credentials.exists():
            # Create a new cluster with default values
            cluster = Cluster(
                cloud_credential=credentials.first(),
                name="cluster", 
                owner=request.user,
                status="n",
                spackdir="/opt/cluster/spack",
                num_login_nodes=1)

            cluster.save()

            return redirect('backend-create-cluster', pk=cluster.pk)
        else:
            # Redirect to the credentials creation page with a message
            messages.error(self.request, "Please create a credential before creating a cluster.")
            return redirect('credentials')  # Adjust to your credential creation view name

class ClusterUpdateView(LoginRequiredMixin, UpdateView):
    """Custom UpdateView for Cluster model"""

    model = Cluster
    template_name = "cluster/update_form.html"
    form_class = ClusterForm

    def get_mp_formset(self, **kwargs):
        def formfield_cb(model_field, **kwargs):
            field = model_field.formfield(**kwargs)
            cluster = self.object
            
            if model_field.name == "export":
                if cluster.shared_fs is None:
                    # Create and save the shared filesystem, exports, and mount points
                    shared_fs = Filesystem(
                        name=f"{cluster.name}-sharedfs",
                        cloud_credential=cluster.cloud_credential,
                        cloud_id=cluster.cloud_id,
                        cloud_state=cluster.cloud_state,
                        cloud_region=cluster.cloud_region,
                        cloud_zone=cluster.cloud_zone,
                        subnet=cluster.subnet,
                        fstype="n",
                        impl_type=FilesystemImpl.BUILT_IN,
                    )
                    shared_fs.save()

                    export = FilesystemExport(filesystem=shared_fs, export_name="/opt/cluster")
                    export.save()
                    export = FilesystemExport(filesystem=shared_fs, export_name="/home")
                    export.save()

                    cluster.shared_fs = shared_fs
                    cluster.save()

                    # Create and save mount points
                    export = cluster.shared_fs.exports.all()[0]
                    mp = MountPoint(
                        export=export,
                        cluster=cluster,
                        mount_order=0,
                        mount_options="defaults,nofail,nosuid",
                        mount_path="/opt/cluster",
                    )
                    mp.save()
                    export = cluster.shared_fs.exports.all()[1]
                    mp = MountPoint(
                        export=export,
                        cluster=cluster,
                        mount_order=1,
                        mount_options="defaults,nofail,nosuid",
                        mount_path="/home",
                    )
                    mp.save()
            
            # Continue with the usual logic for handling exports
            if cluster.shared_fs is not None:
                fsquery = (
                    Filesystem.objects.exclude(
                        impl_type=FilesystemImpl.BUILT_IN
                    )
                    .filter(cloud_state__in=["m", "i"])
                    .values_list("pk", flat=True)
                )
                # Add back our cluster's filesystem
                fsystems = list(fsquery) + [cluster.shared_fs.id]
                field.queryset = FilesystemExport.objects.filter(
                    filesystem__in=fsystems
                )
            
            return field


        # This creates a new class on the fly
        FormClass = inlineformset_factory(  # pylint: disable=invalid-name
            Cluster,
            MountPoint,
            form=ClusterMountPointForm,
            formfield_callback=formfield_cb,
            can_delete=True,
            extra=0,
        )

        if self.request.POST:
            kwargs["data"] = self.request.POST
        return FormClass(instance=self.object, **kwargs)

    def get_partition_formset(self, **kwargs):
        def formfield_cb(model_field, **kwargs):
            field = model_field.formfield(**kwargs)
            cluster = self.object

            if not cluster.partitions.exists():
                logger.info("No partitions exist, creating a default one.")
                # Create and save the default partition with hardcoded values
                default_partition = ClusterPartition(
                    name="batch",
                    machine_type="c2-standard-60",
                    dynamic_node_count=4,
                    vCPU_per_node=30,
                    cluster=cluster  # Set the cluster for the partition
                )
                default_partition.save()
            return field

        # This creates a new class on the fly
        FormClass = inlineformset_factory(  # pylint: disable=invalid-name
            Cluster,
            ClusterPartition,
            form=ClusterPartitionForm,
            formfield_callback=formfield_cb,
            can_delete=True,
            extra=0,
        )

        if self.request.POST:
            kwargs["data"] = self.request.POST
        return FormClass(instance=self.object, **kwargs)

    def get_success_url(self):
        logger.info(f"Current cluster state { self.object.cloud_state }")
        if self.object.cloud_state == "m":
            # Perform live cluster reconfiguration
            return reverse("backend-reconfigure-cluster", kwargs={"pk": self.object.pk})
        elif self.object.cloud_state == "nm":
            # Perform live cluster reconfiguration
            return reverse("backend-start-cluster", kwargs={"pk": self.object.pk})

    def _get_region_info(self):
        if not hasattr(self, "region_info"):
            self.region_info = cloud_info.get_region_zone_info(
                "GCP", self.get_object().cloud_credential.detail
            )
        return self.region_info

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        subnet_regions = {
            sn.id: sn.cloud_region
            for sn in VirtualSubnet.objects.filter(
                cloud_credential=self.get_object().cloud_credential
            ).all()
        }
        subnet_regions = {
            sn.id: sn.cloud_region
            for sn in VirtualSubnet.objects.filter(
                cloud_credential=self.get_object().cloud_credential
            )
            .filter(Q(cloud_state="i") | Q(cloud_state="m"))
            .all()
        }
        
        context["subnet_regions"] = json.dumps(subnet_regions)
        context["object"] = self.object
        context["region_info"] = json.dumps(self._get_region_info())
        context["navtab"] = "cluster"
        context["mountpoints_formset"] = self.get_mp_formset()
        context["cluster_partitions_formset"] = self.get_partition_formset()
        context["title"] = "Create cluster" if self.object.status == "n" else "Update cluster"
        return context


    def form_valid(self, form):
        logger.info("In form_valid")
        context = self.get_context_data()
        mountpoints = context["mountpoints_formset"]
        partitions = context["cluster_partitions_formset"]

        if self.object.status == "n":
            # If creating a new cluster generate unique cloud id.
            unique_str = secrets.token_hex(4)
            self.object.cloud_id = self.object.name + "-" + unique_str
            suffix = self.object.cloud_id.split("-")[-1]
            self.object.cloud_id = self.object.name + "-" + suffix
        
        self.object.cloud_region = self.object.subnet.cloud_region

        machine_info = cloud_info.get_machine_types(
            "GCP",
            self.object.cloud_credential.detail,
            self.object.cloud_region,
            self.object.cloud_zone,
        )
        disk_info = {
            x["name"]: x
            for x in cloud_info.get_disk_types(
                "GCP",
                self.object.cloud_credential.detail,
                self.object.cloud_region,
                self.object.cloud_zone,
            )
            if x["name"].startswith("pd-")
        }
        
        if self.object.status != "n" and self.object.status != "r":
            form.add_error(None, "It is not newly created cluster or it is not running yet.")
            return self.form_invalid(form)

        # Verify Disk Types & Sizes
        try:
            my_info = disk_info[self.object.controller_disk_type]
            if self.object.controller_disk_size < my_info["minSizeGB"]:
                form.add_error(
                    "controller_disk_size",
                    "Minimum Disk Size for "
                    f"{self.object.controller_disk_type} is "
                    f"{my_info['minSizeGB']}"
                )
                return self.form_invalid(form)
            if self.object.controller_disk_size > my_info["maxSizeGB"]:
                form.add_error(
                    "controller_disk_size",
                    "Maximum Disk Size for "
                    f"{self.object.controller_disk_type} is "
                    f"{my_info['maxSizeGB']}"
                )
                return self.form_invalid(form)

        except KeyError:
            form.add_error("controller_disk_type", "Invalid Disk Type")
            return self.form_invalid(form)

        try:
            my_info = disk_info[self.object.login_node_disk_type]
            if self.object.login_node_disk_size < my_info["minSizeGB"]:
                form.add_error(
                    "login_node_disk_size",
                    "Minimum Disk Size for "
                    f"{self.object.login_node_disk_type} is "
                    f"{my_info['minSizeGB']}"
                )
                return self.form_invalid(form)
            if self.object.login_node_disk_size > my_info["maxSizeGB"]:
                form.add_error(
                    "login_node_disk_size",
                    "Maximum Disk Size for "
                    f"{self.object.login_node_disk_type} is "
                    f"{my_info['maxSizeGB']}"
                )
                return self.form_invalid(form)

        except KeyError:
            form.add_error("login_node_disk_type", "Invalid Disk Type")
            return self.form_invalid(form)

        # Verify formset validity (surprised there's no method to do this)
        for formset, formset_name in [
            (mountpoints, "mountpoints"),
            (partitions, "partitions"),
        ]:
            if not formset.is_valid():
                form.add_error(None, f"Error in {formset_name} section")
                return self.form_invalid(form)

        # Get the existing MountPoint objects associated with the cluster
        existing_mount_points = MountPoint.objects.filter(cluster=self.object)

        # Iterate through the existing mount points and check if they are in the updated formset
        for mount_point in existing_mount_points:
            if not any(mount_point_form.instance == mount_point for mount_point_form in mountpoints.forms):
                # The mount point is not in the updated formset, so delete it
                mount_point_path = mount_point.mount_path
                mount_point_id = mount_point.pk
                logger.info(f"Deleting mount point: {mount_point_path}, ID: {mount_point_id}")
                mount_point.delete()

       # Get the existing ClusterPartition objects associated with the cluster
        existing_partitions = ClusterPartition.objects.filter(cluster=self.object)

        # Iterate through the existing partitions and check if they are in the updated formset
        for partition in existing_partitions:
            if not any(partition_form.instance == partition for partition_form in partitions.forms):
                # The partition is not in the updated formset, so delete it
                partition_name = partition.name
                partition_id = partition.pk
                logger.info(f"Deleting partition: {partition_name}, ID: {partition_id}")
                partition.delete()

        try:
            with transaction.atomic():
                # Save the modified Cluster object
                self.object.save()
                self.object = form.save()
                mountpoints.instance = self.object
                mountpoints.save()

                partitions.instance = self.object
                parts = partitions.save()
                
                try:
                    for part in parts:
                        part.vCPU_per_node = machine_info[part.machine_type]["vCPU"] // (1 if part.enable_hyperthreads else 2)
                        # Validate GPU choice
                        if part.GPU_type:
                            try:
                                accel_info = machine_info[part.machine_type]["accelerators"][part.GPU_type]
                                if (
                                    part.GPU_per_node < accel_info["min_count"]
                                    or part.GPU_per_node > accel_info["max_count"]
                                ):
                                    raise ValidationError(
                                        "Invalid number of GPUs of type " f"{part.GPU_type}"
                                    )
                            except KeyError as err:
                                raise ValidationError(f"Invalid GPU type {part.GPU_type}") from err
                        # Add validation for machine_type and disk_type combinations here
                        invalid_combinations = [
                            ("c3-", "pd-standard"),
                            ("h3-", "pd-standard"),
                            ("h3-", "pd-ssd"),
                        ]
                        for machine_prefix, disk_type in invalid_combinations:
                            if part.machine_type.startswith(machine_prefix) and part.boot_disk_type == disk_type:
                                logger.info("invalid disk")
                                raise ValidationError(
                                    f"Invalid combination: machine_type {part.machine_type} cannot be used with disk_type {disk_type}."
                                )
                except KeyError as err:
                    raise ValidationError("Error in Partition - invalid machine type: " f"{part.machine_type}") from err

                # Continue with saving the 'parts' if no validation errors were raised
                parts = partitions.save()

        except ValidationError as ve:
            form.add_error(None, ve)
            return self.form_invalid(form)

        msg = (
            "Provisioning a new cluster. This may take up to 15 minutes."
        )
        
        if self.object.status == "r":
            msg = "Reconfiguring running cluster, this may take few minutes."

        messages.success(self.request, msg)

        # Be kind... Check filesystems to verify all in the same zone as us.
        for mp in self.object.mount_points.exclude(
            export__filesystem__impl_type=FilesystemImpl.BUILT_IN
        ):
            if mp.export.filesystem.cloud_zone != self.object.cloud_zone:
                messages.warning(
                    self.request,
                    "Possibly expensive: Filesystem "
                    f"{mp.export.filesystem.name} is in a different zone "
                    f"({mp.export.filesystem.cloud_zone}) than the cluster!",
                )

        return super().form_valid(form)


class ClusterDeleteView(LoginRequiredMixin, DeleteView):
    """Custom DeleteView for Cluster model"""

    model = Cluster
    template_name = "cluster/check_delete.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "cluster"
        return context

    def get_success_url(self):
        cluster = Cluster.objects.get(pk=self.kwargs["pk"])
        messages.success(self.request, f"Cluster {cluster.name} deleted.")
        return reverse("clusters")


class ClusterDestroyView(LoginRequiredMixin, generic.DetailView):
    """Custom View to confirm Cluster destroy"""

    model = Cluster
    template_name = "cluster/check_destroy.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        applications = Application.objects.filter(cluster=context["cluster"].id)
        jobs = Job.objects.filter(application__in=applications)
        context["applications"] = applications
        context["jobs"] = jobs
        context["navtab"] = "cluster"
        return context


class ClusterCostView(LoginRequiredMixin, generic.DetailView):
    """Custom view for a cluster's cost analysis"""

    model = Cluster
    template_name = "cluster/cost.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "cluster"

        cluster_users = []
        for user in User.objects.all():
            spend = user.total_spend(cluster_id=context["cluster"].id)
            if spend > 0:
                cluster_users.append(
                    (
                        spend,
                        user.total_jobs(cluster_id=context["cluster"].id),
                        user,
                    )
                )

        cluster_apps = []
        for app in Application.objects.filter(cluster=context["cluster"].id):
            cluster_apps.append((app.total_spend(), app))

        context["users_by_spend"] = sorted(
            cluster_users, key=lambda x: x[0], reverse=True
        )
        context["apps_by_spend"] = sorted(
            cluster_apps, key=lambda x: x[0], reverse=True
        )
        return context


class ClusterLogFileView(LoginRequiredMixin, StreamingFileView):
    """View for cluster provisioning logs"""

    bucket = utils.load_config()["server"]["gcs_bucket"]
    valid_logs = [
        {"title": "Terraform Log", "type": TerraformLogFile, "args": ()},
        {
            "title": "Startup Log",
            "type": GCSFile,
            "args": (bucket, "tmp/setup.log"),
        },
        {
            "title": "Ansible Sync Log",
            "type": GCSFile,
            "args": (bucket, "tmp/ansible.log"),
        },
        {
            "title": "System Log",
            "type": GCSFile,
            "args": (bucket, "var/log/messages"),
        },
        {
            "title": "Slurm slurmctld.log",
            "type": GCSFile,
            "args": (bucket, "var/log/slurm/slurmctld.log"),
        },
        {
            "title": "Slurm resume.log",
            "type": GCSFile,
            "args": (bucket, "var/log/slurm/resume.log"),
        },
        {
            "title": "Slurm suspend.log",
            "type": GCSFile,
            "args": (bucket, "var/log/slurm/suspend.log"),
        },
    ]

    def _create_file_info_object(self, logfile_info, *args, **kwargs):
        return logfile_info["type"](*logfile_info["args"], *args, **kwargs)

    def get_file_info(self):
        logid = self.kwargs.get("logid", -1)
        cluster_id = self.kwargs.get("pk")
        cluster = get_object_or_404(Cluster, pk=cluster_id)
        ci = ClusterInfo(cluster)
        tf_dir = ci.get_terraform_dir()
        bucket_prefix = f"clusters/{cluster.id}/controller_logs"

        entry = self.valid_logs[logid]
        if entry["type"] == TerraformLogFile:
            extra_args = [tf_dir]
        elif entry["type"] == GCSFile:
            extra_args = [bucket_prefix]
        else:
            extra_args = []
        return self._create_file_info_object(entry, *extra_args)


class ClusterLogView(LoginRequiredMixin, generic.DetailView):
    """View to display cluster log files"""

    model = Cluster
    template_name = "cluster/log.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["log_files"] = [
            {"id": n, "title": entry["title"]}
            for n, entry in enumerate(ClusterLogFileView.valid_logs)
        ]
        context["navtab"] = "cluster"
        return context


class ClusterCostExportView(LoginRequiredMixin, generic.DetailView):
    """Export raw cost data per cluster as CSV"""

    model = Cluster

    def get(self, request, *args, **kwargs):
        response = HttpResponse(content_type="text/csv")
        writer = csv.writer(response)
        writer.writerow(["Job ID", "User", "Application", "Partition",
                         "Number of Nodes", "Ranks per Node", "Runtime (sec)",
                         "Node Price (per hour)", "Job Cost"])

        for job in Job.objects.filter(
                cluster=self.kwargs["pk"]).values_list("id", "user__username",
                "application__name", "partition__name", "number_of_nodes",
                "ranks_per_node", "runtime", "node_price", "job_cost"):
            writer.writerow(job)

        response["Content-Disposition"] = "attachment; filename='report.csv'"
        return response


# For APIs


class ClusterViewSet(viewsets.ModelViewSet):
    """Custom ModelViewSet for Cluster model"""

    permission_classes = (IsAuthenticated,)
    # queryset = Cluster.objects.all().order_by('name')
    serializer_class = ClusterSerializer

    def get_queryset(self):
        # cluster admins can see all the clusters
        if Role.CLUSTERADMIN in [x.id for x in self.request.user.roles.all()]:
            queryset = Cluster.objects.all().order_by("name")
        # ordinary user can only see clusters authorised to use
        else:
            queryset = Cluster.objects.filter(
                authorised_users__id=self.request.user.id
            ).order_by("name")
        return queryset

    @action(methods=["get"], detail=True, permission_classes=[IsAuthenticated])
    def get_users(self, request, unused_pk):
        cluster = self.get_object()
        auth_users = cluster.authorised_users.all()
        return Response(
            [{"username": user.username, "uid": user.id} for user in auth_users]
        )

    @action(methods=["get"], detail=True, permission_classes=[IsAuthenticated])
    def get_instance_limits(self, request, unused_pk):
        cluster = self.get_object()
        limits = cluster.instance_limits()
        return Response(
            [
                {"instance_name": entry[0].name, "nodes": entry[1]}
                for entry in limits
            ]
        )

    @action(
        methods=["get"],
        detail=True,
        permission_classes=[IsAuthenticated],
        url_path="filesystem.fact",
        suffix=".fact",
    )
    def ansible_filesystem(self, request, unused_pk):
        fs_type_translator = {
            " ": "none",
            "n": "nfs",
            "e": "efs",
            "l": "lustre",
            "b": "beegfs",
        }
        cluster = self.get_object()
        mounts = [
            {
                "path": mp.mount_path,
                "src": mp.mount_source,
                "fstype": fs_type_translator[mp.fstype],
                "opts": mp.mount_options,
            }
            for mp in cluster.mount_points.all()
        ]
        return JsonResponse({"mounts": mounts})


class InstancePricingViewSet(viewsets.ViewSet):
    """ModelviewSet providing GCP instance pricing"""

    permission_classes = (IsAuthenticated,)
    authentication_classes = [SessionAuthentication, TokenAuthentication]

    def retrieve(self, request, pk=None):
        partition = get_object_or_404(ClusterPartition, pk=pk)
        instance_type = partition.machine_type
        cluster = partition.cluster

        price = cloud_info.get_instance_pricing(
            "GCP",
            cluster.cloud_credential.detail,
            cluster.cloud_region,
            cluster.cloud_zone,
            instance_type,
            (partition.GPU_type, partition.GPU_per_node),
        )
        return JsonResponse(
            {"instance": instance_type, "price": price, "currency": "USD"}
        )  # TODO: Currency

    def list(self, request):
        return JsonResponse({})


class InstanceAvailabilityViewSet(viewsets.ViewSet):
    """ModelviewSet providing GCP instance availability across locations"""

    permission_classes = (IsAuthenticated,)
    authentication_classes = [SessionAuthentication, TokenAuthentication]

    def retrieve(self, request, pk=None):
        cluster = get_object_or_404(
            Cluster, pk=request.query_params.get("cluster", -1)
        )
        region = request.query_params.get("region", None)
        zone = request.query_params.get("zone", None)

        try:
            region_info = cloud_info.get_region_zone_info(
                "GCP", cluster.cloud_credential.detail
            )
            if zone not in region_info.get(region, []):
                return JsonResponse({})

            machine_info = cloud_info.get_machine_types(
                "GCP", cluster.cloud_credential.detail, region, zone
            )
            return JsonResponse(machine_info.get(pk, {}))

        # Want to fail gracefully here
        except Exception:  # pylint: disable=broad-except
            pass

        return JsonResponse({})

    def list(self, request):
        cluster = get_object_or_404(
            Cluster, pk=request.query_params.get("cluster", -1)
        )
        region = request.query_params.get("region", None)
        zone = request.query_params.get("zone", None)

        try:
            region_info = cloud_info.get_region_zone_info(
                "GCP", cluster.cloud_credential.detail
            )
            if zone not in region_info.get(region, []):
                logger.info(
                    "Unable to retrieve data for zone %s in region %s",
                    zone,
                    region,
                )
                return JsonResponse({})

            machine_info = cloud_info.get_machine_types(
                "GCP", cluster.cloud_credential.detail, region, zone
            )
            return JsonResponse({"machine_types": list(machine_info.keys())})

        # Can't do a lot about API failures, just log it and move one
        except Exception as err:  # pylint: disable=broad-except
            logger.exception("Exception during cloud API query:", exc_info=err)
            pass

        return JsonResponse({})


class DiskAvailabilityViewSet(viewsets.ViewSet):
    """API View providing GCP disk availability across locations"""

    permission_classes = (IsAuthenticated,)
    authentication_classes = [SessionAuthentication, TokenAuthentication]

    def list(self, request):
        cluster = get_object_or_404(
            Cluster, pk=request.query_params.get("cluster", -1)
        )
        region = request.query_params.get("region", None)
        zone = request.query_params.get("zone", None)

        try:
            region_info = cloud_info.get_region_zone_info(
                "GCP", cluster.cloud_credential.detail
            )
            if zone not in region_info.get(region, []):
                logger.info(
                    "Unable to retrieve data for zone %s in region %s",
                    zone,
                    region,
                )
                return JsonResponse({})

            info = cloud_info.get_disk_types(
                "GCP", cluster.cloud_credential.detail, region, zone
            )
            return JsonResponse({"disks": info})

        # Can't do a lot about API failures, just log it and move one
        except Exception as err:  # pylint: disable=broad-except
            logger.exception("Exception during cloud API query:", exc_info=err)
            pass

        return JsonResponse({})

# Other supporting views


class BackendCreateCluster(BackendAsyncView):
    """A view to make async call to create a new cluster"""

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        creds = cluster.cloud_credential.detail
        return (cluster, creds)

    def cmd(self, unused_task_id, unused_token, cluster, creds):
        ci = ClusterInfo(cluster)
        ci.prepare(creds)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create Cluster", *args)
        return HttpResponseRedirect(
            reverse("cluster-update", kwargs={"pk": pk})
        )


class BackendReconfigureCluster(BackendAsyncView):
    """View to reconfigure the cluster."""

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return (cluster,)

    def cmd(self, unused_task_id, unused_token, cluster):
        ci = ClusterInfo(cluster)
        ci.update()
        ci.reconfigure_cluster()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Live Reconfigure the Cluster", *args)
        return HttpResponseRedirect(
            reverse("cluster-detail", kwargs={"pk": pk})
        )


class BackendStartCluster(BackendAsyncView):
    """A view to make async call to create a new cluster"""

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        creds = cluster.cloud_credential.detail
        return (cluster, creds)

    def cmd(self, unused_task_id, unused_token, cluster, creds):
        ci = ClusterInfo(cluster)
        ci.start_cluster(creds)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Start Cluster", *args)
        return HttpResponseRedirect(
            reverse("cluster-detail", kwargs={"pk": pk})
        )


class BackendDestroyCluster(BackendAsyncView):
    """A view to make async call to destroy a cluster"""

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return (cluster,)

    def cmd(self, unused_task_id, unused_token, cluster):
        ci = ClusterInfo(cluster)
        ci.stop_cluster()

    async def post(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Destroy Cluster", *args)
        return HttpResponseRedirect(
            reverse("cluster-detail", kwargs={"pk": pk})
        )


class BackendSyncCluster(LoginRequiredMixin, generic.View):
    """Backend handler for cluster syncing"""

    def get(self, request, pk, *args, **kwargs):
        def response(message):
            logger.info("Received SYNC Complete: %s", message)
            if message.get("cluster_id") != pk:
                logger.error(
                    "Cluster ID mismatch versus to callback: "
                    "expected %s, %s",
                    pk,
                    message.get("cluster_id"),
                )
            cluster = Cluster.objects.get(pk=pk)
            cluster.status = message.get("status", "r")
            cluster.save()
            return True

        cluster = get_object_or_404(Cluster, pk=pk)
        cluster.status = "i"
        cluster.save()
        c2.send_command(pk, "SYNC", data={}, on_response=response)

        return HttpResponseRedirect(
            reverse("cluster-detail", kwargs={"pk": pk})
        )

class BackendClusterStatus(LoginRequiredMixin, generic.View):
    """Backend handler for cluster syncing"""

    def get(self, request, pk, *args, **kwargs):
        """
        This handles GET request with parameter pk.
        for example: /backend/cluster-status/50 
        """
        cluster = get_object_or_404(Cluster, pk=pk)
        logger.info(f"Current cluster {pk} status: {cluster.status}")

        return JsonResponse({'status': cluster.status})


class BackendAuthUserGCP(BackendAsyncView):
    """Backend handler to authorise GCP users on the cluster"""

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return cluster

    def cmd(self, task_id, token, cluster, username):
        # from ..cluster_manager.update_cluster import auth_user_gcloud
        # auth_user_gcloud(cluster, token, username, task_id)
        raise NotImplementedError()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_access_to_cluster(request.user, pk)

        cluster = await self.get_orm(pk)
        record = await self.create_task(
            "Auth User GCP", cluster, request.user.username
        )
        return JsonResponse({"taskid": record.id})


class BackendAuthUserGCP2(LoginRequiredMixin, generic.View):
    """Handler for stage 2 of the GCP user auth process"""

    # Process - A "GET" to get started from user's browser
    #     This will send a C2 command to cluster to start the process
    #     Cluster will then respond with a URL for user to visit
    #     We use the 'Task' DB entry to inform client browser of new URL
    #     Client POSTs back to this class the verify key
    #     We use C2 to UPDATE to send to cluster
    #     We get an ACK back from cluster, and delete the Task
    #     Google side should update browser message to show completion

    def get(self, request, pk):
        cluster = get_object_or_404(Cluster, pk=pk)
        user = request.user

        try:
            user_uid = user.socialaccount_set.first().uid
        except AttributeError:
            # User doesn't have a Google SocialAccount.
            messages.error(
                request,
                "You are not signed in with a Google Account. This is required",
            )
            return HttpResponseRedirect(
                reverse("user-gcp-auth", kwargs={"pk": pk})
            )

        logger.info(
            "Beginning User GCS authorization process for %s on %s",
            user,
            cluster.name,
        )
        task = Task.objects.create(
            owner=user,
            title="Auth User GCP",
            data={"status": "Contacting Cluster"},
        )
        task.save()

        cluster_id = cluster.id
        cluster_name = cluster.name
        task_id = task.id

        def callback(message):
            logger.info(
                "GCS Auth Status message received from cluster %s: %s",
                cluster_name,
                message["status"],
            )
            task = Task.objects.get(pk=task_id)
            task.data.update(message)
            task.save()
            if "exit_status" in message:
                logger.info(
                    "Final result from cluster %s for user auth to GCS was "
                    "status code %s",
                    cluster_name,
                    message["exit_status"],
                )
                task.delete()

        message_data = {
            "login_uid": user_uid,
        }
        comm_id = c2.send_command(
            cluster_id,
            "REGISTER_USER_GCS",
            on_response=callback,
            data=message_data,
        )
        task.data["comm_id"] = comm_id
        task.save()

        return JsonResponse({"taskid": task_id})

    def post(self, request, pk):
        cluster = get_object_or_404(Cluster, pk=pk)

        try:
            logger.debug("Received POST from browser for GCS Auth.")
            task_id = request.POST["task_id"]
            task = get_object_or_404(Task, pk=task_id)
            comm_id = request.POST["comm_id"]

            verify_key = request.POST["verify_key"]

            if task.data.get("ackid", None) != comm_id:
                logger.error(
                    "Ack ID mismatch: expected %s, received %s",
                    task.data.get("ackid", None),
                    comm_id,
                )
                return HttpResponseNotFound()

            c2.send_update(cluster.id, comm_id, data={"verify_key": verify_key})

        except KeyError as ke:
            logger.error("Missing POST data", exc_info=ke)
            return HttpResponseNotFound()
        return JsonResponse({})


class AuthUserGCP(LoginRequiredMixin, generic.View):
    """A view keep interactive watch on adding a user's GCS auth creds"""

    def get(self, request, pk):
        cluster = Cluster.objects.get(pk=pk)
        return render(
            request,
            "cluster/user_auth_gcp.html",
            context={"cluster": cluster, "navtab": "cluster"},
        )
