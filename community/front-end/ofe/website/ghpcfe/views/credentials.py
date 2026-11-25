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
""" credentials.py """

import logging
from collections.abc import Mapping

from django.http import HttpResponseRedirect, JsonResponse
from django.urls import reverse, reverse_lazy
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from django.db import transaction
from django.db.models import RestrictedError
from rest_framework import mixins, viewsets
from rest_framework.views import APIView
from rest_framework.permissions import BasePermission, IsAuthenticated
from rest_framework.response import Response
from rest_framework import status
from ..models import Credential
from ..forms import CredentialForm
from ..serializers import CredentialSerializer
from ..permissions import CredentialPermission, SuperUserRequiredMixin
from ..cluster_manager import credentials as credential_store
from ..cluster_manager import validate_credential
from .. import grafana
from grafana_api.grafana_api import GrafanaClientError

logger = logging.getLogger(__name__)

# Shown when a credential cannot be saved because sealing it needs the
# database key and the server cannot read that key
DATABASE_KEY_UNREADABLE_MESSAGE = (
    "TKFE cannot read its database key; re-run deploy.sh (see Upgrading in "
    "the admin guide) and try again."
)


def _database_key_unreadable(form, err):
    """Add the form error for a save that failed because the database key
    cannot be read (err, a CredentialError), which is logged"""
    logger.warning("Credential not saved: %s", err)
    form.add_error(None, DATABASE_KEY_UNREADABLE_MESSAGE)


def _warn_on_state_bucket_access(request, credential):
    """Warn, without blocking, if a credential cannot use the state bucket."""
    try:
        warning = validate_credential.check_state_bucket_access(credential)
    except Exception:  # pylint: disable=broad-except
        logger.exception("Could not check the credential's state bucket access")
        warning = (
            "Could not check whether this credential can use the Terraform "
            "state bucket; see the server log."
        )
    if warning:
        messages.warning(request, warning)


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
        try:
            context["identity_email"] = credential_store.identity_email(
                self.object
            )
        except (credential_store.CredentialError, ValueError) as err:
            # ValueError: stored text that is not a token at all. Its text
            # can quote a character of the stored text, so only its type is
            # logged; CredentialError's own text names no key.
            if isinstance(err, credential_store.CredentialError):
                reason = str(err)
            else:
                reason = (
                    "stored text is not a valid token "
                    f"({type(err).__name__})"
                )
            logger.warning(
                "Credential %s cannot be read: %s", self.object.pk, reason
            )
            context["identity_email"] = "(cannot be read)"
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
            # Saved first, in one transaction with the datasource: a save
            # that fails (for example without the database key) creates no
            # datasource, and a datasource that cannot be created leaves no
            # credential
            with transaction.atomic():
                self.object.save()
                # A key credential gets its own datasource; one without a key
                # uses the server's GCE datasource
                grafana.add_gcp_datasource(self.object)
        except credential_store.CredentialError as err:
            # Sealing the key needs the database key; nothing was saved
            self.object.pk = None
            _database_key_unreadable(form, err)
            return self.form_invalid(form)
        except GrafanaClientError as e:
            # The row was rolled back
            self.object.pk = None
            if "Client Error 409: data source with the same name already exists" in str(e):
                # Add an error message to the form
                form.add_error(None, "Credential with the same name already exists or this name can't be used. Please change the name.")
                return self.form_invalid(form)
            else:
                # Handle other GrafanaClientError cases
                messages.error(self.request, f"Grafana Error: {e}")
                raise e  # Raise the error to handle it at a higher level

        messages.success(self.request, f"Credential {self.object.name} validated and saved.")
        _warn_on_state_bucket_access(self.request, self.object)
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

    def form_valid(self, form):
        try:
            response = super().form_valid(form)
        except credential_store.CredentialError as err:
            # Credential.save seals the key before writing, so a database
            # key that cannot be read leaves the row as it was
            _database_key_unreadable(form, err)
            return self.form_invalid(form)
        _warn_on_state_bucket_access(self.request, self.object)
        return response

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
            messages.success(self.request, "Credential successfully deleted.")
            return delete_request
        except RestrictedError:
            msg = """Credential deletion failed due to a foreign key constraint.
            It is used by other objects such as clusters. In order to 
            delete the credential, archived objects should be completely deleted.
             """
            messages.error(request, msg)
            return HttpResponseRedirect(reverse('credentials'))

# For APIs


class SuperUserOnly(BasePermission):
    """Only a superuser, as on the credential pages"""

    message = "Only a superuser can create a credential"

    def has_permission(self, request, view):  # pylint: disable=unused-argument
        return bool(request.user and request.user.is_superuser)


def _form_errors(form):
    """A form's errors as the API returns them: each field's messages, and
    those of no one field under non_field_errors"""
    return {
        ("non_field_errors" if field == "__all__" else field): list(errors)
        for field, errors in form.errors.items()
    }


class CredentialViewSet(
    mixins.ListModelMixin,
    mixins.RetrieveModelMixin,
    mixins.CreateModelMixin,
    viewsets.GenericViewSet,
):
    """Lists, shows and creates credentials.

    PUT, PATCH and DELETE are not allowed: a credential is changed or
    deleted only on its pages, by a superuser. Creating one also needs a
    superuser and runs the credential page's form, so it is checked and
    its key sealed as on that page.
    """

    permission_classes = (
        IsAuthenticated,
        CredentialPermission,
    )
    queryset = Credential.objects.all()
    serializer_class = CredentialSerializer

    def get_permissions(self):
        if self.action == "create":
            # As on the credential pages, a superuser needs no cluster-admin
            # role, and a cluster admin who is not a superuser cannot create
            return [IsAuthenticated(), SuperUserOnly()]
        return super().get_permissions()

    def create(self, request, *args, **kwargs):
        if not isinstance(request.data, Mapping):
            # A JSON body that is a list, a string, a number or null
            return Response(
                {
                    "non_field_errors": [
                        "The request body must be a JSON object or form "
                        "data holding the credential's fields."
                    ]
                },
                status=status.HTTP_400_BAD_REQUEST,
            )
        # The credential page's fields. Without a kind the request uploads
        # a key, as the command-line client's does.
        data = {
            name: request.data.get(name)
            for name in CredentialForm._meta.fields  # pylint: disable=protected-access
            if request.data.get(name) is not None
        }
        data.setdefault("kind", credential_store.KIND_KEY)
        form = CredentialForm(data=data)
        if not form.is_valid():
            return Response(
                _form_errors(form), status=status.HTTP_400_BAD_REQUEST
            )
        credential = form.save(commit=False)
        credential.owner = request.user
        try:
            # Credential.save seals the key first; nothing is saved if the
            # database key cannot be read
            credential.save()
        except credential_store.CredentialError as err:
            _database_key_unreadable(form, err)
            return Response(
                _form_errors(form), status=status.HTTP_503_SERVICE_UNAVAILABLE
            )
        return Response(self.get_serializer(credential).data)


class CredentialValidateAPIView(APIView):
    """Validate credential against cloud platform"""

    def post(self, request):
        credential = request.data.__getitem__("detail").rstrip()
        result = validate_credential.validate_credential("GCP", credential)
        res = {"validated": result}
        return JsonResponse(res)
