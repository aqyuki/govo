package a

// Directives in another file than the type grant nothing.
//
/* want "GOVD001: factory: Amount is not a protected type declared in this file" */ //govo:factory Amount
/* want "GOVD001: scalar: Amount is not a protected type declared in this file" */ //govo:scalar Amount
func doubled(a Amount) Amount {
	return a * 2 // want "GOV003: untyped constant scales protected type Amount"
}

/* want "GOVD001: op: Tag is not a protected type declared in this file" */ //govo:op Tag
func lowered(t Tag) Tag {
	return Tag(string(t)) // want "GOV001" "GOV002"
}

func scaledOutside(a Amount) {
	a *= 2 // want "GOV003: untyped constant scales protected type Amount"
	_ = a
}
