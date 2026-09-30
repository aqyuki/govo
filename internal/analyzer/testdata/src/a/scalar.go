package a

//govo:protect
type Amount uint // want Amount:"&{}"

//govo:protect
type Rate float64 // want Rate:"&{}"

//govo:protect
type Tag string // want Tag:"&{}"

const perYen = 100

//govo:factory Amount
func NewAmount(n uint) (Amount, error) { return Amount(n), nil }

// An op both extracts and constructs, like a factory and a converter.
//
//govo:op Tag
func (t Tag) Upper() Tag {
	raw := string(t)
	return Tag(raw)
}

//govo:op Tag
func (t *Tag) Trim() Tag { return Tag(string(*t)) }

//govo:op Tag
func Join(t Tag, sep string) Tag {
	build := func() Tag { return Tag(string(t) + sep) }
	return build()
}

// An op still reports untyped constants.
//
//govo:op Tag
func (t Tag) Suffixed() Tag {
	return t + "-x" // want "GOV003: untyped constant used as protected type Tag"
}

// An op, too, grants permission only for the types it names.
//
//govo:op Amount
func (a Amount) Tagged(t Tag) Amount {
	_ = string(t) // want "GOV002: direct extraction from protected type Tag"
	return Amount(uint(a))
}

// A pointer conversion sharing storage is permitted in an op.
//
//govo:op Tag
func (t *Tag) Raw() Tag {
	p := (*string)(t)
	return Tag(*p)
}

/* want "GOVD001: op: Tag is not returned by this API" */ //govo:op Tag
func TagString(t Tag) string { return string(t) } // want "GOV002"

/* want "GOVD001: op: Tag is not accepted by this API" */ //govo:op Tag
func ParseTag(s string) Tag { return Tag(s) } // want "GOV001"

/* want "GOVD001: op requires a function or method" */ //govo:op Tag
var _ = 0

// A scalar multiplies or divides by untyped constants.
//
//govo:op Amount
//govo:scalar Amount
func (a Amount) Scaled() Amount {
	half := a / 2
	half *= 3
	half /= 4
	_ = 2 * a * 3
	_ = a * perYen
	_ = a * (1 + 1)
	_ = func() Amount { return a * 10 }()

	_ = a + 1   // want "GOV003: untyped constant used as protected type Amount"
	_ = a % 3   // want "GOV003: untyped constant used as protected type Amount"
	_ = 100 / a // want "GOV003: untyped constant used as protected type Amount"
	_ = a > 0   // want "GOV003: untyped constant used as protected type Amount"
	half -= 1   // want "GOV003: untyped constant used as protected type Amount"
	half++      // want "GOV003"

	return half
}

// Scaling needs no conversion, so a factory suffices.
//
//govo:factory Amount
//govo:scalar Amount
func (a Amount) Half() Amount { return a / 2 }

// The directive may precede the marker and applies to a factory as well,
// as for a conversion between units.
//
//govo:scalar Amount
//govo:factory Amount
func AmountFromYen(yen uint) Amount { return Amount(yen) * perYen }

// A scalar permits only the types it names.
//
//govo:factory Amount Rate
//govo:scalar Rate
func Mixed(a Amount, r Rate) (Amount, Rate) {
	return a * 2, r * 0.5 // want "GOV003: untyped constant scales protected type Amount"
}

// Only untyped constants are scalars, not other untyped expressions.
//
//govo:factory Amount
//govo:scalar Amount
func Shifted(a Amount, u uint) Amount {
	return a * (1 << u) // want "GOV003: untyped expression used as protected type Amount"
}

/* want "GOVD001: scalar: this API is not a factory or op of Amount" */ //govo:scalar Amount
func Unmarked(a Amount) Amount {
	return a * 2 // want "GOV003: untyped constant scales protected type Amount"
}

// A converter extracts but does not construct.
//
//govo:converter Amount
/* want "GOVD001: scalar: this API is not a factory or op of Amount" */ //govo:scalar Amount
func (a Amount) Cents() uint { return uint(a * 100) } // want "GOV003: untyped constant scales"

//govo:op Tag
/* want "GOVD001: scalar: Tag does not have a numeric underlying type" */ //govo:scalar Tag
func (t Tag) Repeat() Tag { return t }

//govo:factory Amount
/* want "GOVD001: scalar: Other is not a protected type declared in this file" */ //govo:scalar Other
func unknownScalar(a Amount) Amount { return a }

//govo:factory Amount
/* want "GOVD001: scalar requires a type name unless this file declares exactly one protected type" */ //govo:scalar
func Unnamed(a Amount) Amount { return a }

/* want "GOVD001: scalar requires a function or method" */ //govo:scalar Amount
var _ = 0
