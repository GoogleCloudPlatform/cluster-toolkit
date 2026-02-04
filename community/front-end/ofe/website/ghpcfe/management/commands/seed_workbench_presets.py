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

"""Workbench configuration seeds"""

from django.core.management.base import BaseCommand
from ghpcfe.models import WorkbenchPreset


class Command(BaseCommand):
    """Helper to create workbenches with preset types
    """
    help = (
        "Seeds default presets into the workbench preset model so normal "
        "users can create workbenches of an approved machine type"
    )

    # def add_arguments(self, parser):
    #     parser.add_argument('poll_ids', nargs='+', type=int)

    def handle(self, *args, **options):
        self.stdout.write("Populating Workbench Presets...", ending="\n")

        # create small preset
        wbpreset1 = WorkbenchPreset()
        wbpreset1.name = "Small - 1x core with 3840 Memory"
        wbpreset1.machine_type = "n1-standard-1"
        wbpreset1.category = "Recommended"
        wbpreset1.save()
        self.stdout.write(
            str(wbpreset1)
            + ", Machine Type: "
            + wbpreset1.machine_type
            + ", Category: "
            + wbpreset1.category,
            ending="\n",
        )

        # create medium preset
        wbpreset2 = WorkbenchPreset()
        wbpreset2.name = "Medium - 2x cores with 7680 Memory"
        wbpreset2.machine_type = "n1-standard-2"
        wbpreset2.category = "Recommended"
        wbpreset2.save()
        self.stdout.write(
            str(wbpreset2)
            + ", Machine Type: "
            + wbpreset2.machine_type
            + ", Category: "
            + wbpreset2.category,
            ending="\n",
        )

        # create large preset
        wbpreset3 = WorkbenchPreset()
        wbpreset3.name = "Large - 4x cores with 15360 Memory"
        wbpreset3.machine_type = "n1-standard-4"
        wbpreset3.category = "Recommended"
        wbpreset3.save()
        self.stdout.write(
            str(wbpreset3)
            + ", Machine Type: "
            + wbpreset3.machine_type
            + ", Category: "
            + wbpreset3.category,
            ending="\n",
        )

        # create X-large preset
        wbpreset4 = WorkbenchPreset()
        wbpreset4.name = "X-Large - 8x cores with 30720 Memory"
        wbpreset4.machine_type = "n1-standard-8"
        wbpreset4.category = "Recommended"
        wbpreset4.save()
        self.stdout.write(
            str(wbpreset4)
            + ", Machine Type: "
            + wbpreset4.machine_type
            + ", Category: "
            + wbpreset4.category,
            ending="\n",
        )

        self.stdout.write("Completed populating Workbench PResets", ending="\n")
