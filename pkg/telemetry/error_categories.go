// Copyright 2026 "Google LLC"
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

package telemetry

import (
	"context"
	"os"
)

const (
	ErrTypePermissionDenied = "PermissionDenied"
	ErrTypeFileNotFound     = "FileNotFound"
	ErrTypeValidation       = "ValidationError"
	ErrTypeNetwork          = "NetworkError"
	ErrTypeTimeout          = "TimeoutError"
	ErrTypeCanceled         = "CanceledError"
	ErrTypeQuotaExceeded    = "QuotaExceeded"
	ErrTypeAuthentication   = "AuthenticationFailed"
	ErrTypeProvisioning     = "ProvisioningFailed"
	ErrTypeStockout         = "Stockout"
	ErrTypeAPIDisabled      = "APIDisabled"
	ErrTypeResourceExists   = "ResourceAlreadyExists"
	ErrTypeUnknown          = "Unknown"
)

var exactErrMatchers = []struct {
	target   error
	category string
}{
	{os.ErrPermission, ErrTypePermissionDenied},
	{os.ErrNotExist, ErrTypeFileNotFound},
	{context.DeadlineExceeded, ErrTypeTimeout},
	{context.Canceled, ErrTypeCanceled},
}

var substringErrMatchers = []struct {
	substring string
	category  string
}{
	{"quota exceeded", ErrTypeQuotaExceeded},
	{"limit exceeded", ErrTypeQuotaExceeded},
	{"unauthorized", ErrTypeAuthentication},
	{"not authenticated", ErrTypeAuthentication},
	{"requires authentication", ErrTypeAuthentication},
	{"permission denied", ErrTypePermissionDenied},
	{"403 forbidden", ErrTypePermissionDenied},
	{"access denied", ErrTypePermissionDenied},
	{"not found", ErrTypeFileNotFound},
	{"error 404", ErrTypeFileNotFound},
	{"validation failed", ErrTypeValidation},
	{"invalid argument", ErrTypeValidation},
	{"invalid value", ErrTypeValidation},
	{"instance is currently unavailable", ErrTypeStockout},
	{"sufficient capacity", ErrTypeStockout},
	{"enough resources available", ErrTypeStockout},
	{"resource pool exhausted", ErrTypeStockout},
	{"api is disabled", ErrTypeAPIDisabled},
	{"has not been used in project", ErrTypeAPIDisabled},
	{"enable the api", ErrTypeAPIDisabled},
	{"already exists", ErrTypeResourceExists},
	{"alreadyexists", ErrTypeResourceExists},
	{"failed to provision", ErrTypeProvisioning},
	{"deployment failed", ErrTypeProvisioning},
	{"timeout", ErrTypeTimeout},
	{"deadline", ErrTypeTimeout},
	{"connection refused", ErrTypeNetwork},
	{"dial tcp", ErrTypeNetwork},
	{"connection reset", ErrTypeNetwork},
}
