package c

//govo:protect
type Code string // want Code:"&{}"

func use(c Code) {
	//govo:ignore GOVO003 // with a reason
	_ = c == "A"

	_ = c == "B" /* want "GOVO004: ignore directive has no reason" */ //govo:ignore GOVO003
}
