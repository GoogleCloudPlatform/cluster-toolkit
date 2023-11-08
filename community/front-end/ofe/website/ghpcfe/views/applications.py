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
import logging

from django.contrib import messages
from django.contrib.auth.mixins import LoginRequiredMixin
from django.http import HttpResponseRedirect
from django.shortcuts import get_object_or_404
from django.urls import reverse
from django.urls import reverse_lazy
from django.views import generic
from rest_framework import viewsets
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response

from ..cluster_manager import c2
from ..cluster_manager import spack
from ..cluster_manager import utils
from ..cluster_manager.clusterinfo import ClusterInfo
from ..forms import ApplicationEditForm
from ..forms import ApplicationForm
from ..forms import CustomInstallationApplicationForm
from ..forms import SpackApplicationForm
from ..models import Application
from ..models import Cluster
from ..models import CustomInstallationApplication
from ..models import SpackApplication
from ..serializers import ApplicationSerializer
from .view_utils import GCSFile
from .view_utils import StreamingFileView

logger = logging.getLogger(__name__)


class ApplicationListView(LoginRequiredMixin, generic.ListView):
    """Custom ListView for Application model"""

    model = Application
    template_name = "application/list.html"

    def get_queryset(self):
        queryset = super().get_queryset()
        if self.request.user.has_admin_role():
            pass
        else:
            wanted_items = set()
            for application in queryset:
                cluster = application.cluster
                if (
                    self.request.user in cluster.authorised_users.all()
                    and cluster.status == "r"
                    and application.status == "r"
                ):
                    wanted_items.add(application.pk)
            queryset = queryset.filter(pk__in=wanted_items)
        for item in queryset:
            if hasattr(item, "spackapplication"):
                item.type = "spack"
            elif hasattr(item, "custominstallationapplication"):
                item.type = "custom"
            else:
                item.type = "pre-installed"
        return queryset

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for application in Application.objects.all():
            if application.status in ["p", "q", "i"]:
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context["loading"] = loading
        context["navtab"] = "application"
        short_status_messages = {
            "n": "Newly configured",
            "p": "Being prepared",
            "q": "Queueing",
            "i": "Being installed",
            "r": "Installed and ready",
            "e": "Installation failed",
            "x": "Cluster destroyed",
        }
        context["status_messages"] = short_status_messages
        return context


class ApplicationDetailView(generic.DetailView):
    """Custom DetailView for Application model"""

    model = Application
    template_name = "application/detail.html"

    def get_template_names(self):
        logger.debug(
            "ApplicationDetailView:  Object type: %s", type(self.get_object())
        )
        if hasattr(self.get_object(), "spackapplication"):
            return ["application/spack_detail.html"]
        return super().get_template_names()

    def get_context_data(self, **kwargs):
        admin_view = 0
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(**kwargs)
        if hasattr(self.get_object(), "spackapplication"):
            spack_application = SpackApplication.objects.get(
                pk=context["application"].id
            )
            context["application"].spack_spec = spack_application.spack_spec
            load = context["application"].load_command
            if load and load.startswith("spack load /"):
                context["application"].spack_hash = load.split("/", 1)[1]
        context["navtab"] = "application"
        context["admin_view"] = admin_view
        return context


class ApplicationCreateSelectView(LoginRequiredMixin, generic.ListView):
    """Custom view to select application install types"""

    model = Cluster
    template_name = "application/select_form.html"

    def get_queryset(self):
        queryset = super().get_queryset()
        return queryset.filter(status="r")

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "application"
        return context

    def post(self, request):
        """Custom post handler to redirect based on application type"""

        if request.POST["application-type"] == "spack":
            itemtype = "application-create-spack-cluster"
        elif request.POST["application-type"] == "custom":
            itemtype = "application-create-install"
        elif request.POST["application-type"] == "installed":
            itemtype = "application-create"
        return HttpResponseRedirect(
            reverse(itemtype, kwargs={"cluster": request.POST["cluster"]})
        )


