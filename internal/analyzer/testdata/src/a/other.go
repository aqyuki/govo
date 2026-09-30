package a

const Outside Code = "X" // want "GOVO001"

const (
	OutsideFirst  Number = iota // want "GOVO001"
	OutsideSecond               // want "GOVO001"
)

const Converted Code = Code("X") // want "GOVO001"

const (
	ConvertedFirst  Code = Code("A") // want "GOVO001"
	ConvertedSecond                  // want "GOVO001"
)

const (
	//govo:ignore GOVO001 // approved constant
	Ignored Code = "I"
)

func wrong(s string, c Code) {
	_ = Code(s)   // want "GOVO001"
	_ = Alias(s)  // want "GOVO001"
	_ = string(c) // want "GOVO002"
}
