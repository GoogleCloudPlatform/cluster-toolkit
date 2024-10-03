## Description

Creates a Pub/Sub topic

Primarily used for FSI - MonteCarlo Tutorial: **[fsi-montecarlo-on-batch-tutorial]**.

[fsi-montecarlo-on-batch-tutorial]: ../docs/tutorials/fsi-montecarlo-on-batch/README.md

### Example

The following example creates a Pub/Sub topic.

```yaml
  - id: pubsub_topic
    source: community/modules/pubsub/topic
```

Also see usages in this
[example blueprint](../../../examples/fsi-montecarlo-on-batch.yaml).

## License

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
Copyright 2023 Google LLC

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
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 4.42 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 4.42 |
| <a name="provider_random"></a> [random](#provider\_random) | ~> 3.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_pubsub_schema.example](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_schema) | resource |
| [google_pubsub_topic.example](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/pubsub_topic) | resource |
| [random_id.resource_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_deployment_name"></a> [deployment\_name](#input\_deployment\_name) | The name of the current deployment | `string` | n/a | yes |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to add to the instances. Key-value pairs. | `map(string)` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Project in which the HPC deployment will be created | `string` | n/a | yes |
| <a name="input_schema_id"></a> [schema\_id](#input\_schema\_id) | The name of the pubsub schema to be created | `string` | `null` | no |
| <a name="input_schema_json"></a> [schema\_json](#input\_schema\_json) | The JSON definition of the pubsub topic schema | `string` | `"{  \n  \"name\" : \"Avro\",  \n  \"type\" : \"record\", \n  \"fields\" : \n      [\n       {\"name\" : \"ticker\", \"type\" : \"string\"},\n       {\"name\" : \"epoch_time\", \"type\" : \"int\"},\n       {\"name\" : \"iteration\", \"type\" : \"int\"},\n       {\"name\" : \"start_date\", \"type\" : \"string\"},\n       {\"name\" : \"end_date\", \"type\" : \"string\"},\n       {\n           \"name\":\"simulation_results\",\n           \"type\":{\n               \"type\": \"array\",  \n               \"items\":{\n                   \"name\":\"Child\",\n                   \"type\":\"record\",\n                   \"fields\":[\n                       {\"name\":\"price\", \"type\":\"double\"}\n                   ]\n               }\n           }\n       }\n      ]\n }\n"` | no |
| <a name="input_topic_id"></a> [topic\_id](#input\_topic\_id) | The name of the pubsub topic to be created | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_topic_id"></a> [topic\_id](#output\_topic\_id) | Name of the topic that was created. |
| <a name="output_topic_schema"></a> [topic\_schema](#output\_topic\_schema) | Name of the topic schema that was created. |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
