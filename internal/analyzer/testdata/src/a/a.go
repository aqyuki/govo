package a

import "unsafe"

//govo:protect
type Code string // want Code:"&{}"

//govo:factory Code
func NewCode(s string) Code { return Code(s) }

//govo:converter Code
func (c Code) String() string { return string(c) }

const Local Code = "X"

//govo:protect
type Number int // want Number:"&{}"

const (
	First Number = iota
	Second
	Third
)

const raw = "X"

type Alias = Code

//govo:protect
type Pointer unsafe.Pointer // want "GOVO004"

//govo:protect
type Codes []Code // want "GOVO004"

//govo:factory Code
func NewOther(s string) Code { return Code(s) }

//govo:converter Code
func ConvertCode(c Code) string { return string(c) }

func local(c Code) bool {
	var wrong Code = "X" // want "GOVO001"
	var named Code = raw // want "GOVO001"
	_ = wrong
	_ = named
	return c == "X" // want "GOVO003"
}

//govo:protect
type Flag bool // want Flag:"&{}"

//govo:protect	extra // want "GOVO004"
type Tabbed string

func tabs(c Code) {
	//govo:ignore	GOVO003	// tab separated
	_ = c == "X"
}

//govo:protect
type hidden string // want hidden:"&{}"

func Hidden() hidden { return hidden("h") }

type Holder struct{ H hidden }

const Raw = "X"

func trusted(x, y int) Flag {
	_ = Flag(x == y)
	return x == y // want "GOVO001: untyped expression used as protected type Flag"
}
