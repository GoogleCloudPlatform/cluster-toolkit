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

package gke

import (
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"maps"
	"slices"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// gatewayGracePeriod protects gateways a concurrent submit may have just created.
	gatewayGracePeriod = 2 * time.Minute
)

// storageConsumerGVRs are the kinds that may reference a gateway claim. ReplicaSets are covered by Deployments.
var storageConsumerGVRs = []schema.GroupVersionResource{
	podGVR, jobSetGVR, jobGVR, cronJobGVR, deploymentGVR, statefulSetGVR, daemonsetGVR,
}

// CancelJob deletes a job from the GKE cluster by name.
// Jobs are filtered via cluster name and location provided through CancelOptions.
func (g *GKEOrchestrator) CancelJob(name string, opts orchestrator.CancelOptions) error {
	g.namespace = opts.GKENamespace
	if err := g.configureKubectl(opts.ClusterName, opts.ClusterLocation, opts.ProjectID); err != nil {
		return err
	}

	if _, err := g.getDynamicClient(); err != nil {
		return fmt.Errorf("failed to initialize k8s client: %w", err)
	}

	ns, err := g.getCurrentNamespace(opts.ClusterName, opts.ClusterLocation, opts.ProjectID)
	if err != nil {
		return err
	}
	foundNamespace := ns

	status, err := g.getJobSetStatus(name, foundNamespace)
	actionVerb := "Cancel"
	if err == nil && (status == "Completed" || status == "Failed") {
		actionVerb = "Cleanup"
		logging.Info("Cleaning up resources for the '%s' job '%s' in cluster '%s'...", status, name, opts.ClusterName)
	} else {
		logging.Info("Canceling job '%s' in cluster '%s'...", name, opts.ClusterName)
	}

	// Read before the delete so this job's gateways skip the grace period.
	var ownClaims []string
	if js, err := g.kubeClient.GetResource(jobSetGVR, foundNamespace, name); err == nil && js != nil {
		ownClaims = collectClaimNames(js.Object)
	}

	// Sweep the whole namespace to also catch gateways of TTL-deleted jobs.
	if err := g.kubeClient.DeleteJobSet(foundNamespace, name); err != nil {
		opErr := fmt.Errorf("%s operation failed for %s in namespace %s: %w", strings.ToLower(actionVerb), name, foundNamespace, err)
		if !apierrors.IsNotFound(err) {
			return opErr
		}
		logging.Info("Job '%s' was not found in namespace '%s'; it may already have been removed by --gke-ttl-after-finished. Checking for unused storage gateways...", name, foundNamespace)
		g.reclaimStorageGateways(foundNamespace, name, ownClaims)
		return opErr
	}
	logging.Info("%s operation on Job '%s' completed successfully.", actionVerb, name)
	g.reclaimStorageGateways(foundNamespace, name, ownClaims)
	return nil
}

func isToolkitGatewayClaimName(claim string) bool {
	return strings.HasPrefix(claim, gcsFuseGatewayPrefix+"-") ||
		strings.HasPrefix(claim, filestoreGatewayPrefix+"-")
}

func isToolkitManaged(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	return obj.GetLabels()[managedByLabel] == managedByValue
}

// reclaimStorageGateways deletes unused toolkit gateways in the namespace. Failures only warn.
func (g *GKEOrchestrator) reclaimStorageGateways(namespace, cancelledJobSet string, ownClaims []string) {
	if g.kubeClient == nil {
		return
	}
	pvcs, err := g.kubeClient.ListResources(pvcGVR, namespace, managedByLabel+"="+managedByValue)
	if err != nil {
		logging.Warn("Could not list managed storage gateways in namespace '%s': %v", namespace, err)
		return
	}
	pvcs = slices.DeleteFunc(pvcs, func(pvc unstructured.Unstructured) bool {
		name := pvc.GetName()
		return !isToolkitGatewayClaimName(name) ||
			(!slices.Contains(ownClaims, name) && time.Since(pvc.GetCreationTimestamp().Time) < gatewayGracePeriod)
	})
	if len(pvcs) == 0 {
		return
	}

	consumers, err := g.namespaceClaimConsumers(namespace, cancelledJobSet)
	if err != nil {
		logging.Warn("Skipping storage cleanup in namespace '%s': failed to determine active storage consumers: %v", namespace, err)
		return
	}
	pvcs = slices.DeleteFunc(pvcs, func(pvc unstructured.Unstructured) bool {
		user, used := consumers[pvc.GetName()]
		if used {
			logging.Info("Storage gateway '%s' is still used by %s in namespace '%s'. Preserving gateway.", pvc.GetName(), user, namespace)
		}
		return used
	})
	if len(pvcs) == 0 {
		return
	}

	// Re-check right before deleting to narrow the race with a concurrent submit.
	consumers, err = g.namespaceClaimConsumers(namespace, cancelledJobSet)
	if err != nil {
		logging.Warn("Skipping storage cleanup in namespace '%s': failed to re-verify active storage consumers: %v", namespace, err)
		return
	}
	for i := range pvcs {
		if user, used := consumers[pvcs[i].GetName()]; used {
			logging.Info("Storage gateway '%s' was claimed by %s in namespace '%s' while cleaning up. Preserving gateway.", pvcs[i].GetName(), user, namespace)
			continue
		}
		g.deleteStorageGateway(namespace, &pvcs[i])
	}
}

