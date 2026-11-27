// Copyright 2026 Google LLC
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

package clusterdirector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// ProdEndpoint is the production Cluster Director (Hypercompute Cluster)
	// API endpoint.
	ProdEndpoint = "hypercomputecluster.googleapis.com"

	// DefaultEndpoint is the endpoint every client uses. Only the production
	// API is supported.
	DefaultEndpoint = ProdEndpoint

	// DefaultAPIVersion is the API version exposing the imported clusters
	// (import existing resources) path.
	DefaultAPIVersion = "v1alpha"

	cloudPlatformScope    = "https://www.googleapis.com/auth/cloud-platform"
	defaultTimeout        = 60 * time.Second
	defaultMaxRetries     = 3
	defaultRetryBaseDelay = 1 * time.Second
	maxRetryDelay         = 30 * time.Second
)

// Client talks to the Cluster Director REST API.
type Client struct {
	httpClient     *http.Client
	endpoint       string
	apiVersion     string
	maxRetries     int
	retryBaseDelay time.Duration
	quotaProject   string
	userAgent      string
}

// Option customizes a Client.
type Option func(*Client)

// WithEndpoint overrides the API endpoint with a host or full URL. It exists
// so tests can point the client at a local fake server.
func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		if endpoint != "" {
			c.endpoint = endpoint
		}
	}
}

// WithHTTPClient injects an HTTP client. The caller is responsible for
// authentication; this is primarily useful in tests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithRetry overrides the maximum number of retry attempts and initial backoff
// delay for transient HTTP errors (429, 502, 503, 504) and transport errors.
func WithRetry(maxRetries int, baseDelay time.Duration) Option {
	return func(c *Client) {
		if maxRetries >= 0 {
			c.maxRetries = maxRetries
		}
		if baseDelay > 0 {
			c.retryBaseDelay = baseDelay
		}
	}
}

// WithQuotaProject sets the project that API calls are billed and
// quota-checked against (the `x-goog-user-project` header). Calls made with
// user credentials need it.
func WithQuotaProject(project string) Option {
	return func(c *Client) { c.quotaProject = project }
}

// WithUserAgent sets the User-Agent header sent with every API call.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) { c.userAgent = userAgent }
}

// NewClient returns a Client authenticated with Application Default
// Credentials, i.e. the credentials produced by
// `gcloud auth application-default login`.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	c := &Client{
		endpoint:       DefaultEndpoint,
		apiVersion:     DefaultAPIVersion,
		maxRetries:     defaultMaxRetries,
		retryBaseDelay: defaultRetryBaseDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		ts, err := google.DefaultTokenSource(ctx, cloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("could not find Application Default Credentials, run `gcloud auth application-default login`: %w", err)
		}
		c.httpClient = oauth2.NewClient(ctx, ts)
		c.httpClient.Timeout = defaultTimeout
	}
	return c, nil
}

// baseURL returns the collection URL for clusters in a location.
func (c *Client) baseURL(project, location string) string {
	return fmt.Sprintf("%s/%s/projects/%s/locations/%s/clusters",
		normalizeEndpoint(c.endpoint), c.apiVersion, url.PathEscape(project), url.PathEscape(location))
}

// normalizeEndpoint turns a bare host into an https URL.
func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimSuffix(endpoint, "/")
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "https://" + endpoint
}

// CreateCluster imports cluster under clusterID. It returns the long
// running operation started by the API.
func (c *Client) CreateCluster(ctx context.Context, project, location, clusterID string, cluster *Cluster) (*Operation, error) {
	body, err := json.Marshal(cluster)
	if err != nil {
		return nil, fmt.Errorf("could not encode cluster payload: %w", err)
	}
	u := fmt.Sprintf("%s?clusterId=%s", c.baseURL(project, location), url.QueryEscape(clusterID))

	op := &Operation{}
	if err := c.do(ctx, http.MethodPost, u, body, op); err != nil {
		return nil, err
	}
	return op, nil
}

