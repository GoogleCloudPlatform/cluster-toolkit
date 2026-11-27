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

package gke

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
	"path/filepath"
	"strings"
	"testing"
)

const burstableJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: policy-test
spec:
  replicatedJobs:
  - name: pathways-head
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-rm
              image: img:v1
              resources:
                requests:
                  cpu: "2"
                  memory: 8Gi
            - name: pathways-proxy
              image: img:v1
              resources:
                requests:
                  cpu: "8"
                  memory: 32Gi
`

const boundedJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: bounded-test
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "4"
                  memory: 16Gi
                limits:
                  cpu: "4"
                  memory: 16Gi
`

const partialLimitsJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: partial-test
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "4"
                  memory: 16Gi
                limits:
                  cpu: "4"
`

const initContainerUnboundedManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: init-test
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            initContainers:
            - name: native-sidecar
              image: img:v1
              resources:
                requests:
                  cpu: "1"
                  memory: 4Gi
            containers:
            - name: workload-container
              image: img:v1
              resources:
                limits:
                  cpu: "4"
                  memory: 16Gi
`

const standaloneBurstableJobManifest = `apiVersion: batch/v1
kind: Job
metadata:
  name: standalone-cpu
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: workload-container
        image: img:v1
        resources:
          requests:
            cpu: "4"
            memory: 16Gi
`

const limitsOnlyJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: gpu-test
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                limits:
                  nvidia.com/gpu: "8"
`

const partialMemLimitJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: partial-mem-test
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  memory: 16Gi
                limits:
                  memory: 16Gi
`

const cpuOnlyRequestJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: cpu-only-req
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "4"
`

const fullNodeCPUJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: full-node-cpu
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                limits:
                  cpu: 62770m
`

const terminatingJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: term-job
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            activeDeadlineSeconds: 300
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "4"
                  memory: 16Gi
`

const dupContainerNameJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: dup-names
spec:
  replicatedJobs:
  - name: group-a
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "2"
  - name: group-b
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "2"
`

const priorityClassBurstableJobSetManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: pc-job
spec:
  replicatedJobs:
  - name: main-job
    replicas: 1
    template:
      spec:
        template:
          spec:
            priorityClassName: high
            containers:
            - name: workload-container
              image: img:v1
              resources:
                requests:
                  cpu: "4"
                  memory: 16Gi
`

const mixedHeadAndBestEffortWorkerManifest = `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: pw-promote
spec:
  replicatedJobs:
  - name: pathways-head
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-proxy
              image: img:v1
              resources:
                requests:
                  cpu: "2"
                  memory: 8Gi
                limits:
                  memory: 16Gi
  - name: worker
    replicas: 1
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-worker
              image: img:v1
