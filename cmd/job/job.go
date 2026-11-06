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
	// gkeOrchestratorFactory overrides registry-based orchestrator construction when set in unit tests.
	gkeOrchestratorFactory func() orchestrator.JobOrchestrator
	orc                    orchestrator.JobOrchestrator
)

// JobCmd represents the base command for job-related operations.
var JobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage jobs on the cluster.",
	Long: `Submit batch and distributed workloads on GKE clusters, and monitor or manage workloads (list, logs, cancel, inspect) across GKE and Kubernetes clusters.
Set GCLUSTER_ORCHESTRATOR=gke|k8s to override automatic KUBECONFIG detection, or GCLUSTER_AIR_GAPPED=true in disconnected environments.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "gke-template-extract" {
			return nil
		}

		usedSaved, err := applySavedContextDefaults(cmd)
		if err != nil {
			return err
		}

		det, err := (&orchestrator.Detector{}).Detect(clusterName, location, projectID)
		if err != nil {
			return err
		}
		if clusterName == "" && (location != "" || projectID != "") && (!det.Forced || det.Type != orchestrator.TypeK8s) {
			return validateTypeGKEFlags(cmd.Name(), det, clusterName, location, projectID)
		}
		reconcileTypeK8sFlags(cmd, det)

		if err := validateOrchestratorFlags(cmd.Name(), det, clusterName, location, projectID); err != nil {
			return err
		}
		if usedSaved && det.Type == orchestrator.TypeGKE {
			logging.Info("Using saved job config defaults (run 'gcluster job config show' to view or 'gcluster job config unset all' to clear).")
		}
		logging.Info("Orchestrator: %s (%s)", det.Type, det.Reason)

		return initCommandOrchestrator(cmd, det)
	},
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

func applySavedContextDefaults(cmd *cobra.Command) (bool, error) {
	var ctx Context
	if override := strings.TrimSpace(os.Getenv(orchestrator.EnvVar)); override == "" || strings.EqualFold(override, string(orchestrator.TypeGKE)) {
		var err error
		ctx, err = loadContext()
		if err != nil {
			return false, err
		}
	}
	usedSaved := false
	if !cmd.Flags().Changed("cluster") {
		clusterName = ctx.ClusterName
		usedSaved = usedSaved || ctx.ClusterName != ""
	}
	if !cmd.Flags().Changed("location") {
		location = ctx.Location
		usedSaved = usedSaved || ctx.Location != ""
	}
	if !cmd.Flags().Changed("project") {
		projectID = ctx.ProjectID
		usedSaved = usedSaved || ctx.ProjectID != ""
	}
	return usedSaved, nil
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
				return fmt.Errorf("kubeconfig context or cluster %q was not found in KUBECONFIG; check 'kubectl config get-contexts'", cluster)
			}
			return fmt.Errorf("kubeconfig context or cluster %q was not found in KUBECONFIG (for a Kubernetes cluster, check 'kubectl config get-contexts'; for a GKE cluster, also specify --location and --project)", cluster)
		}
		if det.DanglingContext != "" {
			return fmt.Errorf("kubeconfig current-context %q was not found in KUBECONFIG; run 'kubectl config get-contexts' or pass --cluster <context>", det.DanglingContext)
		}
		if det.Forced {
			return fmt.Errorf("no Kubernetes current-context is configured in KUBECONFIG; run 'kubectl config use-context <context>' or pass --cluster <context>")
		}
		return fmt.Errorf("no Kubernetes context found in KUBECONFIG and no GKE --cluster/--location/--project flags were provided; configure KUBECONFIG (see 'kubectl config current-context') or pass --cluster, --location, and --project for GKE")
	}
	return nil
}

func validateTypeGKEFlags(cmdName string, det orchestrator.Detection, cluster, loc, proj string) error {
	var k8sHint string
	if det.KubeContext != "" && cmdName != SubmitCmd.Name() && (!det.Forced || det.Type != orchestrator.TypeGKE) {
		k8sHint = fmt.Sprintf(" (or set %s=k8s to use KUBECONFIG context %q directly without gcloud)", orchestrator.EnvVar, det.KubeContext)
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
