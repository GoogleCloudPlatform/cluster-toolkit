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

""" clusters.py """

import os
from asgiref.sync import sync_to_async
from django.shortcuts import get_object_or_404
from django.contrib.auth.mixins import LoginRequiredMixin
from django.contrib.auth.views import redirect_to_login
from django.contrib.auth.mixins import UserPassesTestMixin
from django.core.exceptions import PermissionDenied
from django.urls import reverse_lazy
from django.conf import settings
from django.http import (
    HttpResponseRedirect,
    JsonResponse,
)
from django.urls import reverse
from django.views import generic
from django.views.generic.edit import CreateView
from ..models import StartupScript, Image, Credential
from ..forms import StartupScriptForm, ImageForm, ImageImportForm
from ..cluster_manager.image import ImageBackend
from ..cluster_manager.cloud_info import get_region_zone_info
from ..cluster_manager.image_import import *
from ..views.asyncview import BackendAsyncView
from pathlib import Path
import json
from django.contrib import messages

import logging

logger = logging.getLogger(__name__)


class ImagesListView(LoginRequiredMixin, generic.ListView):
    """Custom ListView for StartupScript and Images model"""

    model = StartupScript
    template_name = "image/list.html"

    def get_queryset(self):
        # If user is admin, return all objects.
        if self.request.user.has_admin_role():
            startup_scripts = StartupScript.objects.all()
            images = Image.objects.all()
            return startup_scripts, images
        else:
            # Retrieve startup scripts and images owned by the user
            startup_scripts = StartupScript.objects.filter(owner=self.request.user)
            images = Image.objects.filter(owner=self.request.user)
            
            # Retrieve startup scripts and images authorized for the user
            authorized_startup_scripts = StartupScript.objects.filter(authorised_users=self.request.user)
            authorized_images = Image.objects.filter(authorised_users=self.request.user)
            
            # Combine the owned and authorized objects
            startup_scripts |= authorized_startup_scripts
            images |= authorized_images
            
            return startup_scripts, images
        
    def get_context_data(self, *args, **kwargs):
        loading = 0
        admin_view = 0        
        if self.request.user.has_admin_role():
            admin_view = 1
        context = super().get_context_data(*args, **kwargs)
        context["loading"] = loading
        context["admin_view"] = admin_view
        context["navtab"] = "image"

        startup_scripts, images = self.get_queryset()
        context["startupscripts"] = startup_scripts
        context["images"] = images

        return context
    
class StartupScriptDetailView(LoginRequiredMixin, generic.DetailView):
    """Custom DetailView for StartupScript model"""

    model = StartupScript
    template_name = "image/startup-script-view.html"

    def is_admin_or_authorized_user(self, startup_script):
        user = self.request.user
        return (
            user.has_admin_role()
            or user == startup_script.owner
            or user in startup_script.authorised_users.all()
        )

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        startup_script = self.get_object()

        # Check if the user is an admin, the owner, or authorized for the startup script
        if self.is_admin_or_authorized_user(startup_script):
            file_path = Path(settings.MEDIA_ROOT) / startup_script.content.name
            try:
                with open(file_path, 'r') as file:
                    try:
                        context["file_contents"] = file.read()
                    except UnicodeDecodeError:
                        context["file_contents"] = "Error: Unable to decode file"
            except IOError:
                context["file_contents"] = "Error: Unable to read file"
        else:
            raise PermissionDenied()
        
        context["navtab"] = "image"
        return context
    
class StartupScriptCreateView(LoginRequiredMixin, generic.CreateView):
    """Custom CreateView for StartupScript model"""

    success_url = reverse_lazy("images")
    form_class = StartupScriptForm
    template_name = "image/startup-script-create.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "image"
        return context
    
    # Set currently logged-in user as owner.
    def form_valid(self, form):
        form.instance.owner = self.request.user
        return super().form_valid(form)


class StartupScriptDeleteView(UserPassesTestMixin, generic.View):
    """Custom view for deleting StartupScript objects"""

    def test_func(self):
        return self.request.user.is_superuser

    def post(self, request, *args, **kwargs):
        startup_script = StartupScript.objects.get(pk=self.kwargs['pk'])
        file_path = Path(settings.MEDIA_ROOT) / startup_script.content.name
        try:
            os.remove(file_path)
            logger.info("File deleted successfully.")
        except FileNotFoundError:
            logger.error("Error: File not found.")
        except PermissionError:
            logger.error("Error: Permission denied.")
        except Exception as e:
            logger.exception(f"Error: {str(e)}")

        startup_script.delete()
        response = {'success': True}
        return JsonResponse(response)

class ImageCreateView(LoginRequiredMixin, CreateView):
    """Custom CreateView for Image model"""

    form_class = ImageForm
    template_name = "image/image-create.html"

    def get_success_url(self):
        image = self.object
        success_url = reverse("backend-create-image", kwargs={"pk": image.pk})
        return success_url

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs["user"] = self.request.user
        return kwargs

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "image"
        return context
    
    # Set currently logged-in user as owner.
    def form_valid(self, form):
        form.instance.owner = self.request.user
        return super().form_valid(form)
    
