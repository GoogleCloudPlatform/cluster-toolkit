# TKFE Advanced Administration
<!--
0        1         2         3         4         5         6         7        8
1234567890123456789012345678901234567890123456789012345678901234567890234567890
-->
This guide outlines some advanced features for deploying and administering
TKFE.

- Multi-project configuration, including clusters in another project
- Creation and management of service accounts
- Least-privilege roles for users
- Least-privilege enabled APIs for projects

---

## Multi-project Configuration

By default, TKFE provisions resources in the project it is deployed in, as the
keyless provisioning account (`<deployment_name>-prov-sa`) that the deployment
script creates and the server registers as the *OFE provisioning account*
credential. Usually, the single project for both deploying and then
administering GCP resources via TKFE is sufficient.

It is possible though to separate concerns, such that one project deploys TKFE
and other projects administer GCP resources via the TKFE portal, to allow finer
user access management. Separation of projects is a matter of registering a
credential for each project in the TKFE portal:

1. Create a service account in the other project with the roles TKFE needs,
   for example with the helper script (see
   [Service Account Management](#service-account-management)).
1. Prefer to let TKFE impersonate it, with no key. Grant the web server's
   account the *Service Account Token Creator* role on the new account:

   ```bash
   gcloud iam service-accounts add-iam-policy-binding \
     <SERVICE_ACCOUNT_NAME>@<OTHER_PROJECT_ID>.iam.gserviceaccount.com \
     --project=<OTHER_PROJECT_ID> \
     --member="serviceAccount:<deployment_name>-fe-sa@<TKFE_PROJECT_ID>.iam.gserviceaccount.com" \
     --role="roles/iam.serviceAccountTokenCreator"
   ```

   Then, on the *Credentials* page, add a credential of kind
   *Impersonated service account (no key)* with the account and the other
   project's ID.
1. Alternatively, create a JSON key for the account and add a credential of
   kind *Uploaded service account key*. Image builds need this kind (see the
   [Admin Guide](admin_guide.md#credential-management)).

### Clusters in another project

A new cluster's blueprint grants the cluster's service accounts access to
resources in the TKFE project: *Storage Object Viewer* on the TKFE control
bucket, limited by an IAM condition to the files nodes read (every account),
and, for the controller only, *Storage Object User*
on the objects under `clusters/<cluster ID>/` (an IAM condition),
*Pub/Sub Publisher* on the command-and-control topic, and
*Pub/Sub Subscriber* on the cluster's command subscription. Terraform makes
these grants as the cluster's credential. When that credential provisions in
another project, its service account therefore needs, in the TKFE project,
permission to read and set the IAM policy of:

- the control bucket (`storage.buckets.getIamPolicy` and
  `storage.buckets.setIamPolicy`), for example from *Storage Admin* on the
  bucket;
- the command-and-control topic, named after the deployment
  (`pubsub.topics.getIamPolicy` and `pubsub.topics.setIamPolicy`), for
  example from *Pub/Sub Admin* on the topic;
- the cluster's command subscription,
  `<deployment_name>-cluster_<cluster ID>`
  (`pubsub.subscriptions.getIamPolicy` and
  `pubsub.subscriptions.setIamPolicy`). TKFE creates it when the cluster
  starts, so grant these on the TKFE project, for example with
  *Pub/Sub Admin*.

Permission to set the control bucket's IAM policy is control of the bucket:
the account can grant itself any role on it and then change or replace any
object, including the server's boot files such as `webserver/startup.sh`,
which the TKFE server runs as root when it boots. Trust that service account,
and everyone who can impersonate it or holds a key for it, as you trust the
TKFE provisioning account (see
[The provisioning account and its roles](#the-provisioning-account-and-its-roles)).
Grant it on the control bucket only, never on the TKFE project. No predefined
role holds only the two bucket permissions: *Storage Admin* and
*Storage Legacy Bucket Owner* also let the account write and delete objects.
The narrowest grant is a custom role with only
`storage.buckets.getIamPolicy` and `storage.buckets.setIamPolicy`, granted
on the bucket; it removes the direct object access, but not the account's
ability to grant itself that access.

TKFE makes none of these grants itself. Before it creates or updates the
cluster, it checks them as the credential's service account and, if one is missing, stops
with a message naming the identity, the permissions and the resource. A
cluster with existing service accounts (see the
[Admin Guide](admin_guide.md#using-existing-service-accounts)) is not checked:
TKFE makes no grants for it, so give its accounts their access directly.

With an impersonated credential, `gcluster create` also looks up each
machine type in the cluster's project as the web server's account (it uses
the server's own credentials, not the impersonated account). Grant that
account `compute.machineTypes.get` in the other project, for example with
*Compute Viewer*; otherwise the cluster's create log shows a 403 for
`compute.machineTypes.get`. A credential with an uploaded key does not need
this. TKFE makes no such grant.

#### Grafana dashboards

A cluster whose credential has an uploaded key gets a Grafana data source of
its own, which reads Cloud Monitoring as the key's account. A cluster whose
credential is impersonated uses the server's own data source, which reads
Cloud Monitoring as the web server's account. That account has
*Monitoring Viewer* only in the TKFE project, so for a cluster in another
project with an impersonated credential, grant it there; TKFE makes no such
grant:

```bash
gcloud projects add-iam-policy-binding <OTHER_PROJECT_ID> \
  --member="serviceAccount:<deployment_name>-fe-sa@<TKFE_PROJECT_ID>.iam.gserviceaccount.com" \
  --role="roles/monitoring.viewer"
```

A multi-project setup needs one extra step. Terraform state for every resource
TKFE provisions is held in a Cloud Storage bucket in the deployment project
(see [Terraform State](admin_guide.md#terraform-state)), and a service account
can write the state of the resources it provisions only if it has access to
that bucket. After creating an additional service account, add it to
`state_bucket_writers` in
`tf/state_bucket/deployments/<deployment_name>/admin.auto.tfvars` on the client
machine and re-apply that directory:

```shell
terraform -chdir=tf/state_bucket/deployments/<deployment_name> apply
```

`deploy.sh` never overwrites `admin.auto.tfvars`, so the entry survives
redeploys. Without this step, provisioning with that credential fails when
Terraform tries to write state.

---

## Service Account Management

[Service accounts](https://cloud.google.com/iam/docs/service-accounts) are
needed by TKFE to provision GCP resources on behalf of projects. They are
registered to the TKFE portal as credentials, either impersonated (no key) or
with an uploaded JSON key, as outlined in the
[Admin Guide](admin_guide.md#credential-management). The deployment script
creates the keyless provisioning account for the TKFE project; other accounts
may be needed for a multi-project configuration, or when a service account
with custom roles is required.

### The provisioning account and its roles

The helper script grants these project roles, each for the resources named:

| Role | Needed for |
| ---- | ---------- |
| `roles/compute.admin` | VMs, instance templates, disks, addresses, firewalls and health checks of clusters, networks, image builds and workbenches |
| `roles/storage.admin` | Buckets, objects and bucket IAM: the Slurm controller and startup-script buckets, and the cluster accounts' grants on the TKFE control bucket |
| `roles/file.editor` | Filestore instances, and listing them |
| `roles/notebooks.admin` | Workbench instances, and listing their locations |
| `roles/servicenetworking.networksAdmin` | Private service access for networks |
| `roles/cloudsql.admin` | Cloud SQL instances, databases and users for Slurm accounting |
| `roles/bigquery.connectionAdmin` | The BigQuery connection for Slurm accounting |
| `roles/secretmanager.admin` | Secrets, versions and secret IAM: container registry passwords and the Slurm controller's Cloud SQL secret |
| `roles/artifactregistry.admin` | Container registries and their IAM; listing and deleting container images |
| `roles/cloudbuild.builds.editor` | Cloud Build builds that copy container images into a registry |
| `roles/pubsub.admin` | IAM on the command-and-control topic and each cluster's subscription |
| `roles/monitoring.viewer` | Grafana's Cloud Monitoring data source, when this account's key is registered |
| `roles/serviceusage.serviceUsageConsumer` | Checks of enabled APIs, and the quota project |
| `roles/iam.serviceAccountAdmin` | Service accounts of clusters, image builds and workbenches, and their IAM |
| `roles/iam.serviceAccountUser` | Running VMs and builds as the cluster, image-builder, workbench and Cloud Build accounts |
| `roles/iam.roleAdmin` | The custom role of each cluster and image-builder account |
| `roles/resourcemanager.projectIamAdmin` | Project-level role grants to cluster, image-builder and workbench accounts |

The last four are IAM administration roles, granted on the whole project
without IAM conditions, because an IAM condition cannot limit them to the
accounts, custom roles and grants that TKFE creates. Whoever controls the
TKFE server controls this account. TKFE grants only the web server's account
permission to impersonate it, and every change it makes is recorded in Cloud
Audit Logs. Because the account can grant any role in the project, a
compromise of the web server can reach project-owner level; the organization
policy below narrows this, within the limits it lists.
Run TKFE in a project of its own (see the
[Admin Guide](admin_guide.md#provisioning-account)).

### Hardening the provisioning account with an organization policy

This is a recommendation for the TKFE project, which an administrator sets up
outside TKFE. TKFE does not create, check or need it.

What TKFE already prevents: TKFE never grants the basic roles *Owner*,
*Editor* or *Viewer*. The accounts of new clusters and image builds get their
roles from the `community/modules/iam/service-account-profiles` module, which
refuses basic roles, `roles/iam.*` roles and `roles/resourcemanager.*` roles
in its resource bindings, and accepts only custom roles, not predefined ones,
in its list of extra roles. Besides the provisioning account's own roles,
listed above, TKFE grants `roles/iam.*` roles only on single service
accounts: the web server's account may impersonate the provisioning account,
a cluster's Slurm controller may act as that cluster's compute accounts, and
a workbench's user may act as that workbench's account.

What remains: whoever controls the provisioning account (anyone who controls
the TKFE server) can still grant any role in the project, including *Owner*
or *Editor*, by calling the IAM API directly. An IAM condition cannot prevent
this. A Google Cloud organization policy can: a custom constraint on IAM allow
policies that refuses any change which grants these roles. See Google's
[Use custom organization policies for IAM](https://docs.cloud.google.com/iam/docs/org-policy-custom-constraints)
page, which this section follows.

1. Define the custom constraint in a file, for example `constraint.yaml`,
   replacing `ORGANIZATION_ID`:

   ```yaml
   name: organizations/ORGANIZATION_ID/customConstraints/custom.denyBasicRoleGrants
   resourceTypes:
   - iam.googleapis.com/AllowPolicy
   methodTypes:
   - CREATE
   - UPDATE
   condition: "resource.bindings.exists(binding, RoleNameMatches(binding.role, ['roles/owner', 'roles/editor']))"
   actionType: DENY
   displayName: Do not allow the Owner or Editor role to be granted
   ```

   Add `'roles/viewer'` to the list to refuse grants of *Viewer* too. TKFE
   does not need any of these roles. The condition is checked against the
   role bindings that a change adds or modifies, so existing grants, such as
   the project owner's, do not block other changes.
1. Set up the constraint with
   `gcloud org-policies set-custom-constraint constraint.yaml`.
1. Enforce it with an organization policy, in a file such as `policy.yaml`:

   ```yaml
   name: folders/FOLDER_ID/policies/custom.denyBasicRoleGrants
   spec:
     rules:
     - enforce: true
   dryRunSpec:
     rules:
     - enforce: true
   ```

   Enforce it on the organization (`organizations/ORGANIZATION_ID/...`) or
   on a folder that holds the TKFE project, rather than on the project
   alone, so that it is set and changed outside the project, where the
   provisioning account and the project's own IAM administrators have no
   access.
1. Try it in dry run first, which logs the grants it would refuse without
   refusing them:
   `gcloud org-policies set-policy policy.yaml --update-mask=dryRunSpec`.
   When the logs show nothing that should be allowed, enforce it with
   `gcloud org-policies set-policy policy.yaml --update-mask=spec`.
   A policy can take up to 15 minutes to take effect.

Who needs which role: defining the constraint and setting the policy need
*Organization Policy Administrator* (`roles/orgpolicy.policyAdmin`) on the
organization. TKFE's deployment and its provisioning account need no extra
role for this.

Limits:

- It does not stop the account creating a custom role with the same
  permissions as *Owner* or *Editor* and granting that. Refusing every
  custom role, for example with
  `RoleNameStartsWith(binding.role, ['projects/'])`, would break TKFE,
  whose clusters and image builds use custom roles of their own.
- The supported role functions are `RoleNameMatches`, `RoleNameStartsWith`,
  `RoleNameEndsWith` and `RoleNameContains`, so the constraint can match
  role names only, not the permissions a role holds.
- It does not affect grants that Google makes by default, such as *Owner*
  for a project's creator. An operation that automatically grants a refused
  role to a service agent or default service account can fail; Google's
  page describes how to handle this.
- Google's page does not state the feature's launch stage (preview or
  generally available).

### Creating a service account via the helper script

The helper script included in the TKFE repo can be used to create a service
account with the roles above when used by a user that has privileges within
the project (e.g. *Owner*). The roles could then be modified via `gcloud` or
GCP Console (both covered below).

To create a service account, with no key:

```bash
script/service_account.sh create <PROJECT_ID> <SERVICE_ACCOUNT_NAME> [--ask]
```

The name must be 6 to 30 characters long. If an account with this name
already exists, it is reused and given the roles, so the same command also
brings an existing account up to date. If that existing account has
user-managed keys, whose holders would get the roles, the script first warns,
naming each key with the command that deletes it; if the keys cannot be
listed, it warns that they were not checked and gives the command that lists
them. It then grants the roles anyway, unless `--ask` is given: then it asks
"Continue and grant the roles to this account?", and *N*, or no answer,
grants nothing and exits with status 3. The script creates the account only
when Google reports that it does not exist (`NOT_FOUND`), or, when reading the
account fails in another way (Google can answer `PERMISSION_DENIED` for an
account deleted moments ago), when the project's list of service accounts does
not include it; if the account is in that list, it is reused. If it cannot
check, for example because you may not read the project's service accounts or
the network fails, it prints gcloud's errors and exits with status 2, creating
and granting nothing. To let TKFE impersonate the account,
grant the web server's account *Service Account Token Creator* on it, as in
[Multi-project Configuration](#multi-project-configuration).

Only if a key credential is needed (for example for image builds), write a
key file:

```bash
script/service_account.sh credential <PROJECT_ID> <SERVICE_ACCOUNT_NAME> <PATH_TO_KEY_FILE>
```

The key file is plain text: paste its content into the TKFE credential form,
then delete the file. The account holds the provisioning roles, including IAM
administration, so anyone who holds the key can change the project's IAM
policy; delete the key with `gcloud iam service-accounts keys delete` when it
is no longer needed.

The script also has options to *list*, *check* and *delete* - see the
built-in help for instructions:

```bash
script/service_account.sh help
```

**Note to administrators/developers:** if the roles required for a
TKFE service account changes, due to changes within GCP, the script
must be modified (and docs including the list of roles above,
updated).

### Creating a service account via the GCP Console

A user with project privileges can also create service accounts via the GCP
Console.

1. Log in to the [GCP console](https://console.cloud.google.com/) and select
   the GCP project that the service account will provision in.

1. From the Navigation menu, select *IAM & Admin*, then *Service Accounts*.
   - Click the *CREATE SERVICE ACCOUNT* button.
   - Name the service account, optionally provide a description, and then
     click the *CREATE* button.

1. Grant the service account the roles listed in
   [The provisioning account and its roles](#the-provisioning-account-and-its-roles).

1. Click *Done* button.

1. Either let TKFE impersonate the account (recommended): open the account,
   select *Principals with access*, click *GRANT ACCESS*, enter the web
   server's account
   (`<deployment_name>-fe-sa@<TKFE_PROJECT_ID>.iam.gserviceaccount.com`), give
   it the *Service Account Token Creator* role and save. Then add a credential
   of kind *Impersonated service account (no key)* in TKFE.

1. Or, for a key credential, locate the new service account from the list,
   click *Manage Keys* from the *Actions* menu.
   - Click *ADD KEY*, then *Create new key*.
     - Select JSON as key type, and click the *CREATE* button.
     - A JSON key file will then be downloaded.
     - Copy the generated JSON content which should then be pasted into the
       credential creation form within the TKFE, with the kind
       *Uploaded service account key*. Then delete the downloaded file.
   - Click *Validate and Save* to register the new credential to TKFE.

### Creating a service account using `gcloud`

Alternatively the `gcloud` command line tool can be used to create a suitable
service account (this is what the helper script does with additional checks).
To create a service account with the required roles:

```bash
gcloud iam service-accounts create <SERVICE_ACCOUNT_NAME> --project=<PROJECT_ID>
for roleid in compute.admin \
              storage.admin \
              file.editor \
              notebooks.admin \
              servicenetworking.networksAdmin \
              cloudsql.admin \
              bigquery.connectionAdmin \
              secretmanager.admin \
              artifactregistry.admin \
              cloudbuild.builds.editor \
              pubsub.admin \
              monitoring.viewer \
              serviceusage.serviceUsageConsumer \
              iam.serviceAccountAdmin \
              iam.serviceAccountUser \
              iam.roleAdmin \
              resourcemanager.projectIamAdmin; do \
     gcloud projects add-iam-policy-binding <PROJECT_ID> \
      --member="serviceAccount:<SERVICE_ACCOUNT_NAME>@<PROJECT_ID>.iam.gserviceaccount.com" \
      --role="roles/$roleid" --condition=None; \
done
```

Then either grant the web server's account *Service Account Token Creator* on
the new account, as in [Multi-project Configuration](#multi-project-configuration),
and register it as an impersonated credential (recommended), or create a key
for a key credential:

```bash
gcloud iam service-accounts keys create <PATH_TO_KEY_FILE> \
    --iam-account=<SERVICE_ACCOUNT_NAME>@<PROJECT_ID>.iam.gserviceaccount.com
```

and copy the key JSON text from `PATH_TO_KEY_FILE` into the credentials form
on the TKFE, then delete the file.

The credential can now be used to create network, storage and compute resources
from the TKFE.

The roles can be changed to give finer control as outlined in the next section.

---

### Custom roles/permissions and APIs

The projects and user account used for deploying TKFE can be more tightly
controlled with respect to the enabled APIs and roles/permissions.

#### User Account

Rather than using *Owner* role, or the high-level roles stated in the Admin
Guide, the user account deploying TKFE can use a custom set of least-privilege
roles.  The complete list of required permissions is as follows:
<!-- TODO: this list needs regularly checking and maintaining -->

```Text
 compute.acceleratorTypes.list
 compute.addresses.use
 compute.disks.create
 compute.disks.get
 compute.firewalls.create
 compute.firewalls.delete
 compute.firewalls.get
 compute.globalOperations.get
 compute.instances.create
 compute.instances.delete
 compute.instances.get
 compute.instances.getSerialPortOutput
 compute.instances.setLabels
 compute.instances.setMetadata
 compute.instances.setServiceAccount
 compute.instances.setTags
 compute.machineTypes.list
 compute.networks.create
 compute.networks.delete
 compute.networks.get
 compute.networks.updatePolicy
 compute.projects.get
 compute.regionOperations.get
 compute.routers.create
 compute.routers.delete
 compute.routers.get
 compute.routers.update
 compute.subnetworks.create
 compute.subnetworks.delete
 compute.subnetworks.get
 compute.subnetworks.use
 compute.subnetworks.useExternalIp
 compute.zoneOperations.get
 compute.zones.get
 compute.zones.list
 file.instances.create
 file.instances.delete
 file.instances.get
 file.operations.get
 iam.roles.create
 iam.roles.delete
 iam.roles.get
 iam.roles.update
 iam.serviceAccounts.actAs
 iam.serviceAccounts.create
 iam.serviceAccounts.delete
 iam.serviceAccounts.get
 iam.serviceAccounts.getIamPolicy
 iam.serviceAccounts.setIamPolicy
 pubsub.subscriptions.create
 pubsub.subscriptions.delete
 pubsub.subscriptions.get
 pubsub.subscriptions.getIamPolicy
 pubsub.subscriptions.setIamPolicy
 pubsub.topics.attachSubscription
 pubsub.topics.create
 pubsub.topics.delete
 pubsub.topics.get
 pubsub.topics.getIamPolicy
 pubsub.topics.setIamPolicy
 resourcemanager.projects.get
 resourcemanager.projects.getIamPolicy
 resourcemanager.projects.setIamPolicy
 secretmanager.secrets.create
 secretmanager.secrets.delete
 secretmanager.secrets.get
 secretmanager.secrets.getIamPolicy
 secretmanager.secrets.setIamPolicy
 secretmanager.versions.access
 secretmanager.versions.add
 secretmanager.versions.destroy
 secretmanager.versions.get
 storage.buckets.create
 storage.buckets.delete
 storage.buckets.get
 storage.buckets.getIamPolicy
 storage.buckets.setIamPolicy
 storage.objects.create
 storage.objects.delete
 storage.objects.get
 storage.objects.list
```

<!--TODO: For a TKFE Service Account (the one registered with a credential to administer resources via the portal) is... -->

#### Project APIs

In a multi-project configuration, the enabled project APIs can also be
reduced to a subset of those APIs only needed for the functions required.

The project TKFE is deployed in needs every API in the
[Admin Guide list](admin_guide.md#gcp-project), even if it is not used to
administer GCP resources via the TKFE portal: the deployment script checks for
all of them and enables any that are missing (asking first, except with
`--config`). Among them are the Secret Manager API, for the server's
configuration and database-key secrets, and the IAM Service Account
Credentials API, through which the server acts as the keyless provisioning
account.

Another project that administers GCP resources via the TKFE portal (that has a
service account created within it, as covered above), needs the following APIs:

```Text
 Compute Engine API
 Cloud Monitoring API
 Cloud Resource Manager
 Cloud Logging API
 Cloud OS Login API
 Cloud Filestore API
 Cloud Billing API
 Vertex AI API
```
