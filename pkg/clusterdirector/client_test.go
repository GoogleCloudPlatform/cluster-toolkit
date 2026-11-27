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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// testClient returns a Client talking to srv without authentication.
func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(context.Background(),
		WithEndpoint(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(3, time.Millisecond))
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	return client
}

func TestCreateCluster(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotContentType string
	var gotBody Cluster

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("request body is not a valid cluster payload: %v", err)
		}
		_, _ = io.WriteString(w, `{"name": "projects/p/locations/us-central1/operations/op-1", "done": false}`)
	}))
	defer srv.Close()

	want := &Cluster{Name: "ctktest"}
	op, err := testClient(t, srv).CreateCluster(context.Background(), "p", "us-central1", "ctktest", want)
	if err != nil {
		t.Fatalf("CreateCluster() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("CreateCluster() method = %s, want %s", gotMethod, http.MethodPost)
	}
	if wantPath := "/v1alpha/projects/p/locations/us-central1/clusters"; gotPath != wantPath {
		t.Errorf("CreateCluster() path = %s, want %s", gotPath, wantPath)
	}
	if wantQuery := "clusterId=ctktest"; gotQuery != wantQuery {
		t.Errorf("CreateCluster() query = %s, want %s", gotQuery, wantQuery)
	}
	if gotContentType != "application/json" {
		t.Errorf("CreateCluster() Content-Type = %s, want application/json", gotContentType)
	}
	if diff := cmp.Diff(*want, gotBody); diff != "" {
		t.Errorf("CreateCluster() payload diff (-want +got):\n%s", diff)
	}
	if op.Name != "projects/p/locations/us-central1/operations/op-1" || op.Done {
		t.Errorf("CreateCluster() operation = %+v, want the pending operation op-1", op)
	}
}

func TestGetCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/v1alpha/projects/p/locations/us-central1/clusters/ctktest"; r.URL.Path != want {
			t.Errorf("GetCluster() path = %s, want %s", r.URL.Path, want)
		}
		_, _ = io.WriteString(w, `{"name": "projects/p/locations/us-central1/clusters/ctktest", "labels": {"env": "dev"}}`)
	}))
	defer srv.Close()

	got, err := testClient(t, srv).GetCluster(context.Background(), "p", "us-central1", "ctktest")
	if err != nil {
		t.Fatalf("GetCluster() returned an unexpected error: %v", err)
	}
	want := &Cluster{
		Name:   "projects/p/locations/us-central1/clusters/ctktest",
		Labels: map[string]string{"env": "dev"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetCluster() diff (-want +got):\n%s", diff)
	}
}

func TestUpdateCluster(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	var gotBody Cluster

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Errorf("request body is not a valid cluster payload: %v", err)
		}
		_, _ = io.WriteString(w, `{"name": "operations/op-3", "done": false}`)
	}))
	defer srv.Close()

	want := &Cluster{Name: "ctktest"}
	op, err := testClient(t, srv).UpdateCluster(context.Background(), "p", "us-central1", "ctktest", want, UpdateResourceMask)
	if err != nil {
		t.Fatalf("UpdateCluster() returned an unexpected error: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("UpdateCluster() method = %s, want %s", gotMethod, http.MethodPatch)
	}
	if wantPath := "/v1alpha/projects/p/locations/us-central1/clusters/ctktest"; gotPath != wantPath {
		t.Errorf("UpdateCluster() path = %s, want %s", gotPath, wantPath)
	}
	// The API rejects wildcards, so every path is listed explicitly.
	if wantQuery := "updateMask=description%2Cnetwork_resources%2Cstorage_resources%2Corchestrator%2Clabels"; gotQuery != wantQuery {
		t.Errorf("UpdateCluster() query = %s, want %s", gotQuery, wantQuery)
	}
	if diff := cmp.Diff(*want, gotBody); diff != "" {
		t.Errorf("UpdateCluster() payload diff (-want +got):\n%s", diff)
	}
	if op.Name != "operations/op-3" {
		t.Errorf("UpdateCluster() operation = %+v, want op-3", op)
	}
}