// GetCluster reads an imported cluster.
func (c *Client) GetCluster(ctx context.Context, project, location, clusterID string) (*Cluster, error) {
	u := fmt.Sprintf("%s/%s", c.baseURL(project, location), url.PathEscape(clusterID))

	cluster := &Cluster{}
	if err := c.do(ctx, http.MethodGet, u, nil, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// UpdateResourceMask is the update mask covering every field this package
// writes: the cluster description, imported network, storage, compute sets,
// and ownership labels. Wildcards are not supported by the API, so the paths
// are listed explicitly.
const UpdateResourceMask = "description,network_resources,storage_resources,orchestrator,labels"

// UpdateCluster amends an imported cluster in place. mask selects which
// fields of cluster to write; an empty mask defaults to UpdateResourceMask.
//
// This is what keeps an imported cluster current when a deployment grows or
// shrinks: the cluster ID is stable, so the resource set is patched rather than
// the cluster being deleted and recreated.
func (c *Client) UpdateCluster(ctx context.Context, project, location, clusterID string, cluster *Cluster, mask string) (*Operation, error) {
	if mask == "" {
		mask = UpdateResourceMask
	}
	body, err := json.Marshal(cluster)
	if err != nil {
		return nil, fmt.Errorf("could not encode cluster payload: %w", err)
	}
	u := fmt.Sprintf("%s/%s?updateMask=%s",
		c.baseURL(project, location), url.PathEscape(clusterID), url.QueryEscape(mask))

	op := &Operation{}
	if err := c.do(ctx, http.MethodPatch, u, body, op); err != nil {
		return nil, err
	}
	return op, nil
}

// DeleteCluster removes an imported cluster from Cluster Director. Deleting an
// imported cluster removes only the Cluster Director record; it does not
// destroy the underlying GCP resources.
func (c *Client) DeleteCluster(ctx context.Context, project, location, clusterID string) (*Operation, error) {
	u := fmt.Sprintf("%s/%s", c.baseURL(project, location), url.PathEscape(clusterID))

	op := &Operation{}
	if err := c.do(ctx, http.MethodDelete, u, nil, op); err != nil {
		return nil, err
	}
	return op, nil
}

// ListNodes returns all compute nodes associated with a cluster in Cluster
// Director, following pagination tokens until all pages have been collected.
func (c *Client) ListNodes(ctx context.Context, project, location, clusterID string) ([]Node, error) {
	base := fmt.Sprintf("%s/%s/nodes", c.baseURL(project, location), url.PathEscape(clusterID))

	nodes := []Node{}
	pageToken := ""
	for {
		u := base
		if pageToken != "" {
			u = fmt.Sprintf("%s?pageToken=%s", base, url.QueryEscape(pageToken))
		}
		var page ListNodesResponse
		if err := c.do(ctx, http.MethodGet, u, nil, &page); err != nil {
			return nil, err
		}
		nodes = append(nodes, page.Nodes...)
		if page.NextPageToken == "" {
			return nodes, nil
		}
		pageToken = page.NextPageToken
	}
}

// GetOperation polls a long running operation by its resource name.
func (c *Client) GetOperation(ctx context.Context, name string) (*Operation, error) {
	u := fmt.Sprintf("%s/%s/%s", normalizeEndpoint(c.endpoint), c.apiVersion, strings.TrimPrefix(name, "/"))

	op := &Operation{}
	if err := c.do(ctx, http.MethodGet, u, nil, op); err != nil {
		return nil, err
	}
	return op, nil
}

// WaitForOperation polls op until it completes, the context is cancelled or
// timeout elapses. A nil or already completed operation is returned as is.
func (c *Client) WaitForOperation(ctx context.Context, op *Operation, pollInterval, timeout time.Duration) (*Operation, error) {
	if op == nil || op.Done || op.Name == "" {
		return op, operationError(op)
	}
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return op, waitContextError(op.Name, ctx.Err())
		case <-ticker.C:
			polled, err := c.GetOperation(ctx, op.Name)
			if err != nil {
				if ctx.Err() != nil {
					return op, waitContextError(op.Name, ctx.Err())
				}
				return op, err
			}
			op = polled
			if op.Done {
				return op, operationError(op)
			}
		}
	}
}

