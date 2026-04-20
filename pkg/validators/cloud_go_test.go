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

package validators

import (
	"errors"
	"hpc-toolkit/pkg/config"
	"strings"

	"google.golang.org/api/googleapi"
	. "gopkg.in/check.v1"
)

type CloudGoSuite struct{}

var _ = Suite(&CloudGoSuite{})

func (s *CloudGoSuite) TestGetErrorReason(c *C) {
	err := googleapi.Error{
		Details: []interface{}{
			"invalid type",
			map[string]interface{}{
				"reason": "MY_REASON",
				"metadata": map[string]interface{}{
					"key": "value",
				},
			},
		},
	}

	reason, metadata := getErrorReason(err)
	c.Assert(reason, Equals, "MY_REASON")
	c.Assert(metadata["key"], Equals, "value")

	errEmpty := googleapi.Error{}
	r, m := getErrorReason(errEmpty)
	c.Assert(r, Equals, "")
	c.Assert(m, IsNil)
}

func (s *CloudGoSuite) TestNewDisabledServiceError(c *C) {
	err := newDisabledServiceError("My Title", "my-name", "my-project")

	hErr, ok := err.(config.HintError)
	c.Assert(ok, Equals, true)
	c.Assert(strings.Contains(hErr.Hint, "My Title"), Equals, true)
	c.Assert(strings.Contains(hErr.Hint, "my-name"), Equals, true)
	c.Assert(strings.Contains(hErr.Err.Error(), "my-project"), Equals, true)
}

func (s *CloudGoSuite) TestHandleServiceUsageError(c *C) {
	c.Assert(handleServiceUsageError(nil, "my-project"), IsNil)

	err := errors.New("normal error")
	c.Assert(handleServiceUsageError(err, "my-project").Error(), Equals, "unhandled error: normal error")

	googleErr := &googleapi.Error{
		Details: []interface{}{
			map[string]interface{}{
				"reason":   "SERVICE_DISABLED",
				"metadata": map[string]interface{}{},
			},
		},
	}

	handledErr := handleServiceUsageError(googleErr, "my-project")
	c.Assert(strings.Contains(handledErr.Error(), "Service Usage API service is disabled"), Equals, true)

	googleErr2 := &googleapi.Error{
		Details: []interface{}{
			map[string]interface{}{
				"reason":   "USER_PROJECT_DENIED",
				"metadata": map[string]interface{}{},
			},
		},
	}
	handledErr2 := handleServiceUsageError(googleErr2, "my-project")
	c.Assert(strings.Contains(handledErr2.Error(), "my-project"), Equals, true) // projectError

	googleErr3 := &googleapi.Error{
		Details: []interface{}{
			map[string]interface{}{
				"reason":   "SU_MISSING_NAMES",
				"metadata": map[string]interface{}{},
			},
		},
	}
	handledErr3 := handleServiceUsageError(googleErr3, "my-project")
	c.Assert(handledErr3, IsNil)
}

func (s *CloudGoSuite) TestIsValidatorExplicit(c *C) {
	bp := config.Blueprint{
		Validators: []config.Validator{
			{Validator: "my-validator"},
		},
	}

	c.Assert(isValidatorExplicit(bp, "my-validator"), Equals, true)
	c.Assert(isValidatorExplicit(bp, "other-validator"), Equals, false)
}
