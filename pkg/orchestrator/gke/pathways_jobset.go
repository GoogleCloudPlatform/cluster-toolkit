// Copyright 2024 Google LLC
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

const pathwaysJobSetTemplate = `
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: {{.WorkloadName}}
  labels:
    gcluster.google.com/workload: {{.WorkloadName}}
    kueue.x-k8s.io/queue-name: {{.KueueQueueName}}
spec:
  failurePolicy:
    maxRestarts: {{.MaxRestarts}}
  replicatedJobs:
  - name: pathways-head
    replicas: 1
    template:
      spec:
        parallelism: 1
        completions: 1
        backoffLimit: 0
        template:
          metadata:
            labels:
              cloud.google.com/gke-nodepool: cpu-np
{{- if .GCSFuseEnabled }}
            annotations:
              gke-gcsfuse/volumes: "true"
{{- end }}
          spec:
            restartPolicy: Never
            containers:
            - name: pathways-proxy
              image: {{.Pathways.ProxyServerImage}}
              args:
              - --gcs_location={{.Pathways.GCSLocation}}
              - --cluster_name={{.ClusterName}}
              - --project_id={{.ProjectID}}
              {{- if .Pathways.ProxyArgs}}
              - {{.Pathways.ProxyArgs}}
              {{- end}}
            - name: pathways-rm
              image: {{.Pathways.ServerImage}}
              args:
              {{- if .Pathways.ServerArgs}}
              - {{.Pathways.ServerArgs}}
              {{- end}}
            {{- if not .Pathways.Headless}}
            - name: workload-container
              image: {{.FullImageName}}
              command:
              - "/bin/bash"
              - "-c"
              - |
                {{.CommandToRun}}
              volumeMounts:
              - name: dshm
                mountPath: /dev/shm
{{.VolumeMountsYAML}}
            {{- end}}
            {{- if .Pathways.ColocatedPythonSidecarImage}}
            - name: python-sidecar
              image: {{.Pathways.ColocatedPythonSidecarImage}}
            {{- end}}
            volumes:
            - name: dshm
              emptyDir:
                medium: Memory
{{.VolumesYAML}}
  - name: worker
    replicas: {{.NumSlices}}
    template:
      spec:
        parallelism: {{.VmsPerSlice}}
        completions: {{.VmsPerSlice}}
        backoffLimit: 0
        podFailurePolicy:
          rules:
          - action: FailJob
            onExitCodes:
              containerName: "workload-container"
              operator: In
              values: [1]
        template:
          metadata:
            labels:
              gcluster.google.com/workload: {{.WorkloadName}}
            annotations:
              gke-gcsfuse/volumes: "true"
          spec:
            restartPolicy: Never
            containers:
            - name: pathways-worker
              image: {{.Pathways.WorkerImage}}
              args:
                - --cluster_name={{.ClusterName}}
                - --project_id={{.ProjectID}}
                {{- if .Pathways.WorkerArgs}}
                - {{.Pathways.WorkerArgs}}
                {{- end}}
              volumeMounts:
              - name: dshm
                mountPath: /dev/shm
            - name: workload-container
              image: {{.FullImageName}}
              command:
              - "/bin/bash"
              - "-c"
              - |
                {{.CommandToRun}}
              volumeMounts:
              - name: dshm
                mountPath: /dev/shm
{{.VolumeMountsYAML}}
            volumes:
            - name: dshm
              emptyDir:
                medium: Memory
{{.VolumesYAML}}
`
