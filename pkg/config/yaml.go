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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyJson "github.com/zclconf/go-cty/cty/json"
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
		return Blueprint{}, YamlCtx{}, fmt.Errorf("%s, filename=%s: %v", errMsgFileLoadError, f, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	yamlCtx, err := NewYamlCtx(data)
	if err != nil { // YAML parsing error
		return Blueprint{}, yamlCtx, err
	}

	var bp Blueprint
	if err = decoder.Decode(&bp); err != nil {
		return Blueprint{}, yamlCtx, parseYamlV3Error(err)
	}
	return bp, yamlCtx, nil
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
func NewYamlCtx(data []byte) (YamlCtx, error) {
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}

	var c nodeCapturer
	m := map[yPath]Pos{}

	// error may happen if YAML is not valid, regardless of Blueprint schema
	if err := yaml.Unmarshal(data, &c); err != nil {
		return YamlCtx{m, lines}, parseYamlV3Error(err)
	}

	var walk func(n *yaml.Node, p yPath, posOf *yaml.Node)
	walk = func(n *yaml.Node, p yPath, posOf *yaml.Node) {
		n = normalizeYamlNode(p, n)
		if posOf == nil { // use position of node itself if posOf is not set
			posOf = n
		}
		m[p] = Pos{posOf.Line, posOf.Column}

		if n.Kind == yaml.MappingNode {
			for i := 0; i < len(n.Content); i += 2 {
				// for mapping items use position of the key
				walk(n.Content[i+1], p.Dot(n.Content[i].Value), n.Content[i])
			}
		} else if n.Kind == yaml.SequenceNode {
			for i, c := range n.Content {
				walk(c, p.At(i), nil)
			}
		}
	}
	if c.n != nil {
		walk(c.n, "", nil)
	}
	return YamlCtx{m, lines}, nil
}

type nodeCapturer struct{ n *yaml.Node }

func nodeToPosErr(n *yaml.Node, err error) PosError {
	return PosError{Pos{Line: n.Line, Column: n.Column}, err}
}

func (c *nodeCapturer) UnmarshalYAML(n *yaml.Node) error {
	c.n = n
	return nil
}

// UnmarshalYAML implements a custom unmarshaler from YAML string to ModuleKind
func (mk *ModuleKind) UnmarshalYAML(n *yaml.Node) error {
	var kind string
	err := n.Decode(&kind)
	if err == nil && IsValidModuleKind(kind) {
		mk.kind = kind
		return nil
	}
	return nodeToPosErr(n, errors.New(`kind must be "packer" or "terraform" or removed from YAML`))
}

// MarshalYAML implements a custom marshaler from ModuleKind to YAML string
func (mk ModuleKind) MarshalYAML() (interface{}, error) {
	return mk.String(), nil
}

// UnmarshalYAML is a custom unmarshaler for Module.Use, that will print nice error message.
func (ms *ModuleIDs) UnmarshalYAML(n *yaml.Node) error {
	var ids []ModuleID
	if err := n.Decode(&ids); err != nil {
		return nodeToPosErr(n, errors.New("`use` must be a list of module ids"))
	}
	*ms = ids
	return nil
}

// YamlValue is wrapper around cty.Value to handle YAML unmarshal.
type YamlValue struct {
	v cty.Value // do not use this field directly, use Wrap() and Unwrap() instead
}

// Unwrap returns wrapped cty.Value.
func (y YamlValue) Unwrap() cty.Value {
	if y.v == cty.NilVal {
		// we can't use 0-value of cty.Value (NilVal)
		// instead it should be a proper null(any) value
		return cty.NullVal(cty.DynamicPseudoType)
	}
	return y.v
}

func (y *YamlValue) Wrap(v cty.Value) {
	y.v = v
}

// UnmarshalYAML implements custom YAML unmarshaling.
func (y *YamlValue) UnmarshalYAML(n *yaml.Node) error {
	var err error
	switch n.Kind {
	case yaml.ScalarNode:
		err = y.unmarshalScalar(n)
	case yaml.MappingNode:
		err = y.unmarshalObject(n)
	case yaml.SequenceNode:
		err = y.unmarshalTuple(n)
	default:
		err = nodeToPosErr(n, fmt.Errorf("cannot decode node with unknown kind %d", n.Kind))
	}
	return err
}

