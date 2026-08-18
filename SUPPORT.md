# Cluster Toolkit Support Matrix

This document outlines the officially supported operating systems, architectures, and runtime environments for the Google Cloud Cluster Toolkit (`gcluster`).

## Supported Operating Systems & Architectures

| Distribution / OS | Minimum Version | Supported Architectures | Status |
| :--- | :--- | :--- | :--- |
| **Debian Linux** | Bullseye (11) | `amd64`, `arm64` | Fully Supported & CI Tested |
| **Ubuntu Linux** | 20.04 LTS | `amd64`, `arm64` | Fully Supported |
| **RHEL / Rocky Linux** | 8.x | `amd64`, `arm64` | Fully Supported |

## Compatibility Policy

* Binaries are built with **Go 1.26+** and compiled with **`CGO_ENABLED=0`** to ensure full static portability and eliminate strict runtime `glibc` dependency constraints across supported Linux environments.
* Automated CI pipelines validate every pull request against baseline minimum and modern userland environments.
