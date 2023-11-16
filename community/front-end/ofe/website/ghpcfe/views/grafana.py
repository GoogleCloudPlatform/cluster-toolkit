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

""" Grafana integration views """

from django.contrib.auth.mixins import LoginRequiredMixin
from django.views.generic import base
from revproxy.views import ProxyView

class GrafanaProxyView(LoginRequiredMixin, ProxyView):
    """Proxy View"""
    upstream = "http://127.0.0.1:3000/grafana"

    def get_proxy_request_headers(self, request):
        headers = super().get_proxy_request_headers(request)
        headers["X-WEBAUTH-USER"] = request.user.email
        headers["Host"] = request.get_host()
        return headers

    def dispatch(self, request, path):
        response = super().dispatch(request, path)
        response["X-Frame-Options"] = "SAMEORIGIN"
        return response

class GrafanaView(LoginRequiredMixin, base.TemplateView):
    template_name = "grafana.html"

    def get_context_data(self, *args, **kwargs):
        context = super().get_context_data(*args, **kwargs)
        context["navtab"] = "grafana"
        return context
