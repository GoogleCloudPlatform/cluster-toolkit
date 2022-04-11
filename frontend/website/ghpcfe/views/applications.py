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

""" applications.py """

from collections import defaultdict
from asgiref.sync import sync_to_async
from rest_framework import viewsets
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from django.contrib.auth.mixins import LoginRequiredMixin, UserPassesTestMixin
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect
from django.urls import reverse, reverse_lazy
from django.views import generic
from django.contrib import messages
from django.shortcuts import get_object_or_404
from ..models import Application, SpackApplication, \
    CustomInstallationApplication, Cluster
from ..serializers import ApplicationSerializer
from ..forms import ApplicationForm, SpackApplicationForm, \
    CustomInstallationApplicationForm, ApplicationEditForm
from ..cluster_manager import spack, cloud_info, c2, utils
from ..cluster_manager.clusterinfo import ClusterInfo
from .asyncview import BackendAsyncView
from rest_framework.authtoken.models import Token
from .view_utils import GCSFile, StreamingFileView

import logging
logger = logging.getLogger(__name__)


class ApplicationListView(generic.ListView):
    """ Custom ListView for Application model """
    model = Application
    template_name = 'application/list.html'

    def get_queryset(self):
        qs = super().get_queryset()
        if self.request.user.has_admin_role():
            pass
        else:
            wanted_items = set()
            for application in qs:
                cluster = application.cluster
                if self.request.user in cluster.authorised_users.all() and cluster.status == 'r' \
                   and application.status == 'r':
                    wanted_items.add(application.pk)
            qs = qs.filter(pk__in = wanted_items)
        for item in qs:
            type = 'pre-installed'
            if hasattr(item, 'spackapplication'):
                type = 'spack'
            elif hasattr(item, 'custominstallationapplication'):
                type = 'custom'
            item.type = type
        return qs

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for application in Application.objects.all():
            if (application.status == 'p' or application.status == 'q' or application.status == 'i'):
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context['loading'] = loading
        context['navtab'] = 'application'
        short_status_messages = {
            "n": "Newly configured",
            "p": "Being prepared",
            "q": "Queueing",
            "i": "Being installed",
            "r": "Installed and ready",
            "e": "Installation failed",
            "x": "Cluster destroyed"
        }
        context['status_messages'] = short_status_messages
        return context


class ApplicationDetailView(generic.DetailView):
    """ Custom DetailView for Application model """
    model = Application
    template_name = 'application/detail.html'

    def get_template_names(self):
        logger.info(f"ApplicationDetailView:  Object type: {type(self.get_object())}")
        if hasattr(self.get_object(), 'spackapplication'):
            return ["application/spack_detail.html"]
        else:
            return super().get_template_names()

    def get_context_data(self, **kwargs):
        admin_view = 0
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'application'
        context['admin_view'] = admin_view
        return context


class ApplicationCreateSelectView(generic.ListView):
    """ Custom view to select application install types """
    model = Cluster
    template_name = 'application/select_form.html'

    def get_queryset(self):
        qs = super().get_queryset()
        return qs.filter(status='r')

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'application'
        return context

    def post(self, request):
        if request.POST["application-type"] == "spack":
            type = 'application-create-spack-cluster'
        elif request.POST["application-type"] == "custom":
            type = 'application-create-install'
        elif request.POST["application-type"] == "installed":
            type = 'application-create'
        return HttpResponseRedirect(reverse(type, kwargs={'cluster': request.POST["cluster"]}))


class ApplicationCreateView(generic.CreateView):
    """ Custom CreateView for Application model """

    success_url = reverse_lazy('applications')
    template_name = 'application/create_form.html'
    form_class = ApplicationForm


    def get_initial(self):
        return {'cluster': Cluster.objects.get(pk=self.kwargs['cluster'])}

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['cluster'] = Cluster.objects.get(pk=self.kwargs['cluster'])
        context['navtab'] = 'application'
        return context

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.status = 'r'
        ci = ClusterInfo(self.object.cluster)
        self.object.install_loc = ci.get_app_install_loc(form.cleaned_data['installation_path'])
        self.object.save()
        return HttpResponseRedirect(self.get_success_url())


class CustomInstallationApplicationCreateView(generic.CreateView):

    template_name = 'application/custom_install_create_form.html'
    form_class = CustomInstallationApplicationForm

    def get_initial(self):
        return {'cluster': Cluster.objects.get(pk=self.kwargs['cluster'])}

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        context['cluster'] = Cluster.objects.get(pk=self.kwargs['cluster'])
        context['navtab'] = 'application'
        return context

    def get_success_url(self):
        return reverse('application-detail', kwargs={'pk': self.object.pk})

    def form_valid(self, form):
        self.object = form.save(commit=False)
        ci = ClusterInfo(self.object.cluster)
        self.object.install_loc = ci.get_app_install_loc(form.cleaned_data['install_loc'])
        if form.cleaned_data['module_name']:
            self.object.load_command = f"module load {module_name}"
        self.object.save()
        messages.success(self.request, f'Application "{self.object.name}" created in database. Click "Install" button below to actually install it on cluster.')
        return HttpResponseRedirect(self.get_success_url())


