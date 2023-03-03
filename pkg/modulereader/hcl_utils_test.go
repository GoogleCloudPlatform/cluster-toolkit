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
