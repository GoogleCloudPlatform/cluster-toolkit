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

import logging, json
from django.http import JsonResponse
from django.shortcuts import get_object_or_404
from django.views import View
from django.shortcuts import redirect
from django.contrib import messages
from django.urls import reverse
from google.cloud.devtools import cloudbuild_v1
from google.cloud import artifactregistry_v1
from google.oauth2 import service_account
from google.api_core import exceptions
from google.protobuf import duration_pb2
from ..models import ContainerRegistry
from ..forms import PullContainerForm
from ..permissions import LoginRequiredMixin
from .view_utils import container_images_cache, RegistryDataHelper
from ..views.asyncview import BackendAsyncView
from asgiref.sync import sync_to_async

logger = logging.getLogger(__name__)

""" containers.py """


class PullToArtifactRegistryView(LoginRequiredMixin, BackendAsyncView):
    """
    Async view to pull an image from a remote repository into GCP Artifact Registry using Cloud Build.
    """

    @sync_to_async
    def is_duplicate_request(self, registry, source_uri, container_tag):
        normalized_dest = self.normalize_dest_image_sync(registry, source_uri, container_tag)
        for build in registry.build_info or []:
            if build["dest_image"] == normalized_dest and build["status"] in ("s", "i"):
                return True
        return False

    @sync_to_async
    def normalize_dest_image(self, registry, source_uri, tag):
        return self.normalize_dest_image_sync(registry, source_uri, tag)

    @staticmethod
    def normalize_dest_image_sync(registry, source_uri, tag):
        source_uri = source_uri.split("://")[-1]
        source_domain = source_uri.split('/')[0]
        normalized_source = "".join(ch for ch in source_domain if ch.isalnum())

        last_component = source_uri.rsplit('/', 1)[-1]
        image_name = last_component.split(':')[0]
        source_label = last_component.split(':')[-1]

        image_name = f"{normalized_source}-{image_name}-{source_label}"
        registry_url = registry.get_registry_url().replace("https://", "")
        return f"{registry_url}/{image_name}:{tag}"

    async def dispatch(self, request, *args, **kwargs):
        await sync_to_async(request.session.load)()
        _ = await sync_to_async(lambda: request.user.is_authenticated)()
        return await super().dispatch(request, *args, **kwargs)

    async def post(self, request, registry_id):
        registry = await sync_to_async(get_object_or_404)(ContainerRegistry, id=registry_id)
        form = PullContainerForm(request.POST)
        if form.is_valid():
            data = form.cleaned_data
            source_uri = data.get("source_uri")
            container_tag = data.get("container_tag")
            repo_username = data.get("repo_username")
            repo_password = data.get("repo_password")

            # Properly wrapped duplicate check
            if await self.is_duplicate_request(registry, source_uri, container_tag):
                messages.error(request, "Duplicate image/tag detected. Already pulled.")
                return redirect(reverse("registry-detail", args=[registry.id]))

            await self.create_task(
                "Pull Container via Cloud Build",
                registry, source_uri, container_tag,
                repo_username, repo_password
            )
            messages.success(request, "Pull operation started successfully.")
            return redirect(reverse("registry-detail", args=[registry.id]))
        else:
            messages.error(request, "Invalid input provided.")
            return redirect(reverse("registry-detail", args=[registry.id]))

    def cmd(self, unused_task_id, unused_token, registry, source_uri, tag, registry_username, registry_password):
        try:
            from google.cloud.devtools import cloudbuild_v1
            from google.oauth2 import service_account
            from google.protobuf import duration_pb2

            dest_image = self.normalize_dest_image_sync(registry, source_uri, tag)
            logger.info(f"Destination image: {dest_image}")

            credential_info = json.loads(registry.cluster.cloud_credential.detail)
            credentials = service_account.Credentials.from_service_account_info(credential_info)
            scoped_credentials = credentials.with_scopes(["https://www.googleapis.com/auth/cloud-platform"])
            client = cloudbuild_v1.CloudBuildClient(credentials=scoped_credentials)
            project_id = registry.get_project_id()

            build_steps = []
            if registry_username and registry_password:
                source_repo = source_uri.split('/')[0]
                source_login_cmd = f"echo '{registry_password}' | docker login {source_repo} --username '{registry_username}' --password-stdin"
                build_steps = [
                    cloudbuild_v1.BuildStep(
                        name="gcr.io/cloud-builders/docker",
                        entrypoint="bash",
                        args=["-c", source_login_cmd]
                    )
                ]

            build_steps.extend([
                cloudbuild_v1.BuildStep(name="gcr.io/cloud-builders/docker", args=["pull", source_uri]),
                cloudbuild_v1.BuildStep(name="gcr.io/cloud-builders/docker", args=["tag", source_uri, dest_image]),
                cloudbuild_v1.BuildStep(name="gcr.io/cloud-builders/docker", args=["push", dest_image])
            ])

            #build = cloudbuild_v1.Build(steps=build_steps, timeout=duration_pb2.Duration(seconds=3600))
            # Significantly faster if using E2_HIGHCPU_8 instead:
            build = cloudbuild_v1.Build(
                steps=build_steps,
                timeout=duration_pb2.Duration(seconds=3600),
                options=cloudbuild_v1.BuildOptions(
                    machine_type=cloudbuild_v1.BuildOptions.MachineType.E2_HIGHCPU_8
                )
            )

            operation = client.create_build(project_id=project_id, build=build)

            build_id = operation.metadata.build.id if operation.metadata and operation.metadata.build else "unknown"

            registry.build_info = registry.build_info or []
            registry.build_info.append({
                "build_id": build_id,
                "status": "i",
                "dest_image": dest_image,
                "source_image": source_uri,
                "name": f"{dest_image.split('/')[-1].split(':')[0]}",
                # "name": f"{dest_image.split('/')[-1].split(':')[0]} ({source_uri.split(':')[-1]})",
                "tags": tag
            })
            registry.save()

        except Exception as e:
            logger.error(f"Failed to start pull operation: {str(e)}", exc_info=True)
            build_info = registry.build_info or []
            existing_record = next((b for b in build_info if b["build_id"] == build_id), None)
            if existing_record:
                existing_record["status"] = "f"
            else:
                build_info.append({
                    "build_id": build_id,
                    "status": "f",
                    "dest_image": dest_image,
                    "source_image": source_uri,
                    "name": f"{dest_image.split('/')[-1].split(':')[0]}",
                    # "name": f"{dest_image.split('/')[-1].split(':')[0]} ({source_uri.split(':')[-1]})",
                    "tags": tag
                })
            registry.build_info = build_info
            registry.save()


