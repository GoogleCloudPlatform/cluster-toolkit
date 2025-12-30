## Description
The tag module creates a tag key and the associated tag values.

If the key already exists, then the tag values passed are associated with the existing tag key.

The module creates of two resources; [google_tags_tag_key](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_key) and [google_tags_tag_value](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/tags_tag_value).

### Example
The following example creates a TagKey, TagValue, and LocationTagBinding resource.

```yaml
  - id: gke-h4d-fw-tags
    source: modules/management/tags
    settings:
      tag_key_parent: "projects/my-gcp-project"
      tag_key_short_name: "fw-falcon-tagkey"
      tag_key_description: "tagkey for firewall falcon VPC"
      tag_key_purpose: "GCE_FIREWALL"
      tag_key_purpose_data: "network=PROJECT_ID/NETWORK"
      tag_value:
        - short_name: "fw-falcon-tagvalue-1"
        - short_name: "fw-falcon-tagvalue-2"
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->

<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->