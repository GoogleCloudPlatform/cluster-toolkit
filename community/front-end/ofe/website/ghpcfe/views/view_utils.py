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

"""Common helpers used in multiple views"""

from cachetools import TTLCache, cached
from pathlib import Path

from google.cloud import artifactregistry_v1
from google.oauth2 import service_account
from google.auth.transport.requests import Request
from google.api_core import exceptions

from django.http import HttpResponseNotFound, FileResponse, HttpResponseRedirect
from django.views import generic
from django.contrib import messages
from django.shortcuts import reverse

from ..cluster_manager import cloud_info

import logging
import json
import time
import ast

logger = logging.getLogger(__name__)

container_images_cache = TTLCache(maxsize=100, ttl=300)


class LocalFile:
    """Local file access helper"""

    def __init__(self, filename):
        self.filename = Path(filename)

    def get_file(self):
        return self.filename

    def open(self):
        return self.get_file().open("rb")

    def exists(self):
        return self.get_file().exists()

    def get_filename(self):
        return self.get_file().name


class TerraformLogFile(LocalFile):
    """Terraform log file helper"""

    def __init__(self, prefix):
        self.prefix = Path(prefix)
        super().__init__("terraform.log")

    def set_prefix(self, prefix):
        self.prefix = Path(prefix)

    def get_file(self):
        for phase in ["destroy", "apply", "plan", "init"]:
            tf_log = self.prefix / f"terraform_{phase}_log.stderr"

            if (not tf_log.exists()) or tf_log.stat().st_size == 0:
                tf_log = self.prefix / f"terraform_{phase}_log.stdout"

            if tf_log.exists():
                break

        if tf_log.exists():
            logger.info("Found terraform log file %s", tf_log.as_posix())
        else:
            logger.warning("Found no terraform log files")

        return tf_log

    def get_filename(self):
        return "terraform.log"


class GCSFile:
    """GCS file access helper"""

    def __init__(self, bucket, basepath, prefix):
        self.bucket = bucket
        self.basepath = basepath
        self.prefix = prefix

    def get_path(self):
        return "/".join([self.prefix, self.basepath])

    def exists(self):
        return cloud_info.gcs_get_blob(self.bucket, self.get_path()).exists()

    def open(self):
        logger.debug(
            "Attempting to open gs://%s%s", self.bucket, self.get_path()
        )
        return cloud_info.gcs_get_blob(self.bucket, self.get_path()).open(
            mode="rb", chunk_size=4096
        )

    def get_filename(self):
        return self.basepath.split("/")[-1]


class StreamingFileView(generic.base.View):
    """View for a file that is being updated"""

    def get(self, request, *args, **kwargs):
        try:
            file_info = self.get_file_info()
            if file_info.exists():
                return FileResponse(
                    file_info.open(),
                    filename=file_info.get_filename(),
                    as_attachment=False,
                    content_type="text/plain",
                )
            return HttpResponseNotFound("Log file does not exist")

        # Not a lot we can do, regardless of error type, so just report back
        except Exception as err: # pylint: disable=broad-except
            logger.warning("Exception trying to stream file", exc_info=err)
            return HttpResponseNotFound("Log file not found")