func TestUpdateClusterDefaultsTheMask(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"name": "operations/op-4", "done": true}`)
	}))
	defer srv.Close()

	if _, err := testClient(t, srv).UpdateCluster(
		context.Background(), "p", "us-central1", "ctktest", &Cluster{Name: "ctktest"}, ""); err != nil {
		t.Fatalf("UpdateCluster() returned an unexpected error: %v", err)
	}
	if wantQuery := "updateMask=description%2Cnetwork_resources%2Cstorage_resources%2Corchestrator%2Clabels"; gotQuery != wantQuery {
		t.Errorf("UpdateCluster() with an empty mask sent query = %s, want %s", gotQuery, wantQuery)
	}
}

func TestDeleteCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("DeleteCluster() method = %s, want %s", r.Method, http.MethodDelete)
		}
		_, _ = io.WriteString(w, `{"name": "operations/op-2", "done": true}`)
	}))
	defer srv.Close()

	op, err := testClient(t, srv).DeleteCluster(context.Background(), "p", "us-central1", "ctktest")
	if err != nil {
		t.Fatalf("DeleteCluster() returned an unexpected error: %v", err)
	}
	if !op.Done {
		t.Errorf("DeleteCluster() operation = %+v, want a completed operation", op)
	}
}

func TestListNodes(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if want := "/v1alpha/projects/p/locations/us-central1/clusters/ctkdemo/nodes"; r.URL.Path != want {
			t.Errorf("ListNodes() path = %s, want %s", r.URL.Path, want)
		}
		switch calls {
		case 1:
			if r.URL.RawQuery != "" {
				t.Errorf("first ListNodes() query = %q, want empty", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"nodes": [{"name": "projects/p/locations/us-central1/clusters/ctkdemo/nodes/ctkdemo-0", "zone": "us-central1-a", "state": "ACTIVE"}], "nextPageToken": "page-2"}`)
		case 2:
			if r.URL.RawQuery != "pageToken=page-2" {
				t.Errorf("second ListNodes() query = %q, want pageToken=page-2", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"nodes": [{"name": "projects/p/locations/us-central1/clusters/ctkdemo/nodes/ctkdemo-1", "zone": "us-central1-a", "state": "ACTIVE"}]}`)
		default:
			t.Fatalf("unexpected extra ListNodes() call #%d", calls)
		}
	}))
	defer srv.Close()

	got, err := testClient(t, srv).ListNodes(context.Background(), "p", "us-central1", "ctkdemo")
	if err != nil {
		t.Fatalf("ListNodes() returned an unexpected error: %v", err)
	}
	want := []Node{
		{Name: "projects/p/locations/us-central1/clusters/ctkdemo/nodes/ctkdemo-0"},
		{Name: "projects/p/locations/us-central1/clusters/ctkdemo/nodes/ctkdemo-1"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListNodes() diff (-want +got):\n%s", diff)
	}
}

func TestAPIErrors(t *testing.T) {
	tests := []struct {
		name            string
		code            int
		body            string
		wantAlreadyHave bool
		wantNotFound    bool
		wantMessage     string
	}{
		{
			name:            "already exists",
			code:            http.StatusConflict,
			body:            `{"error": {"code": 409, "message": "Cluster ctktest already exists.", "status": "ALREADY_EXISTS"}}`,
			wantAlreadyHave: true,
			wantMessage:     "already exists",
		},
		{
			name:         "not found",
			code:         http.StatusNotFound,
			body:         `{"error": {"code": 404, "message": "Cluster ctktest not found.", "status": "NOT_FOUND"}}`,
			wantNotFound: true,
			wantMessage:  "not found",
		},
		{
			name:        "permission denied",
			code:        http.StatusForbidden,
			body:        `{"error": {"code": 403, "message": "Permission denied.", "status": "PERMISSION_DENIED"}}`,
			wantMessage: "Permission denied",
		},
		{
			name:        "unparsable body",
			code:        http.StatusBadGateway,
			body:        `<html>bad gateway</html>`,
			wantMessage: "bad gateway",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			_, err := testClient(t, srv).CreateCluster(context.Background(), "p", "us-central1", "ctktest", &Cluster{})
			if err == nil {
				t.Fatalf("CreateCluster() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantMessage) {
				t.Errorf("CreateCluster() error = %q, want it to contain %q", err, tc.wantMessage)
			}
			if got := IsAlreadyExists(err); got != tc.wantAlreadyHave {
				t.Errorf("IsAlreadyExists(%v) = %t, want %t", err, got, tc.wantAlreadyHave)
			}
			if got := IsNotFound(err); got != tc.wantNotFound {
				t.Errorf("IsNotFound(%v) = %t, want %t", err, got, tc.wantNotFound)
			}
		})
	}
}

func TestWaitForOperation(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls++
		if want := "/v1alpha/operations/op-1"; r.URL.Path != want {
			t.Errorf("WaitForOperation() path = %s, want %s", r.URL.Path, want)
		}
		if polls < 2 {
			_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": false}`)
			return
		}
		_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": true}`)
	}))
	defer srv.Close()

	op, err := testClient(t, srv).WaitForOperation(context.Background(),
		&Operation{Name: "operations/op-1"}, time.Millisecond, time.Minute)
	if err != nil {
		t.Fatalf("WaitForOperation() returned an unexpected error: %v", err)
	}
	if !op.Done {
		t.Errorf("WaitForOperation() operation = %+v, want a completed operation", op)
	}
	if polls < 2 {
		t.Errorf("WaitForOperation() polled %d times, want at least 2", polls)
	}
}