func (y *YamlValue) unmarshalScalar(n *yaml.Node) error {
	var s interface{}
	if err := n.Decode(&s); err != nil {
		return err
	}
	ty, err := gocty.ImpliedType(s)
	if err != nil {
		return nodeToPosErr(n, err)
	}
	v, err := gocty.ToCtyValue(s, ty)
	if err != nil {
		return err
	}
	y.Wrap(v)

	if l, is := IsYamlExpressionLiteral(y.Unwrap()); is { // HCL literal
		var e Expression
		if e, err = ParseExpression(l); err != nil {
			// TODO: point to exact location within expression, see Diagnostic.Subject
			return nodeToPosErr(n, err)
		}
		y.Wrap(e.AsValue())
	} else if y.Unwrap().Type() == cty.String && hasVariable(y.Unwrap().AsString()) { // "simple" variable
		e, err := SimpleVarToExpression(y.Unwrap().AsString())
		if err != nil {
			// TODO: point to exact location within expression, see Diagnostic.Subject
			return nodeToPosErr(n, err)
		}
		y.Wrap(e.AsValue())
	}
	return nil
}

func (y *YamlValue) unmarshalObject(n *yaml.Node) error {
	var my map[string]YamlValue
	if err := n.Decode(&my); err != nil {
		return err
	}
	mv := map[string]cty.Value{}
	for k, y := range my {
		mv[k] = y.Unwrap()
	}
	y.Wrap(cty.ObjectVal(mv))
	return nil
}

func (y *YamlValue) unmarshalTuple(n *yaml.Node) error {
	var ly []YamlValue
	if err := n.Decode(&ly); err != nil {
		return err
	}
	lv := []cty.Value{}
	for _, y := range ly {
		lv = append(lv, y.Unwrap())
	}
	y.Wrap(cty.TupleVal(lv))
	return nil
}

// UnmarshalYAML implements custom YAML unmarshaling.
func (d *Dict) UnmarshalYAML(n *yaml.Node) error {
	var v YamlValue
	if err := n.Decode(&v); err != nil {
		return err
	}
	ty := v.Unwrap().Type()
	if !ty.IsObjectType() {
		return nodeToPosErr(n, fmt.Errorf("must be a mapping, got %s", ty.FriendlyName()))
	}
	for k, w := range v.Unwrap().AsValueMap() {
		d.Set(k, w)
	}
	return nil
}

// MarshalYAML implements custom YAML marshaling.
func (d Dict) MarshalYAML() (interface{}, error) {
	o, _ := cty.Transform(d.AsObject(), func(p cty.Path, v cty.Value) (cty.Value, error) {
		if e, is := IsExpressionValue(v); is {
			s := string(hclwrite.Format(e.Tokenize().Bytes()))
			return cty.StringVal("((" + s + "))"), nil
		}
		return v, nil
	})

	j := ctyJson.SimpleJSONValue{Value: o}
	b, err := j.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}
	var g interface{}
	err = json.Unmarshal(b, &g)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	return g, nil
}

// yaml.v3 errors are either TypeError - collection of error message or single error message.
// Parse error messages to extract short error message and position.
func parseYamlV3Error(err error) error {
	errs := Errors{}
	switch err := err.(type) {
	case *yaml.TypeError:
		for _, s := range err.Errors {
			errs.Add(parseYamlV3ErrorString(s))
		}
	case PosError:
		errs.Add(err)
	default:
		errs.Add(parseYamlV3ErrorString(err.Error()))
	}

	if !errs.Any() { // should never happen
		errs.Add(parseYamlV3ErrorString(err.Error()))
	}
	return errs
}

// parseYamlV3Error attempts to extract position and nice error message from yaml.v3 error message.
// yaml.v3 errors are unstructured, use string parsing to extract information.
// If no position can be extracted, returns error without position.
// Else returns PosError{Pos{Line: line_number}, error_message}.
func parseYamlV3ErrorString(s string) error {
	match := regexp.MustCompile(`^(yaml: )?(line (\d+): )?(.*)$`).FindStringSubmatch(s)
	if match == nil {
		return errors.New(s)
	}
	lns, errMsg := match[3], match[4]
	ln, _ := strconv.Atoi(lns) // Atoi returns 0 on error, which is fine here
	return PosError{Pos{Line: ln}, errors.New(errMsg)}
}
