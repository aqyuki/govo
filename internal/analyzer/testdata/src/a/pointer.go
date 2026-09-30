package a

//govo:protect
type Token string // want Token:"&{}"

//govo:protect
type Key [2]byte // want Key:"&{}"

// A pointer that shares the storage of a protected value under another type
// allows both reads and writes, so a conversion to one needs both markers.

//govo:factory Token
//govo:converter Token
func tokenView(t *Token, s *string) (Token, *string, *Token) {
	_ = (*Code)(t) // want "GOVO001: direct construction of protected type Code"
	return *t, (*string)(t), (*Token)(s)
}

//govo:factory Token
func NewToken(s *string) (Token, *Token) {
	return Token(*s), (*Token)(s) // want `GOVO002: direct extraction from protected type Token; use a //govo:converter function$`
}

//govo:converter Token
func (t *Token) Raw() *string {
	return (*string)(t) // want `GOVO001: direct construction of protected type Token; use a //govo:factory function$`
}

//govo:factory Key
//govo:converter Key
func keyView(k Key, raw []byte) (Key, *Key, *[2]byte) {
	return k, (*Key)(raw), (*[2]byte)(&k)
}

// Unmarked functions in the type declaration file are not trusted.
func unmarkedPointers(t *Token, raw []byte) {
	_ = (*string)(t) // want "GOVO001: direct construction of protected type Token"
	_ = (*Key)(raw)  // want "GOVO001: direct construction of protected type Key"
}