class ImageDetailView(LoginRequiredMixin, generic.DetailView):
    """Custom DetailView for Image model"""

    model = Image
    template_name = "image/image-view.html"

    def is_admin_or_authorized_user(self, image):
        user = self.request.user
        return (
            user.has_admin_role()
            or user == image.owner
            or user in image.authorised_users.all()
        )
    
    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        image = self.get_object()
        startup_scripts = image.startup_script.all()
        context["startup_scripts"] = startup_scripts

        # Check if the user is an admin, the owner, or authorized for the image
        if self.is_admin_or_authorized_user(image):
            context["navtab"] = "image"
            return context
        else:
            raise PermissionDenied()


class ImageDeleteView(UserPassesTestMixin, generic.View):
    """Custom view for deleting Image objects"""

    def test_func(self):
        return self.request.user.is_superuser

    def post(self, request, *args, **kwargs):
        image = Image.objects.get(pk=self.kwargs['pk'])
        if image.source_image_project == "Imported":
            image.delete()
            response = {'success': True, 'import': True}
        else:
            img_backend = ImageBackend(image)
            img_backend.delete_image()
            image.delete()
            response = {'success': True}
        return JsonResponse(response)
    
    
class ImageStatusView(LoginRequiredMixin, generic.View):
    """Custom view for Image model that returns Image status"""

    def is_admin_or_authorized_user(self, image):
        user = self.request.user
        return (
            user.has_admin_role()
            or user == image.owner
            or user in image.authorised_users.all()
        )

    def get(self, request, pk, *args, **kwargs):
        image = get_object_or_404(Image, pk=pk)

        # Check if the user is an admin, the owner, or authorized for the image
        if self.is_admin_or_authorized_user(image):
            response = {'status': image.status}
            return JsonResponse(response)

        else:
            raise PermissionDenied()
      
class BackendCreateImage(BackendAsyncView):
    """A view to make async call to create a new image"""

    @sync_to_async
    def get_orm(self, image_id):
        image = Image.objects.get(pk=image_id)
        creds = image.cloud_credential
        return (image, creds)

    def cmd(self, unused_task_id, unused_token, image, creds):
        img_backend = ImageBackend(image)
        img_backend.prepare()
        
    async def get(self, request, pk):
        """this will invoke the background tasks and return immediately"""
        # Mixins don't yet work with Async views
        if not await sync_to_async(lambda: request.user.is_authenticated)():
            return redirect_to_login(request.get_full_path)
        await self.test_user_is_cluster_admin(request.user)

        args = await self.get_orm(pk)
        await self.create_task("Create Image", *args)
        return HttpResponseRedirect(
            reverse("images")
        )
    
class BackendListRegions(LoginRequiredMixin, generic.View):
    """Custom view that returns json of available GCP regions."""

    def get(self, request, pk, *args, **kwargs):
        credentials = get_object_or_404(Credential, pk=pk)
        regions = get_region_zone_info("GCP", credentials.detail)
        return JsonResponse(regions)


class ImageImportView(LoginRequiredMixin, CreateView):
    """Custom CreateView for Image model"""

    form_class = ImageImportForm
    template_name = "image/image-import.html"

    def get_success_url(self):
        success_url = reverse("images")
        return success_url

    def get_form_kwargs(self):
        kwargs = super().get_form_kwargs()
        kwargs["user"] = self.request.user
        return kwargs

    def get_image_list(self):
        prexisting_images_list = []
        for cred in Credential.objects.all():
            prexisting_images = list_project_images(cred)
            prexisting_images_list += prexisting_images
        return prexisting_images_list

    def get_context_data(self, **kwargs):
        prexisting_images_list = self.get_image_list()
        prexisting_images_json = json.dumps(prexisting_images_list)
        context = super().get_context_data(**kwargs)
        context["navtab"] = "image"
        context["image_choice"] = prexisting_images_list
        context["image_jsonstr"] = prexisting_images_json
        return context
    
    # Set currently logged-in user as owner.
    def form_valid(self, form):
        form.instance.owner = self.request.user
        form.instance.source_image_project = "Imported" #form.cleaned_data['name']
        form.instance.source_image_family = "Imported" #form.cleaned_data['family']
        selected_cred_id = int(form.data["cloud_credential"])
        selected_cred_obj = get_object_or_404(Credential, pk=selected_cred_id)
        usr_input_select = form.data["inputOption"]
        if usr_input_select == "text":
            form_name = form.data["textInputName"]
            form_family = form.data["textInputFamily"]
        elif usr_input_select == "dropdown":
            image_list = form.data["dropdown"].split(",")
            form_name = image_list[1]
            form_family = image_list[3]      
        else:
            messages.error(self.request, 'Please choose how you want to select an image')
            return self.form_invalid(form)
        if form_name != "" and form_family != "":
            form.instance.name = form_name
            form.instance.family = form_family
        else:
            messages.error(self.request, 'Please enter an image and family name')
            return self.form_invalid(form)
        image_isvalid = verify_image(selected_cred_obj,form_name,form_family)
        if image_isvalid:
            form.instance.status = "r"
        else:
            messages.error(self.request, 'Image name/family does not match any existing images, using given credential')
            return self.form_invalid(form)    
        return super().form_valid(form)
    
