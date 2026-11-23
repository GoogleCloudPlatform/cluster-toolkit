// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package job

import (
	"fmt"
	"os"
	"strings"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/gke"

	"github.com/spf13/cobra"
)

var (
	clusterName  string
	location     string
	projectID    string
	gkeNamespace string
	skipPrereqs  bool
)

var (
	// OnTelemetryProjectResolved is an optional hook wired by package cmd to
	// propagate the resolved GCP project ID to the root telemetry collector.
	OnTelemetryProjectResolved func(projectID string)
	// gkeOrchestratorFactory overrides registry-based orchestrator construction when set in unit tests.
	gkeOrchestratorFactory func() orchestrator.JobOrchestrator
	orc                    orchestrator.JobOrchestrator
)

// JobCmd represents the base command for job-related operations.
var JobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage jobs on the cluster.",
	Long: `Submit batch and distributed workloads on GKE clusters, and monitor or manage workloads (list, logs, cancel, inspect) across GKE and Kubernetes clusters.
Set GCLUSTER_ORCHESTRATOR=gke|k8s to override automatic kubeconfig detection, or GCLUSTER_AIR_GAPPED=true in disconnected environments.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "gke-template-extract" {
			reportTelemetryProjectID("")
			return nil
		}

		projectFlagChanged := cmd.Flags().Changed("project")
		savedClusterName, savedGKE, err := applySavedContextDefaults(cmd)
		if err != nil {
			return err
		}
		var det orchestrator.Detection
		defer func() {
			reportTelemetryProjectID(resolveTelemetryProjectID(projectID, projectFlagChanged, det))
		}()
		savedClusterUsed := !cmd.Flags().Changed("cluster") && savedClusterName != ""

		det, err = detectJobOrchestrator(cmd, savedClusterName, savedGKE)
		if err != nil {
			return err
		}
		if isMissingClusterWithGKEFlags(det) {
			logSavedDefaultsIfActive(orchestrator.Detection{Type: orchestrator.TypeGKE}, savedClusterUsed, savedGKE)
			return validateTypeGKEFlags(cmd.Name(), det, clusterName, location, projectID)
		}
		if err := validateExplicitClusterFlag(cmd, det); err != nil {
			return err
		}
		reconcileTypeK8sFlags(cmd, det)
		logSavedDefaultsIfActive(det, savedClusterUsed, savedGKE)

		if err := validateOrchestratorFlags(cmd.Name(), det, clusterName, location, projectID); err != nil {
			return err
		}
		logging.Info("Orchestrator: %s (%s)", det.Type, det.Reason)

		// Report before initCommandOrchestrator so FatalHook paths observe the resolved project.
		reportTelemetryProjectID(resolveTelemetryProjectID(projectID, projectFlagChanged, det))
		return initCommandOrchestrator(cmd, det)
	},
}

func reportTelemetryProjectID(proj string) {
	if OnTelemetryProjectResolved != nil {
		OnTelemetryProjectResolved(proj)
	}
}

func resolveTelemetryProjectID(proj string, projectFlagChanged bool, det orchestrator.Detection) string {
	if det.Type == orchestrator.TypeK8s {
		return ""
	}
	if det.Type == "" {
		if projectFlagChanged {
			return strings.TrimSpace(proj)
		}
		return ""
	}
	if p := strings.TrimSpace(proj); p != "" {
		return p
	}
	if det.Type == orchestrator.TypeGKE {
		if p := gke.ExtractProjectIDFromContext(det.KubeContext); p != "" {
			return p
		}
		return gke.ExtractProjectIDFromContext(det.ClusterName)
	}
	return ""
}

func canOverrideSavedGKEWithK8sCluster(cmdName string, clusterChanged, explicitGKEFlags, savedGKE bool, savedClusterName string) bool {
	if cmdName == SubmitCmd.Name() || explicitGKEFlags || !savedGKE {
		return false
	}
	return clusterChanged && clusterName != "" && clusterName != savedClusterName
}

func detectJobOrchestrator(cmd *cobra.Command, savedClusterName string, savedGKE bool) (orchestrator.Detection, error) {
	d := &orchestrator.Detector{}
	explicitGKEFlags := cmd.Flags().Changed("location") || cmd.Flags().Changed("project")
	k8sClusterOverride := canOverrideSavedGKEWithK8sCluster(cmd.Name(), cmd.Flags().Changed("cluster"), explicitGKEFlags, savedGKE, savedClusterName)
	if strings.TrimSpace(os.Getenv(orchestrator.EnvVar)) == "" && k8sClusterOverride {
		if det, ok, err := detectExplicitClusterWithSavedGKE(d); err != nil || ok {
			return det, err
		}
	}
	det, err := d.Detect(clusterName, location, projectID)
	if err != nil {
		return orchestrator.Detection{}, err
	}
	return applyGKEFlagPrecedence(det, explicitGKEFlags, savedGKE, k8sClusterOverride), nil
}

func detectExplicitClusterWithSavedGKE(d *orchestrator.Detector) (orchestrator.Detection, bool, error) {
	bareDet, err := d.Detect(clusterName, "", "")
	if err != nil {
		return orchestrator.Detection{}, false, err
	}
	if bareDet.Type != orchestrator.TypeK8s {
		return bareDet, true, nil
	}
	if bareDet.KubeContext == "" {
		return orchestrator.Detection{}, false, nil
	}
	fullDet, err := d.Detect(clusterName, location, projectID)
	if err != nil {
		return orchestrator.Detection{}, false, err
	}
	if fullDet.Type == orchestrator.TypeGKE && fullDet.KubeContext != "" && fullDet.KubeContext != bareDet.KubeContext {
		return fullDet, true, nil
	}
	return bareDet, true, nil
}

func applyGKEFlagPrecedence(det orchestrator.Detection, explicitGKEFlags, savedGKE, k8sClusterOverride bool) orchestrator.Detection {
	if det.Forced || det.Type != orchestrator.TypeK8s || (!explicitGKEFlags && (k8sClusterOverride || !savedGKE)) {
		return det
	}
	reason := "GKE project/location configured"
	if det.KubeContext != "" {
		reason = fmt.Sprintf("GKE project/location configured (ignoring non-GKE kubeconfig context %q)", det.KubeContext)
	}
	return orchestrator.Detection{
		Type:        orchestrator.TypeGKE,
		KubeContext: det.KubeContext,
		ClusterName: clusterName,
		Reason:      reason,
	}
}

func validateExplicitClusterFlag(cmd *cobra.Command, det orchestrator.Detection) error {
	if !cmd.Flags().Changed("cluster") || clusterName != "" {
		return nil
	}
	if cmd.Name() == SubmitCmd.Name() && det.Forced && det.Type == orchestrator.TypeK8s {
		return orchestrator.UnsupportedK8sSubmitError()
	}
	if cmd.Name() == SubmitCmd.Name() || det.Type == orchestrator.TypeGKE {
		return validateTypeGKEFlags(cmd.Name(), det, clusterName, location, projectID)
	}
	return fmt.Errorf("cluster name is required when --cluster is set; pass a non-empty value or omit --cluster to use kubeconfig current-context")
}

func isMissingClusterWithGKEFlags(det orchestrator.Detection) bool {
	return clusterName == "" && (location != "" || projectID != "") && (!det.Forced || det.Type != orchestrator.TypeK8s)
}

func logSavedDefaultsIfActive(det orchestrator.Detection, savedClusterUsed, savedGKE bool) {
	if savedClusterUsed || (det.Type == orchestrator.TypeGKE && savedGKE) {
		logging.Info("Using saved job config defaults (run 'gcluster job config show' to view or 'gcluster job config unset all' to clear).")
		return
	}
	if det.Type == orchestrator.TypeK8s && !det.Forced && savedGKE && det.KubeContext != "" {
		logging.Warn("Ignoring saved job config project/location defaults for kubeconfig context %q; pass --location/--project or set %s=gke to target GKE.", det.KubeContext, orchestrator.EnvVar)
	}
}

func initCommandOrchestrator(cmd *cobra.Command, det orchestrator.Detection) error {
	var err error
	if gkeOrchestratorFactory != nil {
		orc = gkeOrchestratorFactory()
	} else {
		orc, err = orchestrator.NewJobOrchestrator(det)
		if err != nil {
			return err
		}
	}

	if !skipPrereqs {
		if err := ensureBasicPrerequisitesFor(cmd, det.Type, projectID); err != nil {
			return err
		}
	}

	resolvedLoc, err := orc.Initialize(clusterName, location, projectID)
	if err != nil {
		return err
	}
	location = resolvedLoc
	return nil
}

func applySavedContextDefaults(cmd *cobra.Command) (string, bool, error) {
	var ctx Context
	if override := strings.TrimSpace(os.Getenv(orchestrator.EnvVar)); override == "" || strings.EqualFold(override, string(orchestrator.TypeGKE)) {
		var err error
		ctx, err = loadContext()
		if err != nil {
			return "", false, err
		}
	}
	savedClusterName := strings.TrimSpace(ctx.ClusterName)
	savedGKE := false
	if !cmd.Flags().Changed("cluster") {
		clusterName = savedClusterName
	}
	if !cmd.Flags().Changed("location") {
		location = ctx.Location
		savedGKE = savedGKE || strings.TrimSpace(ctx.Location) != ""
	}
	if !cmd.Flags().Changed("project") {
		projectID = ctx.ProjectID
		savedGKE = savedGKE || strings.TrimSpace(ctx.ProjectID) != ""
	}
	clusterName = strings.TrimSpace(clusterName)
	location = strings.TrimSpace(location)
	projectID = strings.TrimSpace(projectID)
	return savedClusterName, savedGKE, nil
}

func reconcileTypeK8sFlags(cmd *cobra.Command, det orchestrator.Detection) {
	if det.Type != orchestrator.TypeK8s {
		return
	}
	if clusterName == "" && det.ClusterName != "" {
		clusterName = det.ClusterName
	}
	if !cmd.Flags().Changed("location") {
		location = ""
	}
	if !cmd.Flags().Changed("project") {
		projectID = ""
	}
}

func validateOrchestratorFlags(cmdName string, det orchestrator.Detection, cluster, loc, proj string) error {
	if det.Type == orchestrator.TypeK8s {
		if cmdName == SubmitCmd.Name() {
			if det.KubeContext != "" || det.Forced {
				return orchestrator.UnsupportedK8sSubmitError()
			}
			return validateTypeGKEFlags(cmdName, det, cluster, loc, proj)
		}
		return validateTypeK8sFlags(det, cluster)
	}
	return validateTypeGKEFlags(cmdName, det, cluster, loc, proj)
}

func validateTypeK8sFlags(det orchestrator.Detection, cluster string) error {
	if det.KubeContext == "" {
		if cluster != "" {
			if det.Forced {
				return fmt.Errorf("kubeconfig context or cluster %q not found; run 'kubectl config get-contexts'", cluster)
			}
			return fmt.Errorf("kubeconfig context or cluster %q not found (run 'kubectl config get-contexts', or add --location and --project for GKE)", cluster)
		}
		if det.DanglingContext != "" {
			return fmt.Errorf("kubeconfig current-context %q not found; run 'kubectl config get-contexts' or pass --cluster <context>", det.DanglingContext)
		}
		if det.Forced {
			return fmt.Errorf("no current-context set in kubeconfig; run 'kubectl config use-context <context>' or pass --cluster <context>")
		}
		return fmt.Errorf("no kubeconfig context found and no GKE flags provided; set a kubeconfig context ('kubectl config use-context <context>') or pass --cluster, --location, and --project")
	}
	return nil
}

func validateTypeGKEFlags(cmdName string, det orchestrator.Detection, cluster, loc, proj string) error {
	var k8sHint string
	if det.KubeContext != "" && cmdName != SubmitCmd.Name() && (!det.Forced || det.Type != orchestrator.TypeGKE) {
		k8sHint = fmt.Sprintf(" (or set %s=k8s to use kubeconfig context %q directly)", orchestrator.EnvVar, det.KubeContext)
	}
	if cluster == "" {
		return fmt.Errorf("cluster name is required; please specify it using the --cluster flag or set a default value using 'gcluster job config set cluster <value>'%s", k8sHint)
	}
	if loc == "" {
		return fmt.Errorf("location is required; please specify it using the --location flag or set a default value using 'gcluster job config set location <value>'%s", k8sHint)
	}
	if proj == "" {
		return fmt.Errorf("project ID is required; please specify it using the --project flag or set a default value using 'gcluster job config set project <value>'%s", k8sHint)
	}
	return nil
}

func init() {
	gke.RegisterProviders()

	JobCmd.PersistentFlags().StringVarP(&clusterName, "cluster", "c", "", "Name of the target GKE cluster or Kubernetes kubeconfig context (optional in Kubernetes mode when current-context is set).")
	JobCmd.PersistentFlags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster (GKE only).")
	JobCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID (GKE only).")
	JobCmd.PersistentFlags().StringVar(&gkeNamespace, "gke-namespace", "", "Target Kubernetes namespace for the operation. If omitted, automatic detection is used.")
	JobCmd.PersistentFlags().BoolVar(&skipPrereqs, "skip-prereqs", false, "Skip local environment prerequisite checks (kubectl, gcloud, registry auth, etc.) before taking action.")

	JobCmd.AddCommand(SubmitCmd)
	JobCmd.AddCommand(CancelJobCmd)
	JobCmd.AddCommand(ListWorkloadsCmd)
	JobCmd.AddCommand(LogsCmd)
	JobCmd.AddCommand(ConfigCmd)
	JobCmd.AddCommand(InspectCmd)
}
