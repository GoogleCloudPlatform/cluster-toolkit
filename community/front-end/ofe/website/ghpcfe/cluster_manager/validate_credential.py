#!/usr/bin/env python3
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

"""Cloud credential validation routines"""

import logging
import json
import warnings

from google.oauth2 import service_account

logger = logging.getLogger(__name__)


def validate_credential(cloud_provider, credential_detail):
    """communicate with a cloud provider to validate a credential"""

    validated = False
    if cloud_provider == "GCP":
        validated = _validate_credential_gcp(credential_detail)

    return validated


def _validate_credential_gcp(credential_detail):

    # catch errors for incorrect format
    try:
        info = json.loads(credential_detail)
    except Exception as err:  # pylint: disable=broad-except
        logger.info("Failed to parse credential Json: %s", err)
        return False

    # I've seen different error conditions, including a warning to indicate
    # corrupted private key. Need to catch that but not other harmless warnings.
    warnings.simplefilter("ignore", ResourceWarning)
    warnings.simplefilter("error", UserWarning)
    try:
        service_account.Credentials.from_service_account_info(
            info
        )
    except Exception as err: #pylint: disable=broad-except
        logger.info("Credential validation failed: %s", err)
        return False

    return True
