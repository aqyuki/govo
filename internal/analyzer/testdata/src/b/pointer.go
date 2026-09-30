package b

import "a"

func pointerConversions(t *a.Token, s *string, raw []byte) {
	_ = (*string)(t)  // want "GOV001: direct construction of protected type Token"
	_ = (*a.Token)(s) // want "GOV001: direct construction of protected type Token"
	_ = (*a.Key)(raw) // want "GOV001: direct construction of protected type Key"

	//govo:ignore GOV001 // Shared buffer required by an external API
	_ = (*string)(t)
}
