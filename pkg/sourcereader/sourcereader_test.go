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

package sourcereader

import (
	"embed"
	"testing"

	. "gopkg.in/check.v1"
)

const (
	pkrKindString = "packer"
	tfKindString  = "terraform"
)

//go:embed modules
var testEmbeddedFS embed.FS

type zeroSuite struct{}

var _ = Suite(&zeroSuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *zeroSuite) TestIsEmbeddedPath(c *C) {
	// True: Is an embedded path
	ret := IsEmbeddedPath("modules/anything/else")
	c.Assert(ret, Equals, true)

	// True: Windows backslash path
	ret = IsEmbeddedPath(`modules\anything\else`)
	c.Assert(ret, Equals, true)

	// False: Local path
	ret = IsEmbeddedPath("./modules/else")
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath(`.\modules\else`)
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath("./modules")
	c.Assert(ret, Equals, false)

	ret = IsEmbeddedPath("../modules/")
	c.Assert(ret, Equals, false)

	// False, other
	ret = IsEmbeddedPath("github.com/modules")
	c.Assert(ret, Equals, false)
}

func (s *zeroSuite) TestIsLocalPath(c *C) {
	// False: Embedded Path
	ret := IsLocalPath("modules/anything/else")
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`modules\anything\else`)
	c.Assert(ret, Equals, false)

	// True: Local path
	ret = IsLocalPath("./anything/else")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`.\anything\else`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath("./modules")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath("../modules/")
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`..\modules\`)
	c.Assert(ret, Equals, true)

	// True: Windows drive paths (absolute, volume-relative, and roots)
	ret = IsLocalPath(`C:\modules\network\vpc`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`C:/modules/network/vpc`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`C:modules\network\vpc`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`d:modules/network/vpc`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`C:`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`C:\`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`C:/`)
	c.Assert(ret, Equals, true)

	// True: Drive-relative local paths (classified as local due to structural ambiguity with single-character opaque URIs)
	ret = IsLocalPath(`C:manifest.json`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`s:manifest.json`)
	c.Assert(ret, Equals, true)

	// True: UNC network paths
	ret = IsLocalPath(`\\server\share\modules\vpc`)
	c.Assert(ret, Equals, true)

	ret = IsLocalPath(`//server/share/modules/vpc`)
	c.Assert(ret, Equals, true)

	// False: Single-character URL schemes and forced getters must NOT be treated as local drive paths
	ret = IsLocalPath(`s://bucket/modules/vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`S://bucket/modules/vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`c://bucket/modules/vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`C://bucket/modules/vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`s::https://example.com/modules/vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`s:\\bucket\modules\vpc`)
	c.Assert(ret, Equals, false)

	// False: Unnormalized multi-slash local paths are intentionally classified as remote/non-local
	// as an intentional design tradeoff to avoid collision with RFC 3986 hierarchical URIs.
	ret = IsLocalPath(`C://modules/network/vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`C:\\modules\network\vpc`)
	c.Assert(ret, Equals, false)

	ret = IsLocalPath(`C:\\\\modules\\network\\vpc`)
	c.Assert(ret, Equals, false)

	// False, other
	ret = IsLocalPath("github.com/modules")
	c.Assert(ret, Equals, false)
}

func (s *zeroSuite) TestIsRemotePath(c *C) {
	// False: Is an embedded path
	ret := IsRemotePath("modules/anything/else")
	c.Check(ret, Equals, false)

	// False: Local path
	ret = IsRemotePath("./anything/else")
	c.Check(ret, Equals, false)

	ret = IsRemotePath("./modules")
	c.Check(ret, Equals, false)

	ret = IsRemotePath("../modules/")
	c.Check(ret, Equals, false)

	// False: Drive-relative local paths (classified as local due to structural ambiguity with single-character opaque URIs)
	ret = IsRemotePath(`C:manifest.json`)
	c.Check(ret, Equals, false)

	ret = IsRemotePath(`s:manifest.json`)
	c.Check(ret, Equals, false)

	// True, other
	ret = IsRemotePath("github.com/modules")
	c.Check(ret, Equals, true)

	// True, genetic git repository
	ret = IsRemotePath("git::https://gitlab.com/modules")
	c.Check(ret, Equals, true)

	// True, single-character URL schemes and forced protocol getters
	ret = IsRemotePath("s://bucket/modules/vpc")
	c.Check(ret, Equals, true)

	ret = IsRemotePath("s::https://gitlab.com/modules")
	c.Check(ret, Equals, true)

	// True: Unnormalized multi-slash local paths are intentionally classified as remote
	// as an intentional design tradeoff to avoid collision with RFC 3986 hierarchical URIs.
	ret = IsRemotePath(`C://modules/network/vpc`)
	c.Check(ret, Equals, true)

	ret = IsRemotePath(`C:\\modules\network\vpc`)
	c.Check(ret, Equals, true)

	ret = IsRemotePath(`C:\\\\modules\\network\\vpc`)
	c.Check(ret, Equals, true)

	// True, invalid path though nor local nor embedded
	ret = IsRemotePath("wut:://modules")
	c.Check(ret, Equals, true)
}

func (s *zeroSuite) TestFactory(c *C) {
	c.Check(Factory("./modules/anything/else"), FitsTypeOf, LocalSourceReader{})            // Local
	c.Check(Factory("modules/anything/else"), FitsTypeOf, EmbeddedSourceReader{})           // Embedded
	c.Check(Factory("github.com/modules"), FitsTypeOf, GoGetterSourceReader{})              // GitHub
	c.Check(Factory("git::https://gitlab.com/modules"), FitsTypeOf, GoGetterSourceReader{}) // Git
	c.Check(Factory("s://bucket/modules/vpc"), FitsTypeOf, GoGetterSourceReader{})          // Single-character URL
	c.Check(Factory("s::https://gitlab.com/modules"), FitsTypeOf, GoGetterSourceReader{})   // Single-character forced getter
}
