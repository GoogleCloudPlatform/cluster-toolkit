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

from collections import defaultdict
import json
from pathlib import Path
from asgiref.sync import sync_to_async
from rest_framework import viewsets
from rest_framework.authentication import SessionAuthentication, TokenAuthentication
from rest_framework.permissions import IsAuthenticated
from rest_framework.decorators import action
from rest_framework.response import Response
from django.shortcuts import render, redirect, get_object_or_404
from django.db import transaction
from django.db.models import Q
from django.contrib.auth.mixins import LoginRequiredMixin, UserPassesTestMixin
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect, JsonResponse, \
    Http404, HttpResponseBadRequest, HttpResponseNotFound, \
    FileResponse
from django.urls import reverse, reverse_lazy
from django.forms import inlineformset_factory
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from django.conf import settings
from ..models import Application, Cluster, Credential, Job, \
    MachineType, InstanceType, Filesystem, FilesystemExport, MountPoint, \
    FilesystemImpl, Role, ClusterPartition, VirtualSubnet, Task, User
from ..serializers import ClusterSerializer
from ..forms import ClusterForm, ClusterMountPointForm, ClusterPartitionForm
from ..cluster_manager import cloud_info, c2, utils
from ..cluster_manager.clusterinfo import ClusterInfo
from ..views.asyncview import BackendAsyncView
from rest_framework.authtoken.models import Token

from .view_utils import TerraformLogFile, GCSFile, StreamingFileView

import logging
logger = logging.getLogger(__name__)


class ClusterListView(generic.ListView):
    """ Custom ListView for Cluster model """
    model = Cluster
    template_name = 'cluster/list.html'

    def get_queryset(self):
        qs = super().get_queryset()
        wanted_items = set()
        for cluster in qs:
            if self.request.user in cluster.authorised_users.all() and cluster.status == 'r':
                wanted_items.add(cluster.pk)
        return qs.filter(pk__in = wanted_items)

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for cluster in self.get_queryset():
            if (cluster.status == 'c' or cluster.status == 'i' or cluster.status == 't'):
                loading = 1
                break
        admin_view = 0
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(*args, **kwargs)
        context['loading'] = loading
        context['admin_view'] = admin_view
        context['navtab'] = 'cluster'
        return context


class ClusterDetailView(LoginRequiredMixin, generic.DetailView):
    """ Custom DetailView for Cluster model """
    model = Cluster
    template_name = 'cluster/detail.html'

    def get_context_data(self, **kwargs):
        admin_view = 0
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'cluster'
        context['admin_view'] = admin_view
        # Perform extra query to populate instance types data
#        context['cluster_instance_types'] = \
#            ClusterInstanceType.objects.filter(cluster=self.kwargs['pk'])
        return context


class ClusterCreateView1(LoginRequiredMixin, generic.ListView):
    """ Custom view for the first step of cluster creation """
    model = Credential
    template_name = 'credential/select_form.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'cluster'
        return context

    def post(self, request):
        return HttpResponseRedirect(reverse('cluster-create2', kwargs={'credential': request.POST["credential"]}))


