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

"""Common helpers used in multiple views"""

from pathlib import Path

from django.http import HttpResponseNotFound, FileResponse
from django.views import generic

from ..cluster_manager import cloud_info

import logging

logger = logging.getLogger(__name__)


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
