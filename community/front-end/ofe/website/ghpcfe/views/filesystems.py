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
""" filesystems.py """

from asgiref.sync import sync_to_async
from django.shortcuts import get_object_or_404
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect, Http404
from django.urls import reverse
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from ..models import (
    Credential,
    Filesystem,
    FilesystemImpl,
    FILESYSTEM_IMPL_INFO,
    MountPoint,
    FilesystemExport,
)
from ..cluster_manager import filesystem as cm_fs
from ..views.asyncview import BackendAsyncView
from ..forms import FilesystemImportForm
from ..permissions import SuperUserRequiredMixin
from .view_utils import TerraformLogFile, StreamingFileView


class FilesystemListView(SuperUserRequiredMixin, generic.ListView):
    """Custom ListView for Cluster model"""

    model = Filesystem
    template_name = "filesystem/list.html"

    def get_queryset(self):
        return super().get_queryset().exclude(impl_type=FilesystemImpl.BUILT_IN)

    def get_context_data(self, *args, **kwargs):
        loading = 0
        for fs in Filesystem.objects.all():
            if "c" in fs.cloud_state or "d" in fs.cloud_state:
                loading = 1
                break
        context = super().get_context_data(*args, **kwargs)
        context["loading"] = loading
        context["navtab"] = "fs"
        return context


class FilesystemCreateView1(SuperUserRequiredMixin, generic.ListView):
    """Custom view for the first step of Filesystem creation"""

    model = Credential
    template_name = "credential/select_form.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        return context

    def post(self, request):
        return HttpResponseRedirect(
            reverse(
                "fs-create2", kwargs={"credential": request.POST["credential"]}
            )
        )


class FilesystemCreateView2(SuperUserRequiredMixin, generic.TemplateView):
    """Custom view for the first step of Filesystem creation"""

    template_name = "filesystem/impl_select_form.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["impl_list"] = [
            (k.value, v["name"])
            for k, v in FILESYSTEM_IMPL_INFO.items()
            if v["class"]
        ]
        context["navtab"] = "fs"
        return context

    def get(self, request, credential, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=credential)
        return super().get(request, *args, **kwargs)

    def post(self, request, credential, *args, **kwargs):
        try:
            fi = FilesystemImpl(int(request.POST["fs_impl"]))
            tgt = f"{FILESYSTEM_IMPL_INFO[fi]['url-key']}-create"
        except KeyError:
            raise Http404(  # pylint: disable=raise-missing-from
                "Cannot find Filesystem implementation type "
                f"'{request.POST['impl']}'"
            )

        return HttpResponseRedirect(
            reverse(tgt, kwargs={"credential": credential})
        )


class FilesystemRedirectView(SuperUserRequiredMixin, generic.RedirectView):
    """Filesystem view that redirects to another implementation"""

    permanent = False
    query_string = True

    target = "detail"

    def get_redirect_url(self, *args, **kwargs):
        fs = get_object_or_404(Filesystem, pk=kwargs["pk"])
        key = FILESYSTEM_IMPL_INFO[fs.impl_type]["url-key"]
        self.pattern_name = f"{key}-{self.target}"
        return super().get_redirect_url(*args, **kwargs)


class FilesystemDeleteView(SuperUserRequiredMixin, DeleteView):
    """Custom DeleteView for Filesystem model"""

    model = Filesystem
    template_name = "filesystem/check_delete.html"

    def get_object(self, queryset=None):
        obj = super().get_object(queryset)
        if self.model == Filesystem:
            # Initially, we're set to Filesystem, switch to our actual type
            self.model = FILESYSTEM_IMPL_INFO[obj.impl_type]["class"]
            return super().get_object(queryset)
        else:
            return obj

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        return context

    def get_success_url(self):
        fs = self.get_object()
        info = FILESYSTEM_IMPL_INFO[fs.impl_type]
        messages.success(self.request, f'{info["name"]} - {fs.name} deleted.')
        return reverse("filesystems")


class FilesystemDestroyView(SuperUserRequiredMixin, generic.DetailView):
    """Custom View to confirm filesystem destroy"""

    model = Filesystem
    template_name = "filesystem/check_destroy.html"

    def get_context_data(self, *args, **kwargs):
        context = super().get_context_data(**kwargs)
        fs = get_object_or_404(Filesystem, pk=self.kwargs["pk"])
        exports = fs.exports.all()
        mounts = MountPoint.objects.filter(
            export__in=list(exports.values_list("id", flat=True))
        ).filter(cluster__cloud_state__in=["cm", "m", "dm"])

        context["mounts"] = mounts
        context["exports"] = exports
        context["navtab"] = "fs"
        return context