class ApplicationCreateView(LoginRequiredMixin, generic.CreateView):
    """Custom CreateView for Application model"""

    success_url = reverse_lazy("applications")
    template_name = "application/create_form.html"
    form_class = ApplicationForm

    def get_initial(self):
        return {"cluster": Cluster.objects.get(pk=self.kwargs["cluster"])}

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["cluster"] = Cluster.objects.get(pk=self.kwargs["cluster"])
        context["navtab"] = "application"
        return context

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.status = "r"
        cluster = ClusterInfo(self.object.cluster)
        self.object.install_loc = cluster.get_app_install_loc(
            form.cleaned_data["installation_path"]
        )
        self.object.save()
        return HttpResponseRedirect(self.get_success_url())


class CustomInstallationApplicationCreateView(LoginRequiredMixin, generic.CreateView):  # pylint: disable=line-too-long
    """CreateView for Custom Installation of Application"""

    template_name = "application/custom_install_create_form.html"
    form_class = CustomInstallationApplicationForm

    def get_initial(self):
        return {"cluster": Cluster.objects.get(pk=self.kwargs["cluster"])}

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["cluster"] = Cluster.objects.get(pk=self.kwargs["cluster"])
        context["navtab"] = "application"
        return context

    def get_success_url(self):
        return reverse("application-detail", kwargs={"pk": self.object.pk})

    def form_valid(self, form):
        self.object = form.save(commit=False)
        cluster = ClusterInfo(self.object.cluster)
        self.object.install_loc = cluster.get_app_install_loc(
            form.cleaned_data["install_loc"]
        )
        if form.cleaned_data["module_name"]:
            self.object.load_command = (
                f'module load {form.cleaned_data["module_name"]}'
            )
        self.object.save()
        messages.success(
            self.request,
            f'Application "{self.object.name}" created in database. Click '
            '"Install" button below to actually install it on cluster.',
        )
        return HttpResponseRedirect(self.get_success_url())


class SpackApplicationCreateView(LoginRequiredMixin, generic.CreateView):
    """Custom CreateView for Application model"""

    # success_url = reverse_lazy('applications'})
    template_name = "application/spack_create_form.html"
    form_class = SpackApplicationForm

    def get_initial(self):
        return {"cluster": Cluster.objects.get(pk=self.kwargs["cluster"])}

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["cluster"] = Cluster.objects.get(pk=self.kwargs["cluster"])
        context["navtab"] = "application"
        return context

    def get_success_url(self):
        return reverse("application-detail", kwargs={"pk": self.object.pk})

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.install_loc = self.object.cluster.spack_install
        if self.object.version:
            # We need to insert the version immediately following the app name
            # and eventually support compiler...
            self.object.spack_spec = (
                f"@{self.object.version}"
                f'{self.object.spack_spec if self.object.spack_spec else ""}'
            )

        # Check if install_partition is not null
        if not self.object.install_partition:
            messages.error(
                self.request,
                'Please select an "Install Partition" before saving the application.'
            )
            return self.form_invalid(form)

        self.object.save()
        form.save_m2m()
        messages.success(
            self.request,
            f'Application "{self.object.name}" created in database. Click '
            '"Spack install" button below to actually install it on cluster.',
        )
        return HttpResponseRedirect(self.get_success_url())


class ApplicationUpdateView(LoginRequiredMixin, generic.UpdateView):
    """Custom UpdateView for Application model"""

    model = Application
    template_name = "application/edit_form.html"
    form_class = ApplicationEditForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "application"
        return context

    def get_success_url(self):
        return reverse_lazy("application-detail", kwargs={"pk": self.object.pk})


class ApplicationDeleteView(LoginRequiredMixin, generic.DeleteView):
    """Custom DeleteView for Application model"""

    model = Application
    success_url = reverse_lazy("applications")
    template_name = "application/check_delete.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "application"
        return context


class ApplicationLogFileView(LoginRequiredMixin, StreamingFileView):
    """View for application installation logs"""

    bucket = utils.load_config()["server"]["gcs_bucket"]
    valid_logs = [
        {
            "title": "Installation Output",
            "type": GCSFile,
            "args": (bucket, "stdout"),
        },
        {
            "title": "Installation Error Log",
            "type": GCSFile,
            "args": (bucket, "stderr"),
        },
    ]

    def _create_file_info_object(self, logfile_info, *args, **kwargs):
        return logfile_info["type"](*logfile_info["args"], *args, **kwargs)

    def get_file_info(self):
        logid = self.kwargs.get("logid", -1)
        application_id = self.kwargs.get("pk")
        application = get_object_or_404(Application, pk=application_id)

        cluster_id = application.cluster.id
        bucket_prefix = f"clusters/{cluster_id}/installs/{application_id}"

        entry = self.valid_logs[logid]
        return self._create_file_info_object(entry, *[bucket_prefix])


