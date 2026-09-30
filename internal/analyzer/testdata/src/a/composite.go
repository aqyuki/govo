package a

//govo:protect
type Codes []Code // want Codes:"&{}"

//govo:protect
type ID [4]byte // want ID:"&{}"

//govo:protect
type Set map[string]bool // want Set:"&{}"

type IDAlias = ID

// The type declaration file may construct and extract implicitly.
func NewCodes(raw []string) Codes {
	codes := Codes{}
	for _, r := range raw {
		codes = append(codes, NewCode(r))
	}

	return codes
}

func NewID(raw [4]byte) ID { return raw }

func NewSet(raw map[string]bool) (Set, error) { return Set(raw), nil }

func (s Set) Raw() map[string]bool { return s }

func (id ID) Bytes() [4]byte { return [4]byte(id) }

func (c Codes) Strings() []Code {
	var out []Code = c
	return out
}

func literals() {
	_ = Codes{"X"} // want "GOVO001: untyped constant used as protected type Code"
	_ = ID{1, 2, 3, 4}
	_ = Set{"a": true}
}

func pair() ([]Code, error) { return Codes{}, nil }
