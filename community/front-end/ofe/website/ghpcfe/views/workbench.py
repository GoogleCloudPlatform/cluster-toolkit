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
""" workbench.py """

import json
from collections import defaultdict
from asgiref.sync import sync_to_async

from django.shortcuts import get_object_or_404
from django.contrib.auth.mixins import LoginRequiredMixin
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect
from django.urls import reverse
from django.views import generic
from django.views.generic.edit import CreateView, DeleteView, UpdateView
from django.contrib import messages
from django.db.models import Q
from django.db import transaction
from django.forms import inlineformset_factory

from ..models import (
    Cluster,
    Credential,
    Workbench,
    VirtualSubnet,
    MountPoint,
    WorkbenchMountPoint,
    Filesystem,
    FilesystemExport,
    FilesystemImpl,
)
from ..forms import WorkbenchForm, WorkbenchMountPointForm
from ..cluster_manager import cloud_info
from ..cluster_manager.workbenchinfo import WorkbenchInfo
from .asyncview import BackendAsyncView


class WorkbenchListView(LoginRequiredMixin, generic.ListView):
    """Custom ListView for Cluster model"""

    model = Workbench
    template_name = "workbench/list.html"

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for cluster in Workbench.objects.all():
            if cluster.status in ["c", "i", "t"]:
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context["loading"] = loading
        context["navtab"] = "workbench"
        return context


class WorkbenchDetailView(LoginRequiredMixin, generic.DetailView):
    """Custom DetailView for Cluster model"""

    model = Workbench
    template_name = "workbench/detail.html"

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "workbench"

        return context


class WorkbenchCreateView1(LoginRequiredMixin, generic.ListView):
    """Custom view for the first step of cluster creation"""

    model = Credential
    template_name = "credential/select_form.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "workbench"
        return context

    def post(self, request):
        return HttpResponseRedirect(
            reverse(
                "workbench-create2",
                kwargs={"credential": request.POST["credential"]},
            )
        )


