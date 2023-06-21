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

// yPath is a helper for YamlCtx to build "Path". It's agnostic to the Blueprint structure.
type yPath string

// At is a builder method for a path of a child in a sequence.
func (p yPath) At(i int) yPath {
	return yPath(fmt.Sprintf("%s[%d]", p, i))
}

// Dot is a builder method for a path of a child in a mapping.
func (p yPath) Dot(k string) yPath {
	if p == "" {
		return yPath(k)
	}
	return yPath(fmt.Sprintf("%s.%s", p, k))
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
	return bp, NewYamlCtx(data), nil
}

// YamlCtx is a contextual information to render errors.
type YamlCtx struct {
	pathToPos map[yPath]Pos
	Lines     []string
}

// Pos returns a position of a given path if one is found.
func (c YamlCtx) Pos(p Path) (Pos, bool) {
	pos, ok := c.pathToPos[yPath(p.String())]
	return pos, ok
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
func normalizeYamlNode(p yPath, n *yaml.Node) *yaml.Node {
	fmt.Printf("node: %#v, path: %#v", n, string(p))
	switch {
	case n.Kind == yaml.ScalarNode && regexp.MustCompile(`^deployment_groups\[\d+\]\.modules\[\d+\]\.outputs\[\d+\]$`).MatchString(string(p)):
		return syntheticOutputsNode(n.Value, n.Line, n.Column)
	default:
		return n
	}
}

// NewYamlCtx creates a new YamlCtx from a given YAML data.
// NOTE: The data should be a valid blueprint YAML (previously used to parse Blueprint),
// this function will panic if it's not valid YAML and doesn't validate Blueprint structure.
func NewYamlCtx(data []byte) YamlCtx {
	var c nodeCapturer
	if err := yaml.Unmarshal(data, &c); err != nil {
		panic(err) // shouldn't happen
	}
	if c.n == nil {
		return YamlCtx{} // empty
	}

	m := map[yPath]Pos{}
	var walk func(n *yaml.Node, p yPath)
	walk = func(n *yaml.Node, p yPath) {
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
	walk(c.n, "")

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