class ContainerBuildStatusView(LoginRequiredMixin, View):
    def get(self, request, registry_id):
        registry = get_object_or_404(ContainerRegistry, id=registry_id)
        # Build a dict mapping build_id to status
        build_statuses = {}
        for info in registry.build_info:
            build_id = info.get("build_id", "unknown")
            status = info.get("status", "n")
            build_statuses[build_id] = status
        return JsonResponse({"build_statuses": build_statuses})


class GetContainerImagesView(LoginRequiredMixin, View):
    """View to fetch container images dynamically based on selected registry"""

    def get(self, request, registry_id, *args, **kwargs):
        try:
            registry = ContainerRegistry.objects.get(id=registry_id)
        except ContainerRegistry.DoesNotExist:
            return JsonResponse({"error": "Registry not found"}, status=404)

        helper = RegistryDataHelper(request, registry)
        container_images, _ = helper.get_data()

        return JsonResponse(container_images, safe=False)


# class DeleteContainerView(LoginRequiredMixin, View):
#     """
#     Deletes the entire Docker package (including all digests/versions) from Artifact Registry.
#     """

#     def post(self, request, registry_id, image_name):
#         """
#         Expects `image_name` to be the full resource path returned by list_docker_images
#         (e.g. 'projects/.../dockerImages/myimage@sha256:abcdef...').
#         We then remove everything after '@' and replace '/dockerImages/' with '/packages/'.
#         This yields the package name with no digest, so that delete_package removes the entire image.
#         """
#         registry = get_object_or_404(ContainerRegistry, id=registry_id)

#         # Load credentials
#         try:
#             credential_info = json.loads(registry.cluster.cloud_credential.detail)
#             credentials = service_account.Credentials.from_service_account_info(credential_info)
#             client = artifactregistry_v1.ArtifactRegistryClient(credentials=credentials)
#         except Exception as e:
#             logger.exception("Failed to load Artifact Registry credentials.")
#             messages.error(request, f"Invalid or missing cloud credentials: {e}")
#             return redirect("registry-detail", pk=registry_id)

#         logger.info("User requested to delete the ENTIRE Docker image package: %s", image_name)

#         try:
#             # Replace 'dockerImages/' with 'packages/'
#             package_resource = image_name.replace("/dockerImages/", "/packages/")
#             logger.info("After replacing 'dockerImages' -> 'packages': %s", package_resource)