class RegistryDataHelper:
    """
    Helper class to fetch container images from Artifact Registry, normalise them,
    and merge them with build info.
    """

    def __init__(self, request, registry):
        self.request = request
        self.registry = registry
        self.project_id = registry.get_project_id()
        self.cloud_region = registry.cluster.cloud_region
        self.loading = 0

    @classmethod
    @cached(container_images_cache)
    def fetch_container_images(cls, request, project_id, registry, cloud_region):
        """
        Fetch container images from Artifact Registry.
        """
        logger.info("Initiating fetch_container_images for registry ID: %s", registry.id)
        try:
            # Load credentials from the registry's cloud_credential.
            credential_info = json.loads(registry.cluster.cloud_credential.detail)
            credentials = service_account.Credentials.from_service_account_info(credential_info)
            
            # Initialise Artifact Registry client.
            client = artifactregistry_v1.ArtifactRegistryClient(credentials=credentials)
            parent = f"projects/{project_id}/locations/{cloud_region}/repositories/{registry.repository_id}"
            logger.info("Requesting Docker images from: %s", parent)
            
            # Fetch images.
            response = client.list_docker_images(parent=parent)
            if not response:
                logger.warning(f"No images found in registry: {registry.repository_id}")
                return []
            images = []
            for image in response:
                # Extract image name and tags.
                image_name = image.name.split("/")[-1].split("@")[0]
                full_resource_name = image.name
                raw_tags = image.tags
                extracted_tags = cls.ensure_list(raw_tags)
                logger.debug("Image '%s': raw tags = %s, extracted tags = %s",
                            image_name, raw_tags, extracted_tags)
                images.append({
                    "resource_name": full_resource_name,
                    "name": image_name,
                    "tags": extracted_tags,
                    "uri": image.uri,
                    "update_time": image.update_time.isoformat() if image.update_time else ''
                })
            
            image_count = len(images)
            if image_count > 0:
                logger.info("Successfully fetched %d images for registry ID: %s", image_count, registry.id)
            else:
                logger.info("No images found for registry ID: %s", registry.id)
            return images

        except exceptions.PermissionDenied as e:
            logger.error("Permission Denied: %s", e, exc_info=True)
            messages.error(request, "Permission Denied while fetching images.")
            return []
        except json.JSONDecodeError:
            logger.error("Invalid credential JSON format.")
            messages.error(request, "Invalid credential format.")
            return []
        except Exception as e:
            logger.error("Error fetching images: %s", e, exc_info=True)
            messages.error(request, "Unexpected error occurred while fetching images.")
            return []

    def merge_build_info(self, container_images):
        image_index = {}
        
        for img in container_images:
            norm_uri = self.normalize_image(img.get("uri", ""))
            if norm_uri not in image_index:
                image_index[norm_uri] = {
                    "resource_name": img.get("resource_name"),  # ADD THIS LINE
                    "name": img.get("name"),
                    "uri": img.get("uri"),
                    "tags": set(img.get("tags", [])),
                    "builds": [],
                }
            else:
                image_index[norm_uri]["tags"].update(img.get("tags", []))
                if not image_index[norm_uri].get("resource_name"):
                    image_index[norm_uri]["resource_name"] = img.get("resource_name")

        pending_rows = {}

        for info in self.registry.build_info:
            build_id = info.get("build_id", "unknown")
            build_status = info.get("status", "n")
            build_url = self.registry.get_build_url(build_id)
            dest_image = info.get("dest_image")
            tags = self.ensure_list(info.get("tags"))
            norm_dest = self.normalize_image(dest_image)

            target_dict = image_index if norm_dest in image_index else pending_rows

            if norm_dest not in target_dict:
                target_dict[norm_dest] = {
                    "resource_name": None,
                    "name": info.get("name"),
                    "uri": dest_image,
                    "tags": set(tags),
                    "builds": [],
                }

            target_dict[norm_dest]["tags"].update(tags)
            target_dict[norm_dest]["builds"].append({
                "build_id": build_id,
                "status": build_status,
                "url": build_url,
            })

            if build_status == "i":
                self.loading = 1

        container_images = list(image_index.values()) + list(pending_rows.values())

        for img in container_images:
            img["tags"] = sorted(img["tags"])

        return container_images

    def get_data(self):
        """
        Retrieve container images merged with build info.
        Returns a tuple (container_images, loading)
        """
        images = self.fetch_container_images(self.request, self.project_id, self.registry, self.cloud_region)
        images = self.merge_build_info(images)

        return images, self.loading

    @staticmethod
    def ensure_list(tags):
        """
        Convert tags into a list.
        If tags is already a list, return it;
        if it's a string representation of a list, evaluate it safely;
        otherwise, return it as a single-item list.
        """
        if isinstance(tags, list):
            return tags
        if isinstance(tags, str):
            tags = tags.strip()
            if tags.startswith('[') and tags.endswith(']'):
                try:
                    evaluated = ast.literal_eval(tags)
                    return evaluated if isinstance(evaluated, list) else [evaluated]
                except Exception:
                    return [tags]
            return [tags]
        return []

    @staticmethod
    def normalize_image(image_str):
        if not image_str:
            return ""
        return image_str.split("@")[0].split(":")[0]

    @staticmethod
    def match_artifact(build_dest, artifact_uri):
        """
        Compare two image references by normalizing them.
        Returns True if they match.
        """
        norm_build = RegistryDataHelper.normalize_image(build_dest)
        norm_artifact = RegistryDataHelper.normalize_image(artifact_uri)
        logger.debug("Matching artifact: build='%s', artifact='%s'", norm_build, norm_artifact)
        return norm_build == norm_artifact
