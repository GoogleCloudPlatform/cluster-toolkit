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
""" credentials.py """

from django.http import HttpResponseRedirect, JsonResponse
from django.urls import reverse, reverse_lazy
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from rest_framework import viewsets
from rest_framework.views import APIView
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from rest_framework import status
from ..models import Credential
from ..forms import CredentialForm
from ..serializers import CredentialSerializer
from ..permissions import CredentialPermission, SuperUserRequiredMixin
from ..cluster_manager import validate_credential
from .. import grafana
from grafana_api.grafana_api import GrafanaClientError


class CredentialListView(SuperUserRequiredMixin, generic.ListView):
    """Custom ListView for Credential model"""

    model = Credential
    template_name = "credential/list.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "credential"
        return context


class CredentialDetailView(SuperUserRequiredMixin, generic.DetailView):
    """Custom DetailView for Credential model"""

    model = Credential
    template_name = "credential/detail.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "credential"
        return context


class CredentialCreateView(SuperUserRequiredMixin, CreateView):
    """Custom CreateView for Credential model"""

    success_url = reverse_lazy("credentials")
    template_name = "credential/create_form.html"
    form_class = CredentialForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "credential"
        return context

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.owner = self.request.user

        try:
            grafana.add_gcp_datasource(self.object.name, self.object.detail)
        except GrafanaClientError as e:
            if "Client Error 409: data source with the same name already exists" in str(e):
                # Add an error message to the form
                form.add_error(None, "Credential with the same name already exists or this name can't be used. Please change the name.")
                return self.form_invalid(form)
            else:
                # Handle other GrafanaClientError cases
                messages.error(self.request, f"Grafana Error: {e}")
                raise e  # Raise the error to handle it at a higher level
            
        self.object.save()
        messages.success(self.request, f"Credential {self.object.name} validated and saved.")
        return HttpResponseRedirect(self.get_success_url())

class CredentialUpdateView(SuperUserRequiredMixin, UpdateView):
    """Custom UpdateView for Credential model"""

    model = Credential
    success_url = reverse_lazy("credentials")
    template_name = "credential/update_form.html"
    form_class = CredentialForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "credential"
        return context

    def get_initial(self):
        initial = super().get_initial()
        # do not show existing credential details in edit form
        initial[ "detail" ] = ""
        return initial

class CredentialDeleteView(SuperUserRequiredMixin, DeleteView):
    """Custom DeleteView for Credential model"""

    model = Credential
    template_name = "credential/check_delete.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "credential"
        return context

    def get_success_url(self):
        return reverse("credentials")
    
    def post(self, request, *args, **kwargs):
        try:
            delete_request = super().delete(request, *args, **kwargs)
            messages.success(self.request, f"Credential successfully deleted.")
            return delete_request
        except RestrictedError:
            msg = """Credential deletion failed due to a foreign key constraint.
            It is used by other objects such as clusters. In order to 
            delete the credential, archived objects should be completely deleted.
             """
            messages.error(request, msg)
            return HttpResponseRedirect(reverse('credentials'))

# For APIs


class CredentialViewSet(viewsets.ModelViewSet):
    """Custom ModelViewSet for Crendential model"""

    permission_classes = (
        IsAuthenticated,
        CredentialPermission,
    )
    queryset = Credential.objects.all()
    serializer_class = CredentialSerializer

    def create(self, request):
        request.data._mutable = True  # pylint: disable=protected-access
        request.data["owner"] = request.user.id
        request.data._mutable = False  # pylint: disable=protected-access
        serializer = CredentialSerializer(data=request.data)
        if serializer.is_valid():
            credential = serializer.save()
            data = serializer.data
            data.update({"id": credential.id})
            return JsonResponse(data)
        else:
            print(serializer.errors)
            return Response(
                serializer.errors, status=status.HTTP_400_BAD_REQUEST
            )


class CredentialValidateAPIView(APIView):
    """Validate credential against cloud platform"""

    def post(self, request):
        credential = request.data.__getitem__("detail").rstrip()
        result = validate_credential.validate_credential("GCP", credential)
        res = {"validated": result}
        return JsonResponse(res)