func waitContextError(opName string, err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("canceled waiting for operation %s: %w", opName, err)
	}
	return fmt.Errorf("timed out waiting for operation %s: %w", opName, err)
}

// OperationError is returned when a Cluster Director long-running operation
// completes with a server-side failure status.
type OperationError struct {
	Name   string
	Status *Status
}

// Error implements the error interface.
func (e *OperationError) Error() string {
	return fmt.Sprintf("operation %s failed: %s (code %d)", e.Name, e.Status.Message, e.Status.Code)
}

// operationError converts the error carried by a completed operation into a
// Go error.
func operationError(op *Operation) error {
	if op == nil || op.Error == nil {
		return nil
	}
	return &OperationError{Name: op.Name, Status: op.Error}
}

// isRetryableStatus reports whether an HTTP status code represents a transient
// condition (rate limit or gateway/backend unavailability) that is safe to
// retry with backoff.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// sleepWithContext pauses for d or returns early if ctx is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryBackoff returns baseDelay * 2^(attempt-1) capped at maxRetryDelay,
// clamping before doubling so large attempt counts cannot overflow int64.
func retryBackoff(baseDelay time.Duration, attempt int) time.Duration {
	if baseDelay >= maxRetryDelay {
		return maxRetryDelay
	}
	d := baseDelay
	for i := 1; i < attempt; i++ {
		if d > maxRetryDelay/2 {
			return maxRetryDelay
		}
		d *= 2
	}
	return d
}

// do executes an authenticated request and decodes a successful response into
// out. Transient transport errors and retryable HTTP statuses (429, 502, 503,
// 504) are retried with bounded exponential backoff. Non-2xx responses are
// returned as an *APIError.
func (c *Client) do(ctx context.Context, method, u string, body []byte, out any) error {
	maxRetries, baseDelay := c.retryPolicy()
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, retryBackoff(baseDelay, attempt)); err != nil {
				return fmt.Errorf("%s %s retry aborted (%w): %w", method, u, err, lastErr)
			}
		}
		payload, retryable, err := c.send(ctx, method, u, body)
		if err == nil {
			return decodeResponse(method, u, payload, out)
		}
		lastErr = err
		if !retryable || ctx.Err() != nil || attempt == maxRetries {
			return err
		}
	}
	return lastErr
}

// retryPolicy returns the client's retry count and base backoff delay, with
// invalid values replaced by safe defaults.
func (c *Client) retryPolicy() (int, time.Duration) {
	maxRetries := max(c.maxRetries, 0)
	baseDelay := c.retryBaseDelay
	if baseDelay <= 0 {
		baseDelay = defaultRetryBaseDelay
	}
	return maxRetries, baseDelay
}

// send makes one HTTP request and returns the response payload. On failure it
// also reports whether the request may be retried: transport and read errors
// and retryable HTTP statuses can be, malformed requests and other API errors
// can't.
func (c *Client) send(ctx context.Context, method, u string, body []byte) ([]byte, bool, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, false, fmt.Errorf("could not build %s request for %s: %w", method, u, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.quotaProject != "" {
		req.Header.Set("x-goog-user-project", c.quotaProject)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("%s %s failed: %w", method, u, err)
	}
	payload, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, true, fmt.Errorf("could not read response from %s %s: %w", method, u, readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, isRetryableStatus(resp.StatusCode), newAPIError(method, u, resp.StatusCode, payload)
	}
	return payload, false, nil
}

// decodeResponse unmarshals a successful response payload into out, if out is
// set and the payload isn't empty.
func decodeResponse(method, u string, payload []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("could not decode response from %s %s: %w", method, u, err)
	}
	return nil
}

