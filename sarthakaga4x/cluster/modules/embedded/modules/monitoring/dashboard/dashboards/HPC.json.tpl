{
  "displayName": "${title}: ${deployment_name}",
  "labels": ${jsonencode(labels)},
  "gridLayout": {
    "columns": 2,
    "widgets": [
      {
        "text": {
          "content": "HPC metrics from the ${deployment_name} deployment of the Cluster Toolkit.",
          "format": "MARKDOWN"
        },
        "title": "${title}"
      },
      {
        "title": "VM Instance - Memory utilization",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MEAN"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/memory/percent_used\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - CPU Utilization",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MEAN"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/cpu/utilization\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\"",
                  "pickTimeSeriesFilter": {
                    "direction": "TOP",
                    "numTimeSeries": 20,
                    "rankingMethod": "METHOD_MEAN"
                  }
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - CPU utilization (agent)",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MEAN"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/cpu/utilization\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                },
                "unitOverride": "%"
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Disk read operations",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/disk/read_ops_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Disk write operations",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/disk/write_ops_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Disk Read Bytes",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/disk/read_bytes_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Disk Write Bytes",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/disk/write_bytes_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "Throttled read bytes",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/disk/throttled_read_bytes_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "Throttled write bytes",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/disk/throttled_write_bytes_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Received packets",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/network/received_packets_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "VM Instance - Sent packets",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/network/sent_packets_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "VM Instance - Received bytes",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/network/received_bytes_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Sent bytes",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_MEAN",
                    "groupByFields": [
                      "metric.label.\"instance_name\"",
                      "metric.label.\"loadbalanced\"",
                      "resource.label.\"project_id\"",
                      "resource.label.\"instance_id\"",
                      "resource.label.\"zone\""
                    ],
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/network/sent_bytes_count\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\"",
                  "secondaryAggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MEAN"
                  }
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "GCE VM Instance - Network Traffic Bytes (agent)",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/interface/traffic\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "Network Packets (agent)",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_RATE"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/interface/packets\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "TCP connections",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MEAN"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/network/tcp_connections\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                },
                "unitOverride": "1"
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "VM Instance - CPU utilization for steal",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "STACKED_BAR",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MAX"
                  },
                  "filter": "metric.type=\"agent.googleapis.com/cpu/utilization\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\" metric.label.\"cpu_state\"=\"steal\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      },
      {
        "title": "VM Instance - CPU utilization [MEAN]",
        "xyChart": {
          "chartOptions": {
            "mode": "COLOR"
          },
          "dataSets": [
            {
              "minAlignmentPeriod": "60s",
              "plotType": "LINE",
              "targetAxis": "Y1",
              "timeSeriesQuery": {
                "apiSource": "DEFAULT_CLOUD",
                "timeSeriesFilter": {
                  "aggregation": {
                    "alignmentPeriod": "60s",
                    "crossSeriesReducer": "REDUCE_NONE",
                    "perSeriesAligner": "ALIGN_MEAN"
                  },
                  "filter": "metric.type=\"compute.googleapis.com/instance/cpu/utilization\" resource.type=\"gce_instance\" metadata.user_labels.\"ghpc_deployment\"=\"${deployment_name}\""
                }
              }
            }
          ],
          "timeshiftDuration": "0s",
          "yAxis": {
            "label": "y1Axis",
            "scale": "LINEAR"
          }
        }
      }%{ for widget in widgets ~},
      ${widget}
      %{endfor ~}
    ]
  }
}
