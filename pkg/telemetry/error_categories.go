// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"context"
	"os"
	"regexp"
	"strings"
)

const (
	ErrTypePermissionDenied = "PermissionDenied"
	ErrTypeResourceNotFound = "ResourceNotFound"
	ErrTypeValidation       = "ValidationError"
	ErrTypeNetwork          = "NetworkError"
	ErrTypeTimeout          = "TimeoutError"
	ErrTypeCanceled         = "CanceledError"
	ErrTypeQuotaExceeded    = "QuotaExceeded"
	ErrTypeAuthentication   = "AuthenticationFailed"
	ErrTypeProvisioning     = "ProvisioningFailed"
	ErrTypeStockout         = "Stockout"
	ErrTypeAPIDisabled      = "APIDisabled"
	ErrTypeResourceExists   = "ResourceAlreadyExists"
	ErrTypeUnknown          = "Unknown"
)

var exactErrMatchers = []struct {
	target   error
	category string
}{
	{os.ErrPermission, ErrTypePermissionDenied},
	{os.ErrNotExist, ErrTypeResourceNotFound},
	{context.DeadlineExceeded, ErrTypeTimeout},
	{context.Canceled, ErrTypeCanceled},
}

var substringErrMatchers = []struct {
	substring string
	category  string
}{
	{"quota exceeded", ErrTypeQuotaExceeded},
	{"limit exceeded", ErrTypeQuotaExceeded},
	{"unauthorized", ErrTypeAuthentication},
	{"not authenticated", ErrTypeAuthentication},
	{"requires authentication", ErrTypeAuthentication},
	{"permission denied", ErrTypePermissionDenied},
	{"403 forbidden", ErrTypePermissionDenied},
	{"access denied", ErrTypePermissionDenied},
	{"not found", ErrTypeResourceNotFound},
	{"error 404", ErrTypeResourceNotFound},
	{"validation failed", ErrTypeValidation},
	{"invalid argument", ErrTypeValidation},
	{"invalid value", ErrTypeValidation},
	{"instance is currently unavailable", ErrTypeStockout},
	{"sufficient capacity", ErrTypeStockout},
	{"enough resources available", ErrTypeStockout},
	{"resource pool exhausted", ErrTypeStockout},
	{"api is disabled", ErrTypeAPIDisabled},
	{"has not been used in project", ErrTypeAPIDisabled},
	{"enable the api", ErrTypeAPIDisabled},
	{"already exists", ErrTypeResourceExists},
	{"alreadyexists", ErrTypeResourceExists},
	{"failed to provision", ErrTypeProvisioning},
	{"deployment failed", ErrTypeProvisioning},
	{"timeout", ErrTypeTimeout},
	{"deadline", ErrTypeTimeout},
	{"connection refused", ErrTypeNetwork},
	{"dial tcp", ErrTypeNetwork},
	{"connection reset", ErrTypeNetwork},
}