#             # Strip off any '@sha256:...' part by splitting at '@'
#             #    The package name is everything BEFORE '@'
#             no_digest_package = package_resource.split("@", 1)[0]
#             logger.info("Final package resource (no digest): %s", no_digest_package)

#             # Delete the entire package (all versions/digests)
#             client.delete_package(name=no_digest_package)
#             messages.success(request, "Deleted entire container package successfully.")
#             logger.info("Successfully deleted package %s", no_digest_package)

#         except exceptions.NotFound:
#             logger.warning("Package resource not found: %s", no_digest_package, exc_info=True)
#             messages.error(request, "Package not found in Artifact Registry.")
#         except exceptions.PermissionDenied:
#             logger.warning("Permission denied while deleting: %s", no_digest_package, exc_info=True)
#             messages.error(request, "You do not have permission to delete this container.")
#         except Exception as e:
#             logger.error("Error deleting entire package '%s': %s", no_digest_package, e, exc_info=True)
#             messages.error(request, f"Error deleting image: {e}")

#         return redirect("registry-detail", pk=registry_id)


class DeleteContainerView(LoginRequiredMixin, View):
    """
    Deletes the entire Docker package (all digests) from Artifact Registry,
    and removes the corresponding builds from ContainerRegistry.build_info.
    """

    def post(self, request, registry_id, image_name):
        registry = get_object_or_404(ContainerRegistry, id=registry_id)

        # 1) Load credentials
        try:
            credential_info = json.loads(registry.cluster.cloud_credential.detail)
            credentials = service_account.Credentials.from_service_account_info(credential_info)
            client = artifactregistry_v1.ArtifactRegistryClient(credentials=credentials)
        except Exception as e:
            logger.exception("Failed to load Artifact Registry credentials.")
            messages.error(request, f"Invalid or missing cloud credentials: {e}")
            return redirect("registry-detail", pk=registry_id)

        logger.info("User requested to delete the ENTIRE Docker image: %s", image_name)

        # 2) Convert '.../dockerImages/...@sha256:...' to '.../packages/<NAME>'
        #    We remove any '@sha256:' portion so we can delete the entire package.
        package_resource = image_name.replace("/dockerImages/", "/packages/")
        logger.info("Replaced 'dockerImages' -> 'packages': %s", package_resource)
        
        no_digest_package = package_resource.split("@", 1)[0]
        logger.info("Final package resource (no digest): %s", no_digest_package)

        # The actual package name portion (the final path component)
        # e.g. "nvcrio-container-toolkit-v1.17.5-ubuntu20.04"
        package_name_only = no_digest_package.rsplit("/", 1)[-1]
        logger.info("Extracted package name only: %s", package_name_only)

        # 3) Delete the entire package (removing all versions/digests)
        try:
            client.delete_package(name=no_digest_package)
            messages.success(request, "Deleted entire container package successfully.")
            logger.info("Successfully deleted package %s", no_digest_package)
        except exceptions.NotFound:
            logger.warning("Package resource not found: %s", no_digest_package, exc_info=True)
            messages.error(request, "Package not found in Artifact Registry.")
        except exceptions.PermissionDenied:
            logger.warning("Permission denied while deleting package: %s", no_digest_package, exc_info=True)
            messages.error(request, "You do not have permission to delete this container.")
        except Exception as e:
            logger.error("Error deleting package '%s': %s", no_digest_package, e, exc_info=True)
            messages.error(request, f"Error deleting image: {e}")
            return redirect("registry-detail", pk=registry_id)

        # 4) Remove matching build_info entries
        #    We look for any builds whose 'dest_image' references this same package name.
        #    Typically 'dest_image' might be something like:
        #       "us-central1-docker.pkg.dev/<PROJECT>/<REPO>/nvcrio-container-toolkit-v1.17.5-ubuntu20.04:latest"
        #    We'll do a substring check for `package_name_only`.
        old_build_info = registry.build_info or []
        new_build_info = []
        removed_count = 0

        for build in old_build_info:
            dest_image = build.get("dest_image", "")
            # If this build references the same container name, skip it
            if package_name_only in dest_image:
                logger.info("Removing build %s referencing package %s from registry ID %s",
                            build.get("build_id"), package_name_only, registry.id)
                removed_count += 1
            else:
                new_build_info.append(build)

        if removed_count > 0:
            registry.build_info = new_build_info
            registry.save(update_fields=["build_info"])
            logger.info("Removed %d build_info entries for package %s", removed_count, package_name_only)

        return redirect("registry-detail", pk=registry_id)
