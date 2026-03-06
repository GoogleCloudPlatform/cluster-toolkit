/*
Copyright 2026 Google LLC

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

package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
)

const (
	defaultModel  = "gemini-1.5-pro"
	defaultRegion = "us-central1"
)

type Client struct {
	projectID string
	region    string
	model     string
	verbose   bool
}

func NewClient(verbose bool, region, model string) *Client {
	if region == "" {
		region = defaultRegion
	}
	if model == "" {
		model = "gemini-2.0-flash-001"
	}
	return &Client{
		region:  region,
		model:   model,
		verbose: verbose,
	}
}

func (c *Client) GenerateFix(content string, failure Failure) (string, error) {
	if c.projectID == "" {
		if err := c.initProjectID(); err != nil {
			return "", err
		}
	}

	token, err := c.getAccessToken()
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`You are an expert software engineer.
The following file failed pre-commit hook '%s'.
Error message: '%s'.
File content:
%s

Please provide the corrected file content. Do not provide any markdown formatting, just the raw code. 
If the file is a Go file, ensure it compiles and follows gofmt.
If the file is a Terraform file, ensure it follows terraform fmt.
Focus your fix on line %d and its immediate context. PRESERVE all other content exactly as is.
Return ONLY the full file content. Do NOT truncate. Do NOT use placeholders.`, failure.Hook, failure.Message, content, failure.Line)

	resp, err := c.callVertexAI(token, prompt)
	if err != nil {
		return "", err
	}

	return cleanResponse(resp), nil
}

func (c *Client) initProjectID() error {
	cmd := exec.Command("gcloud", "config", "get-value", "project")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get project ID: %w", err)
	}
	c.projectID = strings.TrimSpace(string(output))
	return nil
}

func (c *Client) getAccessToken() (string, error) {
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) callVertexAI(token, prompt string) (string, error) {
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent", c.region, c.projectID, c.region, c.model)

	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.2,
			"topP":        0.8,
			"topK":        40,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	if c.verbose {
		fmt.Printf("DEBUG: Calling Vertex AI URL: %s\n", url)
		fmt.Printf("DEBUG: Project ID: %s\n", c.projectID)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-user-project", c.projectID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var bodyBytes []byte
		if resp.Body != nil {
			bodyBytes, _ = io.ReadAll(resp.Body)
		}
		return "", fmt.Errorf("Vertex AI API returned status: %s. Body: %s", resp.Status, string(bodyBytes))
	}

	return parseVertexResponse(resp.Body)
}

func parseVertexResponse(body io.Reader) (string, error) {
	var parsedResp map[string]interface{}
	if err := json.NewDecoder(body).Decode(&parsedResp); err != nil {
		return "", err
	}

	candidates, ok := parsedResp["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates returned from AI")
	}

	candidate := candidates[0].(map[string]interface{})
	contentParts, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response structure")
	}

	parts, ok := contentParts["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("no content parts returned")
	}

	textPart, ok := parts[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected part structure")
	}

	text, ok := textPart["text"].(string)
	if !ok {
		return "", fmt.Errorf("text not found in response")
	}

	return text, nil
}

func cleanResponse(text string) string {
	if strings.Contains(text, "```") {
		var result []string
		lines := strings.Split(text, "\n")
		inCodeBlock := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if inCodeBlock {
				result = append(result, line)
			}
		}
		if len(result) > 0 {
			return strings.Join(result, "\n")
		}
	}

	return strings.TrimSpace(text)
}