const (
	ErrTypeA2HighKueueInvalidArgument                = "A2HIGH_KUEUE_INVALID_ARGUMENT"
	ErrTypeA4XTopologyIssue                          = "A4X_TOPOLOGY_ISSUE"
	ErrTypeA4NcclInstallerFailed                     = "A4_NCCL_INSTALLER_FAILED"
	ErrTypeAccountIdLength                           = "ACCOUNT_ID_LENGTH"
	ErrTypeAnsibleConfigProblem                      = "ANSIBLE_CONFIG_PROBLEM"
	ErrTypeAnsibleDelegateFailure                    = "ANSIBLE_DELEGATE_FAILURE"
	ErrTypeAnsiblePermissions                        = "ANSIBLE_PERMISSIONS"
	ErrTypeApertureDeviceNotReady                    = "APERTURE_DEVICE_NOT_READY"
	ErrTypeApiPostHeadersTimeout                     = "API_POST_HEADERS_TIMEOUT"
	ErrTypeAptGetFailure                             = "APT_GET_FAILURE"
	ErrTypeAptHeaderPackageFailure                   = "APT_HEADER_PACKAGE_FAILURE"
	ErrTypeAptPycurlFailure                          = "APT_PYCURL_FAILURE"
	ErrTypeAssertError                               = "ASSERT_ERROR"
	ErrTypeBatchMountOptions                         = "BATCH_MOUNT_OPTIONS"
	ErrTypeBatchMpiTimeout                           = "BATCH_MPI_TIMEOUT"
	ErrTypeBatchRegionMismatch                       = "BATCH_REGION_MISMATCH"
	ErrTypeBatchResourcePoolExhausted                = "BATCH_RESOURCE_POOL_EXHAUSTED"
	ErrTypeBatchTimeout                              = "BATCH_TIMEOUT"
	ErrTypeBrokenCrdDebPackage                       = "BROKEN_CRD_DEB_PACKAGE"
	ErrTypeCapacityNotFoundInZone                    = "CAPACITY_NOT_FOUND_IN_ZONE"
	ErrTypeCentos7Eol                                = "CENTOS7_EOL"
	ErrTypeCheckBucketNameNotFound                   = "CHECK_BUCKET_NAME_NOT_FOUND"
	ErrTypeCidrRangeInUse                            = "CIDR_RANGE_IN_USE"
	ErrTypeCleanupCompute                            = "CLEANUP_COMPUTE"
	ErrTypeClusterAlreadyHasOperation                = "CLUSTER_ALREADY_HAS_OPERATION"
	ErrTypeCommandFailure                            = "COMMAND_FAILURE"
	ErrTypeComputeAddress                            = "COMPUTE_ADDRESS"
	ErrTypeComputeImagesGetForbidden                 = "COMPUTE_IMAGES_GET_FORBIDDEN"
	ErrTypeComputeVmCreateFail                       = "COMPUTE_VM_CREATE_FAIL"
	ErrTypeComputeVmFailed                           = "COMPUTE_VM_FAILED"
	ErrTypeConcurrentOperationsQuotaExceeded         = "CONCURRENT_OPERATIONS_QUOTA_EXCEEDED"
	ErrTypeConda502                                  = "CONDA_502"
	ErrTypeCondaExc                                  = "CONDA_EXC"
	ErrTypeConditionalMismatch                       = "CONDITIONAL_MISMATCH"
	ErrTypeConnectionFailure                         = "CONNECTION_FAILURE"
	ErrTypeCpusPerVmFamilyQuotaExceeded              = "CPUS_PER_VM_FAMILY_QUOTA_EXCEEDED"
	ErrTypeCreateConnectionTimeout                   = "CREATE_CONNECTION_TIMEOUT"
	ErrTypeDbVersionMismatch                         = "DB_VERSION_MISMATCH"
	ErrTypeDependenciesConflict                      = "DEPENDENCIES_CONFLICT"
	ErrTypeDeploymentManagerBadinput                 = "DEPLOYMENT_MANAGER_BADINPUT"
	ErrTypeDeploymentManagerFailure                  = "DEPLOYMENT_MANAGER_FAILURE"
	ErrTypeDeploymentManagerTimeout                  = "DEPLOYMENT_MANAGER_TIMEOUT"
	ErrTypeDeploymentNameNotString                   = "DEPLOYMENT_NAME_NOT_STRING"
	ErrTypeDeploymentReservationFailure              = "DEPLOYMENT_RESERVATION_FAILURE"
	ErrTypeDeprecatedVar                             = "DEPRECATED_VAR"
	ErrTypeDirNotFound                               = "DIR_NOT_FOUND"
	ErrTypeDisableAutoUpdatesNotSupported            = "DISABLE_AUTO_UPDATES_NOT_SUPPORTED"
	ErrTypeDnsSrvLookupFailed                        = "DNS SRV lookup failed"
	ErrTypeE2StandardStockout                        = "E2 Standard STOCKOUT"
	ErrTypeEndOfStartUpScriptFailure                 = "END_OF_START_UP_SCRIPT_FAILURE"
	ErrTypeEnrootPermissionDenied                    = "ENROOT_PERMISSION_DENIED"
	ErrTypeEntityAlreadyExists                       = "ENTITY_ALREADY_EXISTS"
	ErrTypeFailedHomeDirCreate                       = "FAILED_HOME_DIR_CREATE"
	ErrTypeFailedToCreateCluster                     = "FAILED_TO_CREATE_CLUSTER"
	ErrTypeFailedToCreateTimeout                     = "FAILED_TO_CREATE_TIMEOUT"
	ErrTypeFailedToDestroy                           = "FAILED_TO_DESTROY"
	ErrTypeFailedToDestroyBlockingMig                = "FAILED_TO_DESTROY_BLOCKING_MIG"
	ErrTypeFailedToDestroyGkeTimeout                 = "FAILED_TO_DESTROY_GKE_TIMEOUT"
	ErrTypeFailedToInstallProvider                   = "FAILED_TO_INSTALL_PROVIDER"
	ErrTypeFilestoreApiDisabled                      = "FILESTORE_API_DISABLED"
	ErrTypeFilestoreApiRateLimit                     = "FILESTORE_API_RATE_LIMIT"
	ErrTypeFilestoreCreationError                    = "FILESTORE_CREATION_ERROR"
	ErrTypeFilestoreInternalError                    = "FILESTORE_INTERNAL_ERROR"
	ErrTypeFilestoreProjectDenied                    = "FILESTORE_PROJECT_DENIED"
	ErrTypeFilestorePvcNotConnected                  = "FILESTORE_PVC_NOT_CONNECTED"
	ErrTypeFilestoreRace                             = "FILESTORE_RACE"
	ErrTypeFilestoreVpcLimit                         = "FILESTORE_VPC_LIMIT"
	ErrTypeFilestoreZoneCapacity                     = "FILESTORE_ZONE_CAPACITY"
	ErrTypeFioJobTime                                = "FIO_JOB_TIME"
	ErrTypeFirewallDestroyError                      = "FIREWALL_DESTROY_ERROR"
	ErrTypeFlexNoReservationError                    = "FLEX_NO_RESERVATION_ERROR"
	ErrTypeGcloudCrashed                             = "GCLOUD_CRASHED"
	ErrTypeGcloudOsLoginKeysLimit                    = "GCLOUD_OS_LOGIN_KEYS_LIMIT"
	ErrTypeGcpDisk409                                = "GCP_DISK_409"
	ErrTypeGenericResourceCollision                  = "GENERIC_RESOURCE_COLLISION"
	ErrTypeGg                                        = "GG"
	ErrTypeGhpcDepNotFound                           = "GHPC_DEP_NOT_FOUND"
	ErrTypeGithubError                               = "GITHUB_ERROR"
	ErrTypeGkeGpuDriverConfigNoAttribute             = "GKE_GPU_DRIVER_CONFIG_NO_ATTRIBUTE"
	ErrTypeGkeHyperDiskJobTimeout                    = "GKE_HYPER DISK_JOB_TIMEOUT"
	ErrTypeGkeInternalError                          = "GKE_INTERNAL_ERROR"
	ErrTypeGkeJobTimeout                             = "GKE_JOB_TIMEOUT"
	ErrTypeGkeNodepoolError                          = "GKE_NODEPOOL_ERROR"
	ErrTypeGkeNodepoolStateError                     = "GKE_NODEPOOL_STATE_ERROR"
	ErrTypeGkeNodeVersionNotSupported                = "GKE_NODE_VERSION_NOT_SUPPORTED"
	ErrTypeGkeParallelstoreJobTimeout                = "GKE_PARALLELSTORE_JOB_TIMEOUT"
	ErrTypeGkeRemoteNodeFailure                      = "GKE_REMOTE_NODE_FAILURE"
	ErrTypeGkeReservationValidation                  = "GKE_RESERVATION_VALIDATION"
	ErrTypeGkeSpotInstanceNotFound                   = "GKE_SPOT_INSTANCE_NOT_FOUND"
	ErrTypeGpuQuotaExceeded                          = "GPU_QUOTA_EXCEEDED"
	ErrTypeH4DVmCreateFailed                         = "H4D_VM_CREATE_FAILED"
	ErrTypeHashicorpUnavailability                   = "HASHICORP_UNAVAILABILITY"
	ErrTypeHelmInstallApplyFailure                   = "HELM_INSTALL_APPLY_FAILURE"
	ErrTypeHtcondorComError                          = "HTCONDOR_COM_ERROR"
	ErrTypeHtcondorPacker                            = "HTCONDOR_PACKER"
	ErrTypeIamBindingsExceeded                       = "IAM_BINDINGS_EXCEEDED"
	ErrTypeIamDeniedError                            = "IAM_DENIED_ERROR"
	ErrTypeIamPermissionDenied                       = "IAM_PERMISSION_DENIED"
	ErrTypeIamUnsupportedRole                        = "IAM_UNSUPPORTED_ROLE"
	ErrTypeImageNotFound                             = "IMAGE_NOT_FOUND"
	ErrTypeIncompatibleGkeVersion                    = "INCOMPATIBLE_GKE_VERSION"
	ErrTypeIndexError                                = "INDEX_ERROR"
	ErrTypeInstallProviderFailure                    = "INSTALL_PROVIDER_FAILURE"
	ErrTypeInstallVirtualglFailure                   = "INSTALL_VIRTUALGL_FAILURE"
	ErrTypeInstanceTemplateCreationTimeout           = "INSTANCE_TEMPLATE_CREATION_TIMEOUT"
	ErrTypeInstanceTempNotFoundOnDestroy             = "INSTANCE_TEMP_NOT_FOUND_ON_DESTROY"
	ErrTypeIntegrationTestFailure                    = "INTEGRATION_TEST_FAILURE"
	ErrTypeInvalidArgument                           = "INVALID_ARGUMENT"
	ErrTypeInvalidCidrMasterAuthorizedNetworksConfig = "INVALID_CIDR_MASTER_AUTHORIZED_NETWORKS_CONFIG"
	ErrTypeInvalidCloudUrl                           = "INVALID_CLOUD_URL"
	ErrTypeInvalidTemplateInterpolationValue         = "INVALID_TEMPLATE_INTERPOLATION_VALUE"
	ErrTypeIpaddressNotExist                         = "IPADDRESS_NOT_EXIST"
	ErrTypeIpRangeOverlap                            = "IP_RANGE_OVERLAP"
	ErrTypeJobsetSystemFailedApplyFieldNotInSchema   = "JOBSET_SYSTEM_FAILED_APPLY_FIELD_NOT_IN_SCHEMA"
	ErrTypeJobsetSystemFailedApplyNoEndpoint         = "JOBSET_SYSTEM_FAILED_APPLY_NO_ENDPOINT"
	ErrTypeJobsetSystemInitialized                   = "JOBSET_SYSTEM_INITIALIZED"
	ErrTypeKubectlPathNotExist                       = "KUBECTL_PATH_NOT_EXIST"
	ErrTypeKueueNotInitialized                       = "KUEUE_NOT_INITIALIZED"
	ErrTypeKueueWebhookServiceNotFound               = "KUEUE_WEBHOOK_SERVICE_NOT_FOUND"
	ErrTypeKueueWebhookServiceUnavailable            = "KUEUE_WEBHOOK_SERVICE_UNAVAILABLE"
	ErrTypeLocalExecProvisionerError                 = "LOCAL_EXEC_PROVISIONER_ERROR"
	ErrTypeLoginInstanceNotFound                     = "LOGIN_INSTANCE_NOT_FOUND"
	ErrTypeLustreInstanceCreateFailed                = "LUSTRE_INSTANCE_CREATE_FAILED"
	ErrTypeLustreNetworkInitialisation               = "LUSTRE_NETWORK_INITIALISATION"
	ErrTypeMatchingBuildFailure                      = "MATCHING_BUILD_FAILURE"
	ErrTypeMissingAttributeSubnetSelfLink            = "MISSING_ATTRIBUTE_SUBNET_SELF_LINK"
	ErrTypeMissingEnvironmentVariable                = "MISSING_ENVIRONMENT_VARIABLE"
	ErrTypeMissingPythonPackage                      = "MISSING_PYTHON_PACKAGE"
	ErrTypeMissingSudoPassword                       = "MISSING_SUDO_PASSWORD"
	ErrTypeModuleDownloadFailure                     = "MODULE_DOWNLOAD_FAILURE"
	ErrTypeMonitoringStartupTimeout                  = "MONITORING_STARTUP_TIMEOUT"
	ErrTypeMountHomeTimeout                          = "MOUNT_HOME_TIMEOUT"
	ErrTypeMpiJobTimeout                             = "MPI_JOB_TIMEOUT"
	ErrTypeMungeAuthFailure                          = "MUNGE_AUTH_FAILURE"
	ErrTypeMungeTimeoutTypes                         = "MUNGE_TIMEOUT_TYPES"
	ErrTypeN2QuotaExceeded                           = "N2_QUOTA_EXCEEDED"
	ErrTypeN4MachineIssue                            = "N4_MACHINE_ISSUE"
	ErrTypeNameRegexpError                           = "NAME_REGEXP_ERROR"
	ErrTypeNcclBandwidthLow                          = "NCCL_BANDWIDTH_LOW"
	ErrTypeNcclFailedRun                             = "NCCL_FAILED_RUN"
	ErrTypeNetappVpcExhausted                        = "NETAPP_VPC_EXHAUSTED"
	ErrTypeNetworkValidatorFail                      = "NETWORK_VALIDATOR_FAIL"
	ErrTypeNodepoolCreationError                     = "NODEPOOL_CREATION_ERROR"
	ErrTypeNodesetNotFoundOnCleanup                  = "NODESET_NOT_FOUND_ON_CLEANUP"
	ErrTypeNodeTaintFailure                          = "NODE_TAINT_FAILURE"
	ErrTypeNotEnoughResources                        = "NOT_ENOUGH_RESOURCES"
	ErrTypeNoResourcesInDefaultNamespace             = "NO_RESOURCES_IN_DEFAULT_NAMESPACE"
	ErrTypeNoZoneHaveEnoughResources                 = "NO_ZONE_HAVE_ENOUGH_RESOURCES"
	ErrTypeNullArgument                              = "NULL_ARGUMENT"
	ErrTypeNullValueNotIterable                      = "NULL_VALUE_NOT_ITERABLE"
	ErrTypeNvidiaDriverBuildFailure                  = "NVIDIA_DRIVER_BUILD_FAILURE"
	ErrTypeOmniaInternalServerError                  = "OMNIA_INTERNAL_SERVER_ERROR"
	ErrTypeOmniaNodeReadyTimeout                     = "OMNIA_NODE_READY_TIMEOUT"
	ErrTypeOmniaTimeout                              = "OMNIA_TIMEOUT"
	ErrTypeOmniaYumFailure                           = "OMNIA_YUM_FAILURE"
	ErrTypeOrphanNode                                = "ORPHAN_NODE"
	ErrTypeOsLoginInvalidArgument                    = "OS_LOGIN_INVALID_ARGUMENT"
	ErrTypeOverwriteConflict                         = "OVERWRITE_CONFLICT"
	ErrTypePackerBuildFailure                        = "PACKER_BUILD_FAILURE"
	ErrTypePackerOnetimeWoops                        = "PACKER_ONETIME_WOOPS"
	ErrTypeParallelstoreInstanceCreationFailed       = "PARALLELSTORE_INSTANCE_CREATION_FAILED"
	ErrTypeParallelstoreNotMounted                   = "PARALLELSTORE_NOT_MOUNTED"
	ErrTypeParallelstoreServerSubnetworkIpOverlapped = "PARALLELSTORE_SERVER_SUBNETWORK_IP_OVERLAPPED"
	ErrTypeParamikoConnectFailure                    = "PARAMIKO_CONNECT_FAILURE"
	ErrTypePbsnodesUnknownCommand                    = "PBSNODES_UNKNOWN_COMMAND"
	ErrTypePbsproHostsNotConnected                   = "PBSPRO_HOSTS_NOT_CONNECTED"
	ErrTypePeeringUpdateFailure                      = "PEERING_UPDATE_FAILURE"
	ErrTypePlacementPolicyInUse                      = "PLACEMENT_POLICY_IN_USE"
	ErrTypePlacementPrecondition                     = "PLACEMENT_PRECONDITION"
	ErrTypePluginGettingFailed                       = "PLUGIN_GETTING_FAILED"
	ErrTypePodsFioNotFound                           = "PODS_FIO_NOT_FOUND"
	ErrTypePodFailed                                 = "POD_FAILED"
	ErrTypePostHeaderTimeOut                         = "POST_HEADER_TIME_OUT"
	ErrTypePrimitiveTypedValue                       = "PRIMITIVE_TYPED_VALUE"
	ErrTypePrivateServiceAccessIpExhausted           = "PRIVATE_SERVICE_ACCESS_IP_EXHAUSTED"
	ErrTypeProcessError                              = "PROCESS_ERROR"
	ErrTypeProviderDownloadFailure                   = "PROVIDER_DOWNLOAD_FAILURE"
	ErrTypePsCreationTimeout                         = "PS_CREATION_TIMEOUT"
	ErrTypePsInstanceCreationFailure                 = "PS_INSTANCE_CREATION_FAILURE"
	ErrTypePsSlurmMungeTimeout                       = "PS_SLURM_MUNGE_TIMEOUT"
	ErrTypePvcInternalError13                        = "PVC_INTERNAL_ERROR_13"
	ErrTypePythonModuleImportError                   = "PYTHON_MODULE_IMPORT_ERROR"
	ErrTypeQuotaExceededForQuotaMetric               = "QUOTA_EXCEEDED_FOR_QUOTA_METRIC"
	ErrTypeRdmaNotFound                              = "RDMA_NOT_FOUND"
	ErrTypeReachInstanceGroupManagerLimitation       = "REACH_INSTANCE_GROUP_MANAGER_LIMITATION"
	ErrTypeReachSsdTotalGbLimitation                 = "REACH_SSD_TOTAL_GB_LIMITATION"
	ErrTypeReservation                               = "RESERVATION"
	ErrTypeReservationCapacityNotAvailable           = "RESERVATION_CAPACITY_NOT_AVAILABLE"
	ErrTypeReservationPathInvalid                    = "RESERVATION_PATH_INVALID"
	ErrTypeReservationUnavailableOrExpired           = "RESERVATION_UNAVAILABLE_OR_EXPIRED"
	ErrTypeResourcesNotFound                         = "RESOURCES_NOT_FOUND"
	ErrTypeResourceCreationFailure                   = "RESOURCE_CREATION_FAILURE"
	ErrTypeResourcePostconditionFailed               = "RESOURCE_POSTCONDITION_FAILED"
	ErrTypeResourcePreconditionFailed                = "RESOURCE_PRECONDITION_FAILED"
	ErrTypeResourceStateTimeout                      = "RESOURCE_STATE_TIMEOUT"
	ErrTypeResourceUnavailable                       = "RESOURCE_UNAVAILABLE"
	ErrTypeRouternatResourceNotFound                 = "ROUTERNAT_RESOURCE_NOT_FOUND"
	ErrTypeRouterQuota                               = "ROUTER_QUOTA"
	ErrTypeRtConfigHeaderTimeout                     = "RT_CONFIG_HEADER_TIMEOUT"
	ErrTypeSdkHttp503                                = "SDK_HTTP_503"
	ErrTypeSensitiveVar                              = "SENSITIVE_VAR"
	ErrTypeSerialOutputInternalError                 = "SERIAL_OUTPUT_INTERNAL_ERROR"
	ErrTypeServerError                               = "SERVER_ERROR"
	ErrTypeShouldHaveHadReservation                  = "SHOULD_HAVE_HAD_RESERVATION"
	ErrTypeSinfoControllerFailure                    = "SINFO_CONTROLLER_FAILURE"
	ErrTypeSlurmstepdError                           = "SLURMSTEPD_ERROR"
	ErrTypeSlurmArtifactsCreateFailed                = "SLURM_ARTIFACTS_CREATE_FAILED"
	ErrTypeSlurmDestroyFailureSubnetSelfLink         = "SLURM_DESTROY_FAILURE_SUBNET_SELF_LINK"
	ErrTypeSlurmDownloadModule                       = "SLURM_DOWNLOAD_MODULE"
	ErrTypeSlurmFailedToPowerDown                    = "SLURM_FAILED_TO_POWER_DOWN"
	ErrTypeSlurmJobCredentialError                   = "SLURM_JOB_CREDENTIAL_ERROR"
	ErrTypeSlurmPartitionNotReady                    = "SLURM_PARTITION_NOT_READY"
	ErrTypeSlurmTestRefactorBug                      = "SLURM_TEST_REFACTOR_BUG"
	ErrTypeSlurmTopologySyncFailure                  = "SLURM_TOPOLOGY_SYNC_FAILURE"
	ErrTypeSlurmV5LustreMountFailure                 = "SLURM_V5_LUSTRE_MOUNT_FAILURE"
	ErrTypeSlurmV5LustreRepoFailure                  = "SLURM_V5_LUSTRE_REPO_FAILURE"
	ErrTypeSlurmV5MungeTimeout                       = "SLURM_V5_MUNGE_TIMEOUT"
	ErrTypeSlurmV5OperationCanceledByUser            = "SLURM_V5_OPERATION_CANCELED_BY_USER"
	ErrTypeSlurmV6MungeTimeout                       = "SLURM_V6_MUNGE_TIMEOUT"
	ErrTypeSpackGromacsFailure                       = "SPACK_GROMACS_FAILURE"
	ErrTypeSpackNotFound                             = "SPACK_NOT_FOUND"
	ErrTypeSpackRambleTimeoutLock                    = "SPACK_RAMBLE_TIMEOUT_LOCK"
	ErrTypeSpackRambleUserError                      = "SPACK_RAMBLE_USER_ERROR"
	ErrTypeSpackSetuid                               = "SPACK_SETUID"
	ErrTypeSpotNodePrempted                          = "SPOT_NODE_PREMPTED"
	ErrTypeSpotSettingDeprecated                     = "SPOT_SETTING_DEPRECATED"
	ErrTypeSqueueError                               = "SQUEUE_ERROR"
	ErrTypeSrunConnectionReset                       = "SRUN_CONNECTION_RESET"
	ErrTypeSsdQuota                                  = "SSD_QUOTA"
	ErrTypeSshFailed                                 = "SSH_FAILED"
	ErrTypeSshKeyConflict                            = "SSH_KEY_CONFLICT"
	ErrTypeSslerror                                  = "SSLERROR"
	ErrTypeStartupScriptFailed                       = "STARTUP_SCRIPT_FAILED"
	ErrTypeStartupScriptFailure                      = "STARTUP_SCRIPT_FAILURE"
	ErrTypeStartupScriptNotKnownUntilApply           = "STARTUP_SCRIPT_NOT_KNOWN_UNTIL_APPLY"
	ErrTypeStartupScriptTimeout                      = "STARTUP_SCRIPT_TIMEOUT"
	ErrTypeStateUploadFail                           = "STATE_UPLOAD_FAIL"
	ErrTypeStaticAddressesQuotaExceeded              = "STATIC_ADDRESSES_QUOTA_EXCEEDED"
	ErrTypeStockoutGke                               = "STOCKOUT_GKE"
	ErrTypeStockoutTpu                               = "STOCKOUT_TPU"
	ErrTypeSubnetworkNicInstanceFailure              = "SUBNETWORK_NIC_INSTANCE_FAILURE"
	ErrTypeSubnetConflict                            = "SUBNET_CONFLICT"
	ErrTypeSubnetNotFound                            = "SUBNET_NOT_FOUND"
	ErrTypeSubprocessError                           = "SUBPROCESS_ERROR"
	ErrTypeSyntaxError                               = "SYNTAX_ERROR"
	ErrTypeSystemBenchmarksDirectoryUnsupported      = "SYSTEM_BENCHMARKS_DIRECTORY_UNSUPPORTED"
	ErrTypeSetcommoninstancemetadata                 = "SetCommonInstanceMetadata"
	ErrTypeSubnetNotFoundInZone                      = "Subnet_NOT_FOUND_IN_ZONE"
	ErrTypeTerraformVersionUnavailable               = "TERRAFORM_VERSION_UNAVAILABLE"
	ErrTypeTestCollision                             = "TEST_COLLISION"
	ErrTypeTestZoneValidatorFailed                   = "TEST_ZONE_VALIDATOR_FAILED"
	ErrTypeTfInconsistentDependency                  = "TF_INCONSISTENT_DEPENDENCY"
	ErrTypeTfProjectIamCrash                         = "TF_PROJECT_IAM_CRASH"
	ErrTypeTfProviderInconsistency                   = "TF_PROVIDER_INCONSISTENCY"
	ErrTypeTfWrongAttribute                          = "TF_WRONG_ATTRIBUTE"
	ErrTypeTimeoutOnSuccess                          = "TIMEOUT_ON_SUCCESS"
	ErrTypeTimeOutError                              = "TIME_OUT_ERROR"
	ErrTypeTimeOutWaitingForCondition                = "TIME_OUT_WAITING_FOR_CONDITION"
	ErrTypeTlsHandshakeTimeout                       = "TLS_HANDSHAKE_TIMEOUT"
	ErrTypeTopologyassignmentCheckFail               = "TOPOLOGYASSIGNMENT_CHECK_FAIL"
	ErrTypeTopologyAssertionError                    = "TOPOLOGY_ASSERTION_ERROR"
	ErrTypeTopologyCommandFailure                    = "TOPOLOGY_COMMAND_FAILURE"
	ErrTypeTopologyGcloudCmdFailure                  = "TOPOLOGY_GCLOUD_CMD_FAILURE"
	ErrTypeUnavailableCapactiy                       = "UNAVAILABLE_CAPACTIY"
	ErrTypeUnboundVariable                           = "UNBOUND_VARIABLE"
	ErrTypeUnknownGcsTrainingModule                  = "UNKNOWN_GCS_TRAINING_MODULE"
	ErrTypeValidationError                           = "VALIDATION_ERROR"
	ErrTypeValidatorFailed                           = "VALIDATOR_FAILED"
	ErrTypeValidatorIssue                            = "VALIDATOR_ISSUE"
	ErrTypeValidatorReservationNotFound              = "VALIDATOR_RESERVATION_NOT_FOUND"
	ErrTypeVarInstanceImageNull                      = "VAR_INSTANCE_IMAGE_NULL"
	ErrTypeVersionMismatch                           = "VERSION_MISMATCH"
	ErrTypeWaitForStartupFailure                     = "WAIT_FOR_STARTUP_FAILURE"
	ErrTypeWrfCompileFailure                         = "WRF_COMPILE_FAILURE"
	ErrTypeYamlDoesNotExist                          = "YAML_DOES_NOT_EXIST"
	ErrTypeYamlNotFound                              = "YAML_NOT_FOUND"
	ErrTypeYumBadGateway                             = "YUM_BAD_GATEWAY"
	ErrTypeYumFailsToInstallLustre                   = "YUM_FAILS_TO_INSTALL_LUSTRE"
	ErrTypeKubernetesServiceAccountFailure           = "kubernetes_service_account_failure"
	ErrTypeUnknownNodeFailure                        = "unknown_NODE_FAILURE"
	ErrTypeUnknownSlurmComputeBoot                   = "unknown_SLURM_COMPUTE_BOOT"
	ErrTypeUnknownSlurmResumeTimeout                 = "unknown_SLURM_RESUME_TIMEOUT"
	ErrTypeUnknownStartupTimeoutTpu                  = "unknown_STARTUP_TIMEOUT_TPU"
)