class ClusterCreateView2(LoginRequiredMixin, CreateView):
    """ Custom CreateView for Cluster model """

    template_name = 'cluster/create_form.html'
    form_class = ClusterForm

    def populate_MachineTypes(self):
        families = cloud_info.get_machine_families(
                        "GCP", self.object.cloud_credential.detail,
                        self.object.cloud_region, self.object.cloud_zone)

        MachineType.objects.bulk_create(
                [MachineType(name=fam.name, cpu_arch=fam.common_arch)
                    for fam in families],
                ignore_conflicts=True)

        for family in families:
            fam = MachineType.objects.get(name=family.name)
            InstanceType.objects.bulk_create(
                [InstanceType(name=m["name"], family=fam, num_vCPU=m["vCPU"]) for m in family.members],
                ignore_conflicts=True)

    def find_default_instance_type(self):
        # TODO:  Config Parameter???
        return InstanceType.objects.get(name="c2-standard-60")

    def add_default_mounts(self, cluster):
        export = cluster.shared_fs.exports.all()[0]
        mp = MountPoint(export=export, cluster=cluster,
                mount_order=0, mount_options="defaults,nofail,nosuid",
                mount_path="/opt/cluster")
        mp.save()
        export = cluster.shared_fs.exports.all()[1]
        mp = MountPoint(export=export, cluster=cluster,
                mount_order=1, mount_options="defaults,nofail,nosuid",
                mount_path="/home")
        mp.save()

    def get_initial(self):
        return {'cloud_credential': self.cloud_credential}

    def get(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=kwargs['credential'])
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=request.POST['cloud_credential'])
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.owner = self.request.user
        import secrets
        unique_str = secrets.token_hex(4)
        self.object.cloud_id = self.object.name + "-" + unique_str
        self.object.cloud_region = self.object.subnet.cloud_region
        shared_fs = Filesystem(**{
            "name": f"{self.object.name}-sharedfs",
            "cloud_credential": self.object.cloud_credential,
            "cloud_id": self.object.cloud_id,
            "cloud_state": self.object.cloud_state,
            "cloud_region": self.object.cloud_region,
            "cloud_zone": self.object.cloud_zone,
            "subnet": self.object.subnet,
            "fstype": "n",
            "impl_type": FilesystemImpl.BUILT_IN,
            })
        shared_fs.save()
        export = FilesystemExport(filesystem=shared_fs, export_name="/opt/cluster")
        export.save()
        export = FilesystemExport(filesystem=shared_fs, export_name="/home")
        export.save()
        self.object.shared_fs = shared_fs
        self.object.save()
        form.save_m2m()
        # Must make sure machine types are populated before creating default partition
        self.populate_MachineTypes()
        # This MUST come after the self.object being saved, so that it has its ID
        self.add_default_mounts(self.object)
        default_partition = self.object.partitions.create(**{
            "name": "batch",
            "machine_type": self.find_default_instance_type(),
            "max_node_count": 4,   # TODO:  Config parameter?
            })
        self.object.save()
        messages.success(self.request, "A record for this cluster has been created. Click the 'Edit' button to customise it and click 'Create' button to provision the cluster.")
        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        region_info = cloud_info.get_region_zone_info("GCP", self.cloud_credential.detail)
        subnet_regions = {sn.id: sn.cloud_region for sn in VirtualSubnet.objects.filter(cloud_credential=self.cloud_credential).filter(Q(cloud_state="i") | Q(cloud_state="m")).all()}
        context['subnet_regions'] = json.dumps(subnet_regions)
        context['region_info'] = json.dumps(region_info)
        context['navtab'] = 'cluster'
        return context

    def get_success_url(self):
        # Redirect to backend view that creates cluster files
        return reverse('backend-create-cluster', kwargs={'pk': self.object.pk})


