package a

import "unsafe"

func pointerConversions(t *Token, s *string, raw []byte, k *Key, c *Code, codes Codes) {
	_ = (*string)(t)      // want "GOVO001: direct construction of protected type Token"
	_ = (*Token)(s)       // want "GOVO001: direct construction of protected type Token"
	_ = (*Key)(raw)       // want "GOVO001: direct construction of protected type Key"
	_ = (*[2]byte)(k)     // want "GOVO001: direct construction of protected type Key"
	_ = (*Code)(t)        // want "GOVO001: direct construction of protected type Code"
	_ = (*[1]Code)(codes) // want "GOVO001: direct construction of protected type Codes"
	_ = Key(raw)          // want "GOVO001: direct construction of protected type Key"

	// The same type, an alias of it, and unprotected types share nothing new.
	_ = (*Token)(t)
	_ = (*Alias)(c)
	_ = (*Token)(nil)
	_ = (*[2]byte)(raw)

	// Conversions through unsafe.Pointer are outside the scope.
	_ = unsafe.Pointer(t)
}
