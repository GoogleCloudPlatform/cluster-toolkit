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
	"errors"
	"fmt"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	testClaim = "gcluster-gcsfuse-bucket-training-abc123"
	testPV    = testClaim + "-default"
)

func claimVolumes(claims []string) []interface{} {
	volumes := make([]interface{}, 0, len(claims))
	for i, c := range claims {
		volumes = append(volumes, map[string]interface{}{
			"name":                  fmt.Sprintf("vol-%d", i),
			"persistentVolumeClaim": map[string]interface{}{"claimName": c},
		})
	}
	return volumes
}

// workloadWithClaims builds a controller whose pod template mounts claims.
func workloadWithClaims(kind, name string, labels map[string]interface{}, spec map[string]interface{}, claims ...string) unstructured.Unstructured {
	if spec == nil {
		spec = map[string]interface{}{}
	}
	spec["template"] = map[string]interface{}{"spec": map[string]interface{}{"volumes": claimVolumes(claims)}}
	meta := map[string]interface{}{"name": name, "namespace": "default"}
	if labels != nil {
		meta["labels"] = labels
	}
	return unstructured.Unstructured{Object: map[string]interface{}{"kind": kind, "metadata": meta, "spec": spec}}
}

func jobSetWithClaims(name string, suspend bool, claims ...string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "jobset.x-k8s.io/v1alpha2",
		"kind":       "JobSet",
		"metadata":   map[string]interface{}{"name": name, "namespace": "default"},
		"spec": map[string]interface{}{
			"suspend": suspend,
			"replicatedJobs": []interface{}{
				map[string]interface{}{
					"name": "main-job",
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{"volumes": claimVolumes(claims)},
							},
						},
					},
				},
			},
		},
	}}
}

func withCondition(obj unstructured.Unstructured, condType string) unstructured.Unstructured {
	obj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{map[string]interface{}{"type": condType, "status": "True"}},
	}
	return obj
}

func completedJobSet(name string, claims ...string) *unstructured.Unstructured {
	js := withCondition(*jobSetWithClaims(name, false, claims...), "Completed")
	return &js
}

func podWithClaims(name, phase, jobSetName string, claims ...string) unstructured.Unstructured {
	meta := map[string]interface{}{"name": name, "namespace": "default"}
	if jobSetName != "" {
		meta["labels"] = map[string]interface{}{"jobset.sigs.k8s.io/jobset-name": jobSetName}
	}
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   meta,
		"spec":       map[string]interface{}{"volumes": claimVolumes(claims)},
		"status":     map[string]interface{}{"phase": phase},
	}}
}

// gatewayPVC builds a managed gateway claim of the given age bound to volumeName ("" means unbound).
func gatewayPVC(name, volumeName string, age time.Duration) unstructured.Unstructured {
	pvc := *managedStorageObject("PersistentVolumeClaim", name)
	pvc.SetCreationTimestamp(metav1.NewTime(time.Now().Add(-age)))
	if volumeName != "" {
		pvc.Object["spec"] = map[string]interface{}{"volumeName": volumeName}
	}
	return pvc
}

// gatewayMock returns a client holding one old, bound, fully labelled gateway plus extra objects.
func gatewayMock(extra map[string][]unstructured.Unstructured) *MockKubeClient {
	objects := map[string][]unstructured.Unstructured{
		"persistentvolumeclaims": {gatewayPVC(testClaim, testPV, time.Hour)},
		"persistentvolumes":      {*managedStorageObject("PersistentVolume", testPV)},
	}
	for k, v := range extra {
		objects[k] = v
	}
	return &MockKubeClient{Namespace: "default", Objects: objects}
}

func assertDeleted(t *testing.T, mock *MockKubeClient, wantPVCs, wantPVs []string) {
	t.Helper()
	if got := mock.Deleted["persistentvolumeclaims"]; !reflect.DeepEqual(got, wantPVCs) {
		t.Errorf("deleted PVCs = %v, want %v", got, wantPVCs)
	}
	if got := mock.Deleted["persistentvolumes"]; !reflect.DeepEqual(got, wantPVs) {
		t.Errorf("deleted PVs = %v, want %v", got, wantPVs)
	}
}

func TestCollectClaimNames(t *testing.T) {
	js := jobSetWithClaims("job-a", false, "gcluster-gcsfuse-b-training", "my-own-pvc")
	got := collectClaimNames(js.Object)
	want := []string{"gcluster-gcsfuse-b-training", "my-own-pvc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("collectClaimNames() = %v, want %v", got, want)
	}

	empty := &unstructured.Unstructured{Object: map[string]interface{}{"kind": "JobSet"}}
	if got := collectClaimNames(empty.Object); len(got) != 0 {
		t.Errorf("collectClaimNames() on empty object = %v, want empty", got)
	}
}