class ClusterUpdateView(UpdateView):
    """ Custom UpdateView for Cluster model """

    model = Cluster
    template_name = 'cluster/update_form.html'
    form_class = ClusterForm

    def get_mp_formset(self, **kwargs):
        def formfield_cb(modelField, **kwargs):
            field = modelField.formfield(**kwargs)
            if modelField.name == 'export':
                cluster = self.object
                fsquery = Filesystem.objects    \
                            .exclude(impl_type=FilesystemImpl.BUILT_IN) \
                            .filter(cloud_state__in=['m', 'i']) \
                            .filter(vpc=cluster.subnet.vpc).values_list('pk', flat=True)
                # Add back our cluster's filesystem
                fsystems = list(fsquery) + [cluster.shared_fs.id]
                field.queryset = FilesystemExport.objects.filter(filesystem__in=fsystems)
            return field

        FormClass = inlineformset_factory(
            Cluster, MountPoint,
            form=ClusterMountPointForm,
            formfield_callback=formfield_cb,
            can_delete=True,
            extra=1)

        if self.request.POST:
            kwargs['data'] = self.request.POST
        return FormClass(instance=self.object, **kwargs)

    def get_partition_formset(self, **kwargs):
        def formfield_cb(modelField, **kwargs):
            field = modelField.formfield(**kwargs)
            return field


        FormClass = inlineformset_factory(
            Cluster, ClusterPartition,
            form = ClusterPartitionForm,
            formfield_callback = formfield_cb,
            can_delete = True,
            extra = 1)

        if self.request.POST:
            kwargs['data'] = self.request.POST
        return FormClass(instance=self.object, **kwargs)

    def get_success_url(self):
        # Update the Terraform
        return reverse('backend-update-cluster', kwargs={'pk': self.object.pk})

    def _get_region_info(self):
        if not hasattr(self, 'region_info'):
            self.region_info = cloud_info.get_region_zone_info("GCP", self.get_object().cloud_credential.detail)
        return self.region_info

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs['zone_choices'] = [(x,x) for x in self._get_region_info()[self.get_object().cloud_region]]
        return kwargs

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        subnet_regions = {sn.id: sn.cloud_region for sn in VirtualSubnet.objects.filter(cloud_credential=self.get_object().cloud_credential).all()}
        subnet_regions = {sn.id: sn.cloud_region for sn in VirtualSubnet.objects.filter(cloud_credential=self.get_object().cloud_credential).filter(Q(cloud_state="i") | Q(cloud_state="m")).all()}
        context['subnet_regions'] = json.dumps(subnet_regions)
        context['object'] = self.object
        context['region_info'] = json.dumps(self._get_region_info())
        context['navtab'] = 'cluster'
        context['mountpoints_formset'] = self.get_mp_formset()
        context['cluster_partitions_formset'] = self.get_partition_formset()
        return context

    def form_valid(self, form):
        context = self.get_context_data()
        mountpoints = context['mountpoints_formset']
        partitions = context['cluster_partitions_formset']
        suffix = self.object.cloud_id.split('-')[-1]
        self.object.cloud_id = self.object.name + '-' + suffix

        # Verify formset validity (suprised there's not another method to do this)
        for formset in [mountpoints, partitions]:
            if not formset.is_valid():
                form.add_error(None, "Error in form below")
                return self.form_invalid(form)

        with transaction.atomic():
            self.object = form.save()
            mountpoints.instance = self.object
            mountpoints.save()
            partitions.instance = self.object
            partitions.save()
        msg = "Cluster configuration updated. Click 'Edit' button again to make further changes and click 'Create' button to provision the cluster."
        if (self.object.status == 'r'):
            msg = "Cluster configuration updated. Click 'Edit' button again to make further changes and click 'Sync Cluster' button to apply changes."
        messages.success(self.request, msg)

        # Be kind... Check filesystems to verify all in the same zone as us.
        for mp in self.object.mount_points.exclude(export__filesystem__impl_type=FilesystemImpl.BUILT_IN):
            if mp.export.filesystem.cloud_zone != self.object.cloud_zone:
                messages.warning(self.request, f"Possibly expensive: Filesystem {mp.export.filesystem.name} is in a different zone ({mp.export.filesystem.cloud_zone}) than the cluster!")

        return super().form_valid(form)


class ClusterDeleteView(DeleteView):
    """ Custom DeleteView for Cluster model """

    model = Cluster
    template_name = 'cluster/check_delete.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'cluster'
        return context

    def get_success_url(self):
        cluster = Cluster.objects.get(pk=self.kwargs['pk'])
        messages.success(self.request, f'Cluster {cluster.name} deleted.')
        return reverse('clusters')


