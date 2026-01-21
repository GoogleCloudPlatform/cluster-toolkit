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
""" vpc.py """

from asgiref.sync import sync_to_async
from rest_framework import viewsets
from rest_framework.permissions import IsAuthenticated
from django.shortcuts import redirect, get_object_or_404
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect
from django.urls import reverse, reverse_lazy
from django.forms import inlineformset_factory
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from ..models import (
    Credential,
    VirtualNetwork,
    VirtualSubnet,
    Cluster,
    Workbench,
    Filesystem,
)
from ..forms import VPCForm, VPCImportForm, VirtualSubnetForm
from ..cluster_manager import cloud_info
from ..cluster_manager.vpc import (
    create_subnet,
    delete_subnet,
    create_vpc,
    start_vpc,
    destroy_vpc,
)
from ..views.asyncview import BackendAsyncView
from ..serializers import VirtualNetworkSerializer, VirtualSubnetSerializer
from ..permissions import SuperUserRequiredMixin
from collections import defaultdict
import json

import logging

logger = logging.getLogger(__name__)


# list view
class VPCListView(SuperUserRequiredMixin, generic.ListView):
    """Custom ListView for VirtualNetwork model"""

    model = VirtualNetwork
    template_name = "vpc/list.html"

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for vpc in VirtualNetwork.objects.all():
            if "c" in vpc.cloud_state or "d" in vpc.cloud_state:
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context["loading"] = loading
        context["navtab"] = "vpc"
        return context


# detail view
class VPCDetailView(SuperUserRequiredMixin, generic.DetailView):
    """Custom DetailView for Virtual Network model"""

    model = VirtualNetwork
    template_name = "vpc/detail.html"

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        context["subnets"] = VirtualSubnet.objects.filter(vpc=self.kwargs["pk"])
        vpc = get_object_or_404(VirtualNetwork, pk=self.kwargs["pk"])

        used_in_clusters = []
        used_in_filesystems = []
        used_in_workbenches = []

        for c in Cluster.objects.exclude(status="d"):
            if vpc == c.subnet.vpc:
                used_in_clusters.append(c)

        for fs in Filesystem.objects.exclude(cloud_state__in=["cm", "m", "dm"]):
            if vpc == fs.vpc:
                used_in_filesystems.append(fs)

        for wb in Workbench.objects.exclude(status="d"):
            if vpc == wb.subnet.vpc:
                used_in_workbenches.append(wb)

        context["used_in_clusters"] = used_in_clusters
        context["used_in_filesystems"] = used_in_filesystems
        context["used_in_workbenches"] = used_in_workbenches
        return context


class VPCCreateView1(SuperUserRequiredMixin, generic.ListView):
    """Custom view for the first step of VPC creation"""

    model = Credential
    template_name = "credential/select_form.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        return context

    def post(self, request):
        return HttpResponseRedirect(
            reverse(
                "vpc-create2", kwargs={"credential": request.POST["credential"]}
            )
        )


class VPCImportView1(SuperUserRequiredMixin, generic.ListView):
    """Custom view for the first step of VPC import"""

    model = Credential
    template_name = "credential/select_form.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        return context

    def post(self, request):
        return HttpResponseRedirect(
            reverse(
                "vpc-import2", kwargs={"credential": request.POST["credential"]}
            )
        )


class VPCCreateView2(SuperUserRequiredMixin, CreateView):
    """Custom CreateView for VirtualNetwork model"""

    template_name = "vpc/create_form.html"
    form_class = VPCForm

    def get_initial(self):
        return {
            "cloud_credential": self.cloud_credential,
            "regions": cloud_info.get_region_zone_info(
                "GCP", self.cloud_credential.detail
            ).keys(),
        }

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
        self.object.save()
        form.save_m2m()
        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        return context

    def get_success_url(self):
        # Redirect to backend view that creates cluster files
        return reverse("backend-create-vpc", kwargs={"pk": self.object.pk})


class VPCImportView2(SuperUserRequiredMixin, CreateView):
    """Custom CreateView for importing externally created VPC"""

    template_name = "vpc/import_form.html"
    form_class = VPCImportForm

    def get_initial(self):
        return {
            "cloud_credential": self.cloud_credential,
            "subnets": [(x[2], x[2]) for x in self.subnet_info],
            "vpc": [(x, x) for x in self.vpc_sub_map.keys()],
        }

    def _setup_data(self, cred_id):
        self.cloud_credential = get_object_or_404(Credential, pk=cred_id)
        self.subnet_info = cloud_info.get_subnets(
            "GCP", self.cloud_credential.detail
        )
        self.vpc_sub_map = defaultdict(list)
        for (vpc, region, subnet, cidr) in self.subnet_info:
            self.vpc_sub_map[vpc].append((subnet, region, cidr))

    def get(self, request, *args, **kwargs):
        self._setup_data(kwargs["credential"])
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self._setup_data(request.POST["cloud_credential"])
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.cloud_state = "i"
        self.object.cloud_id = form.data["vpc"]
        self.object.cloud_region = "N/A"  # GCP VPCs are multi-region
        self.object.save()
        form_subnets = form.cleaned_data["subnets"]

        def add_subnet(unused_vpc_name, region, subnet, cidr):
            vs = VirtualSubnet(
                name=subnet,
                vpc=self.object,
                cidr=cidr,
                cloud_id=subnet,
                cloud_region=region,
                cloud_state="i",
                cloud_credential=self.cloud_credential,
            )
            vs.save()

        for subnet in self.subnet_info:
            if subnet[2] in form_subnets:
                add_subnet(*subnet)

        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["vpc_sub_map"] = json.dumps(self.vpc_sub_map)
        context["navtab"] = "vpc"
        return context

    def get_success_url(self):
        messages.success(self.request, f"VPC {self.object.name} imported.")
        return reverse("vpcs")


