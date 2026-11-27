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

import logging
import threading

from asgiref.sync import sync_to_async
from django.shortcuts import get_object_or_404
from django.contrib.auth.views import redirect_to_login
from django.http import HttpResponseRedirect, Http404
from django.urls import reverse
from django.views import generic
from django.views.generic.edit import CreateView, UpdateView, DeleteView
from django.contrib import messages
from ..models import (
    CloudResource,
    Credential,
    Filesystem,
    FilesystemImpl,
    FILESYSTEM_IMPL_INFO,
    FilesystemExport,
)
from ..cluster_manager import cmek
from ..cluster_manager import filesystem as cm_fs
from ..views.asyncview import BackendAsyncView
from ..forms import FilesystemImportForm
from ..permissions import SuperUserRequiredMixin
from .view_utils import TerraformLogFile, StreamingFileView

logger = logging.getLogger(__name__)


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


def may_hold_cloud_resources(fs):
    """Whether a Filesystem may own cloud resources Terraform created.

    Deleting the record of one would orphan them, and the Terraform state
    that is the only way to remove them, so it must be destroyed instead.
    An imported or destroyed filesystem owns nothing. A new one owns nothing
    until an apply has been started for it (`cmek_applied`, stored before
    every Filestore apply, encrypted or not): a failed first apply can
    already have created the share, its key or the key's grant.
    """
    if fs.cloud_state in ("i", "xm"):
        return False
    if fs.cloud_state == "nm":
        return getattr(fs, "cmek_applied", False) is True
    return True


class FilesystemDeleteView(SuperUserRequiredMixin, DeleteView):
    """Custom DeleteView for Filesystem model

    Deletes the record only, without running Terraform, so it is refused
    for a filesystem that may hold cloud resources; destroy that instead.
    """

    model = Filesystem
    template_name = "filesystem/check_delete.html"

    def get(self, request, *args, **kwargs):
        return self._refusal() or super().get(request, *args, **kwargs)

    def post(self, request, *args, **kwargs):
        return self._refusal() or super().post(request, *args, **kwargs)

    def _refusal(self):
        """A redirect that says why the record cannot be deleted, or None."""
        fs = self.get_object()
        if not may_hold_cloud_resources(fs):
            return None
        messages.error(
            self.request,
            f"Cannot delete {fs.name}: it may still have cloud resources "
            "that only a Terraform destroy removes. Destroy it instead.")
        return HttpResponseRedirect(reverse("fs-detail", kwargs={"pk": fs.pk}))

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

        context["mounts"] = fs.active_mounts()
        context["workbench_mounts"] = fs.active_workbench_mounts()
        context["in_use"] = fs.in_use()
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

    def get_queryset(self):
        # Only imported filesystems: this form must not rewrite the record of
        # one OFE deployed, such as a Filestore (404 instead).
        return super().get_queryset().filter(
            impl_type=FilesystemImpl.IMPORTED)

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


class _RunningDestroys:
    """The filesystems whose destroy task runs in this server process.

    A destroy holds the Terraform state lock only during `terraform destroy`,
    not during the `init` and state pull before it, so the lock alone cannot
    refuse a second Destroy pressed in that window; the second task would
    run a second `init` in the same directory and truncate the first's log.
    OFE serves from one process, so this set covers every request. It lives
    in memory only: a restart, which also ends every task, empties it, so a
    Filestore left at "dm" by a restart can still be destroyed again. The
    task removes its id when it ends, whether it succeeds or fails; a task
    is cancelled before it starts only when the process stops.

    The request claims on the event loop and the task releases in a worker
    thread, so the set is guarded by a lock and claim() checks and adds in
    one step.
    """

    def __init__(self):
        self._lock = threading.Lock()
        self._ids = set()

    def claim(self, fs_id):
        """Add fs_id and return True, or return False if it is present."""
        with self._lock:
            if fs_id in self._ids:
                return False
            self._ids.add(fs_id)
        logger.info("Claimed the destroy of filesystem %s", fs_id)
        return True

    def release(self, fs_id):
        with self._lock:
            held = fs_id in self._ids
            self._ids.discard(fs_id)
        if held:
            logger.info("Released the destroy claim of filesystem %s", fs_id)

    def __contains__(self, fs_id):
        with self._lock:
            return fs_id in self._ids


_running_destroys = _RunningDestroys()