class ClusterDestroyView(generic.DetailView):
    """ Custom View to confirm Cluster destroy """

    model = Cluster
    template_name = 'cluster/check_destroy.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        applications = Application.objects.filter(cluster=context['cluster'].id)
        jobs = Job.objects.filter(application__in=applications)
        context['applications'] = applications
        context['jobs'] = jobs
        context['navtab'] = 'cluster'
        return context


class ClusterCostView(generic.DetailView):
    """ Custom view for a cluster's cost analysis """

    model = Cluster
    template_name = 'cluster/cost.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'cluster'

        cluster_users = []
        for user in User.objects.all():
            spend = user.total_spend(cluster_id=context['cluster'].id)
            if spend > 0:
                cluster_users.append((spend, user.total_jobs(cluster_id=context['cluster'].id), user))

        cluster_apps = []
        for app in Application.objects.filter(cluster=context['cluster'].id):
            cluster_apps.append((app.total_spend(), app))


        context['users_by_spend'] = sorted(cluster_users, key=lambda x: x[0], reverse=True)
        context['apps_by_spend'] = sorted(cluster_apps, key=lambda x: x[0], reverse=True)
        return context


class ClusterLogFileView(StreamingFileView):
    bucket = utils.load_config()['server']['gcs_bucket']
    valid_logs = [
        {"title": "Terraform Log", "type": TerraformLogFile, "args": ()},
        {"title": "Startup Log", "type": GCSFile, "args": (bucket, "tmp/setup.log")},
        {"title": "Ansible Sync Log", "type": GCSFile, "args": (bucket, "tmp/ansible.log")},
        {"title": "System Log", "type": GCSFile, "args": (bucket, "var/log/messages")},
        {"title": "Slurm slurmctld.log", "type": GCSFile, "args": (bucket, "var/log/slurm/slurmctld.log")},
        {"title": "Slurm resume.log",    "type": GCSFile, "args": (bucket, "var/log/slurm/resume.log")},
        {"title": "Slurm suspend.log",   "type": GCSFile, "args": (bucket, "var/log/slurm/suspend.log")},
    ]

    def _create_FileInfoObject(self, logFileInfo, *args, **kwargs):
        return logFileInfo["type"](*logFileInfo["args"], *args, **kwargs)

    def get_file_info(self):
        logid = self.kwargs.get('logid', -1)
        cluster_id = self.kwargs.get('pk')
        cluster = get_object_or_404(Cluster, pk=cluster_id)
        ci = ClusterInfo(cluster)
        tf_dir = ci.get_terraform_dir()
        bucket_prefix = f"clusters/{cluster.id}/controller_logs"

        entry = self.valid_logs[logid]
        if entry["type"] == TerraformLogFile:
            extraArgs = [tf_dir]
        elif entry["type"] == GCSFile:
            extraArgs = [bucket_prefix]
        else:
            extraArgs = []
        return self._create_FileInfoObject(entry, *extraArgs)


class ClusterLogView(generic.DetailView):
    """ View to diplay cluster log files """

    model = Cluster
    template_name = 'cluster/log.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['log_files'] = [ { "id": n, "title": entry["title"] }
            for n, entry in enumerate(ClusterLogFileView.valid_logs)
        ]
        context['navtab'] = 'cluster'
        return context


# For APIs

