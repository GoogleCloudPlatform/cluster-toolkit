// Copyright 2023 Google LLC
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

package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Path points to concrete location in the blueprint file.
type Path struct {
	v string
}

func (p Path) String() string {
	return p.v
}

// At is a builder method for a path of a child in a sequence.
func (p Path) At(i int) Path {
	return Path{fmt.Sprintf("%s[%d]", p.v, i)}
}

// Dot is a builder method for a path of a child in a mapping.
func (p Path) Dot(k string) Path {
	if p.v == "" {
		return Path{k}
	}
	return Path{fmt.Sprintf("%s.%s", p.v, k)}
}

// Pos is a position in the blueprint file.
type Pos struct {
	Line   int
	Column int
}

func importBlueprint(f string) (Blueprint, YamlCtx, error) {
	data, err := os.ReadFile(f)
	if err != nil {
		return Blueprint{}, YamlCtx{}, fmt.Errorf("%s, filename=%s: %v", errorMessages["fileLoadError"], f, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var bp Blueprint
	if err = decoder.Decode(&bp); err != nil {
		return Blueprint{}, YamlCtx{}, fmt.Errorf(errorMessages["yamlUnmarshalError"], f, err)
	}
	return bp, newYamlCtx(data), nil
}

// YamlCtx is a contextual information to render errors.
type YamlCtx struct {
	PathToPos map[Path]Pos
	Lines     []string
}

func syntheticOutputsNode(name string, ln int, col int) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{
				Kind:   yaml.ScalarNode,
				Value:  "name",
				Line:   ln,
				Column: col,
			},
			{
				Kind:   yaml.ScalarNode,
				Value:  name,
				Line:   ln,
				Column: col,
			},
		},
		Line:   ln,
		Column: col,
	}
}

// normalizeNode is treating variadic YAML syntax, ensuring that
// there is only one (canonical) way to refer to a piece of blueprint.
// Handled cases:
// * Module.outputs:
// ```
// outputs:
// - name: grog  # canonical path to "grog" value is `...outputs[0].name`
// - mork		 # canonical path to "mork" value is `...outputs[1].name`, NOT `...outputs[1]`
// ```
func normalizeYamlNode(p Path, n *yaml.Node) *yaml.Node {
	switch {
	case n.Kind == yaml.ScalarNode && regexp.MustCompile(`^deployment_groups\[\d+\]\.modules\[\d+\]\.outputs\[\d+\]$`).MatchString(p.String()):
		return syntheticOutputsNode(n.Value, n.Line, n.Column)
	default:
		return n
	}
}

func newYamlCtx(data []byte) YamlCtx {
	var c nodeCapturer
	if err := yaml.Unmarshal(data, &c); err != nil {
		panic(err) // shouldn't happen
	}

	m := map[Path]Pos{}
	var walk func(n *yaml.Node, p Path)
	walk = func(n *yaml.Node, p Path) {
		n = normalizeYamlNode(p, n)
		m[p] = Pos{n.Line, n.Column}
		if n.Kind == yaml.MappingNode {
			for i := 0; i < len(n.Content); i += 2 {
				walk(n.Content[i+1], p.Dot(n.Content[i].Value))
			}
		} else if n.Kind == yaml.SequenceNode {
			for i, c := range n.Content {
				walk(c, p.At(i))
			}
		}
	}
	walk(c.n, Path{""})

	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return YamlCtx{m, lines}
}

type nodeCapturer struct{ n *yaml.Node }

func (c *nodeCapturer) UnmarshalYAML(n *yaml.Node) error {
	c.n = n
	return nil
}