class BackendDestroyFilesystem(BackendAsyncView):
    """A view to make async call to destroy a filesystem"""

    refused = False

    @sync_to_async
    def get_orm(self, fs_id):
        """Refuse the destroy, or set "dm"; get() holds the claim around it.

        This runs in a thread that keeps running if the request is
        cancelled, so it neither takes nor releases the claim: get() does
        both on the event loop.
        """
        fs = Filesystem.objects.get(pk=fs_id)
        if fs.in_use():
            names = sorted(
                {m.cluster.name for m in fs.active_mounts()}
                | {m.workbench.name for m in fs.active_workbench_mounts()})
            logger.info(
                "Refused the destroy of filesystem %s: still mounted by %s",
                fs.id, ", ".join(names))
            self.refused = True
            messages.add_message(
                self.request,
                messages.ERROR,
                "Cannot destroy - Filesystem is still mounted by: "
                f"{', '.join(names)} (see below)",
            )
            return (fs,)
        lock = self._running_lock(fs)
        if lock is not None:
            # Refused before "dm" is set: the running operation owns the
            # state.
            logger.info(
                "Refused the destroy of filesystem %s: its Terraform state "
                "is locked by %s since %s (lock ID %s)", fs.id,
                lock.get("Who", "an unknown holder"),
                lock.get("Created", "an unknown time"),
                lock.get("ID") or "unknown")
            self.refused = True
            messages.add_message(
                self.request, messages.ERROR,
                self._lock_held_message(fs, lock))
            return (fs,)
        if not self._mark_destroying(fs):
            # A request cancelled during the lock read leaves this thread
            # running after its claim is released, so a later destroy may
            # have finished, or the record been deleted, since the load.
            logger.info(
                "Refused the destroy of filesystem %s: it changed while the "
                "request was being handled", fs.id)
            self.refused = True
            messages.add_message(
                self.request, messages.ERROR,
                "This Filestore changed while the request was being "
                "handled; reload the page and try again.")
        return (fs,)

    @staticmethod
    def _mark_destroying(fs):
        """Set "dm" only if the row still holds what was loaded.

        One conditional UPDATE, so it cannot undo a destroy that finished
        after the load, and cannot re-create a deleted record as a save()
        would. It updates CloudResource, the table that holds cloud_state:
        an update through the Filesystem subclass selects the ids first and
        then updates by id, which loses the condition. Only cloud_state
        changes, so the loaded object, given "dm", stays current for cmd's
        saves.
        """
        updated = CloudResource.objects.filter(
            pk=fs.pk, cloud_state=fs.cloud_state,
            status_message=fs.status_message,
        ).update(cloud_state="dm")
        if updated:
            fs.cloud_state = "dm"
        return bool(updated)

    @staticmethod
    def _lock_held_message(fs, lock):
        lock_id = lock.get("ID") or "<lock ID>"
        return (
            "A Terraform operation is already running for this Filestore: "
            "its Terraform state is locked by "
            f"{lock.get('Who', 'an unknown holder')} since "
            f"{lock.get('Created', 'an unknown time')} (lock ID {lock_id}). "
            "Wait for it to finish. Never force-unlock while an operation "
            "is running; see \"A resource reports a stale state lock\" in "
            "the admin guide. Only if no Terraform operation is running for "
            "it, clear the lock with: "
            f"{cm_fs.force_unlock_command(fs, lock_id)}"
        )

    @staticmethod
    def _running_lock(fs):
        """The state lock a running Terraform operation holds, or None.

        A lock that cannot be read (the bucket unreachable, the credential
        file missing) does not refuse the destroy: Terraform's own lock still
        keeps two runs apart, the destroy reports its own error if the state
        cannot be reached, and refusing would leave a Filestore that may hold
        resources with no way to destroy it from OFE. This matches the
        stale-lock check before every Terraform run, which never blocks an
        operation when it cannot read the lock.
        """
        try:
            return cm_fs.running_state_lock(fs)
        except Exception as err:  # pylint: disable=broad-except
            logger.warning(
                "Could not read the Terraform state lock of filesystem %s; "
                "allowing the destroy: %s", fs.id, err)
            return None

    def cmd(self, unused_task_id, unused_token, fs):
        try:
            try:
                cm_fs.destroy_filesystem(fs)
            except Exception as err:
                # Part of the Filestore may already be gone, so it is
                # neither destroyed nor running: "um" keeps its Destroy
                # button for a retry, and the page says why.
                fs.cloud_state = "um"
                fs.status_message = (
                    f"Teardown failed: {err} Part of the Filestore may "
                    "already be gone; destroy it again to finish.")
                fs.save(update_fields=["cloud_state", "status_message"])
                raise
            fs.cloud_state = "xm"
            fs.status_message = ""  # clear any earlier failed destroy
            fs.save()
        finally:
            # After the state is saved, so a request accepted next sees it.
            _running_destroys.release(fs.id)

    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        # Claimed here on the event loop, before get_orm's thread starts, so
        # a second request arriving during the lock read is refused too, and
        # the release below covers every exit up to the task's start. That
        # includes a cancel when the client disconnects: Django cancels the
        # view at once while get_orm's thread runs on and may still set
        # "dm"; with the claim released, that Filestore stays retryable.
        if not _running_destroys.claim(pk):
            logger.info(
                "Refused the destroy of filesystem %s: a destroy started "
                "from this server is still running", pk)
            self.refused = True
            messages.add_message(
                self.request,
                messages.ERROR,
                "A Terraform operation is already running for this "
                "Filestore. Wait for it to finish.",
            )
            return HttpResponseRedirect(
                reverse("fs-detail", kwargs={"pk": pk}))
        try:
            args = await self.get_orm(pk)
            if self.refused:
                _running_destroys.release(pk)
                return HttpResponseRedirect(
                    reverse("fs-detail", kwargs={"pk": pk}))
            await self.create_task("Destroy Filesystem", *args)
        except BaseException as err:
            # No task was started (create_task starts it as its last step,
            # with no await after it), so nothing else would release the
            # claim. A Filestore left at "dm" offers Destroy.
            logger.info(
                "The destroy request for filesystem %s ended with %s before "
                "its task started", pk, type(err).__name__)
            _running_destroys.release(pk)
            raise
        return HttpResponseRedirect(reverse("fs-detail", kwargs={"pk": pk}))


