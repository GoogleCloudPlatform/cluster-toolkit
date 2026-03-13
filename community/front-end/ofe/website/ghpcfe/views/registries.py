# Copyright 2026 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

""" registries.py """

from django.contrib import messages
from django.http import HttpResponseRedirect, JsonResponse, HttpResponse
from django.urls import reverse
from django.shortcuts import get_object_or_404
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.template.loader import render_to_string
from ..models import ContainerRegistry, Cluster
from ..forms import ContainerRegistryForm, PullContainerForm
from ..permissions import LoginRequiredMixin
from google.cloud import artifactregistry_v1
from .view_utils import RegistryDataHelper
import logging
import ast

logger = logging.getLogger(__name__)

""" registries.py """


class RegistryListView(LoginRequiredMixin, generic.ListView):
    """List view for Artifact Registries"""

    model = ContainerRegistry
    template_name = "registry/list.html"
    context_object_name = "registry_list"

    def get_context_data(self, *args, **kwargs):
        context = super().get_context_data(*args, **kwargs)
        loading = 0
        for registry in self.get_queryset():
            if hasattr(registry, "status") and (registry.status in ["n", "c", "i", "t"]):
                loading = 1
                break
        context["loading"] = loading
        context["navtab"] = "registry"
        context["cluster_list"] = Cluster.objects.all()
        return context


class RegistryDetailView(LoginRequiredMixin, generic.DetailView):
    model = ContainerRegistry
    template_name = "registry/detail.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        registry = self.get_object()

        logger.info(f"Fetching details for registry ID {registry.id}, repo_mode={registry.repo_mode}")

        helper = RegistryDataHelper(self.request, registry)
        container_images, loading = helper.get_data()

        if registry.repo_mode == "STANDARD_REPOSITORY":
            context["pull_form"] = PullContainerForm()

        context["container_images"] = container_images
        context["loading"] = loading
        context["navtab"] = "registry"

        return context


class RegistryContainersView(LoginRequiredMixin, generic.ListView):
    def get(self, request, registry_id):
        registry = get_object_or_404(ContainerRegistry, id=registry_id)
        helper = RegistryDataHelper(request, registry)
        artifacts, _ = helper.get_data()

        html = render_to_string(
            "registry/_container_rows.html", 
            {
                "container_images": artifacts,
                "object": registry,
            }, 
            request=request
        )

        return HttpResponse(html)


# # # # # # # # 

# Not implemented (yet?):
class RegistryCreateView(LoginRequiredMixin, CreateView):
    """Create view for a new Artifact Registry (not-implemented)"""

    model = ContainerRegistry
    form_class = ContainerRegistryForm
    template_name = "registry/create_form.html"

    def form_valid(self, form):
        messages.success(self.request, "Container Registry successfully created.")
        return super().form_valid(form)

    def get_success_url(self):
        return reverse("registry")


# Not implemented (yet?):
class RegistryUpdateView(LoginRequiredMixin, UpdateView):
    """Update view for an existing Artifact Registry (not-implemented)"""

    model = ContainerRegistry
    form_class = ContainerRegistryForm
    template_name = "registry/update_form.html"

    def form_valid(self, form):
        messages.success(self.request, "Artifact Registry successfully updated.")
        return super().form_valid(form)

    def get_success_url(self):
        return reverse("registry")


# Not implemented (yet?):
class RegistryDeleteView(LoginRequiredMixin, DeleteView):
    """Delete view for an Artifact Registry"""

    model = ContainerRegistry
    template_name = "registry/delete_confirm.html"

    def get_success_url(self):
        messages.success(self.request, "Artifact Registry successfully deleted.")
        return reverse("registry")