// nopDynamicClient satisfies getDynamicClient without a kubeconfig; CancelJob
// only talks to the cluster through the mocked KubeClient.
type nopDynamicClient struct{ dynamic.Interface }

// cancelTestOrchestrator returns an orchestrator whose CancelJob runs with no
// cluster contact: gcloud/kubectl calls succeed and the namespace is preset.
func cancelTestOrchestrator(kc KubeClient) *GKEOrchestrator {
	ex := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud":  {{ExitCode: 0}, {ExitCode: 0}, {ExitCode: 0}},
		"kubectl": {{ExitCode: 0}, {ExitCode: 0}, {ExitCode: 0}},
	})
	return &GKEOrchestrator{executor: ex, kubeClient: kc, namespace: "default", dynClient: nopDynamicClient{},
		kubeconfigLoader: staticLoader(clientcmdapi.NewConfig())}
}

var cancelOpts = orchestrator.CancelOptions{ClusterName: "c", ClusterLocation: "us-central1", ProjectID: "p"}

// TestCancelJob_SweepsGatewaysLeftByTTLExpiredJobs covers a gateway whose
// JobSet was already removed by --gke-ttl-after-finished: it is no longer
// referenced by any JobSet, but the namespace sweep must still reclaim it.
func TestCancelJob_SweepsGatewaysLeftByTTLExpiredJobs(t *testing.T) {
	mock := gatewayMock(nil)
	orc := cancelTestOrchestrator(mock)
	if err := orc.CancelJob("job-b", cancelOpts); err != nil {
		t.Fatalf("CancelJob() unexpected error: %v", err)
	}
	assertDeleted(t, mock, []string{testClaim}, []string{testPV})
	orc.executor.(*MockExecutor).assertAllKubectlPinned(t, "gke_p_us-central1_c")
}

// TestCancelJob_NotFoundStillReclaimsAndReportsError covers cancelling a job
// that TTL already deleted: storage is still swept, but the caller is told the
// job was not found so a mistyped name is not silently accepted.
func TestCancelJob_NotFoundStillReclaimsAndReportsError(t *testing.T) {
	mock := gatewayMock(nil)
	mock.DeleteJobSetErr = apierrors.NewNotFound(jobSetGVR.GroupResource(), "job-a")

	err := cancelTestOrchestrator(mock).CancelJob("job-a", cancelOpts)
	if err == nil || !apierrors.IsNotFound(errors.Unwrap(err)) {
		t.Fatalf("CancelJob() = %v, want a wrapped NotFound error", err)
	}
	assertDeleted(t, mock, []string{testClaim}, []string{testPV})
}

// TestCancelJob_OtherDeleteErrorSkipsCleanup makes sure a real API failure on
// the JobSet delete aborts before any storage is touched.
func TestCancelJob_OtherDeleteErrorSkipsCleanup(t *testing.T) {
	mock := gatewayMock(nil)
	mock.DeleteJobSetErr = fmt.Errorf("forbidden")
	if err := cancelTestOrchestrator(mock).CancelJob("job-a", cancelOpts); err == nil {
		t.Fatal("CancelJob() expected an error")
	}
	assertDeleted(t, mock, nil, nil)
}

// TestCancelJob_FinishedJobUsesCleanupVerb checks the Cancel/Cleanup choice comes from the fetched JobSet.
func TestCancelJob_FinishedJobUsesCleanupVerb(t *testing.T) {
	for name, tc := range map[string]struct {
		js   *unstructured.Unstructured
		verb string
	}{
		"finished job": {js: completedJobSet("job-a"), verb: "cleanup operation failed"},
		"running job":  {js: jobSetWithClaims("job-a", false), verb: "cancel operation failed"},
	} {
		t.Run(name, func(t *testing.T) {
			mock := gatewayMock(map[string][]unstructured.Unstructured{"jobsets": {*tc.js}})
			mock.DeleteJobSetErr = fmt.Errorf("forbidden")
			err := cancelTestOrchestrator(mock).CancelJob("job-a", cancelOpts)
			if err == nil || !strings.Contains(err.Error(), tc.verb) {
				t.Fatalf("CancelJob() = %v, want error containing %q", err, tc.verb)
			}
		})
	}
}

// TestCancelJob_GracePeriodExemptsOwnGateways covers cancelling right after submit: the job's own fresh
// gateway is reclaimed, while a fresh gateway of another job that may still be mid-submit is kept.
func TestCancelJob_GracePeriodExemptsOwnGateways(t *testing.T) {
	const own, other = "gcluster-gcsfuse-own-serving-111111", "gcluster-gcsfuse-other-serving-222222"
	mock := &MockKubeClient{Namespace: "default", Objects: map[string][]unstructured.Unstructured{
		"persistentvolumeclaims": {gatewayPVC(own, own+"-default", time.Second), gatewayPVC(other, other+"-default", time.Second)},
		"persistentvolumes":      {*managedStorageObject("PersistentVolume", own+"-default"), *managedStorageObject("PersistentVolume", other+"-default")},
		"jobsets":                {*jobSetWithClaims("job-a", false, own)},
	}}
	if err := cancelTestOrchestrator(mock).CancelJob("job-a", cancelOpts); err != nil {
		t.Fatalf("CancelJob() unexpected error: %v", err)
	}
	assertDeleted(t, mock, []string{own}, []string{own + "-default"})
}