class BackendCreateFilesystem(BackendAsyncView):
    """A view to make async call to create a new filesystem"""

    @sync_to_async
    def get_orm(self, fs_id):
        fs = Filesystem.objects.get(pk=fs_id)
        return (fs,)

    def cmd(self, unused_task_id, unused_token, fs):
        # The encryption fields live on the GCPFilestoreFilesystem row, a
        # separate instance Django caches on fs.
        filestore = None
        if FilesystemImpl(fs.impl_type) == FilesystemImpl.GCPFILESTORE:
            filestore = fs.gcpfilestorefilesystem
        # Refused before the reset below: a revoked Filestore that is still
        # deployed must keep its cloud_state, and with it its Destroy button.
        # Re-read first: the record was loaded when the task was queued.
        cmek.keep_flags_current(filestore)
        try:
            cmek.assert_not_revoked(filestore)
        except cmek.CmekConfigError as err:
            fs.status_message = str(err)
            fs.save(update_fields=["status_message"])
            raise
        # Taken before the reset below, which would make a deployed Filestore
        # look new to the blueprint generator: cmek.is_frozen() reads this
        # snapshot. The generator works on the child row, so it is set there
        # too.
        # pylint: disable=protected-access
        deployed = cmek.is_frozen(fs)
        previous_state = fs.cloud_state
        fs.cloud_state = "nm"
        fs.save()
        fs._cmek_deployed = deployed
        if filestore is not None:
            # pylint: disable-next=protected-access
            filestore._cmek_deployed = deployed
            # Stored too, so a Filestore deployed before the flag existed
            # stays frozen once this flow has left it "nm".
            cmek.record_deployed(filestore)
        try:
            cm_fs.create_filesystem(fs)
        except Exception as err:
            # Nothing was generated, so the record goes back to the state it
            # had: one that was never deployed can be created again, and one
            # that is deployed keeps its Destroy button. The page says why
            # (a key ring that could not be checked, a settings error).
            fs.cloud_state = previous_state
            fs.status_message = f"Could not generate the blueprint: {err}"
            fs.save(update_fields=["cloud_state", "status_message"])
            raise

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
        # Nothing is reset here, so the record keeps its cloud_state either
        # way; the page says why the blueprint was not regenerated, as it
        # does for create.
        filestore = None
        if FilesystemImpl(fs.impl_type) == FilesystemImpl.GCPFILESTORE:
            filestore = fs.gcpfilestorefilesystem
        # Re-read first: the record was loaded when the task was queued.
        cmek.keep_flags_current(filestore)
        try:
            cmek.assert_not_revoked(filestore)
        except cmek.CmekConfigError as err:
            fs.status_message = str(err)
            fs.save(update_fields=["status_message"])
            raise
        try:
            cm_fs.update_filesystem(fs)
        except Exception as err:
            fs.status_message = f"Could not generate the blueprint: {err}"
            fs.save(update_fields=["status_message"])
            raise

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