class ClusterViewSet(viewsets.ModelViewSet):
    """ Custom ModelViewSet for Cluster model """
    permission_classes = (IsAuthenticated,)
    #queryset = Cluster.objects.all().order_by('name')
    serializer_class = ClusterSerializer

    def get_queryset(self):
        # cluster admins can see all the clusters
        if Role.CLUSTERADMIN in [x.id for x in self.request.user.roles.all()]:
            queryset = Cluster.objects.all().order_by('name')
        # ordinary user can only see clusters authorised to use
        else:
            queryset = Cluster.objects.filter(authorised_users__id=self.request.user.id).order_by('name')
        return queryset

    @action(methods=['get'], detail=True, permission_classes=[IsAuthenticated])
    def get_users(self, request, pk):
        cluster = self.get_object()
        auth_users = cluster.authorised_users.all()
        return Response([{'username': user.username, 'uid': user.id} for user in auth_users])

    @action(methods=['get'], detail=True, permission_classes=[IsAuthenticated])
    def get_instance_limits(self, request, pk):
        cluster = self.get_object()
        limits = cluster.instance_limits()
        return Response(
            [{'instance_name': entry[0].name, 'nodes': entry[1]} for entry in limits])

    @action(methods=['get'], detail=True, permission_classes=[IsAuthenticated], url_path='filesystem.fact', suffix='.fact')
    def ansible_filesystem(self, request, pk):
        fs_type_translator = {
            ' ' : "none",
            'n' : "nfs",
            'e' : "efs",
            'l' : "lustre",
            'b' : "beegfs",
        }
        cluster = self.get_object()
        mounts = [ { "path": mp.mount_path, "src": mp.mount_source, "fstype": fs_type_translator[mp.fstype], "opts": mp.mount_options } for mp in cluster.mount_points.all() ]
        return JsonResponse({"mounts": mounts})


class InstancePricingViewSet(viewsets.ViewSet):
    permission_classes = (IsAuthenticated,)
    authentication_classes = [SessionAuthentication, TokenAuthentication]
    def retrieve(self, request, pk=None):
        partition = get_object_or_404(ClusterPartition, pk=pk)
        instance_type = partition.machine_type
        cluster = partition.cluster

        price = cloud_info.get_instance_pricing("GCP",
                                                cluster.cloud_credential.detail,
                                                cluster.cloud_region, cluster.cloud_zone,
                                                instance_type.name)
        return JsonResponse({"instance": instance_type.name, "price": price, "currency": "USD"}) #TODO: Currency

    def list(self, request):
        return JsonResponse({})


# Other supporting views

class BackendCreateCluster(BackendAsyncView):
    """ A view to make async call to create a new cluster """

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        creds = cluster.cloud_credential.detail
        return (cluster, creds)

    def cmd(self, task_id, token, cluster, creds):
        ci = ClusterInfo(cluster)
        ci.prepare(creds)

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create Cluster", *args)
        return HttpResponseRedirect(reverse('cluster-detail', kwargs={'pk':pk}))


class BackendUpdateClusterTerraform(BackendAsyncView):
    """ View to apply DB changes to Terraform """
    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return (cluster,)

    def cmd(self, task_id, token, cluster):
        ci = ClusterInfo(cluster)
        ci.update()

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Update Cluster TFVars", *args)
        return HttpResponseRedirect(reverse('cluster-detail', kwargs={'pk':pk}))


class BackendStartCluster(BackendAsyncView):
    """ A view to make async call to create a new cluster """

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return (cluster,)

    def cmd(self, task_id, token, cluster):
        ci = ClusterInfo(cluster)
        ci.start_cluster()

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Start Cluster", *args)
        return HttpResponseRedirect(reverse('cluster-detail', kwargs={'pk':pk}))


class BackendDestroyCluster(BackendAsyncView):
    """ A view to make async call to create a new cluster """
    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return (cluster,)

    def cmd(self, task_id, token, cluster):
        ci = ClusterInfo(cluster)
        ci.stop_cluster()

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Destroy Cluster", *args)
        return HttpResponseRedirect(reverse('cluster-detail', kwargs={'pk': pk}))


class BackendSyncCluster(LoginRequiredMixin, generic.View):

    def get(self, request, pk, *args, **kwargs):
        def response(message):
            logger.info(f"Received SYNC Complete: {message}")
            if message.get('cluster_id') != pk:
                logger.error(f"Cluster ID Mis-match to Callback!  Expected {pk}, Received {message.get('cluster_id')}")
            cluster = Cluster.objects.get(pk=pk)
            cluster.status = message.get('status', 'r')
            cluster.save()
            return True

        cluster = get_object_or_404(Cluster, pk=pk)
        cluster.status = 'i'
        cluster.save()
        c2.send_command(pk, 'SYNC', data={}, onResponse=response)

        return HttpResponseRedirect(reverse('cluster-detail', kwargs={'pk':pk}))