`

func TestCheckNamespaceResourcePolicies(t *testing.T) {
	emptyList := shell.CommandResult{ExitCode: 0, Stdout: `{"items":[]}`}
	forbidden := func(kind string) shell.CommandResult {
		return shell.CommandResult{ExitCode: 1, Stderr: fmt.Sprintf("Error from server (Forbidden): %s is forbidden", kind)}
	}

	for _, tc := range []struct {
		name           string
		namespace      string
		manifest       string
		lrRes          shell.CommandResult
		rqRes          shell.CommandResult
		wantWarns      []string
		forbidLog      string
		checkCallCount bool
		wantCalls      int
	}{
		{
			name:           "empty policies pass cleanly with no logs",
			namespace:      "ml-ns",
			manifest:       burstableJobSetManifest,
			lrRes:          emptyList,
			rqRes:          emptyList,
			checkCallCount: true,
			wantCalls:      2,
		},
		{
			name:      "LimitRange default.memory below request warns clearly without hard-blocking",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"mem-lr"},"spec":{"limits":[{"type":"Container","default":{"memory":"16Gi"}}]}}]}`},
			rqRes:     emptyList,
			wantWarns: []string{`Container "pathways-proxy" requests memory 32Gi, which exceeds LimitRange "mem-lr" default memory limit (16Gi) in namespace "ml-ns"`},
		},
		{
			name:      "LimitRange default.cpu below container request warns clearly",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cpu-lr"},"spec":{"limits":[{"type":"Container","default":{"cpu":"4"}}]}}]}`},
			rqRes:     emptyList,
			wantWarns: []string{`Container "pathways-proxy" requests cpu 8, which exceeds LimitRange "cpu-lr" default cpu limit (4) in namespace "ml-ns"`},
		},
		{
			name:      "LimitRange default equal to container request warns on burst capping",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"mem-lr"},"spec":{"limits":[{"type":"Container","default":{"memory":"32768Mi","cpu":"8000m"}}]}}]}`},
			rqRes:     emptyList,
			wantWarns: []string{
				`LimitRange "mem-lr" in namespace "ml-ns" sets default cpu limit 8; containers without explicit cpu limits (pathways-rm, pathways-proxy) will have bursting capped at 8`,
				`LimitRange "mem-lr" in namespace "ml-ns" sets default memory limit 32Gi; containers without explicit memory limits (pathways-rm, pathways-proxy) will have bursting capped at 32Gi`,
			},
		},
		{
			name:      "multiple Container LimitRange items use last-wins semantics per LimitRange",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"lr-merge"},"spec":{"limits":[{"type":"Pod","default":{"memory":"1Gi"}},{"type":"Container","default":{"memory":"16Gi"}},{"type":"Container","default":{"memory":"64Gi"}}]}}]}`},
			rqRes:     emptyList,
			wantWarns: []string{`LimitRange "lr-merge" in namespace "ml-ns" sets default memory limit 64Gi`},
		},
		{
			name:      "non-Container LimitRange type is ignored",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"pod-lr"},"spec":{"limits":[{"type":"Pod","default":{"memory":"16Gi"}}]}}]}`},
			rqRes:     emptyList,
			forbidLog: "LimitRange",
		},
		{
			name:      "partial limits skip satisfied limits.cpu quota cleanly without LimitRange",
			namespace: "ml-ns",
			manifest:  partialLimitsJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cpu-q"},"spec":{"hard":{"limits.cpu":"100"}}}]}`},
		},
		{
			name:      "partial limits skip satisfied limits.cpu and still warn on limits.memory",
			namespace: "ml-ns",
			manifest:  partialLimitsJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"both-q"},"spec":{"hard":{"limits.cpu":"100","limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{`ResourceQuota "both-q" in namespace "ml-ns" requires explicit limits.memory, but container(s) (workload-container) omit it`},
			forbidLog: "limits.cpu",
		},
		{
			name:      "partial limits ignore LimitRange default.cpu when limits.cpu is already set",
			namespace: "ml-ns",
			manifest:  partialLimitsJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cpu-lr"},"spec":{"limits":[{"type":"Container","default":{"cpu":"2"}}]}}]}`},
			rqRes:     emptyList,
		},
		{
			name:      "partial memory limit still warns on missing limits.cpu",
			namespace: "ml-ns",
			manifest:  partialMemLimitJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cpu-q"},"spec":{"hard":{"limits.cpu":"100"}}}]}`},
			wantWarns: []string{`ResourceQuota "cpu-q" in namespace "ml-ns" requires explicit limits.cpu, but container(s) (workload-container) omit it`},
		},
		{
			name:      "CPU-only request warns under requests.memory ResourceQuota without defaultRequest",
			namespace: "ml-ns",
			manifest:  cpuOnlyRequestJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"req-q"},"spec":{"hard":{"requests.memory":"500Gi"}}}]}`},
			wantWarns: []string{`ResourceQuota "req-q" in namespace "ml-ns" requires explicit requests.memory, but container(s) (workload-container) omit it`},
		},
		{
			name:      "CPU-only request passes legacy memory ResourceQuota when LimitRange sets defaultRequest.memory",
			namespace: "ml-ns",
			manifest:  cpuOnlyRequestJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"req-lr"},"spec":{"limits":[{"type":"Container","defaultRequest":{"memory":"4Gi"}}]}}]}`},
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"req-q"},"spec":{"hard":{"memory":"500Gi"}}}]}`},
		},
		{
			name:      "full-node CPU manifest with limits.cpu passes requests.cpu but warns on limits.memory quota",
			namespace: "ml-ns",
			manifest:  fullNodeCPUJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"fn-q"},"spec":{"hard":{"requests.cpu":"100","limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{`ResourceQuota "fn-q" in namespace "ml-ns" requires explicit limits.memory, but container(s) (workload-container) omit it`},
			forbidLog: "requests.cpu",
		},
		{
			name:      "initContainers omitting limits are inspected",
			namespace: "ml-ns",
			manifest:  initContainerUnboundedManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"mem-q"},"spec":{"hard":{"limits.memory":"100Gi"}}}]}`},
			wantWarns: []string{`container(s) (native-sidecar) omit it`},
		},
		{
			name:      "standalone batch/v1 Job manifest is inspected",
			namespace: "ml-ns",
			manifest:  standaloneBurstableJobManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"mem-lr"},"spec":{"limits":[{"type":"Container","default":{"memory":"8Gi"}}]}}]}`},
			rqRes:     emptyList,
			wantWarns: []string{`Container "workload-container" requests memory 16Gi, which exceeds LimitRange "mem-lr" default memory limit (8Gi) in namespace "ml-ns"`},
		},
		{
			name:      "duplicate container names across replicatedJobs are deduplicated",
			namespace: "ml-ns",
			manifest:  dupContainerNameJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"dup-q"},"spec":{"hard":{"limits.cpu":"100"}}}]}`},
			wantWarns: []string{`container(s) (workload-container) omit it`},
		},
		{
			name:      "cross-resource LimitRange default.cpu does not satisfy limits.memory ResourceQuota",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cpu-lr"},"spec":{"limits":[{"type":"Container","default":{"cpu":"16"}}]}}]}`},
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"ns-quota"},"spec":{"hard":{"limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{`ResourceQuota "ns-quota" in namespace "ml-ns" requires explicit limits.memory`},
		},
		{
			name:      "ResourceQuota limits.memory satisfied when LimitRange sets default.memory",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"mem-lr"},"spec":{"limits":[{"type":"Container","default":{"memory":"64Gi"}}]}}]}`},
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"ns-quota"},"spec":{"hard":{"limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{`will have bursting capped at 64Gi`},
			forbidLog: "ResourceQuota",
		},
		{
			name:      "ignored scoped ResourceQuotas (Terminating, BestEffort, mixed, PriorityClass) followed by unscoped still warn",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     emptyList,
			rqRes: shell.CommandResult{Stdout: `{"items":[
				{"metadata":{"name":"term-q"},"spec":{"scopes":["Terminating"],"hard":{"limits.memory":"100Gi"}}},
				{"metadata":{"name":"be-q"},"spec":{"scopes":["BestEffort"],"hard":{"limits.memory":"100Gi"}}},
				{"metadata":{"name":"mixed-q"},"spec":{"scopes":["NotBestEffort","Terminating"],"hard":{"limits.memory":"100Gi"}}},
				{"metadata":{"name":"pc-q"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"In","values":["high"]}]},"hard":{"limits.cpu":"10"}}},
				{"metadata":{"name":"all-q"},"spec":{"scopeSelector":{"matchExpressions":[]},"hard":{"limits.memory":"500Gi"}}}
			]}`},
			wantWarns: []string{`ResourceQuota "all-q" in namespace "ml-ns" requires explicit limits.memory`},
			forbidLog: `"term-q"`,
		},
		{
			name:      "ResourceQuota scopeSelector with Exists NotBestEffort and NotTerminating warns",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"sel-q"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"NotBestEffort","operator":"Exists"},{"scopeName":"NotTerminating","operator":"Exists"}]},"hard":{"limits.cpu":"100"}}}]}`},
			wantWarns: []string{`ResourceQuota "sel-q" in namespace "ml-ns" requires explicit limits.cpu`},
		},
		{
			name:      "pod with activeDeadlineSeconds skips NotTerminating quota and warns on Terminating quota",
			namespace: "ml-ns",
			manifest:  terminatingJobSetManifest,
			lrRes:     emptyList,
			rqRes: shell.CommandResult{Stdout: `{"items":[
				{"metadata":{"name":"not-term-q"},"spec":{"scopes":["NotTerminating"],"hard":{"limits.cpu":"100"}}},
				{"metadata":{"name":"term-q"},"spec":{"scopes":["Terminating"],"hard":{"limits.memory":"100Gi"}}}
			]}`},
			wantWarns: []string{`ResourceQuota "term-q" in namespace "ml-ns" requires explicit limits.memory`},
			forbidLog: "not-term-q",
		},
		{
			name:      "PriorityClass scopeSelector matches In and Exists and skips NotIn and DoesNotExist on matching pod",
			namespace: "ml-ns",
			manifest:  priorityClassBurstableJobSetManifest,
			lrRes:     emptyList,
			rqRes: shell.CommandResult{Stdout: `{"items":[
				{"metadata":{"name":"pc-notin"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"NotIn","values":["high"]}]},"hard":{"limits.cpu":"10"}}},
				{"metadata":{"name":"pc-dne"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"DoesNotExist"}]},"hard":{"limits.cpu":"10"}}},
				{"metadata":{"name":"pc-in"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"In","values":["high"]},{"scopeName":"PriorityClass","operator":"Exists"}]},"hard":{"limits.memory":"100Gi"}}}
			]}`},
			wantWarns: []string{`ResourceQuota "pc-in" in namespace "ml-ns" requires explicit limits.memory`},
			forbidLog: "pc-notin",
		},
		{
			name:      "PriorityClass DoesNotExist and BestEffort DoesNotExist warn on default-priority burstable pod",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     emptyList,
			rqRes: shell.CommandResult{Stdout: `{"items":[
				{"metadata":{"name":"pc-exists"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"Exists"}]},"hard":{"limits.cpu":"10"}}},
				{"metadata":{"name":"dne-q"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"DoesNotExist"},{"scopeName":"BestEffort","operator":"DoesNotExist"}]},"hard":{"limits.memory":"100Gi"}}}
			]}`},
			wantWarns: []string{`ResourceQuota "dne-q" in namespace "ml-ns" requires explicit limits.memory`},
			forbidLog: "pc-exists",
		},
		{
			name:      "LimitRange default.cpu promotes BestEffort worker to NotBestEffort under NotBestEffort limits.memory quota",
			namespace: "ml-ns",
			manifest:  mixedHeadAndBestEffortWorkerManifest,
			lrRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cpu-lr"},"spec":{"limits":[{"type":"Container","default":{"cpu":"16"}}]}}]}`},
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"nb-mem-q"},"spec":{"scopes":["NotBestEffort"],"hard":{"limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{`ResourceQuota "nb-mem-q" in namespace "ml-ns" requires explicit limits.memory, but container(s) (pathways-worker) omit it`},
		},
		{
			name:           "RBAC 403 on both policies warns for both and proceeds",
			namespace:      "ml-ns",
			manifest:       burstableJobSetManifest,
			lrRes:          forbidden("limitranges"),
			rqRes:          forbidden("resourcequotas"),
			wantWarns:      []string{`Could not read limitrange in namespace "ml-ns" (403 Forbidden)`, `Could not read resourcequota in namespace "ml-ns" (403 Forbidden)`},
			checkCallCount: true,
			wantCalls:      2,
		},
		{
			name:      "RBAC 403 on LimitRange with multi-resource ResourceQuota warns for both resources",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     forbidden("limitranges"),
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"ns-quota"},"spec":{"hard":{"limits.cpu":"100","limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{
				`ResourceQuota "ns-quota" in namespace "ml-ns" requires explicit limits.cpu`,
				`ResourceQuota "ns-quota" in namespace "ml-ns" requires explicit limits.memory`,
			},
		},
		{
			name:      "non-403 kubectl failure with empty stderr falls back to Err and warns",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{ExitCode: -1, Err: errors.New("exec timeout")},
			rqRes:     emptyList,
			wantWarns: []string{"exec timeout"},
		},
		{
			name:      "empty LimitRange JSON degrades to warning even with enforced ResourceQuota",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{ExitCode: 0, Stdout: ""},
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"ns-quota"},"spec":{"hard":{"limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{"Could not parse limitrange list", "LimitRange could not be inspected"},
		},
		{
			name:      "null JSON stdout is rejected as unparsable rather than empty list",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{ExitCode: 0, Stdout: "null\n"},
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"ns-quota"},"spec":{"hard":{"limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{"Could not parse limitrange list", "unexpected empty/null JSON payload", "LimitRange could not be inspected"},
		},
		{
			name:      "empty object or wrong-kind JSON stdout is rejected as unexpected payload",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     shell.CommandResult{ExitCode: 0, Stdout: "{}"},
			rqRes:     shell.CommandResult{ExitCode: 0, Stdout: `{"kind":"Status","status":"Failure","items":[]}`},
			wantWarns: []string{"Could not parse limitrange list", "Could not parse resourcequota list"},
		},
		{
			name:      "unscoped quota omits globalDefault PriorityClass caveat and requests quota includes --cpu/--memory hint",
			namespace: "ml-ns",
			manifest:  cpuOnlyRequestJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"req-q"},"spec":{"hard":{"requests.memory":"500Gi"}}}]}`},
			wantWarns: []string{"Pass both --cpu and --memory", "gcluster job gke-template-extract"},
			forbidLog: "globalDefault PriorityClass",
		},
		{
			name:      "PriorityClass DoesNotExist quota includes globalDefault PriorityClass caveat for empty priorityClassName",
			namespace: "ml-ns",
			manifest:  burstableJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"pc-dne"},"spec":{"scopeSelector":{"matchExpressions":[{"scopeName":"PriorityClass","operator":"DoesNotExist"}]},"hard":{"limits.memory":"100Gi"}}}]}`},
			wantWarns: []string{"globalDefault PriorityClass excludes this quota"},
		},
		{
			name:      "whitespace-only namespace omits -n flag and formats as unquoted current namespace",
			namespace: "  ",
			manifest:  burstableJobSetManifest,
			lrRes:     emptyList,
			rqRes:     shell.CommandResult{Stdout: `{"items":[{"metadata":{"name":"cur-q"},"spec":{"hard":{"limits.memory":"500Gi"}}}]}`},
			wantWarns: []string{`ResourceQuota "cur-q" in the current namespace requires explicit limits.memory`},
			forbidLog: `namespace "current"`,
		},
		{
			name:           "containers with explicit CPU and memory limits skip kubectl queries",
			namespace:      "ml-ns",
			manifest:       boundedJobSetManifest,
			lrRes:          emptyList,
			rqRes:          emptyList,
			checkCallCount: true,
			wantCalls:      0,
		},
		{
			name:           "limits-only GPU manifest without CPU/memory skips kubectl queries",
			namespace:      "ml-ns",
			manifest:       limitsOnlyJobSetManifest,
			lrRes:          emptyList,
			rqRes:          emptyList,
			checkCallCount: true,
			wantCalls:      0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ns := strings.TrimSpace(tc.namespace)
			nsArg := ""
			if ns != "" {
				nsArg = " -n " + ns
			}
			lrCmd := "kubectl get limitrange" + nsArg + " -o json " + kubectlReadTimeout
			rqCmd := "kubectl get resourcequota" + nsArg + " -o json " + kubectlReadTimeout
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				lrCmd: {tc.lrRes},
				rqCmd: {tc.rqRes},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.namespace = tc.namespace
			snapshot, restore := logging.CaptureForTesting(t)
			defer restore()

			orc.checkNamespaceResourcePolicies(tc.manifest)
			restore()
			_, gotLogs := snapshot()

			assertPolicyCheckOutcome(t, gotLogs, mockExec.calls, tc.wantWarns, tc.forbidLog, tc.checkCallCount, tc.wantCalls)
		})
	}
}

