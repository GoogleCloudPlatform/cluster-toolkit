// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

var labelLoopRe = regexp.MustCompile(`%\{~?\s*for\s+[^}]*\bin\s+labels\b`)

// stubTemplateFuncs and sharedTemplateVars stub only the functions and variables
// the discovered templates use today. If a template adds a new one, add it here
// or in sharedTemplateVars; otherwise this test fails with
// "Call to unknown function" or "Unknown variable".
func stubTemplateFuncs() map[string]function.Function {
	return map[string]function.Function{
		"jsonencode": stdlib.JSONEncodeFunc,
		"length":     stdlib.LengthFunc,
		"try":        tryfunc.TryFunc,
		"indent": function.New(&function.Spec{
			Params: []function.Parameter{{Type: cty.Number}, {Type: cty.String}},
			Type:   function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
				return args[1], nil
			},
		}),
		"yamlencode": function.New(&function.Spec{
			Params: []function.Parameter{{Type: cty.DynamicPseudoType}},
			Type:   function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
				b, err := yaml.Marshal(args[0].AsString())
				if err != nil {
					return cty.NilVal, err
				}
				return cty.StringVal(string(b)), nil
			},
		}),
	}
}

func sharedTemplateVars(labels cty.Value) map[string]cty.Value {
	str, emptyList, nullStr := cty.StringVal("test"), cty.ListValEmpty(cty.EmptyObject), cty.NullVal(cty.String)
	return map[string]cty.Value{
		"labels": labels, "name": str, "suffix": cty.StringVal("-01"), "namespace": cty.StringVal("default"),
		"pv_name": str, "pvc_name": str, "capacity": cty.StringVal("100Gi"), "access_mode": cty.StringVal("ReadWriteOnce"),
		"storage_class_name": str, "volume_binding_mode": cty.StringVal("WaitForFirstConsumer"), "reclaim_policy": cty.StringVal("Delete"),
		"location": str, "project": str, "filestore_name": str, "share_name": str, "ip_address": str, "bucket_name": str,
		"instance_name": str, "server_ip": str, "filesystem_name": str, "completion_mode": cty.StringVal("Indexed"),
		"image": str, "restart_policy": cty.StringVal("Never"), "node_count": cty.NumberIntVal(1), "backoff_limit": cty.NumberIntVal(0),
		"command": cty.ListVal([]cty.Value{str}), "mount_options": cty.ListVal([]cty.Value{str}),
		"gcs_annotation": cty.False, "full_node_request": cty.False, "enable_confidential_storage": cty.False,
		"k8s_service_account_name": nullStr, "gpu_limit": nullStr, "cpu_request": nullStr, "tpu_limit": nullStr,
		"memory_request": nullStr, "ephemeral_request": nullStr, "disk_encryption_kms_key": nullStr, "disk_storage_pool": nullStr,
		"topology_zones": cty.NullVal(cty.List(cty.String)), "node_pool_names": cty.ListValEmpty(cty.String),
		"security_context": emptyList, "node_selectors": emptyList, "tolerations": emptyList, "resource_claims": emptyList,
		"claims": emptyList, "volume_mounts": emptyList, "empty_dir_volumes": emptyList, "ephemeral_pd_volumes": emptyList, "pvc_volumes": emptyList,
	}
}

func assertNoDuplicateYAMLKeys(t *testing.T, node *yaml.Node) {
	t.Helper()
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			k := node.Content[i].Value
			if _, dup := seen[k]; dup {
				t.Fatalf("duplicate YAML mapping key %q at line %d", k, node.Content[i].Line)
			}
			seen[k] = struct{}{}
			assertNoDuplicateYAMLKeys(t, node.Content[i+1])
		}
		return
	}
	for _, child := range node.Content {
		assertNoDuplicateYAMLKeys(t, child)
	}
}

func findK8sLabelTemplates(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, root := range []string{"modules", "community/modules"} {
		err := filepath.WalkDir(filepath.Join("..", filepath.FromSlash(root)), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if d.IsDir() || (!strings.HasSuffix(p, ".yaml.tftpl") && !strings.HasSuffix(p, ".yml.tftpl")) {
				return nil
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if bytes.Contains(src, []byte("apiVersion:")) && labelLoopRe.Match(src) {
				paths = append(paths, p)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return paths
}

func TestGKEManifestTemplateLabels(t *testing.T) {
	want := map[string]string{
		"ghpc_ai_assisted": "true", "ghpc_blueprint": "gke-a5x-bm", "bool_false": "false",
		"num_str": "123", "null_str": "null", "on": "off", "yes": "no", "no": "yes", "true": "false", "quoted": `a"b\c`,
	}
	ctyLabels := make(map[string]cty.Value, len(want))
	for k, v := range want {
		ctyLabels[k] = cty.StringVal(v)
	}
	ctx := &hcl.EvalContext{Variables: sharedTemplateVars(cty.MapVal(ctyLabels)), Functions: stubTemplateFuncs()}

	templates := findK8sLabelTemplates(t)
	if len(templates) < 15 {
		t.Fatalf("found %d Kubernetes templates looping over labels, want at least 15", len(templates))
	}
	for _, p := range templates {
		rel, err := filepath.Rel("..", p)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			expr, diags := hclsyntax.ParseTemplate(src, rel, hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatal(diags)
			}
			val, diags := expr.Value(ctx)
			if diags.HasErrors() {
				t.Fatal(diags)
			}
			rendered := val.AsString()
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(rendered), &node); err != nil {
				t.Fatalf("yaml.v3 unmarshal failed: %v\n%s", err, rendered)
			}
			assertNoDuplicateYAMLKeys(t, &node)

			var raw json.RawMessage
			if err := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(rendered), 4096).Decode(&raw); err != nil {
				t.Fatalf("k8s yaml decode failed: %v\n%s", err, rendered)
			}
			var obj struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
			}
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatalf("ObjectMeta unmarshal failed: %v\n%s", err, rendered)
			}
			if !maps.Equal(obj.Metadata.Labels, want) {
				t.Fatalf("labels mismatch:\ngot:  %#v\nwant: %#v\n%s", obj.Metadata.Labels, want, rendered)
			}
		})
	}
}
