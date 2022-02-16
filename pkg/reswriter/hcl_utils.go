// Copyright 2021 Google LLC
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

package reswriter

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"

	"hpc-toolkit/pkg/config"
)

// interfaceStruct is a struct wrapper for converting interface data structures
// to yaml flow style: one line wrapped in {} for maps and [] for lists.
type interfaceStruct struct {
	Elem interface{} `yaml:",flow"`
}

func getType(obj interface{}) string {
	// This does not handle variables with arbitrary types
	str, ok := obj.(string)
	if !ok { // We received a nil value.
		return "null"
	}
	if strings.HasPrefix(str, "{") {
		return "map"
	}
	if strings.HasPrefix(str, "[") {
		return "list"
	}
	return "string"
}

func isLiteralVariable(str string) bool {
	match, err := regexp.MatchString(beginLiteralExp, str)
	if err != nil {
		log.Fatalf("Failed checking if variable is a literal: %v", err)
	}
	return match
}

func handleLiteralVariable(str string) string {
	re := regexp.MustCompile(fullLiteralExp)
	contents := re.FindStringSubmatch(str)
	if len(contents) != 2 {
		log.Fatalf("Incorrectly formatted literal variable: %s", str)
	}

	return contents[1]
}

func handleData(val interface{}) interface{} {
	str, ok := val.(string)
	if !ok {
		// We only need to act on strings
		return val
	}
	if isLiteralVariable(str) {
		return handleLiteralVariable(str)
	} else if !strings.HasPrefix(str, "[") &&
		!strings.HasPrefix(str, "{") {
		return fmt.Sprintf("\"%s\"", str)
	}
	return str
}

func updateStringsInInterface(value interface{}) (interface{}, error) {
	var err error
	switch typedValue := value.(type) {
	case []interface{}:
		for i := 0; i < len(typedValue); i++ {
			typedValue[i], err = updateStringsInInterface(typedValue[i])
			if err != nil {
				break
			}
		}
		return typedValue, err
	case map[string]interface{}:
		retMap := map[string]interface{}{}
		for k, v := range typedValue {
			retMap[handleData(k).(string)], err = updateStringsInInterface(v)
			if err != nil {
				break
			}
		}
		return retMap, err
	default:
		return handleData(value), err
	}
}

func updateStringsInMap(interfaceMap map[string]interface{}) error {
	var err error
	for key, value := range interfaceMap {
		interfaceMap[key], err = updateStringsInInterface(value)
		if err != nil {
			break
		}
	}
	return err
}

func updateStringsInConfig(yamlConfig *config.YamlConfig, kind string) {
	for iGrp, grp := range yamlConfig.ResourceGroups {
		for iRes := 0; iRes < len(grp.Resources); iRes++ {
			if grp.Resources[iRes].Kind != kind {
				continue
			}
			err := updateStringsInMap(
				yamlConfig.ResourceGroups[iGrp].Resources[iRes].Settings)
			if err != nil {
				log.Fatalf("updateStringsInConfig: %v", err)
			}
		}
	}
}

func convertToYaml(wrappedInterface *interfaceStruct) (string, error) {
	by, err := yaml.Marshal(wrappedInterface)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(
		strings.ReplaceAll(string(by[6:]), "'", ""), "\n"), err
}

func flattenInterfaceMap(
	interfaceMap map[string]interface{}, wrapper *interfaceStruct) error {
	for k, v := range interfaceMap {
		wrapper.Elem = v
		yamlStr, err := convertToYaml(wrapper)
		if err != nil {
			return err
		}
		interfaceMap[k] = yamlStr
	}
	return nil
}

func flattenToHCLStrings(yamlConfig *config.YamlConfig, kind string) {
	wrapper := interfaceStruct{Elem: nil}
	for iGrp, grp := range yamlConfig.ResourceGroups {
		for iRes := 0; iRes < len(grp.Resources); iRes++ {
			if grp.Resources[iRes].Kind != kind {
				continue
			}
			err := flattenInterfaceMap(
				yamlConfig.ResourceGroups[iGrp].Resources[iRes].Settings, &wrapper)
			if err != nil {
				log.Fatalf("flattenToHCLStrings: %v", err)
			}
		}
	}
}
