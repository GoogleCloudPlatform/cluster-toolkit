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

# Import errors are expected from pylint here due to Django behaviour
# pylint: disable=import-error

"""Custom setup to add Google Oauth"""

from django.conf import settings
from django.core.management.base import BaseCommand

from grafana_api.grafana_face import GrafanaFace

class Command(BaseCommand):
    """Custom setup to add google oauth"""

    help = "My custom startup command"

    def add_arguments(self, parser):
        parser.add_argument(
            "email",
            type=str,
        )

    def handle(self, *args, **kwargs):
        email = kwargs["email"]

        # one-off user/admin initialisation
        api = GrafanaFace(auth=("admin", "admin"), host="localhost:3000")
        # Change password
        api.admin.change_user_password(1, settings.SECRET_KEY)
        api = GrafanaFace(
            auth=("admin", settings.SECRET_KEY),
            host="localhost:3000"
        )

        # Create SuperUser
        user = api.admin.create_user(
            {
                "name": "",
                "email": email,
                "login": email,
                # We use Proxy-Auth, but must have a non-blank password
                "password": "NotUsed1234",
                "OrgId": 1
            }
        )

        # Make SuperUser an admin and org Admin
        api.admin.change_user_permissions(user["id"], True)
        api.organization.update_user_current_organization(
            user_id = user["id"],
            user = {
                "role": "Admin",
            }
        )

        # Add Datasource for our own self
        api.datasource.create_datasource(
            {
                "name": "default",
                "type": "stackdriver",
                "isDefault": True,
                "access": "proxy",
                "jsonData": {
                    "authenticationType": "gce",
                }
            }
        )