func TestWaitForFailedOperation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": true, "error": {"code": 3, "message": "invalid subnetwork"}}`)
	}))
	defer srv.Close()

	_, err := testClient(t, srv).WaitForOperation(context.Background(),
		&Operation{Name: "operations/op-1"}, time.Millisecond, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "invalid subnetwork") {
		t.Errorf("WaitForOperation() error = %v, want it to report the operation failure", err)
	}
}

func TestWaitForOperationTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": false}`)
	}))
	defer srv.Close()

	_, err := testClient(t, srv).WaitForOperation(context.Background(),
		&Operation{Name: "operations/op-1"}, time.Millisecond, 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("WaitForOperation() error = %v, want a timeout", err)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hypercomputecluster.googleapis.com", "https://hypercomputecluster.googleapis.com"},
		{"https://hypercomputecluster.googleapis.com/", "https://hypercomputecluster.googleapis.com"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
	}
	for _, tc := range tests {
		if got := normalizeEndpoint(tc.in); got != tc.want {
			t.Errorf("normalizeEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDefaultEndpointIsProd(t *testing.T) {
	client, err := NewClient(context.Background(), WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	want := "https://" + ProdEndpoint + "/v1alpha/projects/p/locations/us-central1/clusters"
	if got := client.baseURL("p", "us-central1"); got != want {
		t.Errorf("baseURL() = %q, want %q", got, want)
	}
}

func TestWithEndpointOverridesHost(t *testing.T) {
	client, err := NewClient(context.Background(), WithEndpoint("http://127.0.0.1:8080"), WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	want := "http://127.0.0.1:8080/v1alpha/projects/p/locations/us-central1/clusters"
	if got := client.baseURL("p", "us-central1"); got != want {
		t.Errorf("baseURL() = %q, want %q", got, want)
	}
}

func TestRequestsCarryQuotaProjectAndUserAgent(t *testing.T) {
	var gotProject, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProject, gotUA = r.Header.Get("x-goog-user-project"), r.Header.Get("User-Agent")
		_, _ = io.WriteString(w, `{"name": "projects/p/locations/us-central1/clusters/c"}`)
	}))
	defer srv.Close()
	client, err := NewClient(context.Background(), WithEndpoint(srv.URL), WithHTTPClient(srv.Client()),
		WithQuotaProject("p"), WithUserAgent("cluster-toolkit/v1.2.3"))
	if err != nil {
		t.Fatalf("NewClient() returned an unexpected error: %v", err)
	}
	if _, err := client.GetCluster(context.Background(), "p", "us-central1", "c"); err != nil {
		t.Fatalf("GetCluster() returned an unexpected error: %v", err)
	}
	if gotProject != "p" || gotUA != "cluster-toolkit/v1.2.3" {
		t.Errorf("headers = (x-goog-user-project %q, User-Agent %q), want (%q, %q)", gotProject, gotUA, "p", "cluster-toolkit/v1.2.3")
	}
}

func TestStatusChecksPreferTheCanonicalStatus(t *testing.T) {
	aborted := newAPIError("POST", "u", http.StatusConflict, []byte(`{"error": {"code": 409, "status": "ABORTED", "message": "conflict"}}`))
	exists := newAPIError("POST", "u", http.StatusConflict, []byte(`{"error": {"code": 409, "status": "ALREADY_EXISTS", "message": "exists"}}`))
	badLocation := newAPIError("GET", "u", http.StatusNotFound, []byte(`{"error": {"code": 404, "status": "UNIMPLEMENTED", "message": "location not supported"}}`))
	notFound := newAPIError("GET", "u", http.StatusNotFound, []byte(`{"error": {"code": 404, "status": "NOT_FOUND", "message": "cluster not found"}}`))
	bare404 := newAPIError("GET", "u", http.StatusNotFound, []byte(`Not Found`))

	if IsAlreadyExists(aborted) {
		t.Error("IsAlreadyExists(409 ABORTED) = true, want false")
	}
	if !IsAlreadyExists(exists) {
		t.Error("IsAlreadyExists(409 ALREADY_EXISTS) = false, want true")
	}
	if IsNotFound(badLocation) {
		t.Error("IsNotFound(404 with another status) = true, want false")
	}
	if !IsNotFound(notFound) {
		t.Error("IsNotFound(404 NOT_FOUND) = false, want true")
	}
	if !IsNotFound(bare404) {
		t.Error("IsNotFound(404 without a status body) = false, want true")
	}
}

func TestWaitForOperationImmediateReturns(t *testing.T) {
	c := &Client{}
	if op, err := c.WaitForOperation(context.Background(), nil, 0, 0); op != nil || err != nil {
		t.Errorf("WaitForOperation(nil) = (%v, %v), want (nil, nil)", op, err)
	}
	doneOp := &Operation{Name: "operations/op-done", Done: true}
	if op, err := c.WaitForOperation(context.Background(), doneOp, 0, 0); op != doneOp || err != nil {
		t.Errorf("WaitForOperation(doneOp) = (%v, %v), want (%v, nil)", op, err, doneOp)
	}
	errOp := &Operation{Name: "operations/op-err", Done: true, Error: &Status{Code: 13, Message: "internal"}}
	if _, err := c.WaitForOperation(context.Background(), errOp, 0, 0); err == nil || !strings.Contains(err.Error(), "internal") {
		t.Errorf("WaitForOperation(errOp) error = %v, want internal error", err)
	}
}

func TestWaitForOperationPollError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error": {"code": 500, "message": "backend down", "status": "INTERNAL"}}`)
	}))
	defer srv.Close()

	_, err := testClient(t, srv).WaitForOperation(context.Background(),
		&Operation{Name: "operations/op-1"}, time.Millisecond, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "backend down") {
		t.Errorf("WaitForOperation() error = %v, want backend down", err)
	}
}