var extraSubstringErrMatchers = []struct {
	substring string
	category  string
}{
	{"A e2-standard-2 VM instance is currently unavailable in the", ErrTypeE2StandardStockout},
	{"OPERATION_CANCELED_BY_USER", ErrTypeSlurmV5OperationCanceledByUser},
	{"srun: error: Unable to allocate resources: Required partition not available (inactive or drain)", ErrTypeSlurmPartitionNotReady},
	{"Ensure all nodes are powered down (1 retries left).", ErrTypeSlurmFailedToPowerDown},
	{"Wait for startup script to complete (1 retries left).", ErrTypeStartupScriptTimeout},
	{"Error waiting for SetCommonInstanceMetadata", ErrTypeSetcommoninstancemetadata},
	{"Error: Post \"https://compute.googleapis.com/compute/v1/projects/hpc-toolkit-dev/setCommonInstanceMetadata", ErrTypeSetcommoninstancemetadata},
	{"Sensitive values, or values derived from sensitive values, cannot be used", ErrTypeSensitiveVar},
	{"CEDAR:6001:Failed to connect", ErrTypeHtcondorComError},
	{"site-packages/conda/exceptions.py", ErrTypeCondaExc},
	{"CondaHTTPError: HTTP 502 BAD GATEWAY for url", ErrTypeConda502},
	{"\u2502 Error: Failed to download module", ErrTypeModuleDownloadFailure},
	{"connect: connection refused", ErrTypeKubernetesServiceAccountFailure},
	{"module.wait.null_resource.wait_for_startup (local-exec): startup-script timed out after 2400 seconds", ErrTypeBatchMpiTimeout},
	{"dpkg-deb: error: archive '/tmp/chrome-remote-desktop_current_amd64.deb' uses unknown compression for member", ErrTypeBrokenCrdDebPackage},
	{"Startup script timed out", ErrTypeMonitoringStartupTimeout},
	{"/exacloud at /scratch failed: No such device", ErrTypeSlurmV5LustreMountFailure},
	{"Error: Failed to download metadata for repo 'lustre'", ErrTypeSlurmV5LustreRepoFailure},
	{"Timeout when waiting for file /var/run/munge/munge", ErrTypeSlurmV5MungeTimeout},
	{"Timeout when waiting for file /var/run/munge/munge.socket.2", ErrTypeSlurmV5MungeTimeout},
	{"startup-script timed out", ErrTypeOmniaTimeout},
	{"Yum repo downloading error", ErrTypeOmniaYumFailure},
	{"Could not fetch serial port output: Authentication backend internal server error", ErrTypeOmniaInternalServerError},
	{"nodes to be ready, found", ErrTypeOmniaNodeReadyTimeout},
	{"\u2502 Error: Unsupported attribute", ErrTypeTfWrongAttribute},
	{"\u2502 Error: Inconsistent dependency lock file", ErrTypeTfInconsistentDependency},
	{"VM instance is currently unavailable in the", ErrTypeStockout},
	{"A c2-standard-60 VM instance is currently unavailable", ErrTypeStockout},
	{"does not currently have sufficient capacity for the requested resources", ErrTypeStockout},
	{"nvidia-tesla-t4-vws accelerator(s) is currently unavailable in the", ErrTypeStockout},
	{"try in another zone where Cloud TPU Nodes are offered", ErrTypeStockoutTpu},
	{"not resumed by ResumeTimeout", ErrTypeUnknownStartupTimeoutTpu},
	{"No resources found in default namespace.", ErrTypeNoResourcesInDefaultNamespace},
	{"This object does not have an attribute named\\n\\\"gpu_driver_installation_config\\\".", ErrTypeGkeGpuDriverConfigNoAttribute},
	{"Error waiting for Creating Instance: Error code 8, message: System limit for internal resources has been reached.", ErrTypeFilestoreVpcLimit},
	{"Error waiting for Creating Instance: Error code 13, message: an internal error has occurred", ErrTypeFilestoreInternalError},
	{"Error: Cloud Filestore API service is disabled", ErrTypeFilestoreApiDisabled},
	{"Error creating Disk: googleapi: Error 409", ErrTypeGcpDisk409},
	{"Failed to set permissions on the temporary files Ansible", ErrTypeAnsiblePermissions},
	{"ERROR: The nvidia kernel module was not created.", ErrTypeNvidiaDriverBuildFailure},
	{"Error: Error waiting to create Disk: Error waiting for Creating Disk: Quota 'SSD_TOTAL_GB' exceeded.", ErrTypeSsdQuota},
	{"failed to read the input yaml", ErrTypeYamlNotFound},
	{"net/http: request canceled (Client.Timeout exceeded while awaiting headers)", ErrTypeApiPostHeadersTimeout},
	{"If `reservation_name` is specified in at least one node group", ErrTypePlacementPrecondition},
	{"Error: Resource precondition failed", ErrTypeResourcePreconditionFailed},
	{"ERROR: failed to update topology", ErrTypeSlurmTopologySyncFailure},
	{"One of the configured repositories failed (lustre)", ErrTypeYumFailsToInstallLustre},
	{"not found (required by ./ghpc)", ErrTypeGhpcDepNotFound},
	{"\\\"cmd\\\": \\\"spack --version\\", ErrTypeSpackNotFound},
	{"Failed to upload state to gs", ErrTypeStateUploadFail},
	{"ZONE_RESOURCE_POOL_EXHAUSTED", ErrTypeBatchResourcePoolExhausted},
	{"googlecompute.toolkit_image: Error waiting for startup script to finish: Startup script exited with error.", ErrTypeHtcondorPacker},
	{"The package might be corrupted or you are not allowed to open the file. Check the permissions of the file.", ErrTypeInstallVirtualglFailure},
	{"exit status 1. Output: could not detect end of startup script. Sleeping.", ErrTypeWaitForStartupFailure},
	{"Error: Error waiting for instance to create: Internal error. Please try again or contact Google Support. (Code: '-537542952742641820", ErrTypeGkeRemoteNodeFailure},
	{"InvalidPermissionsError: Attempting to set suid with group writable", ErrTypeSpackSetuid},
	{"srun: error: Node failure on slurm-lustre-new-vpc", ErrTypeLustreNetworkInitialisation},
	{"description - The subnetwork resource 'default' is in 'regions/", ErrTypeBatchRegionMismatch},
	{"startup-script timed out after", ErrTypeBatchTimeout},
	{"Can't access attributes on a primitive-typed value", ErrTypePrimitiveTypedValue},
	{"googlecompute.toolkit_image' errored after", ErrTypePackerBuildFailure},
	{"pbsnodes: command not found", ErrTypePbsnodesUnknownCommand},
	{"INVALID_ARGUMENT: volume.mount_options field is invalid.", ErrTypeBatchMountOptions},
	{"Error waiting for instance to create: Quota 'N2_CPUS' exceeded", ErrTypeN2QuotaExceeded},
	{"The task includes an option with an undefined variable", ErrTypeAnsibleConfigProblem},
	{"sinfo: error: resolve_ctls_from_dns_srv: res_nsearch error: Unknown host", ErrTypeSinfoControllerFailure},
	{"Error: Provider produced inconsistent result", ErrTypeTfProviderInconsistency},
	{"Root object was present, but now absent", ErrTypeTfProjectIamCrash},
	{"Failed to connect to the host via ssh", ErrTypeSshFailed},
	{"Requested entity already exists, alreadyExists", ErrTypeEntityAlreadyExists},
	{"{\"ResourceType\":\"runtimeconfig.v1beta1.waiter\",\"ResourceErrorCode\":\"504\",\"ResourceErrorMessage\":\"Timeout expired.\"}", ErrTypeDeploymentManagerTimeout},
	{"{\"ResourceType\":\"runtimeconfig.v1beta1.waiter\",\"ResourceErrorCode\":\"412\",\"ResourceErrorMessage\":\"Failure condition satisfied.\"}", ErrTypeDeploymentManagerFailure},
	{"{\"ResourceType\":\"runtimeconfig.v1beta1.waiter\",\"ResourceErrorCode\":\"400\",\"ResourceErrorMessage\":{\"statusMessage\":\"Bad Request\"", ErrTypeDeploymentManagerBadinput},
	{"Error 400: Unsupported role in binding:", ErrTypeIamUnsupportedRole},
	{"Error waiting for Creating Router: Quota 'ROUTERS' exceeded.", ErrTypeRouterQuota},
	{"mount of path '/home' failed: <class 'subprocess.TimeoutExpired'>: Command '['mount", ErrTypeMountHomeTimeout},
	{"already exists, alreadyExists", ErrTypeGenericResourceCollision},
	{"startup-script: No package matching 'linux-headers", ErrTypeAptHeaderPackageFailure},
	{"Error code 13, message: an internal error has occurred", ErrTypePsInstanceCreationFailure},
	{"Error: Error waiting to create Instance: Error waiting for Creating Instance: timeout while waiting for state", ErrTypePsCreationTimeout},
	{"Empty hostname produced from delegate_to", ErrTypeAnsibleDelegateFailure},
	{"DEPRECATED: Use `enable_login_public_ips` instead.", ErrTypeDeprecatedVar},
	{"serial port output: Internal error. Please try again or contact Google", ErrTypeSerialOutputInternalError},
	{"Error code 9, message: Cannot modify allocated ranges in CreateConnection", ErrTypePeeringUpdateFailure},
	{"Found more than 1 matching running builds", ErrTypeMatchingBuildFailure},
	{"Could not retrieve the list of available versions for provider", ErrTypeProviderDownloadFailure},
	{"net/http: request canceled (Client.Timeout exceeded while awaiting headers)", ErrTypeRtConfigHeaderTimeout},
	{"failed to fetch resource from kubernetes: client rate limiter Wait returned an error:", ErrTypeNodepoolCreationError},
	{" NodePool a3-ultragpu-8g-a3-ultragpu-pool was created in the error state \"ERROR\"", ErrTypeGkeNodepoolStateError},
	{"Disabling automatic updates is not supported with the selected VM image", ErrTypeDisableAutoUpdatesNotSupported},
	{"not have enough resources available to fulfill the request", ErrTypeNotEnoughResources},
	{"This object does not have an attribute named \"subnetwork_self_link\"", ErrTypeMissingAttributeSubnetSelfLink},
	{"Could not download module", ErrTypeSlurmDownloadModule},
	{"OSError: [Errno 99] Cannot assign requested address", ErrTypeParamikoConnectFailure},
	{"Error: deployment_name input error, cause: value was not of type string", ErrTypeDeploymentNameNotString},
	{"CalledProcessError: Command 'gcloud compute instances describe topologyte-nodeset-0", ErrTypeTopologyGcloudCmdFailure},
	{"googleapiclient.errors.HttpError: <HttpError 503 when requesting", ErrTypeSdkHttp503},
	{"Error: Error creating instance: googleapi: Error 503: Internal error. Please try again or contact Google Support.", ErrTypeComputeVmFailed},
	{"Error: Failed to install provider", ErrTypeFailedToInstallProvider},
	{"Missing sudo password", ErrTypeMissingSudoPassword},
	{"Error waiting for Create Service Networking Connection: Error code 13", ErrTypePvcInternalError13},
	{"ERROR: Could not install packages due to an OSError: [Errno 2] No such file", ErrTypeMissingPythonPackage},
	{"Error: Error waiting for deleting GKE NodePool: timeout while waiting for state to become 'DONE'", ErrTypeFailedToDestroyGkeTimeout},
	{"/parallelstore not mounted", ErrTypeParallelstoreNotMounted},
	{"GCP Error: Quota GPUS_PER_GPU_FAMILY exceeded.", ErrTypeGpuQuotaExceeded},
	{"ERROR: runTest (__main__.SlurmTopologyTest)", ErrTypeSlurmTestRefactorBug},
	{"Failed getting the \"github.com/hashicorp/googlecompute\" plugin:", ErrTypePluginGettingFailed},
	{"Error: timeout while waiting for state to become 'created' (last state: 'creating", ErrTypeFailedToCreateTimeout},
	{"Internal error occurred: failed calling webhook \"mdeployment.kb.io\"", ErrTypeKueueWebhookServiceNotFound},
	{"modules/embedded/modules/compute/gke-node-pool/main.tf line 282", ErrTypeGkeReservationValidation},
	{"modules/embedded/modules/compute/gke-node-pool/main.tf line 273", ErrTypeGkeReservationValidation},
	{"One or more supplied key could not be found in the database", ErrTypeSpackRambleUserError},
	{"The expression result is null. Cannot include a null value in a string", ErrTypeInvalidTemplateInterpolationValue},
	{"does not have the expected topology depth", ErrTypeTopologyAssertionError},
	{"AssertionError: The two sets did not match.", ErrTypeTopologyAssertionError},
	{"package versions have conflicting dependencies", ErrTypeDependenciesConflict},
	{"The 'spot' setting is deprecated", ErrTypeSpotSettingDeprecated},
	{"exit status 1. Output: d of startup script. Sleeping.", ErrTypeStartupScriptFailure},
	{".yaml\" does not exist", ErrTypeYamlDoesNotExist},
	{"failed to destroy group", ErrTypeFailedToDestroy},
	{"CONCURRENT_OPERATIONS_QUOTA_EXCEEDED", ErrTypeConcurrentOperationsQuotaExceeded},
	{"not resumed by ResumeTimeout", ErrTypeUnknownSlurmResumeTimeout},
	{"Something is wrong with the boot of the nodes", ErrTypeUnknownSlurmComputeBoot},
	{"Node failure on", ErrTypeUnknownNodeFailure},
	{"is potential orphan node", ErrTypeOrphanNode},
	{"Could not resolve host: mirrorlist.centos.org; Name or service not known", ErrTypeCentos7Eol},
	{"Error 404: The resource 'projects/centos-cloud/global/images/family/centos-7' was not found", ErrTypeCentos7Eol},
	{"[Errno 14] HTTPS Error 502 - Bad Gateway", ErrTypeYumBadGateway},
	{"attribute \\\"inline\\\": list of string required.", ErrTypePackerOnetimeWoops},
	{"unrecognized arguments", ErrTypeGg},
	{"var.initial_node_count is null", ErrTypeNullArgument},
	{"SSLERROR", ErrTypeSslerror},
	{"gcloud crashed", ErrTypeGcloudCrashed},
	{"Invalid value for \\\"value\\\" parameter: argument must not be null.", ErrTypeValidationError},
	{"Wait for job to run (1 retries left)", ErrTypeMpiJobTimeout},
	{"context deadline exceeded (Client.Timeout exceeded while awaiting headers", ErrTypePostHeaderTimeOut},
	{"validator \\\"test_zone_exists\\\" failed", ErrTypeTestZoneValidatorFailed},
	{"Error: variable \\\"checkpoint_bucket_name\\\" not found", ErrTypeCheckBucketNameNotFound},
	{"Error: unknown module id: \\\"gcs-training\\\"", ErrTypeUnknownGcsTrainingModule},
	{"Error: expected master_authorized_networks_config.0.cidr_blocks.0.cidr_block to contain a valid Value", ErrTypeInvalidCidrMasterAuthorizedNetworksConfig},
	{"does not match expected topology depth", ErrTypeTopologyAssertionError},
	{"Error waiting for instance to create: Quota 'CPUS_PER_VM_FAMILY' exceeded", ErrTypeCpusPerVmFamilyQuotaExceeded},
	{"strconv.ParseInt: parsing \\\"9-o\\\": invalid syntax", ErrTypeA2HighKueueInvalidArgument},
	{"No RDMA interfaces found", ErrTypeRdmaNotFound},
	{"Assertion failed", ErrTypeAssertError},
	{"IndexError: list index out of range", ErrTypeIndexError},
	{"Error: error retrieving image information: googleapi: Error 404: The resource 'projects/ubuntu-os-cloud/global/images/family/ubuntu-2004-lts' was not found, notFound", ErrTypeImageNotFound},
	{"Quota exceeded for quota metric 'Requests per project in the US multi-region' and limit 'Requests per project in the US multi-region per minute' of service 'artifactregistry.googleapis.com'", ErrTypeQuotaExceededForQuotaMetric},
	{"There was a failure in the startup script", ErrTypeStartupScriptFailed},
	{"Found more than 1 matching running build(s)", ErrTypeTestCollision},
	{"Error: Error waiting for creating GKE cluster: Failed to create cluster", ErrTypeFailedToCreateCluster},
	{"Error waiting for Creating Address: Quota 'STATIC_ADDRESSES' exceeded.  Limit: 175.0 in region us-west4", ErrTypeStaticAddressesQuotaExceeded},
	{"Google Compute Engine: Invalid value for field 'resource.IPAddress", ErrTypeIpaddressNotExist},
	{"Error waiting to create Instance: Error waiting for Creating Instance: Error code 3, message: cloud-cont", ErrTypeCidrRangeInUse},
	{"Error adding labels to ComputeAddress, googleapi:,Error 412: Labels fingerprint either invalid or resource labels", ErrTypeComputeAddress},
	{"Error waiting for Create Service Networking Connection: timeout while waiting for state to become 'done: true'", ErrTypeCreateConnectionTimeout},
	{"Error: expected node_config.0.taint.0.effect to be one of [\"NO_SCHEDULE\" \"PREFER_NO_SCHEDULE\" \"NO_EXECUTE\"], got NoSchedule", ErrTypeNodeTaintFailure},
	{"Error: timeout while waiting for state to become 'success' (timeout: 1m0s)", ErrTypeGkeInternalError},
	{"The number of members in the policy", ErrTypeIamBindingsExceeded},
	{"Test NCCL failure common.cu:961 'internal error", ErrTypeNcclFailedRun},
	{"on modules/embedded/modules/management/kubectl-apply/helm_install/main.tf line 15, in resource \\\"helm_release\\\" \\\"apply_chart\\\"", ErrTypeHelmInstallApplyFailure},
	{"Error waiting for Creating Volume: Error code 3, message: bad request error: \\\"Error when creating - Error networks.CreateNetworkV1  - VPC's exhausted for account", ErrTypeNetappVpcExhausted},
	{"Unable to fetch some archives, maybe run apt-get update or try with --fix-missing", ErrTypeAptGetFailure},
	{"Failed to fetch http://deb.debian.org/debian/pool/main/p/pycurl", ErrTypeAptPycurlFailure},
	{"WARNING: cannot create user data directory: cannot create snap home dir", ErrTypeFailedHomeDirCreate},
	{"DEPLOYMENT FAILED(Couldn't find a zone to deploy)", ErrTypeCapacityNotFoundInZone},
	{" Reservation does not have enough resources for the request", ErrTypeReservationCapacityNotAvailable},
	{"RuntimeError: 'squeue' exited with code 1: slurm_load_jobs error: Connection reset by peer", ErrTypeSqueueError},
	{"Error: The 'network_count' must be between 2 and 8. Use the standard Toolkit module at 'modules/network/vpc' for count = 1.", ErrTypeNetworkValidatorFail},
	{"ERROR: (gcloud.compute.os-login.ssh-keys.add) FAILED_PRECONDITION: Login profile size exceeds 32 KiB. Delete profile values to make additional space.", ErrTypeGcloudOsLoginKeysLimit},
	{"ERROR: error fetching git source: retry budget exhausted (3 attempts): fetching git source: fetching git source: source fetch container exited with non-zero status: 128", ErrTypeGithubError},
	{"Error waiting for status: Error waiting for instance to reach desired status RUNNING: timeout while waiting for state to become 'RUNNING'", ErrTypeResourceStateTimeout},
	{"Wait for jobs to complete (1 retries left).", ErrTypeGkeJobTimeout},
	{"Quota exceeded for quota metric 'Requests to public APIs' and limit 'Requests to public APIs per minute per user' of service 'file.googleapis.com'", ErrTypeFilestoreApiRateLimit},
	{"Private Service Access IP address range exhausted or does not exist", ErrTypePrivateServiceAccessIpExhausted},
	{"srun: error: Unable to allocate resources: Connection reset by peer", ErrTypeSrunConnectionReset},
	{"The Job failed to complete within the expected time or failed verification", ErrTypeResourcesNotFound},
	{"Unable to change directory before execution: [Errno 2] No such file or directory", ErrTypeDirNotFound},
	{"Error: Error waiting to create Instance: Error waiting for Creating Instance: Error code 8, message: resource exhausted: system limit for internal resources has been reached", ErrTypeLustreInstanceCreateFailed},
	{"Error: local-exec provisioner error", ErrTypeLocalExecProvisionerError},
	{"pods \"fio\" not found in namespace \"default\"", ErrTypePodsFioNotFound},
	{"Error: Iteration over null value", ErrTypeNullValueNotIterable},
	{"error: timed out waiting for the condition on pods/fio", ErrTypeTimeOutError},
	{"ERROR: No matching distribution found for requests==2.33.0", ErrTypeVersionMismatch},
	{"Error: Error creating instance: googleapi: Error 400: pd-standard disk type cannot be used by n4-standard-2 machine type., badRequest", ErrTypeN4MachineIssue},
	{"subprocess.CalledProcessError: Command '['gcloud", ErrTypeSubprocessError},
	{"FATAL: Error: exit status 1", ErrTypeFilestoreCreationError},
	{"variable 'reservation_affinity.consume_reservation_type' value doesn't match", ErrTypeFlexNoReservationError},
	{"upstream connect error or disconnect/reset before headers.", ErrTypeConnectionFailure},
	{"InvalidUrlError: Cloud URL scheme should be followed by colon and two slashes", ErrTypeInvalidCloudUrl},
	{"Module failed: Could not import the dnf python module using /usr/bin/python3.12", ErrTypePythonModuleImportError},
	{"Task failed: Conditional result (True) was derived from value of type 'str'", ErrTypeConditionalMismatch},
	{"No default subnetwork was found in the region of the instance.", ErrTypeSubnetNotFoundInZone},
	{"DB_VERSION_MISMATCH: Database environment version mismatch", ErrTypeDbVersionMismatch},
	{"timed out waiting for the condition on jobs/fio-benchmark", ErrTypeFioJobTime},
	{"error deleting firewall rule", ErrTypeFirewallDestroyError},
	{"accelerator_topology must be divisible by number of gpus in machine", ErrTypeA4XTopologyIssue},
	{"gcloud.compute.instances.update) HTTPError 404: The resource", ErrTypeGkeSpotInstanceNotFound},
	{"spack-gromacs.yml", ErrTypeSpackGromacsFailure},
	{"Error: googleapi: Error 400: Master version must be one of \\\"RAPID\\\" channel supported versions", ErrTypeIncompatibleGkeVersion},
	{" timed out waiting for the condition on jobs/my-job-04b6", ErrTypeTimeOutWaitingForCondition},
	{"Error waiting for instance to create: timeout while waiting for state to become 'DONE'", ErrTypeInstanceTemplateCreationTimeout},
	{"Error: error creating NodePool: googleapi: Error 400: Reservation name format path is invalid", ErrTypeReservationPathInvalid},
	{"Does not currently have sufficient capacity for the requested resources", ErrTypeNoZoneHaveEnoughResources},
	{"Please use a version with COS", ErrTypeGkeNodeVersionNotSupported},
	{"Unable to locate package terraform. E: Package 'packer' has no installation candidate", ErrTypeHashicorpUnavailability},
	{" Lustre read/write test pod failed to complete. Final phase: Pending Check debug output above.", ErrTypePodFailed},
	{"Subnetworks must be distinct for NICs in the same instance", ErrTypeSubnetworkNicInstanceFailure},
	{" Error: file for staging /workspace/tools/cloud-build/daily-tests/blueprints/system_benchmarks does not exists", ErrTypeSystemBenchmarksDirectoryUnsupported},
	{" not enough resources available to fulfill the request in us-central1-a", ErrTypeResourceUnavailable},
	{"Error generating job credential", ErrTypeSlurmJobCredentialError},
	{"Unable to locate package terraform", ErrTypeHashicorpUnavailability},
	{"Please use formats like projects/{project}/reservations/{reservation}", ErrTypeReservationPathInvalid},
	{"Cluster is running incompatible operation", ErrTypeClusterAlreadyHasOperation},
	{"Could not fetch resource", ErrTypeLoginInstanceNotFound},
	{"Error 403: Permission 'iam.serviceAccounts.get' denied on resource", ErrTypeIamPermissionDenied},
	{"Action failed: Integration tests failed", ErrTypeIntegrationTestFailure},
	{"/bin/bash: line 28: GCLUSTER_GCS_PATH: unbound variable", ErrTypeMissingEnvironmentVariable},
	{"/bin/bash: line 29: GCLUSTER_GCS_PATH: unbound variable", ErrTypeMissingEnvironmentVariable},
	{" Error: deployment variable reservation was not set", ErrTypeDeploymentReservationFailure},
	{"Error: Error waiting for instance to create: The zone 'projects/hpc-toolkit-dev/zones/us-west4-c' does not have enough resources available ", ErrTypeUnavailableCapactiy},
	{" Validators can be silenced or treated as warnings or errors", ErrTypeValidatorIssue},
	{"provided hosts list is empty, only localhost is available. Note that the implicit localhost does not match 'all'", ErrTypeReservation},
	{"subprocess.CalledProcessError: Command 'gcloud compute instances describe f2da0topol-nodeset-0 --zone=us-central1-a --project=hpc-toolkit-dev --format='value(resourceStatus.physicalHost)'' returned non-zero exit status 1.", ErrTypeCommandFailure},
	{"subprocess.CalledProcessError: Command 'gcloud compute instances describe d989ctopol-nodeset-0 --zone=us-central1-a --project=hpc-toolkit-dev --format='value(resourceStatus.physicalHost)'' returned non-zero exit status 1.", ErrTypeProcessError},
	{"unbound variable", ErrTypeUnboundVariable},
	{"unexpected EOF while looking for matching", ErrTypeSyntaxError},
	{"The command '/bin/sh -c wget -q ", ErrTypeOverwriteConflict},
	{" sbatch: error: fetch_config: DNS SRV lookup failed", ErrTypeDnsSrvLookupFailed},
	{"Error 403: Permission 'iam.serviceAccounts.get' denied on resource or it may not exist", ErrTypeIamDeniedError},
	{"subprocess.CalledProcessError: Command 'gcloud compute instances describe a23901topo-nodeset-0 --zone=us-central1-a --project=hpc-toolkit-dev --format='value(resourceStatus.physicalHost)'' returned non-zero exit status 1.", ErrTypeTopologyCommandFailure},
}

