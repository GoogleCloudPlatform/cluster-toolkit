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
""" users.py """

from rest_framework import viewsets
from rest_framework.permissions import IsAuthenticated
from django.contrib.auth.mixins import LoginRequiredMixin
from django.urls import reverse, reverse_lazy
from django.views import generic
from ..models import User
from ..serializers import UserSerializer
from ..forms import UserUpdateForm, UserAdminUpdateForm
from ..permissions import SuperUserRequiredMixin


# list views
class UserListView(SuperUserRequiredMixin, generic.ListView):
    model = User
    template_name = "user/list.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "user"
        return context


class UserDetailView(SuperUserRequiredMixin, generic.DetailView):
    model = User
    template_name = "user/detail.html"

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "user"
        context["loggedin_user"] = self.request.user
        return context


class UserAdminUpdateView(SuperUserRequiredMixin, generic.UpdateView):
    """Allow admin users to update user quota and permissions"""

    model = User
    template_name = "user/adminupdate_form.html"
    form_class = UserAdminUpdateForm

    def get_success_url(self):
        return reverse("user-detail", kwargs={"pk": self.object.pk})

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "user"
        context["loggedin_user"] = self.request.user
        return context


# create/update views
class AccountUpdateView(LoginRequiredMixin, generic.UpdateView):
    """Custom UpdateView for Account model"""

    model = User
    success_url = reverse_lazy("account")
    template_name = "account/update_form.html"
    form_class = UserUpdateForm

    def get_context_data(self, **kwargs):
        context = super().get_context_data(**kwargs)
        context["navtab"] = "user"
        context["quota_type_friendly"] = User.QUOTA_TYPE
        return context

    def setup(self, request, *args, **kwargs):
        kwargs["pk"] = request.user.id
        print("Called AccountUpdateView setup()")
        print(kwargs)
        super().setup(request, *args, **kwargs)


# For APIs


class UserViewSet(viewsets.ReadOnlyModelViewSet):
    """Custom ModelViewSet for User model"""

    permission_classes = (IsAuthenticated,)
    queryset = User.objects.all().order_by("username")
    serializer_class = UserSerializer
