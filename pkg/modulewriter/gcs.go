/*
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package modulewriter writes modules to a deployment directory

package modulewriter

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"

	"cloud.google.com/go/storage"
)

// UploadArtifactsToBackend uploads the expanded blueprint to the GCS backend if configured.
func UploadArtifactsToBackend(bp config.Blueprint, deplDir string) error {
	if bp.TerraformBackendDefaults.Type != "gcs" {
		return nil
	}

	bucket, prefix := getBackendConfig(bp)
	if bucket == "" {
		logging.Error("GCS backend configured but 'bucket' not found. Skipping artifact upload.")
		return nil
	}

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer client.Close()

	// Upload expanded_blueprint.yaml
	srcPath := filepath.Join(ArtifactsDir(deplDir), ExpandedBlueprintName)
	dstPath := filepath.Join(prefix, bp.DeploymentName(), ArtifactsDirName, ExpandedBlueprintName)

	if err := uploadFile(ctx, client, bucket, srcPath, dstPath); err != nil {
		return fmt.Errorf("failed to upload expanded blueprint: %w", err)
	}

	logging.Info("Successfully uploaded %s to gs://%s/%s", ExpandedBlueprintName, bucket, dstPath)
	return nil
}

func getBackendConfig(bp config.Blueprint) (string, string) {
	config := bp.TerraformBackendDefaults.Configuration
	bucketVal := config.Get("bucket")
	if bucketVal.IsNull() || !bucketVal.Type().IsPrimitiveType() {
		return "", ""
	}
	bucket := bucketVal.AsString()

	prefix := ""
	prefixVal := config.Get("prefix")
	if !prefixVal.IsNull() && prefixVal.Type().IsPrimitiveType() {
		prefix = prefixVal.AsString()
	}

	return bucket, prefix
}

func uploadFile(ctx context.Context, client *storage.Client, bucket, src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	dst = strings.TrimPrefix(dst, "/")

	wc := client.Bucket(bucket).Object(dst).NewWriter(ctx)
	_, copyErr := io.Copy(wc, f)
	closeErr := wc.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