var extraRegexErrMatchers = []struct {
	pattern  *regexp.Regexp
	category string
}{
	{regexp.MustCompile("Creating Instance: Error code 14, message: The zone '.+' does not have enough resources available to fulfill the request"), ErrTypeFilestoreZoneCapacity},
	{regexp.MustCompile("Error: NodePool .* was created in the error state"), ErrTypeGkeNodepoolError},
	{regexp.MustCompile("jobset-system/.* failed to run apply: namespaces \\\"jobset-system\\\" not found"), ErrTypeJobsetSystemInitialized},
	{regexp.MustCompile("kueue-system/.* failed to run apply: namespaces \\\"kueue-system\\\" not found"), ErrTypeKueueNotInitialized},
	{regexp.MustCompile("INVALID_ARGUMENT: reserved IP range \\d+.\\d+.\\d+.\\d+\\/\\d+ overlaps with the existing allocated IP range \\d+.\\d+.\\d+.\\d+\\/\\d+ in network .*"), ErrTypeIpRangeOverlap},
	{regexp.MustCompile("\"account_id\" \\(.*\\) must be between 6 and 30 characters long"), ErrTypeAccountIdLength},
	{regexp.MustCompile("error: the path \"/workspace/(?:ml-)?gke(?:-a3(?:mega|high|ultra))?-[^/]+/primary/[^\"]+\" does not exist"), ErrTypeKubectlPathNotExist},
	{regexp.MustCompile("Error from server \\(BadRequest\\): container \"my-job-container\" in pod \"my-job-[a-z0-9-]+\" is waiting to start: ContainerCreating"), ErrTypeFilestorePvcNotConnected},
	{regexp.MustCompile("srun: error: Task launch for StepId=2.0 failed on node [a-z0-9-]+-c2nodeset-[0-2]+: Communication connection failure"), ErrTypeMungeAuthFailure},
	{regexp.MustCompile("Internal error occurred: failed calling webhook \"(.+?)\\.kb\\.io\": failed to call webhook"), ErrTypeKueueWebhookServiceUnavailable},
	{regexp.MustCompile("srun: error: [a-z0-9-]+-a3meganodeset-[0-2]+: task 1: Exited with exit code 1"), ErrTypeApertureDeviceNotReady},
	{regexp.MustCompile("Error: Error creating instance: googleapi: Error 404: The resource 'projects/hpc-toolkit-dev/zones/europe-west1-b/reservations/(.+?)' was not found, notFound"), ErrTypeReservationUnavailableOrExpired},
	{regexp.MustCompile("Version '\\d+\\.\\d+\\.\\d+' for 'terraform' was not found"), ErrTypeTerraformVersionUnavailable},
	{regexp.MustCompile("nodes set down ([a-zA-Z0-9-]+) with reason=Preempted instance"), ErrTypeSpotNodePrempted},
	{regexp.MustCompile("container \"my-job-container\" in pod \"my-job-[a-z0-9-]+\" is waiting to start: trying and failing to pull image"), ErrTypeServerError},
	{regexp.MustCompile("validator [\"']?test_module_not_used[\"']? failed"), ErrTypeValidatorFailed},
}