class FilesystemImportView(SuperUserRequiredMixin, CreateView):
    """View for importing an existing GCP filesystem"""

    template_name = "filesystem/import_form.html"
    form_class = FilesystemImportForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        return context

    def get_initial(self):
        return {"cloud_credential": self.cloud_credential}

    def get(self, request, credential, *args, **kwargs):
        self.cloud_credential = get_object_or_404(Credential, pk=credential)
        return super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        self.cloud_credential = get_object_or_404(
            Credential, pk=kwargs["credential"]
        )
        return super().post(request, *args, **kwargs)

    def form_valid(self, form):
        self.object = form.save(commit=False)
        self.object.cloud_state = "i"
        self.object.cloud_credential = self.cloud_credential
        self.object.impl_type = FilesystemImpl.IMPORTED
        self.object.save()

        export = FilesystemExport(
            filesystem=self.object, export_name=form.data["share_name"]
        )
        export.save()

        return super().form_valid(form)

    def get_success_url(self):
        return reverse("fs-detail", kwargs={"pk": self.object.pk})


class FilesystemImportUpdateView(SuperUserRequiredMixin, UpdateView):
    """View for updating an imported filesystem"""

    model = Filesystem
    template_name = "filesystem/import_update.html"
    form_class = FilesystemImportForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        return context

    def get_initial(self):
        return {"share_name": self.object.exports.first().export_name}

    def form_valid(self, form):
        self.object = form.save()

        export = self.object.exports.first()
        export.export_name = form.data["share_name"]
        export.save()

        return super().form_valid(form)

    def get_success_url(self):
        return reverse("fs-detail", kwargs={"pk": self.object.pk})


class FilesystemImportDetailView(SuperUserRequiredMixin, generic.DetailView):
    """Detail view for imported filesystems"""

    model = Filesystem
    template_name = "filesystem/import_detail.html"

    def get_context_data(self, **kwargs):
        """Perform extra query to populate instance types data"""
        context = super().get_context_data(**kwargs)
        context["navtab"] = "fs"
        context["exports"] = FilesystemExport.objects.filter(
            filesystem=self.kwargs["pk"]
        )
        return context


# Other supporting views


class FilesystemTFLogView(StreamingFileView):
    def get_file_info(self):
        fs_id = self.kwargs.get("pk")
        fs = get_object_or_404(Filesystem, pk=fs_id)
        return TerraformLogFile(prefix=cm_fs.get_terraform_dir(fs))


class BackendDestroyFilesystem(BackendAsyncView):
    """A view to make async call to destroy a filesystem"""

    @sync_to_async
    def get_orm(self, fs_id):
        fs = Filesystem.objects.get(pk=fs_id)
        fs.cloud_state = "dm"
        fs.save()
        return (fs,)

    def cmd(self, unused_task_id, unused_token, fs):
        cm_fs.destroy_filesystem(fs)
        fs.cloud_state = "xm"
        fs.save()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Destroy Filesystem", *args)
        return HttpResponseRedirect(reverse("fs-detail", kwargs={"pk": pk}))


class BackendCreateFilesystem(BackendAsyncView):
    """A view to make async call to create a new filesystem"""

    @sync_to_async
    def get_orm(self, fs_id):
        fs = Filesystem.objects.get(pk=fs_id)
        return (fs,)

    def cmd(self, unused_task_id, unused_token, fs):
        fs.cloud_state = "nm"
        fs.save()
        cm_fs.create_filesystem(fs)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create Filestore", *args)
        return HttpResponseRedirect(reverse("fs-detail", kwargs={"pk": pk}))


class BackendUpdateFilesystem(BackendAsyncView):
    """A view to make async call to update a filesystem"""

    @sync_to_async
    def get_orm(self, fs_id):
        fs = Filesystem.objects.get(pk=fs_id)
        return (fs,)

    def cmd(self, unused_task_id, unused_token, fs):
        cm_fs.update_filesystem(fs)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Update Filestore TF Vars", *args)
        return HttpResponseRedirect(reverse("fs-detail", kwargs={"pk": pk}))


class BackendStartFilesystem(BackendAsyncView):
    """A view to make async call to start a filesystem"""

    @sync_to_async
    def get_orm(self, fs_id):
        fs = Filesystem.objects.get(pk=fs_id)
        fs.status = "cm"
        fs.save()
        return (fs,)

    def cmd(self, unused_task_id, unused_token, fs):
        cm_fs.start_filesystem(fs)
        fs.cloud_state = "m"
        fs.save()

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Start Filestore", *args)
        return HttpResponseRedirect(reverse("fs-detail", kwargs={"pk": pk}))