func TestReclaimStorageGateways_Consumers(t *testing.T) {
	suspended := map[string]interface{}{"suspend": true}
	tests := []struct {
		name    string
		objects map[string][]unstructured.Unstructured
		keep    bool
	}{
		{name: "no other consumers reclaims the gateway"},
		{name: "pods of the cancelled jobset are ignored",
			objects: map[string][]unstructured.Unstructured{"pods": {podWithClaims("job-a-0", "Running", "job-a", testClaim)}}},
		{name: "terminated pods do not keep the gateway alive",
			objects: map[string][]unstructured.Unstructured{"pods": {podWithClaims("b-0", "Succeeded", "job-b", testClaim), podWithClaims("c-0", "Failed", "job-c", testClaim)}}},
		{name: "running pod of another job preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"pods": {podWithClaims("b-0", "Running", "job-b", testClaim)}}},
		{name: "pending pod of another job preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"pods": {podWithClaims("b-0", "Pending", "job-b", testClaim)}}},
		{name: "suspended kueue-queued jobset preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"jobsets": {*jobSetWithClaims("job-queued", true, testClaim)}}},
		{name: "completed jobset does not preserve the gateway",
			objects: map[string][]unstructured.Unstructured{"jobsets": {*completedJobSet("job-done", testClaim)}}},
		{name: "the cancelled jobset itself does not preserve the gateway",
			objects: map[string][]unstructured.Unstructured{"jobsets": {*jobSetWithClaims("job-a", false, testClaim)}}},
		{name: "a job left behind by the cancelled jobset is ignored",
			objects: map[string][]unstructured.Unstructured{"jobs": {workloadWithClaims("Job", "job-a-main-0", map[string]interface{}{"jobset.sigs.k8s.io/jobset-name": "job-a"}, nil, testClaim)}}},
		{name: "deployment scaled to zero preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"deployments": {workloadWithClaims("Deployment", "web", nil, map[string]interface{}{"replicas": int64(0)}, testClaim)}}},
		{name: "statefulset preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"statefulsets": {workloadWithClaims("StatefulSet", "db", nil, nil, testClaim)}}},
		{name: "suspended batch job preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"jobs": {workloadWithClaims("Job", "batch", nil, suspended, testClaim)}}},
		{name: "completed batch job does not preserve the gateway",
			objects: map[string][]unstructured.Unstructured{"jobs": {withCondition(workloadWithClaims("Job", "batch", nil, nil, testClaim), "Complete")}}},
		{name: "cronjob preserves the gateway", keep: true,
			objects: map[string][]unstructured.Unstructured{"cronjobs": {workloadWithClaims("CronJob", "nightly", nil, nil, testClaim)}}},
		{name: "consumers of a different claim are ignored",
			objects: map[string][]unstructured.Unstructured{"pods": {podWithClaims("b-0", "Running", "job-b", "some-other-pvc")}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := gatewayMock(tc.objects)
			(&GKEOrchestrator{kubeClient: mock}).reclaimStorageGateways("default", "job-a", nil)
			if tc.keep {
				assertDeleted(t, mock, nil, nil)
			} else {
				assertDeleted(t, mock, []string{testClaim}, []string{testPV})
			}
		})
	}
}

func TestReclaimStorageGateways_Ownership(t *testing.T) {
	forbidden := fmt.Errorf("forbidden")
	tests := []struct {
		name     string
		pvcs     []unstructured.Unstructured
		pvs      []unstructured.Unstructured
		getErr   error
		wantPVCs []string
		wantPVs  []string
	}{
		{name: "labelled pair is deleted, PV found through spec.volumeName",
			pvcs:     []unstructured.Unstructured{gatewayPVC(testClaim, "custom-pv-name", time.Hour)},
			pvs:      []unstructured.Unstructured{*managedStorageObject("PersistentVolume", "custom-pv-name")},
			wantPVCs: []string{testClaim}, wantPVs: []string{"custom-pv-name"}},
		{name: "unused NFS gateway PVC and bound PV pair is reclaimed",
			pvcs:     []unstructured.Unstructured{gatewayPVC(nfsGatewayPVCName("10.0.0.1", "exports/train"), nfsGatewayPVCName("10.0.0.1", "exports/train")+"-default", time.Hour)},
			pvs:      []unstructured.Unstructured{*managedStorageObject("PersistentVolume", nfsGatewayPVCName("10.0.0.1", "exports/train")+"-default")},
			wantPVCs: []string{nfsGatewayPVCName("10.0.0.1", "exports/train")}, wantPVs: []string{nfsGatewayPVCName("10.0.0.1", "exports/train") + "-default"}},
		{name: "unlabelled bound PV blocks the whole gateway",
			pvcs: []unstructured.Unstructured{gatewayPVC(testClaim, testPV, time.Hour)},
			pvs:  []unstructured.Unstructured{*unmanagedStorageObject("PersistentVolume", testPV)}},
		{name: "an already absent PV does not block reclaiming its PVC",
			pvcs:     []unstructured.Unstructured{gatewayPVC(testClaim, testPV, time.Hour)},
			wantPVCs: []string{testClaim}},
		{name: "an unbound PVC is deleted without touching any PV",
			pvcs:     []unstructured.Unstructured{gatewayPVC(testClaim, "", time.Hour)},
			wantPVCs: []string{testClaim}},
		{name: "an unreadable PV fails safe and deletes nothing",
			pvcs:   []unstructured.Unstructured{gatewayPVC(testClaim, testPV, time.Hour)},
			getErr: forbidden},
		{name: "a claim without the toolkit prefix is never deleted",
			pvcs: []unstructured.Unstructured{gatewayPVC("my-own-pvc", testPV, time.Hour)},
			pvs:  []unstructured.Unstructured{*managedStorageObject("PersistentVolume", testPV)}},
		{name: "a fresh gateway of another job is kept for the grace period",
			pvcs: []unstructured.Unstructured{gatewayPVC(testClaim, testPV, time.Second)},
			pvs:  []unstructured.Unstructured{*managedStorageObject("PersistentVolume", testPV)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &MockKubeClient{Namespace: "default", GetErr: tc.getErr, Objects: map[string][]unstructured.Unstructured{
				"persistentvolumeclaims": tc.pvcs,
				"persistentvolumes":      tc.pvs,
			}}
			(&GKEOrchestrator{kubeClient: mock}).reclaimStorageGateways("default", "job-a", nil)
			assertDeleted(t, mock, tc.wantPVCs, tc.wantPVs)
		})
	}
}

func TestReclaimStorageGateways_ListErrors(t *testing.T) {
	for _, gvr := range append([]schema.GroupVersionResource{pvcGVR}, storageConsumerGVRs...) {
		t.Run(gvr.Resource+" failure deletes nothing", func(t *testing.T) {
			mock := gatewayMock(nil)
			mock.ListErrs = map[string]error{gvr.Resource: fmt.Errorf("api down")}
			(&GKEOrchestrator{kubeClient: mock}).reclaimStorageGateways("default", "job-a", nil)
			assertDeleted(t, mock, nil, nil)
		})
	}

	t.Run("a kind the cluster does not serve is skipped", func(t *testing.T) {
		mock := gatewayMock(nil)
		mock.ListErrs = map[string]error{"jobsets": apierrors.NewNotFound(jobSetGVR.GroupResource(), "")}
		(&GKEOrchestrator{kubeClient: mock}).reclaimStorageGateways("default", "job-a", nil)
		assertDeleted(t, mock, []string{testClaim}, []string{testPV})
	})
}

func TestReclaimStorageGateways_DeleteErrors(t *testing.T) {
	notFound := apierrors.NewNotFound(pvcGVR.GroupResource(), testClaim)

	gone := gatewayMock(nil)
	gone.DeleteErrs = map[string]error{"persistentvolumeclaims": notFound}
	(&GKEOrchestrator{kubeClient: gone}).reclaimStorageGateways("default", "job-a", nil)
	assertDeleted(t, gone, nil, []string{testPV})

	failed := gatewayMock(nil)
	failed.DeleteErrs = map[string]error{"persistentvolumeclaims": fmt.Errorf("forbidden")}
	(&GKEOrchestrator{kubeClient: failed}).reclaimStorageGateways("default", "job-a", nil)
	assertDeleted(t, failed, nil, nil)
}

// A submit that claimed the gateway after the scan bumps its resourceVersion, so the conditional delete conflicts.
func TestReclaimStorageGateways_ConflictPreservesGateway(t *testing.T) {
	mock := gatewayMock(nil)
	mock.DeleteErrs = map[string]error{"persistentvolumeclaims": apierrors.NewConflict(pvcGVR.GroupResource(), testClaim, fmt.Errorf("rv changed"))}
	(&GKEOrchestrator{kubeClient: mock}).reclaimStorageGateways("default", "job-a", nil)
	assertDeleted(t, mock, nil, nil)
}

func TestDeleteStorageGateway_Preconditions(t *testing.T) {
	pvc := gatewayPVC(testClaim, testPV, time.Hour)
	pvc.SetUID("pvc-uid")
	pvc.SetResourceVersion("42")
	boundPV := func(claimUID string) unstructured.Unstructured {
		pv := *managedStorageObject("PersistentVolume", testPV)
		pv.SetUID("pv-uid")
		pv.Object["spec"] = map[string]interface{}{"claimRef": map[string]interface{}{"uid": claimUID}}
		return pv
	}

	t.Run("PVC delete pins uid and resourceVersion, PV delete pins uid", func(t *testing.T) {
		mock := &MockKubeClient{Objects: map[string][]unstructured.Unstructured{"persistentvolumes": {boundPV("pvc-uid")}}}
		(&GKEOrchestrator{kubeClient: mock}).deleteStorageGateway("default", pvc.DeepCopy())
		assertDeleted(t, mock, []string{testClaim}, []string{testPV})
		if pre := mock.DeletePre[testClaim]; pre == nil || *pre.UID != "pvc-uid" || *pre.ResourceVersion != "42" {
			t.Errorf("PVC preconditions = %+v, want uid pvc-uid and resourceVersion 42", pre)
		}
		if pre := mock.DeletePre[testPV]; pre == nil || *pre.UID != "pv-uid" || pre.ResourceVersion != nil {
			t.Errorf("PV preconditions = %+v, want uid pv-uid only", pre)
		}
	})

	t.Run("PV bound to a newer claim is kept", func(t *testing.T) {
		mock := &MockKubeClient{Objects: map[string][]unstructured.Unstructured{"persistentvolumes": {boundPV("newer-uid")}}}
		(&GKEOrchestrator{kubeClient: mock}).deleteStorageGateway("default", pvc.DeepCopy())
		assertDeleted(t, mock, []string{testClaim}, nil)
	})
}

func stampedPVC(age time.Duration, claimedAgo time.Duration, claimedBy string) unstructured.Unstructured {
	pvc := gatewayPVC(testClaim, testPV, age)
	pvc.SetAnnotations(map[string]string{
		lastClaimedAtAnnotation: time.Now().Add(-claimedAgo).UTC().Format(time.RFC3339Nano),
		lastClaimedByAnnotation: claimedBy,
	})
	return pvc
}

func TestWithinGracePeriod(t *testing.T) {
	tests := []struct {
		name      string
		pvc       unstructured.Unstructured
		cancelled string
		ownClaims []string
		want      bool
	}{
		{name: "old gateway reused seconds ago by another job", pvc: stampedPVC(time.Hour, time.Second, "job-b"), cancelled: "job-a", want: true},
		{name: "old gateway last claimed long ago", pvc: stampedPVC(time.Hour, time.Hour, "job-b"), cancelled: "job-a"},
		{name: "cancelled job's own fresh claim is exempt", pvc: stampedPVC(time.Second, time.Second, "job-a"), cancelled: "job-a"},
		{name: "own claim overwritten by another job is protected", pvc: stampedPVC(time.Hour, time.Second, "job-b"), cancelled: "job-a", ownClaims: []string{testClaim}, want: true},
		{name: "unstamped fresh gateway of another job", pvc: gatewayPVC(testClaim, testPV, time.Second), cancelled: "job-a", want: true},
		{name: "unstamped fresh own gateway is exempt", pvc: gatewayPVC(testClaim, testPV, time.Second), cancelled: "job-a", ownClaims: []string{testClaim}},
		{name: "unparsable stamp falls back to creation time", pvc: func() unstructured.Unstructured {
			p := gatewayPVC(testClaim, testPV, time.Hour)
			p.SetAnnotations(map[string]string{lastClaimedAtAnnotation: "yesterday", lastClaimedByAnnotation: "job-b"})
			return p
		}(), cancelled: "job-a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinGracePeriod(&tc.pvc, tc.cancelled, tc.ownClaims); got != tc.want {
				t.Errorf("withinGracePeriod() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestToolkitGatewayClaimPrefixMatchesGenerator(t *testing.T) {
	cases := []string{
		gcsFuseGatewayPVCName("bucket", "training", "abc123"),
		filestoreGatewayPrefix + "-my-fs-share",
		nfsGatewayPVCName("10.0.0.1", "export"),
		nfsGatewayPVCName("nfs-server.example.internal", strings.Repeat("segment/", 30)+"leaf"),
	}
	for _, claim := range cases {
		if !isToolkitGatewayClaimName(claim) {
			t.Errorf("generated claim %q does not match isToolkitGatewayClaimName()", claim)
		}
	}
}

func TestIsFinished(t *testing.T) {
	tests := map[string]struct {
		obj  unstructured.Unstructured
		want bool
	}{
		"running pod":        {podWithClaims("p", "Running", ""), false},
		"pending pod":        {podWithClaims("p", "Pending", ""), false},
		"pod without phase":  {podWithClaims("p", "", ""), false},
		"succeeded pod":      {podWithClaims("p", "Succeeded", ""), true},
		"failed pod":         {podWithClaims("p", "Failed", ""), true},
		"suspended jobset":   {*jobSetWithClaims("a", true), false},
		"running jobset":     {*jobSetWithClaims("a", false), false},
		"completed jobset":   {*completedJobSet("a"), true},
		"complete batch job": {withCondition(workloadWithClaims("Job", "j", nil, nil), "Complete"), true},
		"failed batch job":   {withCondition(workloadWithClaims("Job", "j", nil, nil), "Failed"), true},
		"deployment":         {withCondition(workloadWithClaims("Deployment", "d", nil, nil), "Available"), false},
	}
	for name, tc := range tests {
		if got := isFinished(&tc.obj); got != tc.want {
			t.Errorf("isFinished(%s) = %v, want %v", name, got, tc.want)
		}
	}
}

func TestCreatedByJobSet(t *testing.T) {
	pod := podWithClaims("p", "Running", "job-a")
	job := workloadWithClaims("Job", "job-a-main-0", map[string]interface{}{"jobset.sigs.k8s.io/jobset-name": "job-a"}, nil)
	tests := []struct {
		name string
		obj  unstructured.Unstructured
		gvr  schema.GroupVersionResource
		js   string
		want bool
	}{
		{"pod of the jobset", pod, podGVR, "job-a", true},
		{"pod of another jobset", pod, podGVR, "job-b", false},
		{"job of the jobset", job, jobGVR, "job-a", true},
		{"the jobset itself", *jobSetWithClaims("job-a", false), jobSetGVR, "job-a", true},
		{"another jobset", *jobSetWithClaims("job-b", false), jobSetGVR, "job-a", false},
		{"empty jobset name never matches", pod, podGVR, "", false},
	}
	for _, tc := range tests {
		if got := createdByJobSet(&tc.obj, tc.gvr, tc.js); got != tc.want {
			t.Errorf("%s: createdByJobSet() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func makeTestOwnedBatchJob(name, uid string) unstructured.Unstructured {
	obj := workloadWithClaims("Job", name, map[string]interface{}{workloadLabel: name}, nil)
	obj.Object["metadata"].(map[string]interface{})["uid"] = "job-" + uid
	return obj
}

func makeTestOwnedService(name, uid string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
			"uid":       "svc-" + uid,
			"labels":    map[string]interface{}{workloadLabel: name},
		},
	}}
}

func makeTestUnownedObject(kind, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"kind": kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": "default",
			"labels":    map[string]interface{}{"app": "other-team"},
		},
	}}
}

func TestCancelJob_FallbackBatchJobAndOwnershipGuards(t *testing.T) {
	noMatchErr := errors.New(`the server could not find the requested resource (delete jobsets.jobset.x-k8s.io)`)
	notFoundErr := apierrors.NewNotFound(jobSetGVR.GroupResource(), "tier3-job")

	t.Run("NoMatch deletes owned batch/v1 Job and headless Service", func(t *testing.T) {
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: noMatchErr,
			ListErrs:        map[string]error{"jobsets": noMatchErr},
			Objects: map[string][]unstructured.Unstructured{
				"jobs":     {makeTestOwnedBatchJob("tier3-job", "1")},
				"services": {makeTestOwnedService("tier3-job", "1")},
			},
		}
		if err := cancelTestOrchestrator(mock).CancelJob("tier3-job", cancelOpts); err != nil {
			t.Fatalf("CancelJob(NoMatch fallback) err = %v, want nil", err)
		}
		if !reflect.DeepEqual(mock.Deleted["jobs"], []string{"tier3-job"}) || !reflect.DeepEqual(mock.Deleted["services"], []string{"tier3-job"}) {
			t.Errorf("Deleted = %v, want jobs=[tier3-job] and services=[tier3-job]", mock.Deleted)
		}
	})

	t.Run("NotFound deletes owned batch/v1 Job and skips unowned same-named Service", func(t *testing.T) {
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: notFoundErr,
			Objects: map[string][]unstructured.Unstructured{
				"jobs":     {makeTestOwnedBatchJob("tier3-job", "2")},
				"services": {makeTestUnownedObject("Service", "tier3-job")},
			},
		}
		if err := cancelTestOrchestrator(mock).CancelJob("tier3-job", cancelOpts); err != nil {
			t.Fatalf("CancelJob(NotFound fallback) err = %v, want nil", err)
		}
		if !reflect.DeepEqual(mock.Deleted["jobs"], []string{"tier3-job"}) || len(mock.Deleted["services"]) != 0 {
			t.Errorf("Deleted = %v, want jobs=[tier3-job] and no services deleted", mock.Deleted)
		}
	})

	t.Run("NotFound refuses to delete unowned same-named batch/v1 Job", func(t *testing.T) {
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: notFoundErr,
			Objects: map[string][]unstructured.Unstructured{
				"jobs":     {makeTestUnownedObject("Job", "tier3-job")},
				"services": {makeTestOwnedService("tier3-job", "3")},
			},
		}
		err := cancelTestOrchestrator(mock).CancelJob("tier3-job", cancelOpts)
		if err == nil || !strings.Contains(err.Error(), "missing required label "+workloadLabel+"=tier3-job") {
			t.Fatalf("CancelJob(unowned Job) err = %v, want ownership refusal", err)
		}
		if len(mock.Deleted) != 0 {
			t.Errorf("Deleted = %v, want none", mock.Deleted)
		}
	})

	t.Run("TTL-expired batch/v1 Job still deletes owned headless Service while sparing unowned", func(t *testing.T) {
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: notFoundErr,
			Objects: map[string][]unstructured.Unstructured{
				"services": {makeTestOwnedService("tier3-job", "4")},
			},
		}
		if err := cancelTestOrchestrator(mock).CancelJob("tier3-job", cancelOpts); err != nil {
			t.Fatalf("CancelJob(TTL-expired Tier-3) err = %v, want nil", err)
		}
		if !reflect.DeepEqual(mock.Deleted["services"], []string{"tier3-job"}) {
			t.Errorf("Deleted services = %v, want [tier3-job]", mock.Deleted["services"])
		}
	})
}

func TestCancelJob_FallbackPropagatesForbiddenAndTimeout(t *testing.T) {
	forbiddenErr := apierrors.NewForbidden(jobSetGVR.GroupResource(), "tier3-job", errors.New("denied"))
	timeoutErr := errors.New("context deadline exceeded")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"403 Forbidden propagates without fallback", forbiddenErr},
		{"timeout propagates without fallback", timeoutErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &MockKubeClient{
				Namespace:       "default",
				DeleteJobSetErr: tc.err,
				Objects: map[string][]unstructured.Unstructured{
					"jobs":     {makeTestOwnedBatchJob("tier3-job", "4")},
					"services": {makeTestOwnedService("tier3-job", "4")},
				},
			}
			err := cancelTestOrchestrator(mock).CancelJob("tier3-job", cancelOpts)
			if err == nil || !errors.Is(err, tc.err) {
				t.Fatalf("CancelJob() err = %v, want wrapped %v", err, tc.err)
			}
			if len(mock.Deleted) != 0 {
				t.Errorf("Deleted = %v, want empty (must not fall back on %s)", mock.Deleted, tc.name)
			}
		})
	}

	t.Run("403 reading fallback Job propagates without deleting", func(t *testing.T) {
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: apierrors.NewNotFound(jobSetGVR.GroupResource(), "tier3-job"),
			GetErr:          forbiddenErr,
		}
		err := cancelTestOrchestrator(mock).CancelJob("tier3-job", cancelOpts)
		if err == nil || !errors.Is(err, forbiddenErr) {
			t.Fatalf("CancelJob(fallback GetResource 403) err = %v, want wrapped 403", err)
		}
		if len(mock.Deleted) != 0 {
			t.Errorf("Deleted = %v, want none", mock.Deleted)
		}
	})
}

