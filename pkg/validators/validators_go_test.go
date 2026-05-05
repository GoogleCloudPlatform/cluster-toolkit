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

	. "gopkg.in/check.v1"
)

type ValidatorsGoSuite struct{}

var _ = Suite(&ValidatorsGoSuite{})

func (s *ValidatorsGoSuite) TestProjectError(c *C) {
	err := projectError("my-project")
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "my-project"), Equals, true)
}

func (s *ValidatorsGoSuite) TestHandleClientError(c *C) {
	err := errors.New("some other error")
	c.Assert(handleClientError(err), Equals, err)

	errCreds := errors.New("could not find default credentials")
	hErr := handleClientError(errCreds)
	c.Assert(hErr, NotNil)
	c.Assert(strings.Contains(hErr.Error(), credentialsHint), Equals, true)
}

func (s *ValidatorsGoSuite) TestValidatorError(c *C) {
	innerErr := errors.New("inner error")
	vErr := ValidatorError{Validator: "test_val", Err: innerErr}

	c.Assert(vErr.Unwrap(), Equals, innerErr)
	c.Assert(strings.Contains(vErr.Error(), "test_val"), Equals, true)
	c.Assert(strings.Contains(vErr.Error(), "inner error"), Equals, true)
}

func (s *ValidatorsGoSuite) TestExecute(c *C) {
	// ValidationIgnore should return nil immediately
	bp := config.Blueprint{
		ValidationLevel: config.ValidationIgnore,
	}
	c.Assert(Execute(bp), IsNil)

	// Validation without validators
	bp2 := config.Blueprint{
		ValidationLevel: config.ValidationWarning,
		Validators:      []config.Validator{},
	}
	c.Assert(Execute(bp2), IsNil)
}
