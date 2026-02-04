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

""" Grafana utilities """

import json
import logging

from grafana_api.grafana_face import GrafanaFace

from django.conf import settings

logger = logging.getLogger(__name__)

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


def create_cluster_dashboard(cluster):

    cred_info = json.loads(cluster.cloud_credential.detail)
    # pylint: disable=line-too-long
    panels = [
        {
            "datasource": cluster.cloud_credential.name,
            "fill": 1,
            "fillGradient": 0,
            "gridPos": {
                "h": 8,
                "w": 12,
                "x": 0,
                "y": 0,
            },
            "lines": True,
            "linewidth": 1,
            "renderer": "flot",
            "seriesOverrides": [],
            "targets": [
                {
                    "metricQuery": {
                        "aliasBy": "{{metric.label.instance_name}}",
                        "alignmentPeriod": "cloud-monitoring-auto",
                        "crossSeriesReducer": "REDUCE_NONE",
                        "editorMode": "visual",
                        "filters": [
                            "metadata.user_labels.ghpc_deployment",
                            "=",
                            f"{cluster.cloud_id}"
                        ],
                        "groupBys": [],
                        "metricKind": "GAUGE",
                        "metricType": "compute.googleapis.com/instance/cpu/utilization",
                        "perSeriesAligner": "ALIGN_INTERPOLATE",
                        "projectName": cred_info["project_id"],
                        "query": "",
                        "unit": "10^2.%",
                        "valueType": "DOUBLE",
                    },
                    "queryType": "metrics",
                    "refId": "CPU Utilization",
                }
            ],
            "title": "CPU Utilization",
            "type": "graph",
            "xaxis": {
                "buckets": None,
                "mode": "time",
                "name": None,
                "show": True,
                "values": [],
            },
            "yaxes": [
                {
                    "format": "percent",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": True,
                },
                {
                    "format": "short",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": False,
                },
            ],
        },
        {
            "datasource": cluster.cloud_credential.name,
            "fill": 1,
            "fillGradient": 0,
            "gridPos": {
                "h": 8,
                "w": 12,
                "x": 12,
                "y": 0,
            },
            "lines": True,
            "linewidth": 1,
            "renderer": "flot",
            "seriesOverrides": [],
            "targets": [
                {
                    "metricQuery": {
                        "aliasBy": "{{resource.label.instance_id}} - {{metric.label.state}}",
                        "alignmentPeriod": "cloud-monitoring-auto",
                        "crossSeriesReducer": "REDUCE_NONE",
                        "editorMode": "visual",
                        "filters": [
                            "metadata.user_labels.ghpc_deployment",
                            "=",
                            f"{cluster.cloud_id}"
                        ],
                        "groupBys": [],
                        "metricKind": "GAUGE",
                        "metricType": "agent.googleapis.com/memory/bytes_used",
                        "perSeriesAligner": "ALIGN_INTERPOLATE",
                        "projectName": cred_info["project_id"],
                        "query": "",
                        "unit": "By",
                        "valueType": "DOUBLE",
                    },
                    "queryType": "metrics",
                    "refId": "Memory Used",
                }
            ],
            "title": "Memory Used",
            "type": "graph",
            "xaxis": {
                "buckets": None,
                "mode": "time",
                "name": None,
                "show": True,
                "values": [],
            },
            "yaxes": [
                {
                    "format": "bytes",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": True,
                },
                {
                    "format": "short",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": False,
                },
            ],
        },
        {
            "datasource": cluster.cloud_credential.name,
            "fill": 1,
            "fillGradient": 0,
            "gridPos": {
                "h": 8,
                "w": 12,
                "x": 0,
                "y": 8,
            },
            "lines": True,
            "linewidth": 1,
            "renderer": "flot",
            "seriesOverrides": [],
            "targets": [
                {
                    "metricQuery": {
                        "aliasBy": "{{metric.label.instance_name}}",
                        "alignmentPeriod": "cloud-monitoring-auto",
                        "crossSeriesReducer": "REDUCE_NONE",
                        "editorMode": "visual",
                        "filters": [
                            "metadata.user_labels.ghpc_deployment",
                            "=",
                            f"{cluster.cloud_id}"
                        ],
                        "groupBys": [],
                        "metricKind": "DELTA",
                        "metricType": "compute.googleapis.com/instance/network/sent_bytes_count",
                        "perSeriesAligner": "ALIGN_DELTA",
                        "projectName": cred_info["project_id"],
                        "query": "",
                        "unit": "By",
                        "valueType": "INT64",
                    },
                    "queryType": "metrics",
                    "refId": "DataOut",
                }
            ],
            "title": "Network Bytes Out",
            "type": "graph",
            "xaxis": {
                "buckets": None,
                "mode": "time",
                "name": None,
                "show": True,
                "values": [],
            },
            "yaxes": [
                {
                    "format": "bytes",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": True,
                },
                {
                    "format": "short",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": False,
                },
            ],
        },
        {
            "datasource": cluster.cloud_credential.name,
            "fill": 1,
            "fillGradient": 0,
            "gridPos": {
                "h": 8,
                "w": 12,
                "x": 12,
                "y": 8,
            },
            "lines": True,
            "linewidth": 1,
            "renderer": "flot",
            "seriesOverrides": [],
            "targets": [
                {
                    "metricQuery": {
                        "aliasBy": "{{metric.label.instance_name}}",
                        "alignmentPeriod": "cloud-monitoring-auto",
                        "crossSeriesReducer": "REDUCE_NONE",
                        "editorMode": "visual",
                        "filters": [
                            "metadata.user_labels.ghpc_deployment",
                            "=",
                            f"{cluster.cloud_id}"
                        ],
                        "groupBys": [],
                        "metricKind": "DELTA",
                        "metricType": "compute.googleapis.com/instance/network/received_bytes_count",
                        "perSeriesAligner": "ALIGN_DELTA",
                        "projectName": cred_info["project_id"],
                        "query": "",
                        "unit": "By",
                        "valueType": "INT64",
                    },
                    "queryType": "metrics",
                    "refId": "DataIn",
                }
            ],
            "title": "Network Bytes In",
            "type": "graph",
            "xaxis": {
                "buckets": None,
                "mode": "time",
                "name": None,
                "show": True,
                "values": [],
            },
            "yaxes": [
                {
                    "format": "bytes",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": True,
                },
                {
                    "format": "short",
                    "label": None,
                    "logBase": 1,
                    "max": None,
                    "min": None,
                    "show": False,
                },
            ],
        },

    ]
    # pylint: enable=line-too-long
    dashboard = {
        "dashboard": {
            "id": None,
            "uid": None,
            "title": f"Cluster {cluster.name}",
            "panels": panels,
            "version": 0,
        },
        "filderId": 0,
        "overwrite": True,
    }
    auth = ("admin", settings.SECRET_KEY)
    api = GrafanaFace(auth=auth, host="localhost:3000")
    dash = api.dashboard.update_dashboard(dashboard)
    # {id, slug, status: success, uid, url, version}
    if dash["status"] != "success":
        logger.error("Grafana Dashboard creation failed! Ret: %s", dash)
    return dash