class VPCUpdateView(SuperUserRequiredMixin, UpdateView):
    """Custom UpdateView for VirtualNetwork model"""

    model = VirtualNetwork
    template_name = "vpc/update_form.html"
    form_class = VPCForm

    def get_initial(self):
        initial = {
            "regions": cloud_info.get_region_zone_info(
                "GCP", self.get_object().cloud_credential.detail
            ).keys()
        }
        if self.get_object().cloud_state == "i":
            subnet_info = cloud_info.get_subnets(
                "GCP", self.get_object().cloud_credential.detail
            )
            initial["available_subnets"] = [
                (x[2], x[2])
                for x in subnet_info
                if x[0] == self.get_object().cloud_id
            ]
            initial["subnets"] = [
                x.cloud_id for x in self.get_object().subnets.all()
            ]
        return initial

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.save()

        form_subnets = form.cleaned_data["subnets"]

        def add_subnet(unused_vpc_name, region, subnet, cidr):
            vs = VirtualSubnet(
                name=subnet,
                vpc=self.object,
                cidr=cidr,
                cloud_id=subnet,
                cloud_region=region,
                cloud_state="i",
                cloud_credential=self.object.cloud_credential,
            )
            vs.save()

        subnet_info = cloud_info.get_subnets(
            "GCP", self.get_object().cloud_credential.detail
        )
        # Need list of subnets to delete - not selected, but are in DB
        for sn in self.object.subnets.all():
            if sn.cloud_id not in form_subnets:
                logger.info(
                    "Removing subnet %s from VPC %s",
                    sn.cloud_id,
                    self.object.name,
                )
                sn.delete()

        # Need list of subnets to add - selected, but aren't in DB
        for sn in form_subnets:
            logger.debug("Subnet %s has been selected", sn)
            if self.object.subnets.filter(cloud_id=sn).count() == 0:
                logger.debug("Subnet %s not found in DB", sn)
                # Need to find subnet info in 'subnet_info'
                for subnet in subnet_info:
                    if subnet[2] in form_subnets:
                        logger.info(
                            "Adding new subnet %s from VPC %s",
                            subnet[2],
                            self.object.name,
                        )
                        add_subnet(*subnet)
                        break

        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        return context

    def get_success_url(self):
        vpc = VirtualNetwork.objects.get(pk=self.kwargs["pk"])
        messages.success(self.request, f"VPC {vpc.name} updated.")
        return reverse("vpc-detail", kwargs={"pk": self.object.pk})


class VPCDeleteView(SuperUserRequiredMixin, DeleteView):
    """Custom DeleteView for VirtualNetwork model"""

    model = VirtualNetwork
    template_name = "vpc/check_delete.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        return context

    def delete(self, *args, **kwargs):
        vpc = VirtualNetwork.objects.get(pk=self.kwargs["pk"])
        if vpc.in_use():
            messages.add_message(
                self.request,
                messages.ERROR,
                "Cannot delete. This network is referenced in other resources "
                "see VPC details page for more info",
            )
            return redirect("vpcs")
        try:
            vpc.delete()
        except Exception as err:  # pylint: disable=broad-except
            messages.add_message(
                self.request,
                messages.ERROR,
                f"Cannot delete vpc, encountered unknown error {err}",
            )
            return redirect("vpcs")

        messages.success(self.request, f"VPC {vpc.name} deleted.")
        success_url = self.get_success_url()
        return HttpResponseRedirect(success_url)

    def get_success_url(self):
        return reverse("vpcs")


class VPCDestroyView(SuperUserRequiredMixin, generic.DetailView):
    """Custom View to confirm VirtualNetwork destroy"""

    model = VirtualNetwork
    template_name = "vpc/check_destroy.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        subnets = VirtualSubnet.objects.filter(vpc=context["virtualnetwork"].id)
        context["subnets"] = subnets
        context["navtab"] = "vpc"
        return context