// APIError is returned for non 2xx responses from the Cluster Director API.
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Status     *Status
	Body       string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Status != nil && e.Status.Message != "" {
		return fmt.Sprintf("%s %s: %d %s: %s", e.Method, e.URL, e.StatusCode, e.Status.Status, e.Status.Message)
	}
	return fmt.Sprintf("%s %s: %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// newAPIError parses an error response body into an *APIError.
func newAPIError(method, u string, code int, payload []byte) *APIError {
	e := &APIError{Method: method, URL: u, StatusCode: code, Body: strings.TrimSpace(string(payload))}
	var wrapper struct {
		Error *Status `json:"error"`
	}
	if err := json.Unmarshal(payload, &wrapper); err == nil {
		e.Status = wrapper.Error
	}
	return e
}

// IsAlreadyExists reports whether err indicates that the cluster is already
// imported, which makes importing idempotent.
func IsAlreadyExists(err error) bool {
	return hasStatus(err, http.StatusConflict, "ALREADY_EXISTS")
}

// IsNotFound reports whether err indicates that the cluster is not (or no
// longer) imported, which makes deletion idempotent.
func IsNotFound(err error) bool {
	return hasStatus(err, http.StatusNotFound, "NOT_FOUND")
}

func isDisabledAPIError(text string) bool {
	return strings.Contains(text, "SERVICE_DISABLED") ||
		strings.Contains(text, "has not been used in project") ||
		strings.Contains(text, "it is disabled")
}

func isQuotaProjectDenied(text string) bool {
	return strings.Contains(text, "USER_PROJECT_DENIED") ||
		strings.Contains(text, "serviceusage.services.use")
}

// AccessHint returns a short, actionable explanation when err shows that the
// caller cannot use the Cluster Director API in projectID: the API is not
// enabled, the credentials are missing or expired, the caller lacks quota
// project permission (`serviceusage.services.use`), or the caller/project
// lacks Cluster Director permission. It returns "" for any other error.
func AccessHint(err error, projectID string) string {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	text := apiErr.Body
	if apiErr.Status != nil {
		text += " " + apiErr.Status.Status + " " + apiErr.Status.Message
	}
	switch {
	case isDisabledAPIError(text):
		return fmt.Sprintf("The Cluster Director API is not enabled in project %q. Enable it with:\n  gcloud services enable %s --project=%s",
			projectID, ProdEndpoint, projectID)
	case isQuotaProjectDenied(text):
		return fmt.Sprintf("Your account lacks `serviceusage.services.use` on quota project %q. Grant the Service Usage Consumer role (`roles/serviceusage.serviceUsageConsumer`) on project %q.",
			projectID, projectID)
	case apiErr.StatusCode == http.StatusUnauthorized || (apiErr.Status != nil && apiErr.Status.Status == "UNAUTHENTICATED"):
		return "Your Application Default Credentials are missing or expired. Refresh them with:\n  gcloud auth application-default login"
	case apiErr.StatusCode == http.StatusForbidden || (apiErr.Status != nil && apiErr.Status.Status == "PERMISSION_DENIED"):
		return fmt.Sprintf("Permission denied in project %q. Check that your account can create and manage Cluster Director\n"+
			"clusters in this project, and that the project has access to importing existing clusters into Cluster Director.", projectID)
	}
	return ""
}

// hasStatus reports whether err is an *APIError with the given HTTP status
// code or canonical status string.
func hasStatus(err error, code int, status string) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	// The canonical status in the body is authoritative: HTTP 409 is also
	// used for ABORTED, and HTTP 404 for unknown locations or API versions.
	if apiErr.Status != nil && apiErr.Status.Status != "" {
		return apiErr.Status.Status == status
	}
	return apiErr.StatusCode == code
}