func (g *GKEOrchestrator) deleteStorageGateway(namespace string, pvc *unstructured.Unstructured) {
	claim := pvc.GetName()
	pvName, _, _ := unstructured.NestedString(pvc.Object, "spec", "volumeName")
	if pvName != "" {
		pv, err := g.kubeClient.GetResource(pvGVR, "", pvName)
		switch {
		case apierrors.IsNotFound(err):
			pvName = ""
		case err != nil:
			logging.Warn("Skipping cleanup of storage gateway '%s' in namespace '%s': could not read PersistentVolume '%s' to verify ownership: %v", claim, namespace, pvName, err)
			return
		case !isToolkitManaged(pv):
			logging.Warn("PersistentVolume '%s' bound to gateway '%s' does not carry the '%s: %s' label. Refusing to delete storage gcluster does not own.", pvName, claim, managedByLabel, managedByValue)
			return
		}
	}

	if err := g.kubeClient.DeleteResource(pvcGVR, namespace, claim); err != nil && !apierrors.IsNotFound(err) {
		logging.Warn("Failed to delete PersistentVolumeClaim '%s' in namespace '%s': %v", claim, namespace, err)
		return
	}
	if pvName == "" {
		logging.Info("Reclaimed storage gateway PVC '%s'. Bucket and share data are unaffected.", claim)
		return
	}
	if err := g.kubeClient.DeleteResource(pvGVR, "", pvName); err != nil && !apierrors.IsNotFound(err) {
		logging.Error("Failed to delete PersistentVolume '%s': %v\n"+
			"  The PersistentVolumeClaim was removed, so this volume is now stuck in phase Released with a stale claimRef.\n"+
			"  A future 'gcluster job submit' using the same storage in namespace '%s' will not be able to bind to it and its Pods will stay Pending.\n"+
			"  Remove it manually with:\n"+
			"    kubectl delete pv %s",
			pvName, err, namespace, pvName)
		return
	}
	logging.Info("Reclaimed storage gateway PVC '%s' and PV '%s'. Bucket and share data are unaffected.", claim, pvName)
}

// namespaceClaimConsumers maps each claim used by a live workload to "<resource>/<name>".
func (g *GKEOrchestrator) namespaceClaimConsumers(namespace, cancelledJobSet string) (map[string]string, error) {
	consumers := map[string]string{}
	for _, gvr := range storageConsumerGVRs {
		objs, err := g.kubeClient.ListResources(gvr, namespace, "")
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list %s in namespace %s: %w", gvr.Resource, namespace, err)
		}
		for i := range objs {
			obj := &objs[i]
			if isFinished(obj) || createdByJobSet(obj, gvr, cancelledJobSet) {
				continue
			}
			for _, claim := range collectClaimNames(obj.Object) {
				if _, seen := consumers[claim]; !seen {
					consumers[claim] = gvr.Resource + "/" + obj.GetName()
				}
			}
		}
	}
	return consumers, nil
}

func collectClaimNames(obj interface{}) []string {
	found := map[string]bool{}
	walkForClaimNames(obj, found)
	return slices.Sorted(maps.Keys(found))
}

func walkForClaimNames(node interface{}, found map[string]bool) {
	switch typed := node.(type) {
	case map[string]interface{}:
		if pvc, ok := typed["persistentVolumeClaim"].(map[string]interface{}); ok {
			if claimName, ok := pvc["claimName"].(string); ok && claimName != "" {
				found[claimName] = true
			}
		}
		for _, v := range typed {
			walkForClaimNames(v, found)
		}
	case []interface{}:
		for _, v := range typed {
			walkForClaimNames(v, found)
		}
	}
}

func createdByJobSet(obj *unstructured.Unstructured, gvr schema.GroupVersionResource, jobSetName string) bool {
	if jobSetName == "" {
		return false
	}
	if gvr == jobSetGVR {
		return obj.GetName() == jobSetName
	}
	labels := obj.GetLabels()
	return labels["jobset.sigs.k8s.io/jobset-name"] == jobSetName ||
		labels["gcluster.google.com/workload"] == jobSetName
}

func isFinished(obj *unstructured.Unstructured) bool {
	if phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase"); phase == "Succeeded" || phase == "Failed" {
		return true
	}
	status, _ := parseJobStatus(obj.Object)
	return status == "Succeeded" || status == "Failed"
}
