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

""" Grafana utilities """

import json

from grafana_api.grafana_face import GrafanaFace

from django.conf import settings

def add_gcp_datasource(name, creds):
    auth = ("admin", settings.SECRET_KEY)
    api = GrafanaFace(auth=auth, host="localhost:3000")
    creds = json.loads(creds)
    api.datasource.create_datasource(
        {
            "name": name,
            "type": "stackdriver",
            "access": "proxy",
            "jsonData": {
                "authenticationType": "jwt",
                "tokenUri": creds["token_uri"],
                "clientEmail": creds["client_email"],
                "defaultProject": creds["project_id"],
            },
            "secureJsonData": {
                "privateKey": creds["private_key"],
            }
        }
    )
