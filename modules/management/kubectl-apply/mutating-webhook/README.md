# Generic Mutating Admission Webhook

This directory contains the manifests for a generic mutating admission webhook. The webhook is designed to be easily configurable using [variables](#variables).


## Usage

1.  Ensure that `cert-manager` is being installed as part of your blueprint. You can add code snippet below to your blueprint.
    ```yaml
    - group: installs
      modules:
      # Install cert-manager
      - id: workload-manager-install
        source: modules/management/kubectl-apply
        use: [h4d-cluster]
        settings:
          apply_manifests:
          # cert-manager
          - source: "https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.yaml"
            server_side_apply: true
    ```

2.  `webhook-deployment.yaml.tftpl` and `mutating-webhook-configuration.yaml.tftpl` are the two files that need to be deployed to create a mutating webhook. These two files have variables that need to be defined by the user. Details about the variables are in the [variables](#variables) section.



3.  The variables are updated with custom values from within the blueprint using the `template_vars` parameter within `apply_manifests` setting. Example code on how to use the two files and pass values to the variables: 
    ```yaml
    - group: irdma
      modules:
      # Setup iRDMA Webhook
      - id: irdma-webhook-setup
        source: modules/management/kubectl-apply
        use: [h4d-cluster]
        settings:
          apply_manifests:
          - source: $(ghpc_stage("../../modules/management/kubectl-apply/mutating-webhook/webhook-deployment.yaml.tftpl"))
            template_vars:
              NAMESPACE: "irdma-health-check"
              WEBHOOK_SERVICE_NAME: "irdma-svc"
              WEBHOOK_DEPLOYMENT_NAME: "irdma-webhook-deployment"
              ISSUER_NAME: "selfsigned-issuer"
              CERTIFICATE_NAME: "irdma-webhook-cert"
              SECRET_NAME: "irdma-webhook-tls"
              WEBHOOK_IMAGE: "us-docker.pkg.dev/gce-ai-infra/cluster-toolkit/gke-irdma-webhook-server:v1.0.0"
          - source: $(ghpc_stage("../../modules/management/kubectl-apply/mutating-webhook/mutating-webhook-configuration.yaml.tftpl"))
            template_vars:
              NAMESPACE: "irdma-health-check"
              WEBHOOK_SERVICE_NAME: "irdma-svc"
              CERTIFICATE_NAME: "irdma-webhook-cert"
              MUTATING_WEBHOOK_CONFIGURATION_NAME: "irdma-mutating-webhook-config"

    ```


## Variables

### webhook-deployment.yaml.tftpl

| Variable                              | Description                                                                      | Example                               |
| ------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------- |
| `NAMESPACE`                           | The Kubernetes namespace for all resources.                                      | `my-webhook`                          |
| `WEBHOOK_SERVICE_NAME`                | The name of the webhook service.                                                 | `my-webhook-service`                  |
| `WEBHOOK_DEPLOYMENT_NAME`             | The name of the webhook deployment.                                              | `my-webhook-deployment`               |
| `ISSUER_NAME`                         | The name of the cert-manager Issuer.                                             | `my-webhook-issuer`                   |
| `CERTIFICATE_NAME`                    | The name of the cert-manager Certificate.                                        | `my-webhook-cert`                     |
| `SECRET_NAME`                         | The name of the Kubernetes Secret to store the TLS certificate.                  | `my-webhook-tls`                      |
| `WEBHOOK_IMAGE`                       | The container image for the webhook server.                                      | `my-registry/my-webhook-image:v1.0.0` |


### mutating-webhook-configuration.yaml.tftpl

| Variable                              | Description                                                                      | Example                               |
| ------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------- |
| `NAMESPACE`                           | The Kubernetes namespace for all resources.                                      | `my-webhook`                          |
| `WEBHOOK_SERVICE_NAME`                | The name of the webhook service.                                                 | `my-webhook-service`                  |
| `CERTIFICATE_NAME`                    | The name of the cert-manager Certificate.                                        | `my-webhook-cert`                     |
| `MUTATING_WEBHOOK_CONFIGURATION_NAME` | The name of the MutatingWebhookConfiguration.                                    | `my-webhook-configuration`            |