class WorkbenchCreateView2(LoginRequiredMixin, CreateView):
    """Custom CreateView for Workbench model"""

    template_name = "workbench/create_form.html"
    form_class = WorkbenchForm

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs["cloud_credential"] = self.cloud_credential
        kwargs["user"] = self.request.user
        return kwargs

    def get(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(
            Credential, pk=kwargs["credential"]
        )
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(
            Credential, pk=request.POST["cloud_credential"]
        )
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        mountpoints = []
        if (
            hasattr(self.object, "attached_cluster") and
            self.object.attached_cluster
        ):
            if (
                self.object.subnet.vpc !=
                self.object.attached_cluster.subnet.vpc
            ):
                form.add_error(None, "Cluster and workbench must share a vpc")
                return self.form_invalid(form)

            for cluster_mp in self.object.attached_cluster.mount_points.all():
                wb_mp = WorkbenchMountPoint()
                wb_mp.export = cluster_mp.export
                wb_mp.workbench = self.object
                wb_mp.mount_order = cluster_mp.mount_order
                wb_mp.mount_path = cluster_mp.mount_path
                mountpoints.append(wb_mp)


        self.object.owner = self.request.user
        self.object.cloud_region = self.object.subnet.cloud_region
        self.object.save()
        for wb_mp in mountpoints:
            wb_mp.save()
        form.save_m2m()
        messages.success(
            self.request,
            "A record for this workbench has been created. Please add any "
            "desired storage below.",
        )
        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        region_info = cloud_info.get_region_zone_info(
            "GCP", self.cloud_credential.detail
        )
        subnet_regions = {
            sn.id: sn.cloud_region
            for sn in VirtualSubnet.objects.filter(
                cloud_credential=self.cloud_credential
            )
            .filter(Q(cloud_state="i") | Q(cloud_state="m"))
            .all()
        }

        cluster_subnets = defaultdict(dict)
        for icluster in Cluster.objects.all():
            if icluster.cloud_state == "xm":
                continue
            cluster_subnets[icluster.subnet.id][icluster.id] = icluster.name

        context["cluster_subnets"] = dict(cluster_subnets)
        context["subnet_regions"] = json.dumps(subnet_regions)
        context["region_info"] = json.dumps(region_info)
        context["navtab"] = "workbench"
        return context

    def get_success_url(self):
        # Redirect to backend view that creates cluster files
        return reverse(
            "backend-create-workbench", kwargs={"pk": self.object.pk}
        )


class WorkbenchUpdate(LoginRequiredMixin, UpdateView):
    """Custom DetailView for Cluster model"""

    model = Workbench
    template_name = "workbench/update.html"
    form_class = WorkbenchForm

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs["user"] = self.request.user
        return kwargs

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "workbench"

        context["mountpoints_formset"] = self.get_mp_formset()
        return context

    def get_mp_formset(self, **kwargs):
        def formfield_cb(model_field, **kwargs):
            field = model_field.formfield(**kwargs)
            if model_field.name == "export":
                workbench = self.object
                fsquery = list(
                    Filesystem.objects.all()
                    .exclude(
                        impl_type=FilesystemImpl.BUILT_IN
                    )
                    .filter(cloud_state__in=["m", "i"])
                    .filter(vpc=workbench.subnet.vpc)
                    .values_list("pk", flat=True)
                )
                export_qs = FilesystemExport.objects.filter(
                   filesystem__in=fsquery
                )
                if hasattr(self.object, "attached_cluster"):
                    exp_ids = [x.export.id for x in MountPoint.objects.filter(
                        cluster=self.object.attached_cluster
                    )]
                    export_qs = export_qs | FilesystemExport.objects.filter(
                            id__in=exp_ids
                    )

                field.queryset = export_qs

            return field

        # This creates a new class on the fly
        FormClass = inlineformset_factory(  # pylint: disable=invalid-name
            Workbench,
            WorkbenchMountPoint,
            form=WorkbenchMountPointForm,
            formfield_callback=formfield_cb,
            can_delete=True,
            extra=1,
        )

        if self.request.POST:
            kwargs["data"] = self.request.POST
        return FormClass(instance=self.object, **kwargs)

    def get_success_url(self):
        # Update the Terraform
        return reverse(
            "backend-update-workbench", kwargs={"pk": self.object.pk}
        )

    def form_valid(self, form):
        context = self.get_context_data()
        workbenchmountpoints = context["mountpoints_formset"]

        # Verify formset validity (surprised there's no method to do this)
        for formset in workbenchmountpoints:
            if not formset.is_valid():
                for error in formset.errors:
                    form.add_error(None, error)
                return self.form_invalid(form)

        with transaction.atomic():
            self.object = form.save()
            workbenchmountpoints.instance = self.object
            workbenchmountpoints.save()
        msg = (
            "Workbench configuration updated. Click 'create' to provision "
            "the workbench"
        )
        messages.success(self.request, msg)
        return super().form_valid(form)


class WorkbenchDeleteView(LoginRequiredMixin, DeleteView):
    """Custom DeleteView for Workbench model"""

    model = Workbench
    template_name = "workbench/check_delete.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "workbench"
        return context

    def get_success_url(self):
        workbench = Workbench.objects.get(pk=self.kwargs["pk"])
        messages.success(self.request, f"workbench {workbench.name} deleted.")
        return reverse("workbench")


class WorkbenchDestroyView(LoginRequiredMixin, generic.DetailView):
    """Custom View to confirm Workbench destroy"""

    model = Workbench
    template_name = "workbench/check_destroy.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "workbench"
        return context


class BackendCreateWorkbench(BackendAsyncView):
    """A view to make async call to create a new cluster"""

    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        creds = workbench.cloud_credential.detail
        return (workbench, creds)

    def cmd(self, unused_task_id, unused_token, workbench, creds):

        WorkbenchInfo(workbench).create_workbench_dir(creds)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create Workbench", *args)
        return HttpResponseRedirect(
            reverse("workbench-update", kwargs={"pk": pk})
        )


class BackendStartWorkbench(BackendAsyncView):
    """A view to make async call to create a new cluster"""

    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        return (workbench,)

    def cmd(self, unused_task_id, unused_token, workbench):

        WorkbenchInfo(workbench).start()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Start Workbench", *args)
        return HttpResponseRedirect(
            reverse("workbench-detail", kwargs={"pk": pk})
        )


class BackendDestroyWorkbench(BackendAsyncView):
    """Backend handler for workbench teardown"""

    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        return (workbench,)

    def cmd(self, unused_task_id, unused_token, workbench):

        WorkbenchInfo(workbench).terminate()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Destroy workbench", *args)
        return HttpResponseRedirect(
            reverse("workbench-detail", kwargs={"pk": pk})
        )


class BackendUpdateWorkbench(BackendAsyncView):
    """A view to make async call to create a new cluster"""

    @sync_to_async
    def get_orm(self, workbench_id):
        workbench = Workbench.objects.get(pk=workbench_id)
        return (workbench,)

    def cmd(self, unused_task_id, unused_token, workbench):

        WorkbenchInfo(workbench).copy_startup_script()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Update Workbench", *args)
        return HttpResponseRedirect(
            reverse("workbench-detail", kwargs={"pk": pk})
        )
