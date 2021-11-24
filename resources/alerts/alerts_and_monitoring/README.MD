<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 0.14.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | ~> 3.0 |

## Resources

| Name                                                         | Type     |
| ------------------------------------------------------------ | -------- |
| [google_monitoring_notification_channel](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/monitoring_notification_channel) | resource |
| ["google_monitoring_alert_policy" "firestore_CPU_Utilization"](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| ["google_monitoring_alert_policy" "firestore_Disk_storage" ](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [ "google_monitoring_alert_policy" "compute_engine_memory_Usage"](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| ["google_monitoring_alert_policy" "SQL_Storage" ](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| ["google_monitoring_alert_policy" "Bigquery_execution_time" ](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [ "google_monitoring_alert_policy" "GCS_Request_Count"](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| ["google_monitoring_alert_policy" "SQL_Network_Connection_Count"](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| ["google_monitoring_alert_policy" "SQL_memory" ](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| ["google_monitoring_alert_policy" "Firewall_rule_hit"](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default |
|:-----|-------------|------|---------|
| <a name="input_project"></a> [project\](#input\\_project\) | Project for which the alerts and monitoring  will be created | `string` | n/a |
| <a name="input_display_name"></a> [display\_name](#input\_display\_name) | The name of the current deployment | `string` | ALL ALERTS |
| <a name="input_alert_combiner"></a> [alert\_combiner](#input\_alert\_combiner) | The name of the network to be created. Defaults to {deployment\_name}\_net if null. | `string` | Combiner of alert |
| <a name="input_duration"></a> [duration](#input\_duration) | Duration of the condition | `string`       | 300s |
| <a name="input_condition_comparision"></a> [condition\_comparison](#input\_condition_comparison) | The region where Cloud NAT and Cloud Router will be configured | `string` | COMPARISON_GT |
| <a name="input_aggregations_aligner"></a> [aggregations\_aligner](#input\_aggregations\_aligner) | Aggregation aligner to be displayed | `string` | ALIGN_NONE |
| <a name="input_trigger_count"></a> [trigger\_count](#input\_trigger\_count) | trigger count | `number` | 1 |
| <a name="input_notification_email_addresses"></a> [notification_email_addresses](#input\_notification_email_addresses) | The email for notifications | `List(string)` | zi.wang@hcl.com |
| <a name="input_cpu_threshold"></a> [cpu\_threshold](#input\_cpu_threshold) | Mention the CPU threshold value | `string` | 0.5 |
| <a name="input_filestore_disk_threhold"></a> [filestore\_disk\_threshold](#input\_filestore_disk_threshold) | The threshold value at which alert will be sent  for file store disk | `string` | 80 |
| <a name="input_gce_threshold"></a> [gce\_threshold](#input\_project\_id) | The threshold value at which alert will be sent  for GCE memory usage | `string` | 0.6 |
| <a name="input_cloudsql_storage_threshold"></a> [cloudsql\_storage_threshold](#input\\_cloudsql\_storage_threshold) | The threshold value at which alert will be sent  for cloudsql storage | `string` | 80 |
| <a name="input_query_exection\_time"></a> [query_exection\_time](#input\_query\_exection\_time) | The threshold value at which alert will be sent  if bigquery execution time  overshoots the provided the value | `string` | 2 |
| <a name="input_storage_request_count"></a> [storage_request_count](#input\_storage\_request\_count) | The threshold value at which alert will be sent if the number of storage requests exceeds | `string` | 2000 |
| <a name="input_sql_network_connection"></a> [sql\_network\_connection](#input\sql_network_connection) | The threshold values at which alert will be sent if the number of connection to  cloudsql database exceeds | `string` | 1000 |
| <a name="input_memory_utilization"></a> [memory\_utilization](#input\_memory_utilization) | The threshold value at which alert will be sent if the memory utilization of cloudsql exceeds | `string` | 90 |
| <a name="input_hit_count"></a> [hit\_count](#input\_hit_count) | The threshold value at which alert will be sent if number of hits exceeds the threshold value | `string` | 100 |