class ApplicationLogView(LoginRequiredMixin, generic.DetailView):
    """View to display application log files"""

    model = Application
    template_name = "application/log.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["log_files"] = [
            {"id": n, "title": entry["title"]}
            for n, entry in enumerate(ApplicationLogFileView.valid_logs)
        ]
        context["navtab"] = "application"
        return context


# For APIs


class ApplicationViewSet(viewsets.ModelViewSet):
    """Custom ModelViewSet for Application model"""

    permission_classes = (IsAuthenticated,)
    queryset = Application.objects.all().order_by("name")
    serializer_class = ApplicationSerializer


class SpackPackageViewSet(LoginRequiredMixin, viewsets.ViewSet):
    """Download a list of Spack packages available"""

    def list(self, request):
        return Response(spack.get_package_list())

    def retrieve(self, request, pk=None):
        pkgs = spack.get_package_list()
        if pk in pkgs:
            return Response(spack.get_package_info([pk]))
        return Response("Package Not Found", status=404)


# Other supporting views


class BackendCustomAppInstall(LoginRequiredMixin, generic.View):
    """Backend logic to launch a custom app installation"""

    def get(self, request, pk):
        app = get_object_or_404(CustomInstallationApplication, pk=pk)
        app.status = "p"
        app.save()
        cluster_id = app.cluster.id

        def response(message):
            if message.get("cluster_id") != cluster_id:
                logger.error(
                    "Cluster ID mismatch to callback: expected %s, received %s",
                    pk,
                    message.get("cluster_id"),
                )
            if message.get("app_id") != pk:
                logger.error(
                    "Application ID mismatch to callback:  expected %s, "
                    "received %s",
                    pk,
                    message.get("app_id"),
                )

            if "log_message" in message:
                logger.info("Install log message:  %s", message["log_message"])

            app = Application.objects.get(pk=pk)
            app.status = message["status"]
            if message["status"] == "r":
                # TODO App was installed.  Should have more attributes to set
                pass
            app.save()

        c2.send_command(
            cluster_id,
            "INSTALL_APPLICATION",
            on_response=response,
            data={
                "app_id": app.id,
                "name": app.name,
                "install_script": app.install_script,
                "module_name": app.module_name,
                "module_script": app.module_script,
                "partition": app.install_partition.name,
            },
        )

        return HttpResponseRedirect(
            reverse("application-detail", kwargs={"pk": pk})
        )


class BackendSpackInstall(LoginRequiredMixin, generic.View):
    """Backend logic to launch app installation via Spack"""

    def get(self, request, pk):
        app = get_object_or_404(SpackApplication, pk=pk)
        app.status = "p"
        app.save()
        cluster_id = app.cluster.id

        def response(message):
            if message.get("cluster_id") != cluster_id:
                logger.error(
                    "Cluster ID mismatch versus callback: expected %s, "
                    "received %s",
                    pk,
                    message.get("cluster_id"),
                )
            if message.get("app_id") != pk:
                logger.error(
                    "Application ID mismatch versus callback: expected %s, "
                    "received %s",
                    pk,
                    message.get("app_id"),
                )

            if "log_message" in message:
                logger.info("Install log message: %s", message["log_message"])

            app = Application.objects.get(pk=pk)
            app.status = message["status"]
            if message["status"] == "r":
                # App was installed.  Should have more attributes to set
                app.spack_hash = message.get("spack_hash", "")
                app.load_command = message.get("load_command", "")
                app.installed_architecture = message.get("spack_arch", "")
                app.compiler = message.get("compiler", "")
                app.mpi = message.get("mpi", "")
            app.save()

        c2.send_command(
            cluster_id,
            "SPACK_INSTALL",
            on_response=response,
            data={
                "app_id": app.id,
                "name": app.spack_name,
                "spec": app.spack_spec,
                "partition": app.install_partition.name,
                "extra_sbatch": [f"--gpus={app.install_partition.GPU_per_node}"]
                if app.install_partition.GPU_per_node
                else [],
            },
        )
        return HttpResponseRedirect(
            reverse("application-detail", kwargs={"pk": pk})
        )
