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

package modulereader

import (
	. "gopkg.in/check.v1"
)

func (s *MySuite) TestNormalizeType(c *C) {
	c.Check(
		normalizeType("object({count=number,kind=string})"),
		Equals,
		normalizeType("object({kind=string,count=number})"))

	c.Check(normalizeType("?invalid_type"), Equals, "?invalid_type")

	// `any` is special type, check that it works
	c.Check(normalizeType("object({b=any,a=number})"), Equals, normalizeType("object({a=number,b=any})"))

	c.Check(normalizeType(" object (  {\na=any\n} ) "), Equals, normalizeType("object({a=any})"))

	c.Check(normalizeType(" string # comment"), Equals, normalizeType("string"))
}
