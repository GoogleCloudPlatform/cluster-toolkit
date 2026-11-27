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
""" gcpfilestore.py """

from django.shortcuts import get_object_or_404
from django.http import HttpResponseRedirect
from django.urls import reverse
from django.views import generic
from django.views.generic.edit import UpdateView
from .view_utils import cmek_detail_context
from ..permissions import SuperUserRequiredMixin
from ..cluster_manager import cmek
from ..models import (
    Credential,
    GCPFilestoreFilesystem,
    FilesystemImpl,
    FilesystemExport,
)
from ..forms import FilestoreForm


# detail views
class GCPFilestoreFilesystemDetailView(SuperUserRequiredMixin,
                                       generic.DetailView):
    """Custom DetailView for Filestore  model"""

    model = GCPFilestoreFilesystem
    template_name = "filesystem/filestore_detail.html"

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        context["cmek_detail"] = cmek_detail_context(self.object)
        context["exports"] = FilesystemExport.objects.filter(
            filesystem=self.kwargs["pk"]
        )
        return context


class GCPFilestoreFilesystemUpdateView(SuperUserRequiredMixin, UpdateView):
    """Custom UpdateView for Filestore model"""

    model = GCPFilestoreFilesystem
    template_name = "filesystem/filestore_update_form.html"
    form_class = FilestoreForm

    def get_success_url(self):
        return reverse(
            "backend-filesystem-update-files", kwargs={"pk": self.object.pk}
        )

    def get_initial(self):
        return {"share_name": self.get_object().exports.first().export_name}

    def form_valid(self, form):
        # A revoked key is final: nothing about the Filestore is saved, so
        # the record keeps describing what was deployed.
        try:
            cmek.assert_not_revoked(form.instance)
        except cmek.CmekConfigError as err:
            form.add_error(None, str(err))
            return self.form_invalid(form)

        self.object = form.save(commit=False)
        self.object.cloud_region = self.object.cloud_zone.rsplit("-", 1)[0]
        self.object.impl_type = FilesystemImpl.GCPFILESTORE
        self.object.save()

        # The form disables share_name once the Filestore is deployed, so take
        # it from cleaned_data: that holds the stored name, not the POST.
        export = self.object.exports.first()
        export.export_name = form.cleaned_data["share_name"]
        export.save()

        return HttpResponseRedirect(self.get_success_url())

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        return context


class GCPFilestoreFilesystemCreateView(SuperUserRequiredMixin,
                                       generic.CreateView):
    """Custom view for Filestore creation"""

    template_name = "filesystem/filestore_create_form.html"
    form_class = FilestoreForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        return context

    def get_initial(self):
        return {"cloud_credential": self.cloud_credential}

    def get(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(
            Credential, pk=kwargs["credential"]
        )
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(
            Credential, pk=kwargs["credential"]
        )
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.cloud_region = self.object.cloud_zone.rsplit("-", 1)[0]
        self.object.impl_type = FilesystemImpl.GCPFILESTORE
        self.object.save()

        export = FilesystemExport(
            filesystem=self.object, export_name=form.data["share_name"]
        )
        export.save()
        return HttpResponseRedirect(self.get_success_url())

    def get_success_url(self):
        # Redirect to backend view that creates cluster files
        return reverse(
            "backend-filesystem-create-files", kwargs={"pk": self.object.pk}
        )
