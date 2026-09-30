package a

func sameProject(raw []Code, id ID) {
	_ = Codes(raw)  // want "GOV001: direct construction of protected type Codes"
	_ = []Code(nil) // conversion of untyped nil to an unprotected type
	_ = [4]byte(id) // want "GOV002: direct extraction from protected type ID"

	var codes Codes = raw // want `GOV001: implicit construction of protected type Codes; use a //govo:factory function$`
	_ = codes

	_ = ID{1}      // want "GOV001: direct construction of protected type ID"
	_ = IDAlias{1} // want "GOV001: direct construction of protected type ID"
}