func TestClientMethodErrorsAndEmptyOrInvalidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1alpha/projects/p/locations/us-central1/clusters/empty":
			w.WriteHeader(http.StatusOK)
		case "/v1alpha/projects/p/locations/us-central1/clusters/badjson":
			_, _ = io.WriteString(w, `{not-json`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error": {"code": 400, "message": "bad request", "status": "INVALID_ARGUMENT"}}`)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	ctx := context.Background()

	if _, err := c.GetCluster(ctx, "p", "us-central1", "empty"); err != nil {
		t.Errorf("GetCluster(empty) returned unexpected error: %v", err)
	}
	if _, err := c.GetCluster(ctx, "p", "us-central1", "badjson"); err == nil || !strings.Contains(err.Error(), "could not decode") {
		t.Errorf("GetCluster(badjson) error = %v, want decode error", err)
	}
	if _, err := c.GetCluster(ctx, "p", "us-central1", "err"); err == nil {
		t.Error("GetCluster(err) succeeded, want error")
	}
	if _, err := c.UpdateCluster(ctx, "p", "us-central1", "err", &Cluster{}, ""); err == nil {
		t.Error("UpdateCluster(err) succeeded, want error")
	}
	if _, err := c.DeleteCluster(ctx, "p", "us-central1", "err"); err == nil {
		t.Error("DeleteCluster(err) succeeded, want error")
	}
	if _, err := c.ListNodes(ctx, "p", "us-central1", "err"); err == nil {
		t.Error("ListNodes(err) succeeded, want error")
	}
}

