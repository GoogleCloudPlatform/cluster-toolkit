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
	"testing"
	"time"

	"k8s.io/client-go/dynamic"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	return &GKEOrchestrator{executor: ex, kubeClient: kc, namespace: "default", dynClient: nopDynamicClient{}}
}

var cancelOpts = orchestrator.CancelOptions{ClusterName: "c", ClusterLocation: "us-central1", ProjectID: "p"}

// TestCancelJob_SweepsGatewaysLeftByTTLExpiredJobs covers a gateway whose
// JobSet was already removed by --gke-ttl-after-finished: it is no longer
// referenced by any JobSet, but the namespace sweep must still reclaim it.
func TestCancelJob_SweepsGatewaysLeftByTTLExpiredJobs(t *testing.T) {
	mock := gatewayMock(nil)
	if err := cancelTestOrchestrator(mock).CancelJob("job-b", cancelOpts); err != nil {
		t.Fatalf("CancelJob() unexpected error: %v", err)
	}
	assertDeleted(t, mock, []string{testClaim}, []string{testPV})
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

func TestReclaimStorageGateways_RechecksConsumersBeforeDeleting(t *testing.T) {
	racing := &racingKubeClient{
		MockKubeClient: *gatewayMock(nil),
		lateConsumers:  []unstructured.Unstructured{podWithClaims("job-b-0", "Pending", "job-b", testClaim)},
	}
	(&GKEOrchestrator{kubeClient: racing}).reclaimStorageGateways("default", "job-a", nil)

	assertDeleted(t, &racing.MockKubeClient, nil, nil)
	if racing.listPodCalls < 2 {
		t.Errorf("consumers were listed %d time(s); the pre-delete re-check is missing", racing.listPodCalls)
	}
}

type racingKubeClient struct {
	MockKubeClient
	lateConsumers []unstructured.Unstructured
	listPodCalls  int
}

func (m *racingKubeClient) ListResources(gvr schema.GroupVersionResource, namespace, labelSelector string) ([]unstructured.Unstructured, error) {
	if gvr != podGVR {
		return m.MockKubeClient.ListResources(gvr, namespace, labelSelector)
	}
	m.listPodCalls++
	if m.listPodCalls == 1 {
		return nil, nil
	}
	return m.lateConsumers, nil
}

func TestToolkitGatewayClaimPrefixMatchesGenerator(t *testing.T) {
	name := gcsFuseGatewayPVCName("bucket", "training", "abc123")
	if !isToolkitGatewayClaimName(name) {
		t.Errorf("generated claim %q does not match isToolkitGatewayClaimName()", name)
	}
}

func TestIsToolkitManaged(t *testing.T) {
	if isToolkitManaged(nil) {
		t.Error("a nil object must never be treated as managed")
	}
	if isToolkitManaged(unmanagedStorageObject("PersistentVolume", "pv")) {
		t.Error("an unlabelled object must not be treated as managed")
	}
	if !isToolkitManaged(managedStorageObject("PersistentVolume", "pv")) {
		t.Error("a labelled object must be treated as managed")
	}

	wrongValue := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":   "pv",
			"labels": map[string]interface{}{managedByLabel: "something-else"},
		},
	}}
	if isToolkitManaged(wrongValue) {
		t.Error("a different managed-by value must not be treated as managed")
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