class SpackApplicationCreateView(generic.CreateView):
    """ Custom CreateView for Application model """

    #success_url = reverse_lazy('applications'})
    template_name = 'application/spack_create_form.html'
    form_class = SpackApplicationForm

    def get_initial(self):
        return {'cluster': Cluster.objects.get(pk=self.kwargs['cluster'])}

    def get_context_data(self, **kwargs):
        """ Perform extra query to populate instance types data """
        context = super().get_context_data(**kwargs)
        context['cluster'] = Cluster.objects.get(pk=self.kwargs['cluster'])
        context['navtab'] = 'application'
        return context

    def get_success_url(self):
        return reverse('application-detail', kwargs={'pk': self.object.pk})

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.install_loc = self.object.cluster.spack_install
        if self.object.version:
            # We need to insert the version immediately following the app name
            # and eventually support compiler...
            self.object.spack_spec = f'@{self.object.version}{self.object.spack_spec if self.object.spack_spec else ""}'
        self.object.save()
        form.save_m2m()
        messages.success(self.request, f'Application "{self.object.name}" created in database. Click "Spack install" button below to actually install it on cluster.')
        return HttpResponseRedirect(self.get_success_url())


class ApplicationUpdateView(generic.UpdateView):
    """ Custom UpdateView for Application model """

    model = Application
    template_name = 'application/edit_form.html'
    form_class = ApplicationEditForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'application'
        return context

    def get_success_url(self):
        return reverse_lazy('application-detail', kwargs={'pk': self.object.pk})


class ApplicationDeleteView(generic.DeleteView):
    """ Custom DeleteView for Application model """

    model = Application
    success_url = reverse_lazy('applications')
    template_name = 'application/check_delete.html'

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['navtab'] = 'application'
        return context


class ApplicationLogFileView(StreamingFileView):
    bucket = utils.load_config()['server']['gcs_bucket']
    valid_logs = [
        {"title": "Installation Output", "type": GCSFile, "args": (bucket, "stdout")},
        {"title": "Installation Error Log", "type": GCSFile, "args": (bucket, "stderr")},
    ]

    def _create_FileInfoObject(self, logFileInfo, *args, **kwargs):
        return logFileInfo["type"](*logFileInfo["args"], *args, **kwargs)

    def get_file_info(self):
        logid = self.kwargs.get('logid', -1)
        application_id = self.kwargs.get('pk')
        application = get_object_or_404(Application, pk=application_id)

        cluster_id = application.cluster.id
        bucket_prefix = f"clusters/{cluster_id}/installs/{application_id}"

        entry = self.valid_logs[logid]
        return self._create_FileInfoObject(entry, *[bucket_prefix])


class ApplicationLogView(generic.DetailView):
    """ View to display application log files """

    model = Application
    template_name = "application/log.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context['log_files'] = [ { "id": n, "title": entry["title"] }
            for n, entry in enumerate(ApplicationLogFileView.valid_logs)
        ]
        context['navtab'] = 'application'
        return context


# For APIs

class ApplicationViewSet(viewsets.ModelViewSet):
    """ Custom ModelViewSet for Application model """
    permission_classes = (IsAuthenticated,)
    queryset = Application.objects.all().order_by('name')
    serializer_class = ApplicationSerializer


class SpackPackageViewSet(viewsets.ViewSet):
    """ Download a list of Spack packages available """
    def list(self, request):
        return Response(spack.get_package_list())
    def retrieve(self, request, pk=None):
        pkgs = spack.get_package_list()
        if pk in pkgs:
            return Response(spack.get_package_info([pk]))
        return Response('Package Not Found', status=404)


# Other supporting views

class BackendCustomAppInstall(LoginRequiredMixin, generic.View):
    def get(self, request, pk):
        app = get_object_or_404(CustomInstallationApplication, pk=pk)
        app.status = 'p'
        app.save()
        cluster_id = app.cluster.id

        def response(message):
            if message.get('cluster_id') != cluster_id:
                logger.error(f"Cluster ID Mis-match to Callback!  Expected {pk}, Received {message.get('cluster_id')}")
            if message.get('app_id') != pk:
                logger.error(f"Application ID Mis-match to Callback!  Expected {pk}, Received {message.get('app_id')}")

            if 'log_message' in message:
                logger.info(f"Install log message:  {message['log_message']}")

            app = Application.objects.get(pk=pk)
            app.status = message['status']
            if message['status'] == 'r':
                # App was installed.  Should have more attributes to set
                pass
            app.save()

        c2.send_command(cluster_id, 'INSTALL_APPLICATION', onResponse=response, data={
            'app_id': app.id,
            'name': app.name,
            'install_script': app.install_script,
            'module_name': app.module_name,
            'module_script': app.module_script,
            'partition': app.install_partition.name,
        })

        return HttpResponseRedirect(reverse('application-detail', kwargs={'pk': pk}))

class BackendSpackInstall(LoginRequiredMixin, generic.View):

    def get(self, request, pk):
        app = get_object_or_404(SpackApplication, pk=pk)
        app.status = 'p'
        app.save()
        cluster_id = app.cluster.id

        def response(message):
            if message.get('cluster_id') != cluster_id:
                logger.error(f"Cluster ID Mis-match to Callback!  Expected {pk}, Received {message.get('cluster_id')}")
            if message.get('app_id') != pk:
                logger.error(f"Application ID Mis-match to Callback!  Expected {pk}, Received {message.get('app_id')}")

            if 'log_message' in message:
                logger.info(f"Install log message:  {message['log_message']}")

            app = Application.objects.get(pk=pk)
            app.status = message['status']
            if message['status'] == 'r':
                # App was installed.  Should have more attributes to set
                app.spack_hash = message.get('spack_hash', '')
                app.load_command = message.get('load_command', '')
                app.installed_architecture = message.get('spack_arch', '')
                app.compiler = message.get('compiler', '')
                app.mpi = message.get('mpi', '')
            app.save()

        c2.send_command(cluster_id, 'SPACK_INSTALL', onResponse=response, data={
            'app_id': app.id,
            'name': app.spack_name,
            'spec': app.spack_spec,
            'partition': app.install_partition.name,
        })
        return HttpResponseRedirect(reverse('application-detail', kwargs={'pk': pk}))
