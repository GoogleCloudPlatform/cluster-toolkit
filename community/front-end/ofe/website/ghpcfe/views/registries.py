# Copyright 2025 "Google LLC"
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
from django.http import HttpResponseRedirect
from django.urls import reverse
from django.shortcuts import get_object_or_404
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from ..models import ContainerRegistry
from ..forms import ContainerRegistryForm
from ..permissions import SuperUserRequiredMixin


class RegistryListView(SuperUserRequiredMixin, generic.ListView):
    """List view for container registries"""

    model = ContainerRegistry
    template_name = "registries/list.html"
    context_object_name = "registry_list"

    def get_context_data(self, *args, **kwargs):
        context = super().get_context_data(*args, **kwargs)
        context["navtab"] = "registries"
        return context


class RegistryDetailView(SuperUserRequiredMixin, generic.DetailView):
    """Detail view for a container registry"""

    model = ContainerRegistry
    template_name = "registries/detail.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "registries"
        return context


class RegistryCreateView(SuperUserRequiredMixin, CreateView):
    """Create view for a new container registry"""

    model = ContainerRegistry
    form_class = ContainerRegistryForm
    template_name = "registries/create_form.html"

    def form_valid(self, form):
        messages.success(self.request, "Container Registry successfully created.")
        return super().form_valid(form)

    def get_success_url(self):
        return reverse("registries")


class RegistryUpdateView(SuperUserRequiredMixin, UpdateView):
    """Update view for an existing container registry"""

    model = ContainerRegistry
    form_class = ContainerRegistryForm
    template_name = "registries/update_form.html"

    def form_valid(self, form):
        messages.success(self.request, "Container Registry successfully updated.")
        return super().form_valid(form)

    def get_success_url(self):
        return reverse("registries")


class RegistryDeleteView(SuperUserRequiredMixin, DeleteView):
    """Delete view for a container registry"""

    model = ContainerRegistry
    template_name = "registries/delete_confirm.html"

    def get_success_url(self):
        messages.success(self.request, "Container Registry successfully deleted.")
        return reverse("registries")