func TestCancelJob_NoMatchNotFoundMappingDisableJobSetAndServiceDeleteError(t *testing.T) {
	noMatchErr := &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "jobset.x-k8s.io", Kind: "JobSet"}}

	t.Run("nomatch_missing_job_returns_batch_job_notfound_not_nomatch", func(t *testing.T) {
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: noMatchErr,
		}
		err := cancelTestOrchestrator(mock).CancelJob("gone-job", cancelOpts)
		if err == nil || !apierrors.IsNotFound(err) {
			t.Fatalf("CancelJob(NoMatch + missing job) err = %v, want batch/v1 Job NotFound error", err)
		}
	})

	t.Run("disable_jobset_env_skips_jobset_delete_and_avoids_403", func(t *testing.T) {
		forbiddenErr := apierrors.NewForbidden(jobSetGVR.GroupResource(), "t3-job", errors.New("denied"))
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: forbiddenErr,
			Objects: map[string][]unstructured.Unstructured{
				"jobs":     {makeTestOwnedBatchJob("t3-job", "1")},
				"services": {makeTestOwnedService("t3-job", "1")},
			},
		}
		orc := cancelTestOrchestrator(mock)
		orc.connectivity = &connectivityChecker{
			getenv: func(k string) string {
				if k == disableJobSetEnvVar {
					return "1"
				}
				return ""
			},
		}
		if err := orc.CancelJob("t3-job", cancelOpts); err != nil {
			t.Fatalf("CancelJob(GCLUSTER_DISABLE_JOBSET=1) err = %v, want nil", err)
		}
		if !reflect.DeepEqual(mock.Deleted["jobs"], []string{"t3-job"}) || !reflect.DeepEqual(mock.Deleted["services"], []string{"t3-job"}) {
			t.Errorf("Deleted = %v, want jobs+services=[t3-job]", mock.Deleted)
		}
	})

	t.Run("service_delete_error_propagates_instead_of_being_swallowed", func(t *testing.T) {
		svcDelErr := apierrors.NewForbidden(serviceGVR.GroupResource(), "t3-job", errors.New("svc delete denied"))
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: apierrors.NewNotFound(jobSetGVR.GroupResource(), "t3-job"),
			DeleteErrs:      map[string]error{"services": svcDelErr},
			Objects: map[string][]unstructured.Unstructured{
				"jobs":     {makeTestOwnedBatchJob("t3-job", "1")},
				"services": {makeTestOwnedService("t3-job", "1")},
			},
		}
		err := cancelTestOrchestrator(mock).CancelJob("t3-job", cancelOpts)
		if err == nil || !errors.Is(err, svcDelErr) {
			t.Fatalf("CancelJob(Service delete 403) err = %v, want wrapped %v", err, svcDelErr)
		}
	})
}

