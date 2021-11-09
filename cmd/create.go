/*
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package cmd defines command line utilities for ghpc
package cmd

import (
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/reswriter"
	"log"

	"github.com/spf13/cobra"
)

func init() {
	createCmd.Flags().StringVarP(&yamlFilename, "config", "c", "",
		"Configuration file for the new blueprints")
	if err := createCmd.MarkFlagRequired("config"); err != nil {
		log.Fatalf("error while marking 'config' flag as required: %e", err)
	}
	rootCmd.AddCommand(createCmd)
}

var (
	yamlFilename string
	createCmd    = &cobra.Command{
		Use:   "create",
		Short: "Create a new blueprint.",
		Long:  "Create a new blueprint based on a provided YAML config.",
		Run:   runCreateCmd,
	}
)

func runCreateCmd(cmd *cobra.Command, args []string) {
	blueprintConfig := config.NewBlueprintConfig(yamlFilename)
	blueprintConfig.ExpandConfig()
	reswriter.WriteBlueprint(&blueprintConfig.Config)
}
