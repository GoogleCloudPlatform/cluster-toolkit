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

	c.Check(normalizeType("object({b=string,a=number})"), Equals, "object({a=number,b=string})")

	// `any` is special type, check that it works
	c.Check(normalizeType("object({b=any,a=number})"), Equals, "object({a=number,b=any})")

	c.Check(normalizeType(" object (  {\na=any\n} ) "), Equals, "object({a=any})")

	c.Check(normalizeType(" string # comment"), Equals, "string")
}
