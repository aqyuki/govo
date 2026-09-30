package a

const Outside Code = "X" // want "GOV001"

const (
	OutsideFirst  Number = iota // want "GOV001"
	OutsideSecond               // want "GOV001"
)

const Converted Code = Code("X") // want "GOV001"

const (
	ConvertedFirst  Code = Code("A") // want "GOV001"
	ConvertedSecond                  // want "GOV001"
)

const (
	//govo:ignore GOV001 // approved constant
	Ignored Code = "I"
)

func wrong(s string, c Code) {
	_ = Code(s)   // want "GOV001"
	_ = Alias(s)  // want "GOV001"
	_ = string(c) // want "GOV002"
}
