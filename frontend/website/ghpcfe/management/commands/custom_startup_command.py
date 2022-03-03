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

from django.core.management.base import BaseCommand, CommandError
from ghpcfe.models import Role, User
from rest_framework.authtoken.models import Token
from django.contrib.sites.models import Site
from allauth.socialaccount.models import SocialApp
import yaml

class Command(BaseCommand):
    help = 'My custom startup command'

    def add_arguments(self, parser):
        parser.add_argument('client_id', type=str, help='Client ID',)
        parser.add_argument('secret', type=str, help='Client secret key',)

    def handle(self, *args, **kwargs):
        client_id = kwargs['client_id']
        secret = kwargs['secret']
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
                # initialise database for Google social login
                with open('../configuration.yaml', 'r') as file:
                    config = yaml.safe_load(file)
                    domain_name = config['config']['server']['domain_name']
                    site = Site.objects.get(pk=1)
                    site.name = domain_name
                    site.domain = domain_name
                    site.save()
                    socialapp = SocialApp(provider="google", name="Google API", client_id=client_id, key='', secret=secret)
                    socialapp.save()
                    socialapp.sites.add(site)
        except:
            raise CommandError('Initalization failed.')
