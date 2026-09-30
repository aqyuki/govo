package c

//govo:protect
type Code string // want Code:"&{}"

func use(c Code) {
	//govo:ignore GOV003 // with a reason
	_ = c == "A"

	_ = c == "B" /* want "GOVD003: ignore directive has no reason" */ //govo:ignore GOV003
}
