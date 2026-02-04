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

from allauth.socialaccount.models import SocialApp
from django.contrib.sites.models import Site
from django.core.management.base import BaseCommand
from django.core.management.base import CommandError

from ghpcfe.models import Role
from ghpcfe.models import User


class Command(BaseCommand):
    """Custom setup to add google oauth"""

    help = "My custom startup command"

    def add_arguments(self, parser):
        parser.add_argument(
            "client_id",
            type=str,
            help="Client ID",
        )
        parser.add_argument(
            "secret",
            type=str,
            help="Client secret key",
        )
        parser.add_argument(
            "sitename",
            type=str,
            help="Site Name for Google OAuth",
        )

    def handle(self, *args, **kwargs):
        client_id = kwargs["client_id"]
        secret = kwargs["secret"]
        site_name = kwargs["sitename"]
        try:
            # one-off database initialisation
            records = Role.objects.all()
            if not records:
                roles = []
                # populate Role table
                for role in Role.ROLE_CHOICES:
                    Role.objects.create(id=role[0])
                    roles.append(role[0])
                # give the super user all the roles
                user = User.objects.get(pk=1)
                user.roles.set(roles, clear=True)
                # set the super user with unlimited quota
                user.quota_type = "u"
                user.save()
                # initialise database for Google social login
                site = Site.objects.get(pk=1)
                site.name = site_name
                site.domain = site_name
                site.save()
                socialapp = SocialApp(
                    provider="google",
                    name="Google API",
                    client_id=client_id,
                    key="",
                    secret=secret,
                )
                socialapp.save()
                socialapp.sites.add(site)
        except Exception as err:
            raise CommandError("Initialization failed.") from err
