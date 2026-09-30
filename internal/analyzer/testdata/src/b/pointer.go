package b

import "a"

func pointerConversions(t *a.Token, s *string, raw []byte) {
	_ = (*string)(t)  // want "GOVO001: direct construction of protected type Token"
	_ = (*a.Token)(s) // want "GOVO001: direct construction of protected type Token"
	_ = (*a.Key)(raw) // want "GOVO001: direct construction of protected type Key"

	//govo:ignore GOVO001 // Shared buffer required by an external API
	_ = (*string)(t)
}
