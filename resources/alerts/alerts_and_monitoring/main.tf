resource "google_monitoring_notification_channel" "notification_channel" {
  count        = "${length(var.notification_email_addresses)}"
  project      = "${var.project_id}"
  enabled      = true
  display_name = "Send email to ${element(var.notification_email_addresses, count.index)}"
  type         = "email"

  labels = {
    email_address = "${element(var.notification_email_addresses, count.index)}"
  }
}


resource "google_monitoring_alert_policy" "firestore_CPU_Utilization" {
  display_name = "${var.project_id}-firestore-CPU-Utilization"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"
  
  
  conditions   {
    display_name = "${var.project_id}-CPU-Utilization"
    
    condition_threshold  {
      filter          = "metric.type=\"compute.googleapis.com/instance/cpu/utilization\" AND resource.type=\"gce_instance\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.cpu_threshold
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = var.aggregations_aligner
      }
    }
  }
}

resource "google_monitoring_alert_policy" "firestore_Disk_storage" {
  display_name = "${var.project_id}-firestore-Disk-storage"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"

  conditions   {
    display_name = "${var.project_id}-firestore-Disk-storage"
        
    condition_threshold  {
      filter          = "metric.type=\"file.googleapis.com/nfs/server/used_bytes_percent\" AND resource.type=\"filestore_instance\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.filestore_disk_threshold
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = var.aggregations_aligner
      }
    }
  }
}


resource "google_monitoring_alert_policy" "compute_engine_memory_Usage" {
  display_name = "${var.project_id}-compute-engine-memory-Usage"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"
  
  
  conditions   {
    display_name = "${var.project_id}-compute-engine-memory-Usage"
    
    condition_threshold  {
      filter          = "metric.type=\"compute.googleapis.com/instance/memory/balloon/ram_used\" AND resource.type=\"gce_instance\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.gce_threshold
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = var.aggregations_aligner
      }
    }
  }
}

resource "google_monitoring_alert_policy" "SQL_Storage" {
  display_name = "${var.project_id}-SQL-Storage"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"



  conditions   {
    display_name = "${var.project_id}-SQL-Storage"
        
    condition_threshold  {
      filter          = "metric.type=\"cloudsql.googleapis.com/database/memory/utilization\" AND resource.type=\"cloudsql_database\""
                         
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.cloudsql_storage_threshold
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = var.aggregations_aligner
      }
    }
  }
}

resource "google_monitoring_alert_policy" "Bigquery_execution_time" {
  display_name = "${var.project_id}-Bigquery-query-execution-time"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"

  conditions   {
    display_name = "Bigquery query execution time"
    
    condition_threshold  {
      filter          = "metric.type=\"bigquery.googleapis.com/query/execution_times\" AND resource.type=\"bigquery_project\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.query_exection_time
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = "ALIGN_PERCENTILE_99"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "GCS_Request_Count" {
  display_name = "${var.project_id}-GCS-Request-Count"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"

    conditions   {
    display_name = "${var.project_id}-GCS-Request-Count"
    
    condition_threshold  {
      filter          = "metric.type=\"storage.googleapis.com/api/request_count\" AND resource.type=\"gcs_bucket\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.storage_request_count
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = "ALIGN_DELTA"
      }
    }
  }
}

resource "google_monitoring_alert_policy" "SQL_Network_Connection_Count" {
  display_name = "${var.project_id}-SQL-Network-Connection-Count"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"

    conditions   {
    display_name = "${var.project_id}-SQL-Network-Connection-Count"
    
    condition_threshold  {
      filter          = "metric.type=\"cloudsql.googleapis.com/database/network/connections\" AND resource.type=\"cloudsql_database\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.sql_network_connection
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = var.aggregations_aligner
      }
    }
  }
}
    

resource "google_monitoring_alert_policy" "SQL_memory" {
  display_name = "${var.project_id}-SQL-Memory"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"
    
  conditions   {
    display_name = "${var.project_id}-SQL-Memory"
    
    condition_threshold  {
      filter          = "metric.type=\"cloudsql.googleapis.com/database/memory/utilization\" AND resource.type=\"cloudsql_database\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.memory_utilization
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = var.aggregations_aligner
      }
    }
  }
}

resource "google_monitoring_alert_policy" "Firewall_rule_hit" {
  display_name = "${var.project_id}-Firewall-Rule-Hits"
  combiner     = var.alert_combiner
  notification_channels = "${google_monitoring_notification_channel.notification_channel.*.name}"


  conditions   {
    display_name = "${var.project_id}-Firewall-Rule-Hits"
    
    condition_threshold  {
      filter          = "metric.type=\"firewallinsights.googleapis.com/vm/firewall_hit_count\" AND resource.type=\"gce_instance\""
      
      duration        = var.duration
      comparison      = var.condition_comparison
      threshold_value = var.hit_count
      trigger {
        count = var.trigger_count
      }
      aggregations {
        alignment_period   = var.duration
        per_series_aligner = "ALIGN_DELTA"
      }
    }
  }
}

# resource "google_monitoring_dashboard" "dashboard" {
#   dashboard_json = <<EOF
# {
#   "displayName": "GCE",
#   "gridLayout": {
#     "columns": "2",
#     "widgets": [
#       {
#         "title": "FirewallInsights",
#         "xyChart": {
#           "dataSets": [{
#             "timeSeriesQuery": {
#               "timeSeriesFilter": {
#                 "filter": "metric.type=\"firewallinsights.googleapis.com/vm/firewall_hit_count\" AND resource.type=\"gce_instance\""",
#                 "aggregation": {
#                   "perSeriesAligner": "ALIGN_RATE"
#                 }
#               },
#               "unitOverride": "1"
#             },
#             "plotType": "LINE"
#           }],
#           "timeshiftDuration": "0s",
#           "yAxis": {
#             "label": "y1Axis",
#             "scale": "LINEAR"
#           }
#         }
#       },
#       {
#         "text": {
#           "content": "Firewall Inisghts",
#           "format": "MARKDOWN"
#         }
#       },
#       {
#         "title": "Memory Usage",
#         "xyChart": {
#           "dataSets": [{
#             "timeSeriesQuery": {
#               "timeSeriesFilter": {
#                 "filter": "metric.type=\"compute.googleapis.com/instance/memory/balloon/ram_used\" AND resource.type=\"gce_instance\""",
#                 "aggregation": {
#                   "perSeriesAligner": "ALIGN_RATE"
#                 }
#               },
#               "unitOverride": "1"
#             },
#             "plotType": "STACKED_BAR"
#           }],
#           "timeshiftDuration": "0s",
#           "yAxis": {
#             "label": "y1Axis",
#             "scale": "LINEAR"
#           }
#         }
#       }
#     ]
#   }
# }

# EOF
# }