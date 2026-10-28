// Copyright 2026 Google LLC
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

package modulereader

import (
	. "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"
)

type MetadataSuite struct{}

var _ = Suite(&MetadataSuite{})

func (s *MetadataSuite) TestDay2IgnoredSettingsUnmarshal(c *C) {
	rawYAML := `
ghpc:
  day2_ignored_settings:
    - setting: labels
      attribute: node_config[0].resource_labels
      reason: "GCE 'resource_labels' is in lifecycle.ignore_changes."
      remediation: "gcloud container node-pools update {name} --labels <key=val,...>"
    - setting: version_prefix
      setting_aliases: [min_master_version]
      display_name: "control-plane version setting"
      attribute: min_master_version
      reason: "'min_master_version' is in lifecycle.ignore_changes."
      remediation: "gcloud container clusters upgrade <cluster-name> --master"
    - setting: metadata
      nested_key: ssh-keys
      display_name: "'ssh-keys' within 'metadata'"
      attribute: metadata["ssh-keys"]
      reason: "'ssh-keys' within metadata is ignored."
`
	var mtd Metadata
	err := yaml.Unmarshal([]byte(rawYAML), &mtd)
	c.Assert(err, IsNil)
	c.Assert(mtd.Ghpc.Day2IgnoredSettings, HasLen, 3)

	// First rule: simple setting with remediation
	r1 := mtd.Ghpc.Day2IgnoredSettings[0]
	c.Check(r1.SettingKey, Equals, "labels")
	c.Check(r1.Attribute, Equals, "node_config[0].resource_labels")
	c.Check(r1.Reason, Equals, "GCE 'resource_labels' is in lifecycle.ignore_changes.")
	c.Check(r1.Remediation, Equals, "gcloud container node-pools update {name} --labels <key=val,...>")
	c.Check(r1.SettingAliases, HasLen, 0)
	c.Check(r1.NestedKey, Equals, "")
	c.Check(r1.DisplayName, Equals, "")

	// Second rule: aliases and display_name
	r2 := mtd.Ghpc.Day2IgnoredSettings[1]
	c.Check(r2.SettingKey, Equals, "version_prefix")
	c.Check(r2.SettingAliases, DeepEquals, []string{"min_master_version"})
	c.Check(r2.DisplayName, Equals, "control-plane version setting")
	c.Check(r2.Attribute, Equals, "min_master_version")

	// Third rule: nested_key
	r3 := mtd.Ghpc.Day2IgnoredSettings[2]
	c.Check(r3.SettingKey, Equals, "metadata")
	c.Check(r3.NestedKey, Equals, "ssh-keys")
	c.Check(r3.DisplayName, Equals, "'ssh-keys' within 'metadata'")
	c.Check(r3.Attribute, Equals, "metadata[\"ssh-keys\"]")
}