func assertPolicyCheckOutcome(t *testing.T, gotLogs string, calls []string, wantWarns []string, forbidLog string, checkCalls bool, wantCalls int) {
	t.Helper()
	if checkCalls && len(calls) != wantCalls {
		t.Errorf("kubectl calls = %d (%v), want %d", len(calls), calls, wantCalls)
	}
	if forbidLog != "" && strings.Contains(gotLogs, forbidLog) {
		t.Errorf("unexpected %q in logs:\n%s", forbidLog, gotLogs)
	}
	if len(wantWarns) == 0 {
		if gotLogs != "" {
			t.Errorf("expected no warnings, got logs:\n%s", gotLogs)
		}
		return
	}
	for _, w := range wantWarns {
		if !strings.Contains(gotLogs, w) {
			t.Errorf("expected warning containing %q, got logs:\n%s", w, gotLogs)
		}
	}
}

func TestApplyManifest_NamespacePolicyPreflightPathways(t *testing.T) {
	setupMockMachineConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	lrCmd := "kubectl get limitrange -n ml-ns -o json " + kubectlReadTimeout
	rqCmd := "kubectl get resourcequota -n ml-ns -o json " + kubectlReadTimeout

	job := orchestrator.JobDefinition{
		WorkloadName:              "pw-policy",
		CommandToRun:              "python3 train.py",
		ComputeType:               "v6e-8",
		MachineType:               "ct6e-standard-4t",
		Topology:                  "2x4",
		ClusterLocation:           "us-central1-a",
		NumSlices:                 1,
		NodesPerSlice:             2,
		IsPathwaysJob:             true,
		ImagePullSecretsSpecified: true,
		Pathways: orchestrator.PathwaysJobDefinition{
			GCSLocation:  "gs://bkt/pw",
			HeadNodePool: "cpu-np",
		},
	}
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		lrCmd: {{Stdout: `{"items":[]}`}, {Stdout: `{"items":[]}`}},
		rqCmd: {
			{Stdout: `{"items":[{"metadata":{"name":"unscoped-q"},"spec":{"hard":{"limits.memory":"100Gi"}}}]}`},
			{Stdout: `{"items":[{"metadata":{"name":"nb-q"},"spec":{"scopes":["NotBestEffort"],"hard":{"limits.memory":"100Gi"}}}]}`},
		},
		"kubectl apply": {{ExitCode: 0}, {ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.namespace = "ml-ns"
	orc.machineCapCache["ct6e-standard-4t:us-central1-a"] = MachineTypeCap{GuestCpus: 180, MemoryMb: 737280}
	manifest, err := orc.GeneratePathwaysManifest(job, "test-image:latest", JobProfile{CapacityCount: 4}, false, false)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest: %v", err)
	}

	snap1, restore1 := logging.CaptureForTesting(t)
	if err := orc.ApplyManifest(manifest, "", job.WorkloadName); err != nil {
		restore1()
		t.Fatalf("unscoped quota ApplyManifest() unexpected err: %v", err)
	}
	_, warn1 := snap1()
	restore1()
	if !strings.Contains(warn1, "pathways-proxy") || !strings.Contains(warn1, "pathways-worker") {
		t.Errorf("unscoped quota warnings = %q, want both pathways-proxy and pathways-worker", warn1)
	}

	snap2, restore2 := logging.CaptureForTesting(t)
	if err := orc.ApplyManifest(manifest, "", job.WorkloadName); err != nil {
		restore2()
		t.Fatalf("NotBestEffort quota ApplyManifest() unexpected err: %v", err)
	}
	_, warn2 := snap2()
	restore2()
	if !strings.Contains(warn2, "pathways-proxy") || strings.Contains(warn2, "pathways-worker") {
		t.Errorf("NotBestEffort quota warnings = %q, want head containers without BestEffort pathways-worker", warn2)
	}
	if applied := commandsWithPrefix(mockExec, "kubectl apply"); len(applied) != 2 {
		t.Errorf("expected kubectl apply to run after warning-only preflight, got %v", applied)
	}
}

func TestApplyManifest_NamespacePolicyPreflightCPUAndDryRun(t *testing.T) {
	setupMockMachineConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	lrCmd := "kubectl get limitrange -n ml-ns -o json " + kubectlReadTimeout
	rqCmd := "kubectl get resourcequota -n ml-ns -o json " + kubectlReadTimeout

	t.Run("rendered fractional CPU manifest warns on tight and burst-capping LimitRanges and proceeds to kubectl apply", func(t *testing.T) {
		opts := ManifestOptions{
			WorkloadName:    "cpu-policy",
			FullImageName:   "test-image:latest",
			CommandToRun:    "python3 app.py",
			ComputeType:     "n2d-standard-64",
			MachineType:     "n2d-standard-64",
			ClusterLocation: "us-central1-a",
			NumSlices:       1,
			NodesPerSlice:   1,
			CPURequest:      "4",
			MemoryRequest:   "16Gi",
		}
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			lrCmd: {
				{Stdout: `{"items":[{"metadata":{"name":"tight-lr"},"spec":{"limits":[{"type":"Container","default":{"memory":"8Gi"}}]}}]}`},
				{Stdout: `{"items":[{"metadata":{"name":"cap-lr"},"spec":{"limits":[{"type":"Container","default":{"memory":"64Gi"}}]}}]}`},
			},
			rqCmd:           {{Stdout: `{"items":[]}`}, {Stdout: `{"items":[]}`}},
			"kubectl apply": {{ExitCode: 0}, {ExitCode: 0}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.namespace = "ml-ns"
		manifest, err := orc.GenerateGKEManifest(opts, JobProfile{IsCPUMachine: true, CapacityCount: 64, MemoryMb: 262144})
		if err != nil {
			t.Fatalf("GenerateGKEManifest: %v", err)
		}

		snapTight, restoreTight := logging.CaptureForTesting(t)
		if err := orc.ApplyManifest(manifest, "", opts.WorkloadName); err != nil {
			restoreTight()
			t.Fatalf("tight LimitRange ApplyManifest() unexpected err: %v", err)
		}
		_, warnTight := snapTight()
		restoreTight()
		if !strings.Contains(warnTight, `LimitRange "tight-lr" default memory limit (8Gi)`) {
			t.Errorf("expected tight LimitRange warning in logs, got:\n%s", warnTight)
		}

		snapCap, restoreCap := logging.CaptureForTesting(t)
		if err := orc.ApplyManifest(manifest, "", opts.WorkloadName); err != nil {
			restoreCap()
			t.Fatalf("warn-only LimitRange ApplyManifest() unexpected err: %v", err)
		}
		_, warnCap := snapCap()
		restoreCap()
		if !strings.Contains(warnCap, "will have bursting capped at 64Gi") {
			t.Errorf("expected burst-cap warning in logs, got:\n%s", warnCap)
		}
		if applied := commandsWithPrefix(mockExec, "kubectl apply"); len(applied) != 2 {
			t.Errorf("expected 2 kubectl apply calls after warning-only preflight, got %v", applied)
		}
	})

	t.Run("dry-run skips cluster policy queries", func(t *testing.T) {
		mockExec := NewMockExecutor(nil)
		orc := newTestGKEOrchestrator(mockExec)
		orc.namespace = "ml-ns"
		outPath := filepath.Join(t.TempDir(), "out.yaml")
		if err := orc.ApplyManifest(burstableJobSetManifest, outPath, "policy-test"); err != nil {
			t.Fatalf("ApplyManifest(dry-run) unexpected err: %v", err)
		}
		if len(mockExec.calls) != 0 {
			t.Errorf("expected 0 kubectl calls on dry-run, got %v", mockExec.calls)
		}
	})
}
