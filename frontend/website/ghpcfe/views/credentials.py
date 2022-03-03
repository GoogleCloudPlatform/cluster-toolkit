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

from django.contrib.auth.mixins import LoginRequiredMixin
from django.http import HttpResponseRedirect, HttpResponse
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
from ..permissions import CredentialPermission
from ..cluster_manager import validate_credential
import json

class CredentialListView(LoginRequiredMixin, generic.ListView):
    """ Custom ListView for Credential model """
    model = Credential
    template_name = 'credential/list.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'credential'
        return context


class CredentialDetailView(LoginRequiredMixin, generic.DetailView):
    """ Custom DetailView for Credential model """
    model = Credential
    template_name = 'credential/detail.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'credential'
        return context


class CredentialCreateView(LoginRequiredMixin, CreateView):
    """ Custom CreateView for Credential model """

    success_url = reverse_lazy('credentials')
    template_name = 'credential/create_form.html'
    form_class = CredentialForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'credential'
        return context

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.owner = self.request.user
        self.object.save()
        messages.success(self.request, f'Credential {self.object.name} validated and saved.')
        return HttpResponseRedirect(self.get_success_url())


class CredentialUpdateView(LoginRequiredMixin, UpdateView):
    """ Custom UpdateView for Credential model """

    model = Credential
    success_url = reverse_lazy('credentials')
    template_name = 'credential/update_form.html'
    form_class = CredentialForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'credential'
        return context

    def get_initial(self):
        initial = super().get_initial()
        initial['detail'] = ''  # do not show existing credential details in edit form
        return initial


class CredentialDeleteView(LoginRequiredMixin, DeleteView):
    """ Custom DeleteView for Credential model """

    model = Credential
    template_name = 'credential/check_delete.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'credential'
        return context

    def get_success_url(self):
        credential = Credential.objects.get(pk=self.kwargs['pk'])
        messages.success(self.request, f'Credential {credential.name} deleted.')
        return reverse('credentials')


# For APIs

class CredentialViewSet(viewsets.ModelViewSet):
    """ Custom ModelViewSet for Crendential model """
    permission_classes = (IsAuthenticated, CredentialPermission,)
    queryset = Credential.objects.all()
    serializer_class = CredentialSerializer

    def create(self, request):
        request.data._mutable = True
        request.data['owner'] = request.user.id
        request.data._mutable = False
        serializer = CredentialSerializer(data=request.data)
        if serializer.is_valid():
            credential = serializer.save()
            id = credential.id
            data = serializer.data
            data.update({'id': id})
            return HttpResponse(json.dumps(data), content_type='application/json')
        else:
            print(serializer.errors)
            return Response(serializer.errors, status=status.HTTP_400_BAD_REQUEST)


class CredentialValidateAPIView(APIView):
    """ Validte credential against cloud platform """

    def post(self, request, format=None):
        credential = request.data.__getitem__('detail').rstrip()
        result = validate_credential.validate_credential('GCP', credential)
        res = { "validated" : result }
        return HttpResponse(json.dumps(res), content_type='application/json')