var extraMultiSubstringErrMatchers = []struct {
	substrings []string
	category   string
}{
	{[]string{"resourceInUseByAnotherResource", "ERROR: some placement groups failed to delete"}, ErrTypePlacementPolicyInUse},
	{[]string{"Error waiting for creating GKE cluster", "Google Compute Engine does not have enough resources available"}, ErrTypeStockoutGke},
	{[]string{"does not currently have sufficient capacity for the requested resources", "regions/europe-west1/instances/bulkInsert"}, ErrTypeShouldHaveHadReservation},
	{[]string{"Error creating Instance: googleapi: Error 409: Resource", "already exists"}, ErrTypeFilestoreRace},
	{[]string{"Error: Resource postcondition failed", "Couldn't find the reservation"}, ErrTypeResourcePostconditionFailed},
	{[]string{"Timeout when waiting for file", "_lock/done"}, ErrTypeSpackRambleTimeoutLock},
	{[]string{"Error while installing hashicorp/google", "unexpected EOF"}, ErrTypeInstallProviderFailure},
	{[]string{"Error: Error creating Subnetwork: googleapi: Error 409:", "already exists"}, ErrTypeSubnetConflict},
	{[]string{"startup-script timed out after", "startup-script exit status 0"}, ErrTypeTimeoutOnSuccess},
	{[]string{"Error deleting instance template", "was not found, notFound"}, ErrTypeInstanceTempNotFoundOnDestroy},
	{[]string{"Error: Invalid for_each argument", "modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller/modules/slurm_files/main.tf"}, ErrTypeStartupScriptNotKnownUntilApply},
	{[]string{"gcloud.compute.os-login.ssh-keys.add) INVALID_ARGUMENT", "Delete profile values to make additional space."}, ErrTypeOsLoginInvalidArgument},
	{[]string{"modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller/scripts/cleanup_compute.sh", "Invalid value for field 'instance'"}, ErrTypeCleanupCompute},
	{[]string{"modules/embedded/community/modules/scheduler/schedmd-slurm-gcp-v6-controller/scripts/cleanup_compute.sh", "exit status 1. Output: Deleting compute nodes"}, ErrTypeNodesetNotFoundOnCleanup},
	{[]string{"modules/embedded/community/modules/scripts/wait-for-startup/scripts/wait-for-startup-status.sh", "Could not detect end of startup script"}, ErrTypeEndOfStartUpScriptFailure},
	{[]string{"google_filestore_instance.filestore_instance", "Write access to project 'hpc-toolkit-dev' was denied"}, ErrTypeFilestoreProjectDenied},
	{[]string{"Failed to install provider", "TLS handshake timeout"}, ErrTypeTlsHandshakeTimeout},
	{[]string{"Error: wrf", "InstallError: Compile failed. Check the output log for details"}, ErrTypeWrfCompileFailure},
	{[]string{"Error: project in subnetwork's self_link", "must match subnetwork_project"}, ErrTypeSlurmDestroyFailureSubnetSelfLink},
	{[]string{"Test that execution hosts have joined cluster (1 retries left)", "\"attempts\": 30,"}, ErrTypePbsproHostsNotConnected},
	{[]string{"No resources found in default namespace.", "test-gke-storage-parallelstore.yml"}, ErrTypeGkeParallelstoreJobTimeout},
	{[]string{"jobset-system/jobset-controller-manager failed to run apply", "no endpoints available for service \"kueue-webhook-service\""}, ErrTypeJobsetSystemFailedApplyNoEndpoint},
	{[]string{"jobset-system/jobset-controller-manager failed to run apply", ".spec.template.spec.volumes[name=\"\"].value: field not declared in schema"}, ErrTypeJobsetSystemFailedApplyFieldNotInSchema},
	{[]string{"Check for host topologyAssignment in workloads (1 retries left).", "fatal"}, ErrTypeTopologyassignmentCheckFail},
	{[]string{"failed to destroy group", "Error waiting for Deleting Instance Template"}, ErrTypeFailedToDestroyBlockingMig},
	{[]string{"waiting for instance to create: Internal error", "on modules/embedded/modules/compute/vm-instance/main.tf line 176"}, ErrTypeResourceCreationFailure},
	{[]string{"Error: \\\"name\\\"", "doesn't match regexp"}, ErrTypeNameRegexpError},
	{[]string{"Error: Invalid value for variable", "var.instance_image is null"}, ErrTypeVarInstanceImageNull},
	{[]string{"Error: Error waiting for instance to create: Internal error. Please try again or contact Google Support.", "resource \"google_compute_instance\" \"compute_vm\""}, ErrTypeH4DVmCreateFailed},
	{[]string{"Error: kube-system/nccl-rdma-installer failed to run apply: error when creating", "DaemonSet.apps \"nccl-rdma-installer\" is invalid: spec.template.spec.containers: Required value"}, ErrTypeA4NcclInstallerFailed},
	{[]string{"RouterNat: googleapi: Error 404: The resource", "was not found"}, ErrTypeRouternatResourceNotFound},
	{[]string{"Error waiting to create Instance: Error waiting for Creating Instance: Error code 3, message: The request was invalid: Server subnetwork IP range [\"172.17.167.0/26\"] overlaps with restricted IP range [\"172.17.0.0/16\"]. Please choose a range explicitly that does not overlap", "google_parallelstore_instance"}, ErrTypeParallelstoreServerSubnetworkIpOverlapped},
	{[]string{"test-gke-managed-hyperdisk", "fatal", "\"attempts\": 80"}, ErrTypeGkeHyperDiskJobTimeout},
	{[]string{"Error waiting to create Instance: Error waiting for Creating Instance", "google_parallelstore_instance"}, ErrTypeParallelstoreInstanceCreationFailed},
	{[]string{"nvidia-container-cli: container error: file lookup failed", "permission denied"}, ErrTypeSlurmstepdError},
	{[]string{"Insufficient", "INSTANCE_GROUP_MANAGERS"}, ErrTypeReachInstanceGroupManagerLimitation},
	{[]string{"Error: Error waiting for instance to create: Quota 'SSD_TOTAL_GB' exceeded.", "Quota 'IN_USE_ADDRESSES' exceeded."}, ErrTypeReachSsdTotalGbLimitation},
	{[]string{"Error: Error waiting for instance to create: couldn't find resource", "resource \"google_compute_instance\" \"compute_vm\""}, ErrTypeComputeVmCreateFail},
	{[]string{"Error: Invalid function argument", "on modules/embedded/modules/management/kubectl-apply/main.tf line 33, in locals"}, ErrTypeInvalidArgument},
	{[]string{"(gcloud.compute.os-login.ssh-keys.add) Users instance", "is the subject of a conflict: Multiple concurrent mutations were attempted. Please retry the request"}, ErrTypeSshKeyConflict},
	{[]string{"Error waiting to create Instance: Error waiting for Creating Instance: Error code 0, message", "on modules/embedded/modules/file-system/managed-lustre/main.tf line 61, in resource \"google_lustre_instance\" \"lustre_instance\""}, ErrTypeLustreInstanceCreateFailed},
	{[]string{"Avg bus bandwidth", "\"failed_when_result\": true", "\"rc\": 1"}, ErrTypeNcclBandwidthLow},
	{[]string{"Error 404: The resource", "subnetworks", "was not found, notFound"}, ErrTypeSubnetNotFound},
	{[]string{"mkdir: cannot create directory", "/run/enroot", "Permission denied"}, ErrTypeEnrootPermissionDenied},
	{[]string{"Required 'compute.images.get' permission", "forbidden"}, ErrTypeComputeImagesGetForbidden},
	{[]string{"validator \"test_reservation_exists\" failed", "was not found in any zone of project"}, ErrTypeValidatorReservationNotFound},
}

func init() {
	for i, m := range extraMultiSubstringErrMatchers {
		for j, sub := range m.substrings {
			extraMultiSubstringErrMatchers[i].substrings[j] = strings.ToLower(sub)
		}
	}
	for i, m := range extraSubstringErrMatchers {
		extraSubstringErrMatchers[i].substring = strings.ToLower(m.substring)
	}
}
