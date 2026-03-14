# VPC Peering Module

This module establishes VPC Network Peering between two Google Cloud VPC networks.

## Example Usage

```yaml
  - id: network1
    source: modules/network/vpc
    settings:
      network_name: vpc-1

  - id: network2
    source: modules/network/vpc
    settings:
      network_name: vpc-2

  - id: vpc_peering
    source: modules/network/vpc-peering
    use: [network1, network2]
    settings:
      local_network_self_link: $(network1.network_self_link)
      remote_network_self_link: $(network2.network_self_link)
      create_remote_peering: true
