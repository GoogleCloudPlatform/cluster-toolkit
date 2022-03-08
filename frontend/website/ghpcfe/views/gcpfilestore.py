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

""" gcpfilestore.py """

import itertools, json
from asgiref.sync import sync_to_async
from rest_framework import viewsets
from rest_framework.authentication import SessionAuthentication, TokenAuthentication
from rest_framework.permissions import IsAuthenticated
from rest_framework.decorators import action
from rest_framework.response import Response
from django.shortcuts import render, redirect, get_object_or_404
from django.db.models import Q
from django.contrib.auth.mixins import LoginRequiredMixin, UserPassesTestMixin
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect, JsonResponse, \
    Http404, HttpResponseBadRequest
from django.urls import reverse, reverse_lazy
from django.forms import inlineformset_factory
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from ..models import Credential, GCPFilestoreFilesystem, FilesystemImpl, \
    FilesystemExport, VirtualSubnet
from ..forms import FilestoreForm
from ..views.asyncview import BackendAsyncView
from ..cluster_manager import filesystem as cm_fs
from ..cluster_manager import cloud_info


# detail views
class GCPFilestoreFilesystemDetailView(LoginRequiredMixin, generic.DetailView):
    """ Custom DetailView for Filestore  model """
    model = GCPFilestoreFilesystem
    template_name = 'filesystem/filestore_detail.html'

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'fs'
        context['exports'] = FilesystemExport.objects.filter(filesystem=self.kwargs['pk'])
        return context


class GCPFilestoreFilesystemUpdateView(UpdateView):
    """ Custom UpdateView for Filestore model """

    model = GCPFilestoreFilesystem
    template_name = 'filesystem/filestore_update_form.html'
    form_class = FilestoreForm

    def _get_region_info(self):
        if not hasattr(self, 'region_info'):
            self.region_info = cloud_info.get_region_zone_info("GCP", self.get_object().cloud_credential.detail)
        return self.region_info

    def get_success_url(self):
        return reverse('backend-filesystem-update-files', kwargs={'pk': self.object.pk})

    def get_initial(self):
        return {'share_name': self.get_object().exports.all()[0].export_name}

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs['zone_choices'] = [(x,x) for x in self._get_region_info()[self.get_object().cloud_region]]
        return kwargs


    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        subnet_regions = {sn.id: sn.cloud_region for sn in VirtualSubnet.objects.filter(cloud_credential=self.get_object().cloud_credential).filter(Q(cloud_state="i") | Q(cloud_state="m")).all()}

        context = super().get_context_data(**kwargs)

        context['subnet_regions'] = json.dumps(subnet_regions)
        context['region_info'] = json.dumps(self._get_region_info())
        context['navtab'] = 'fs'
        return context


class GCPFilestoreFilesystemCreateView(LoginRequiredMixin, generic.CreateView):
    """ Custom view for Filestore creation """

    template_name = 'filesystem/filestore_create_form.html'
    form_class = FilestoreForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)

        self.region_info = cloud_info.get_region_zone_info("GCP", self.cloud_credential.detail)
        subnet_regions = {sn.id: sn.cloud_region for sn in VirtualSubnet.objects.filter(cloud_credential=self.cloud_credential).filter(Q(cloud_state="i") | Q(cloud_state="m")).all()}

        context['subnet_regions'] = json.dumps(subnet_regions)
        context['region_info'] = json.dumps(self.region_info)
        context['navtab'] = 'fs'
        return context

    def get_initial(self):
        return {'cloud_credential': self.cloud_credential}

    def get(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=kwargs['credential'])
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=kwargs['credential'])
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.cloud_region = self.object.subnet.cloud_region;
        self.object.impl_type = FilesystemImpl.GCPFILESTORE
        self.object.save()

        export = FilesystemExport(filesystem=self.object, export_name=form.data['share_name'])
        export.save()
        return HttpResponseRedirect(self.get_success_url())

    def get_success_url(self):
        # Redirect to backend view that creates cluster files
        return reverse('backend-filesystem-create-files', kwargs={'pk': self.object.pk})
