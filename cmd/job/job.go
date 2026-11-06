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
	"slices"
	"strings"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	_ "hpc-toolkit/pkg/orchestrator/gke"

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
	Long:  `Submit, monitor, and manage batch and distributed workloads on GKE and Kubernetes clusters.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "gke-template-extract" {
			return nil
		}

		if err := applySavedContextDefaults(cmd); err != nil {
			return err
		}

		det, err := (&orchestrator.Detector{}).Detect(clusterName, location, projectID)
		if err != nil {
			return err
		}
		logging.Info("Orchestrator: %s (%s)", det.Type, det.Reason)
		reconcileTypeK8sFlags(cmd, det)

		if err := validateOrchestratorFlags(det, clusterName, location, projectID); err != nil {
			return err
		}

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
	},
}

func applySavedContextDefaults(cmd *cobra.Command) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(orchestrator.OrchestratorEnvVar)), string(orchestrator.TypeK8s)) {
		return nil
	}
	ctx, err := loadContext()
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("cluster") && clusterName == "" {
		clusterName = ctx.ClusterName
	}
	if !cmd.Flags().Changed("location") && location == "" {
		location = ctx.Location
	}
	if !cmd.Flags().Changed("project") && projectID == "" {
		projectID = ctx.ProjectID
	}
	return nil
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

func validateOrchestratorFlags(det orchestrator.Detection, cluster, loc, proj string) error {
	if det.Type == orchestrator.TypeK8s {
		if det.KubeContext == "" {
			if cluster != "" {
				return fmt.Errorf("kubeconfig context or cluster %q was not found in KUBECONFIG (for a Kubernetes cluster, check 'kubectl config get-contexts'; for a GKE cluster, also specify --location and --project)", cluster)
			}
			return fmt.Errorf("no Kubernetes context found in KUBECONFIG and no GKE --cluster/--location/--project flags were provided; configure KUBECONFIG (see 'kubectl config current-context') or pass --cluster, --location, and --project for GKE")
		}
		return nil
	}
	if cluster == "" {
		return fmt.Errorf("cluster name is required; please specify it using the --cluster flag or set a default value using 'gcluster job config set cluster <value>'")
	}
	if loc == "" {
		return fmt.Errorf("location is required; please specify it using the --location flag or set a default value using 'gcluster job config set location <value>'")
	}
	if proj == "" {
		return fmt.Errorf("project ID is required; please specify it using the --project flag or set a default value using 'gcluster job config set project <value>'")
	}
	return nil
}

func init() {
	if !slices.Contains(orchestrator.RegisteredTypes(), orchestrator.TypeGKE) {
		panic("orchestrator registry missing required gke provider registration")
	}

	JobCmd.PersistentFlags().StringVarP(&clusterName, "cluster", "c", "", "Name of the target cluster or kubeconfig context.")
	JobCmd.PersistentFlags().StringVarP(&location, "location", "l", "", "Location (region or zone) of the GKE cluster.")
	JobCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID.")
	JobCmd.PersistentFlags().StringVar(&gkeNamespace, "gke-namespace", "", "Target Kubernetes namespace for the operation. If omitted, automatic detection is used.")
	JobCmd.PersistentFlags().BoolVar(&skipPrereqs, "skip-prereqs", false, "Skip local environment prerequisite checks (kubectl, gcloud, registry auth, etc.) before taking action.")

	JobCmd.AddCommand(SubmitCmd)
	JobCmd.AddCommand(CancelJobCmd)
	JobCmd.AddCommand(ListWorkloadsCmd)
	JobCmd.AddCommand(LogsCmd)
	JobCmd.AddCommand(ConfigCmd)
	JobCmd.AddCommand(InspectCmd)
}