func TestHasStatusMatchesByStatusString(t *testing.T) {
	err := &APIError{StatusCode: http.StatusBadRequest, Status: &Status{Status: "ALREADY_EXISTS"}}
	if !IsAlreadyExists(err) {
		t.Error("IsAlreadyExists() = false for Status=ALREADY_EXISTS, want true")
	}
	if IsAlreadyExists(context.Canceled) {
		t.Error("IsAlreadyExists(context.Canceled) = true, want false")
	}
}

func TestClientRetriesTransientErrors(t *testing.T) {
	// Verify that CreateCluster replays the full JSON body across retries and
	// WaitForOperation survives transient 503/429 responses during polling.
	createCalls := 0
	pollCalls := 0
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1alpha/projects/p/locations/us-central1/clusters":
			createCalls++
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(b))
			if createCalls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"error": {"code": 503, "message": "service unavailable", "status": "UNAVAILABLE"}}`)
				return
			}
			if createCalls == 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, `{"error": {"code": 429, "message": "rate limited", "status": "RESOURCE_EXHAUSTED"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"name": "operations/op-retry", "done": false}`)
		case "/v1alpha/operations/op-retry":
			pollCalls++
			if pollCalls == 1 {
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = io.WriteString(w, `{"error": {"code": 504, "message": "gateway timeout", "status": "DEADLINE_EXCEEDED"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"name": "operations/op-retry", "done": true}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	ctx := context.Background()

	op, err := c.CreateCluster(ctx, "p", "us-central1", "ctktest", &Cluster{Name: "ctktest"})
	if err != nil {
		t.Fatalf("CreateCluster() returned unexpected error after transient failures: %v", err)
	}
	if createCalls != 3 {
		t.Errorf("CreateCluster() attempts = %d, want 3", createCalls)
	}
	for i, b := range bodies {
		if !strings.Contains(b, `"name":"ctktest"`) {
			t.Errorf("attempt %d request body = %q, want non-empty JSON payload", i+1, b)
		}
	}

	doneOp, err := c.WaitForOperation(ctx, op, time.Millisecond, time.Minute)
	if err != nil {
		t.Fatalf("WaitForOperation() returned unexpected error after transient 504: %v", err)
	}
	if !doneOp.Done || pollCalls != 2 {
		t.Errorf("WaitForOperation() done=%v pollCalls=%d, want true and 2", doneOp.Done, pollCalls)
	}
}

func TestClientDoesNotRetryPermanentErrors(t *testing.T) {
	codes := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusInternalServerError,
	}
	for _, code := range codes {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `{"error": {"message": "permanent failure"}}`)
		}))
		_, err := testClient(t, srv).GetCluster(context.Background(), "p", "us-central1", "ctktest")
		srv.Close()
		if err == nil {
			t.Fatalf("GetCluster() with status %d succeeded, want error", code)
		}
		if calls != 1 {
			t.Errorf("GetCluster() with status %d made %d attempts, want 1 (no retry)", code, calls)
		}
	}
}

func TestClientRetryExhaustionAndContextCancellation(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error": {"code": 503, "message": "still unavailable", "status": "UNAVAILABLE"}}`)
	}))
	defer srv.Close()

	// With maxRetries=2, expect 1 initial + 2 retries = 3 total attempts.
	client, err := NewClient(context.Background(),
		WithEndpoint(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(2, time.Millisecond))
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	_, err = client.GetCluster(context.Background(), "p", "us-central1", "ctktest")
	if err == nil || !strings.Contains(err.Error(), "still unavailable") {
		t.Fatalf("GetCluster() error = %v, want 503 still unavailable", err)
	}
	if calls != 3 {
		t.Errorf("GetCluster() attempts = %d, want 3 (1 initial + 2 retries)", calls)
	}

	// Verify context cancellation aborts retry backoff promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	slowClient, err := NewClient(ctx,
		WithEndpoint(srv.URL),
		WithHTTPClient(srv.Client()),
		WithRetry(3, time.Second))
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	_, err = slowClient.GetCluster(ctx, "p", "us-central1", "ctktest")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("GetCluster() with cancelled context error = %v, want context.DeadlineExceeded", err)
	}
}

