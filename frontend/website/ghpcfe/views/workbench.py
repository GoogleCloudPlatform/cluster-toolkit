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

""" workbench.py """
import json
from asgiref.sync import sync_to_async

from django.shortcuts import get_object_or_404
from django.contrib.auth.mixins import LoginRequiredMixin
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect, HttpResponseBadRequest
from django.urls import reverse
from django.views import generic
from django.views.generic.edit import CreateView, DeleteView
from django.contrib import messages
from django.db.models import Q
from ..models import Credential, Workbench, VirtualSubnet
from ..forms import WorkbenchForm
from ..cluster_manager import cloud_info, workbenchinfo
from .asyncview import BackendAsyncView


class WorkbenchListView(LoginRequiredMixin, generic.ListView):
    """ Custom ListView for Cluster model """
    model = Workbench
    template_name = 'workbench/list.html'

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for cluster in Workbench.objects.all():
            if (cluster.status == 'c' or cluster.status == 'i' or cluster.status == 't'):
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context['loading'] = loading
        context['navtab'] = 'workbench'
        return context


class WorkbenchDetailView(LoginRequiredMixin, generic.DetailView):
    """ Custom DetailView for Cluster model """
    model = Workbench
    template_name = 'workbench/detail.html'

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'workbench'
        workbench_info = workbenchinfo.WorkbenchInfo(context['object'])

        if (context['object'].proxy_uri == "" or context['object'].status == 'c' or context['object'].status == 'i'):
            workbench_info.get_workbench_proxy_uri()

        return context


class WorkbenchCreateView1(LoginRequiredMixin, generic.ListView):
    """ Custom view for the first step of cluster creation """
    model = Credential
    template_name = 'credential/select_form.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'workbench'
        return context

    def post(self, request):
        return HttpResponseRedirect(reverse('workbench-create2', kwargs={'credential': request.POST["credential"]}))


class WorkbenchCreateView2(LoginRequiredMixin, CreateView):
    """ Custom CreateView for Workbench model """

    template_name = 'workbench/create_form.html'
    form_class = WorkbenchForm

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs['cloud_credential'] = self.cloud_credential
        kwargs['user'] = self.request.user
        return kwargs

    def get(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=kwargs['credential'])
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=request.POST['cloud_credential'])
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.owner = self.request.user
        self.object.cloud_region = self.object.subnet.cloud_region;
        self.object.cloud_zone = self.object.subnet.cloud_zone;
        self.object.save()
        form.save_m2m()
        messages.success(self.request, "A record for this workbench has been created. Click the 'Edit' button to customise it.")
        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        region_info = cloud_info.get_region_zone_info("GCP", self.cloud_credential.detail)
        subnet_regions = {sn.id: sn.cloud_region for sn in VirtualSubnet.objects.filter(cloud_credential=self.cloud_credential).filter(Q(cloud_state="i") | Q(cloud_state="m")).all()}
        context['subnet_regions'] = json.dumps(subnet_regions)
        context['region_info'] = json.dumps(region_info)
        context['navtab'] = 'Workbench'
        return context

    def get_success_url(self):
        # Redirect to backend view that creates cluster files
        return reverse('backend-create-workbench', kwargs={'pk': self.object.pk})

class WorkbenchDeleteView(LoginRequiredMixin, DeleteView):
    """ Custom DeleteView for Workbench model """

    model = Workbench
    template_name = 'workbench/check_delete.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'workbench'
        return context

    def get_success_url(self):
        workbench = Workbench.objects.get(pk=self.kwargs['pk'])
        messages.success(self.request, f'workbench {workbench.name} deleted.')
        return reverse('workbench')


class WorkbenchDestroyView(LoginRequiredMixin, generic.DetailView):
    """ Custom View to confirm Workbench destroy """

    model = Workbench
    template_name = 'workbench/check_destroy.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'workbench'
        return context

class BackendCreateWorkbench(BackendAsyncView):
    """ A view to make async call to create a new cluster """

    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        creds = workbench.cloud_credential.detail
        return (workbench, creds)


    def cmd(self, task_id, token, workbench, creds):
        from ..cluster_manager.create_workbench import create_workbench
        create_workbench(workbench, token, credentials=creds)

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create Workbench", *args)
        return HttpResponseRedirect(reverse('workbench-detail', kwargs={'pk':pk}))

class BackendStartWorkbench(BackendAsyncView):
    """ A view to make async call to create a new cluster """

    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        return (workbench,)


    def cmd(self, task_id, token, workbench):
        from cluster_manager.start_workbench import start_workbench
        start_workbench(workbench, token)


    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Start Workbench", *args)
        return HttpResponseRedirect(reverse('workbench-detail', kwargs={'pk':pk}))


class BackendDestroyWorkbench(BackendAsyncView):
    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        return (workbench,)

    def cmd(self, task_id, token, workbench):
        from cluster_manager.destroy_workbench import destroy_workbench
        destroy_workbench(workbench, token)

    async def get(self, request, pk):
        """ this will invoke the background tasks and return immediately """
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Destroy workbench", *args)
        return HttpResponseRedirect(reverse('workbench-detail', kwargs={'pk': pk}))
