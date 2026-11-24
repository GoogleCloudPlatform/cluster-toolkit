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
"""The filesystem list offers the same Delete/Destroy as the Filestore detail."""
from pathlib import Path
from types import SimpleNamespace as NS

from django.template import engines
from django.test import SimpleTestCase

ROOT = Path(__file__).resolve().parents[1]


class ListButtonsTests(SimpleTestCase):
    def buttons(self, cloud_state, applied):
        """Buttons for a row; applied None means not a Filestore."""
        src = (ROOT / "templates/filesystem/list.html").read_text()
        start = src.index('<div class="dropdown-menu"')
        end = src.index("</td>", start)
        row = NS(id=4, cloud_state=cloud_state)
        if applied is not None:
            row.gcpfilestorefilesystem = NS(cloud_state=cloud_state,
                                            cmek_applied=applied)
        page = engines["django"].from_string(
            "{% load static %}" + src[start:end]).render({"fs": row})
        return {name for name in ("Edit", "Delete", "Destroy")
                if f">{name}<" in page}

    def test_filestore_buttons_match_detail_page(self):
        cases = {
            ("nm", False): {"Edit", "Delete"},
            ("nm", True): {"Edit", "Destroy"},
            ("m", True): {"Destroy"},
            ("m", False): {"Destroy"},
            ("um", True): {"Destroy"},
            ("dm", True): {"Destroy"},
            ("cm", True): set(),
            ("xm", True): {"Delete"},
            ("i", False): {"Delete"},
        }
        for (state, applied), want in cases.items():
            with self.subTest(state=state, applied=applied):
                self.assertEqual(self.buttons(state, applied), want)

    def test_other_filesystems_keep_current_buttons(self):
        cases = {
            "nm": {"Edit", "Delete"},
            "xm": {"Delete"},
            "i": {"Delete"},
            "m": {"Destroy"},
            "um": set(),
        }
        for state, want in cases.items():
            with self.subTest(state=state):
                self.assertEqual(self.buttons(state, None), want)
