package a

//govo:protect
type Codes []Code // want Codes:"&{}"

//govo:protect
type ID [4]byte // want ID:"&{}"

//govo:protect
type Set map[string]bool // want Set:"&{}"

type IDAlias = ID

// Marked functions may construct and extract implicitly.
func NewCodes(raw []string) Codes {
	codes := Codes{}
	for _, r := range raw {
		codes = append(codes, NewCode(r))
	}

	return codes
}

//govo:factory ID
func NewID(raw [4]byte) ID { return raw }

//govo:factory Set
func NewSet(raw map[string]bool) (Set, error) { return Set(raw), nil }

//govo:converter Set
func (s Set) Raw() map[string]bool { return s }

//govo:converter ID
func (id ID) Bytes() [4]byte { return [4]byte(id) }

//govo:converter Codes
func (c Codes) Strings() []Code {
	var out []Code = c
	return out
}

//govo:factory Codes ID Set
func literals() (Codes, ID, Set) {
	_ = Codes{"X"} // want "GOV001: untyped constant used as protected type Code"
	return Codes{NewCode("X")}, ID{1, 2, 3, 4}, Set{"a": true}
}

// Unmarked functions in the type declaration file are not trusted.
func unmarkedLiterals() {
	_ = Codes{NewCode("X")} // want "GOV001: direct construction of protected type Codes"
	_ = ID{1, 2, 3, 4}      // want "GOV001: direct construction of protected type ID"
	_ = Set{}
}

func pair() ([]Code, error) { return Codes{}, nil } // want `GOV002: implicit extraction from protected type Codes; use a //govo:converter function$`