func TestCancelJob_DisableJobSetPreservesQueuedJobSetPVCsAnd403Fallback(t *testing.T) {
	t.Run("disable_jobset_still_scans_jobsets_before_reclaiming_pvcs", func(t *testing.T) {
		mock := gatewayMock(map[string][]unstructured.Unstructured{
			"jobs":     {makeTestOwnedBatchJob("t3-job", "1")},
			"services": {makeTestOwnedService("t3-job", "1")},
			"jobsets": {{Object: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "queued-js", "namespace": "default"},
				"spec": map[string]interface{}{
					"replicatedJobs": []interface{}{
						map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"template": map[string]interface{}{
										"spec": map[string]interface{}{
											"volumes": []interface{}{
												map[string]interface{}{"persistentVolumeClaim": map[string]interface{}{"claimName": testClaim}},
											},
										},
									},
								},
							},
						},
					},
				},
			}}},
		})
		orc := cancelTestOrchestrator(mock)
		orc.connectivity = &connectivityChecker{
			getenv: func(k string) string {
				if k == disableJobSetEnvVar {
					return "1"
				}
				return ""
			},
		}
		if err := orc.CancelJob("t3-job", cancelOpts); err != nil {
			t.Fatalf("CancelJob() err = %v", err)
		}
		if len(mock.Deleted["persistentvolumeclaims"]) != 0 {
			t.Errorf("PVC used by queued JobSet must not be reclaimed under %s=1, deleted=%v", disableJobSetEnvVar, mock.Deleted)
		}
	})

	t.Run("forbidden_jobset_confirmed_absent_falls_back_to_batch_job_with_cached_probe", func(t *testing.T) {
		forbErr := apierrors.NewForbidden(jobSetGVR.GroupResource(), "t3-job", errors.New("forbidden"))
		mock := &MockKubeClient{
			Namespace:       "default",
			DeleteJobSetErr: forbErr,
			ListErrs:        map[string]error{"jobsets": forbErr},
			Objects: map[string][]unstructured.Unstructured{
				"jobs":     {makeTestOwnedBatchJob("t3-job", "1")},
				"services": {makeTestOwnedService("t3-job", "1")},
			},
		}
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"gcloud container clusters get-credentials": {{ExitCode: 0}},
			"kubectl get --raw=/readyz":                 {{ExitCode: 0, Stdout: "ok"}},
			"kubectl get --raw /apis/jobset.x-k8s.io":   {{ExitCode: 1, Stderr: "Error from server (NotFound): the server could not find the requested resource"}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.kubeClient = mock
		orc.dynClient = nopDynamicClient{}
		orc.namespace = "default"
		orc.kubeContext = "gke_p_us-central1_c"
		if err := orc.CancelJob("t3-job", cancelOpts); err != nil {
			t.Fatalf("CancelJob(403 JobSet confirmed absent) err = %v", err)
		}
		if !reflect.DeepEqual(mock.Deleted["jobs"], []string{"t3-job"}) {
			t.Errorf("expected fallback batch Job delete, deleted=%v", mock.Deleted)
		}
		probes := 0
		for _, c := range mockExec.calls {
			if strings.Contains(c, "/apis/jobset.x-k8s.io") {
				probes++
			}
		}
		if probes != 1 {
			t.Errorf("expected 1 cached /apis/jobset.x-k8s.io probe, got %d (%v)", probes, mockExec.calls)
		}
	})
}