class VirtualSubnetView(SuperUserRequiredMixin, generic.TemplateView):
    """Custom view for bulk processing subnets for a VPC"""

    template_name = "vpc/virtual_subnet.html"

    def get_formset(self, region_info):
        def formfield_cb(f, **kwargs):
            if f.name == "cloud_region":
                kwargs["widget"].choices = [(x, x) for x in region_info]
            field = f.formfield(**kwargs)
            return field

        return inlineformset_factory(
            VirtualNetwork,
            VirtualSubnet,
            form=VirtualSubnetForm,
            fk_name="vpc",
            formfield_callback=formfield_cb,
            fields=(
                "name",
                "cidr",
                "cloud_region",
                "private_google_access_enabled"
            ),
            can_delete=True,
            extra=1,
        )

    def get(self, *args, **kwargs):
        vpc = VirtualNetwork.objects.get(pk=kwargs["vpc_id"])
        qset = VirtualSubnet.objects.filter(vpc=vpc)
        region_list = list(
            cloud_info.get_region_zone_info(
                "GCP", vpc.cloud_credential.detail
            ).keys()
        )
        formset = self.get_formset(region_list)(
            queryset=qset,
            instance=vpc,
            initial=[{"cloud_region": vpc.cloud_region}],
        )
        return self.render_to_response(
            {
                "virtual_subnets_formset": formset,
                "vpc": vpc,
            }
        )

    def post(self, *args, **kwargs):

        vpc = VirtualNetwork.objects.get(pk=kwargs["vpc_id"])
        region_list = cloud_info.get_region_zone_info(
            "GCP", vpc.cloud_credential.detail
        ).keys()
        formset = self.get_formset(region_list)(
            data=self.request.POST, instance=vpc
        )
        if formset.is_valid():
            formset.save(commit=False)
            for obj in formset.new_objects:
                obj.cloud_credential = vpc.cloud_credential
                obj.save()
                create_subnet(obj)
            for obj, _ in formset.changed_objects:
                obj.save()
                create_subnet(obj)
            for obj in formset.deleted_objects:
                delete_subnet(obj)
                obj.delete()

            messages.success(
                self.request,
                f'Updated Subnets for VPC {vpc.name}. Click "Edit Subnets" '
                'button again to make further changes and click "Apply Cloud '
                'Changes" button to create the VPC and subnets on the cloud.',
            )
            return redirect(
                reverse_lazy("vpc-detail", kwargs={"pk": kwargs["vpc_id"]})
            )
        return self.render_to_response(
            {"virtual_subnets_formset": formset, "vpc": vpc}
        )

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "vpc"
        return context


# For APIs

# Other supporting views


class BackendCreateVPC(BackendAsyncView):
    """A view to make async call to create a new VirtualNetwork"""

    @sync_to_async
    def get_orm(self, vpc_id):
        vpc = VirtualNetwork.objects.get(pk=vpc_id)
        return (vpc,)

    def cmd(self, unused_task_id, unused_token, vpc):
        create_vpc(vpc)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create VPC", *args)
        return HttpResponseRedirect(reverse("vpc-detail", kwargs={"pk": pk}))


class BackendStartVPC(BackendAsyncView):
    """A view to make async call to create a new VirtualNetwork"""

    @sync_to_async
    def get_orm(self, vpc_id):
        vpc = VirtualNetwork.objects.get(pk=vpc_id)
        vpc.cloud_state = "cm"
        vpc.save()
        return (vpc,)

    def cmd(self, unused_task_id, unused_token, vpc):
        start_vpc(vpc)
        vpc.cloud_state = "m"
        vpc.save()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Start VPC", *args)
        return HttpResponseRedirect(reverse("vpc-detail", kwargs={"pk": pk}))


class BackendDestroyVPC(BackendAsyncView):
    """A view to make async call to destroy a VirtualNetwork"""

    @sync_to_async
    def get_orm(self, vpc_id):
        vpc = VirtualNetwork.objects.get(pk=vpc_id)
        if vpc.in_use():
            messages.add_message(
                self.request,
                messages.ERROR,
                "Cannot destroy - VPC is still referenced by other resources "
                "(see below)",
            )
        else:
            vpc.cloud_state = "dm"
            vpc.save()
        return (vpc,)

    def cmd(self, unused_task_id, unused_token, vpc):

        if not vpc.in_use():
            try:
                destroy_vpc(vpc)
                vpc.cloud_state = "xm"
                vpc.save()
            except Exception as err: # pylint: disable=broad-except
                messages.add_message(
                    self.request,
                    messages.ERROR,
                    f"Cannot destroy VPC - unexpected error ({err})",
                )

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Destroy VPC", *args)
        return HttpResponseRedirect(reverse("vpc-detail", kwargs={"pk": pk}))


class VPCViewSet(viewsets.ReadOnlyModelViewSet):
    """Custom ModelViewSet for VirtualNetwork model"""

    permission_classes = (IsAuthenticated,)
    queryset = VirtualNetwork.objects.all()
    serializer_class = VirtualNetworkSerializer


class VirtualSubnetViewSet(viewsets.ReadOnlyModelViewSet):
    """Custom ModelViewSet for VirtualSubnet model"""

    permission_classes = (IsAuthenticated,)
    queryset = VirtualSubnet.objects.all()
    serializer_class = VirtualSubnetSerializer
