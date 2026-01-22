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

"""Permission handling view mixin classes"""

from rest_framework import permissions
from .models import Role
from django.contrib.auth.mixins import LoginRequiredMixin, UserPassesTestMixin


class CredentialPermission(permissions.BasePermission):

    message = "Only users with 'cluster admin' role can access this function"

    def has_permission(self, request, view):   # pylint: disable=unused-argument
        permission = False
        if Role.CLUSTERADMIN in [x.id for x in request.user.roles.all()]:
            permission = True
        return permission


class SuperUserRequiredMixin(LoginRequiredMixin, UserPassesTestMixin):
    def test_func(self):
        return self.request.user.is_superuser
