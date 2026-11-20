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
"""The base template's active-tab script on pages that set no navtab.

Pages such as the sign-in pages and the CMEK admin pages have no navtab, so
the script looked up the element "navtab-", got null and threw a TypeError.
"""
from django.contrib.auth.models import AnonymousUser
from django.template.loader import render_to_string
from django.test import RequestFactory, SimpleTestCase


class BaseNavtabTests(SimpleTestCase):

    def _render(self, **context):
        request = RequestFactory().get("/")
        request.user = AnonymousUser()
        return render_to_string("base_generic.html", context, request=request)

    def test_no_navtab_selects_no_tab(self):
        html = self._render()
        self.assertNotIn('activaTab("navtab-")', html)

    def test_empty_navtab_selects_no_tab(self):
        html = self._render(navtab="")
        self.assertNotIn('activaTab("navtab-")', html)

    def test_a_navtab_still_selects_its_tab(self):
        html = self._render(navtab="fs")
        self.assertIn('activaTab("navtab-fs")', html)

    def test_a_tab_missing_from_the_page_is_skipped(self):
        # e.g. a tab that is only rendered for some users
        html = self._render(navtab="fs")
        self.assertIn("if (el) el.className += ' active';", html)