func TestRetryBackoff(t *testing.T) {
	tests := []struct {
		base    time.Duration
		attempt int
		want    time.Duration
	}{
		{time.Second, 1, time.Second},
		{time.Second, 2, 2 * time.Second},
		{time.Second, 3, 4 * time.Second},
		{time.Second, 6, maxRetryDelay},
		{time.Second, 34, maxRetryDelay},  // bit-shift would overflow negative
		{time.Second, 100, maxRetryDelay}, // bit-shift >= 64 would wrap to 0
		{time.Minute, 1, maxRetryDelay},
	}
	for _, tc := range tests {
		if got := retryBackoff(tc.base, tc.attempt); got != tc.want {
			t.Errorf("retryBackoff(%v, %d) = %v, want %v", tc.base, tc.attempt, got, tc.want)
		}
	}
}

func TestWaitForOperationDistinguishesCancelAndOperationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"name": "operations/op-1", "done": false}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testClient(t, srv).WaitForOperation(ctx, &Operation{Name: "operations/op-1"}, time.Second, time.Minute)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "canceled waiting for operation") {
		t.Errorf("WaitForOperation(canceled) = %v, want canceled message wrapping context.Canceled", err)
	}

	_, opErr := testClient(t, srv).WaitForOperation(context.Background(), &Operation{
		Name:  "operations/op-2",
		Done:  true,
		Error: &Status{Code: 3, Message: "bad network", Status: "INVALID_ARGUMENT"},
	}, time.Millisecond, time.Second)
	var typed *OperationError
	if !errors.As(opErr, &typed) || typed.Name != "operations/op-2" {
		t.Errorf("WaitForOperation(failed op) = %T (%v), want *OperationError", opErr, opErr)
	}
}

func TestAccessHint(t *testing.T) {
	disabled := newAPIError("POST", "u", http.StatusForbidden, []byte(`{"error": {"code": 403, "status": "PERMISSION_DENIED",
		"message": "Cluster Director API has not been used in project p before or it is disabled.",
		"details": [{"reason": "SERVICE_DISABLED"}]}}`))
	quotaDenied := newAPIError("POST", "u", http.StatusForbidden, []byte(`{"error": {"code": 403, "status": "PERMISSION_DENIED",
		"message": "Caller does not have required permission to use project p. Grant the caller the roles/serviceusage.serviceUsageConsumer role, or a custom role with the serviceusage.services.use permission.",
		"details": [{"reason": "USER_PROJECT_DENIED"}]}}`))
	denied := newAPIError("POST", "u", http.StatusForbidden, []byte(`{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "denied"}}`))
	unauthenticated := newAPIError("POST", "u", http.StatusUnauthorized, []byte(`{"error": {"code": 401, "status": "UNAUTHENTICATED", "message": "bad token"}}`))
	invalid := newAPIError("POST", "u", http.StatusBadRequest, []byte(`{"error": {"code": 400, "status": "INVALID_ARGUMENT", "message": "bad"}}`))

	tests := []struct {
		name string
		err  error
		want string // substring of the hint; "" means no hint
	}{
		{"API disabled", disabled, "gcloud services enable hypercomputecluster.googleapis.com --project=p"},
		{"wrapped API disabled", fmt.Errorf("wrapped: %w", disabled), "gcloud services enable"},
		{"quota project denied", quotaDenied, "roles/serviceusage.serviceUsageConsumer"},
		{"permission denied", denied, "Permission denied in project \"p\""},
		{"unauthenticated", unauthenticated, "gcloud auth application-default login"},
		{"invalid argument", invalid, ""},
		{"not an API error", errors.New("boom"), ""},
		{"nil", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AccessHint(tc.err, "p")
			if tc.want == "" {
				if got != "" {
					t.Errorf("AccessHint() = %q, want no hint", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("AccessHint() = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