class BackendAuthUserGCP(BackendAsyncView):

    @sync_to_async
    def get_orm(self, cluster_id):
        cluster = Cluster.objects.get(pk=cluster_id)
        return cluster

    def cmd(self, task_id, token, cluster, username):
        #from ..cluster_manager.update_cluster import auth_user_gcloud
        #auth_user_gcloud(cluster, token, username, task_id)
        raise NotImplementedError()

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_access_to_cluster(request.user, pk)

        cluster = await self.get_orm(pk)
        record = await self.create_task("Auth User GCP", cluster, request.user.username)
        return JsonResponse({'taskid': record.id})


class BackendAuthUserGCP2(LoginRequiredMixin, generic.View):

    # Process - A "GET" to get started from user's browser
    #     This will send a C2 command to cluster to start the process
    #     Cluster will then respond with a URL for user to visit
    #     We use the 'Task' DB entry to inform client browser of new URL
    #     Client POSTs back to this class the verify key
    #     We use C2 to UPDATE to send to cluster
    #     We get an ACK back from cluster, and delet the Task - browser sees this as completion

    def get(self, request, pk):
        cluster = get_object_or_404(Cluster, pk=pk)
        user = request.user

        try:
            user_uid = user.socialaccount_set.first().uid
        except AttributeError:
            # User doesn't have a Google SocialAccount.
            messages.error(request, "You are not signed in with a Google Account. This is required")
            return HttpResponseRedirect(reverse('user-gcp-auth', kwargs={'pk':pk}))


        logger.info(f"Beginning User GCS authorization process for {user} on {cluster.name}")
        task = Task.objects.create(owner=user, title='Auth User GCP', data={'status': 'Contacting Cluster'})
        task.save()

        user_db_id = user.id
        cluster_id = cluster.id
        task_id = task.id

        def callback(message):
            logger.info(f"GCS Auth Status message received from cluster: {message['status']}")
            task = Task.objects.get(pk=task_id)
            task.data.update(message)
            task.save()
            if 'exit_status' in message:
                logger.info(f"Final result from Cluster for User Auth to GCS was code {message['exit_status']}")
                task.delete()
            return


        message_data = {
            'login_uid': user_uid,
        }
        comm_id = c2.send_command(cluster_id, 'REGISTER_USER_GCS', onResponse=callback, data=message_data)
        task.data['comm_id'] = comm_id
        task.save()

        return JsonResponse({'taskid': task_id})

    def post(self, request, pk):
        cluster = get_object_or_404(Cluster, pk=pk)
        user = request.user
        try:
            logger.info("Received POST from browser for GCS Auth.")
            task_id = request.POST['task_id']
            task = get_object_or_404(Task, pk=task_id)
            comm_id = request.POST['comm_id']

            verify_key = request.POST['verify_key']

            if task.data.get('ackid', None) != comm_id:
                logger.error(f"AckID mismatch:  {task.data.get('ackid', None)} != {comm_id}")
                return HttpResponseNotFound()

            c2.send_update(cluster.id, comm_id, data={'verify_key': verify_key})

        except KeyError as ke:
            logger.error("Missing POST data", exc_info=ke)
            return HttpResponseNotFound()
        return JsonResponse({})


class AuthUserGCP(LoginRequiredMixin, generic.View):
    """ A view keep interactive watch on adding a user's GCS auth creds """

    def get(self, request, pk):
        cluster = Cluster.objects.get(pk=pk)
        return render(request, 'cluster/user_auth_gcp.html', context={'cluster': cluster, 'navtab': 'cluster'})
