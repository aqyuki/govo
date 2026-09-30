package b

import "a"

func takeCodes(a.Codes)        {}
func takeRaw([]a.Code)         {}
func takeVariadic(...[]a.Code) {}
func takeBoth(a.Codes, error)  {}
func rawPair() ([]a.Code, error) {
	return nil, nil
}

func twoCodes() (a.Codes, a.Codes) {
	return nil, nil
}

func codesPair() (a.Codes, error) {
	return nil, nil
}

func compositeConstruction(raw []a.Code, bytes [4]byte, m map[string]bool) a.Codes {
	_ = a.Codes{a.NewCode("X")}            // want "GOVO001: direct construction of protected type Codes"
	_ = &a.Set{"x": true}                  // want "GOVO001: direct construction of protected type Set"
	_ = []a.ID{{1}}                        // want "GOVO001: direct construction of protected type ID"
	_ = map[string]a.Set{"k": {"x": true}} // want "GOVO001: direct construction of protected type Set"
	_ = a.Codes(raw)                       // want "GOVO001: direct construction of protected type Codes"

	// Empty literals, make, and nil are zero or empty values.
	_ = a.Codes{}
	_ = a.Set{}
	_ = make(a.Codes, 0, 4)
	var none a.Codes = nil
	_ = none
	var zero a.ID
	_ = zero

	var codes a.Codes = raw         // want "GOVO001: implicit construction of protected type Codes"
	codes = raw                     // want "GOVO001: implicit construction of protected type Codes"
	takeCodes(raw)                  // want "GOVO001: implicit construction of protected type Codes"
	_ = struct{ C a.Codes }{C: raw} // want "GOVO001: implicit construction of protected type Codes"
	_ = map[a.ID]int{bytes: 1}      // want "GOVO001: implicit construction of protected type ID"

	var ids map[a.ID]int
	_ = ids[bytes] // want "GOVO001: implicit construction of protected type ID"

	var id a.ID
	_ = id == bytes // want "GOVO001: implicit construction of protected type ID"

	var set a.Set = m // want "GOVO001: implicit construction of protected type Set"
	_ = set

	var err error
	codes, err = rawPair() // want "GOVO001: implicit construction of protected type Codes"
	_ = err
	takeBoth(rawPair()) // want "GOVO001: implicit construction of protected type Codes"

	_ = codes

	return raw // want "GOVO001: implicit construction of protected type Codes"
}

func compositeExtraction(codes a.Codes, id a.ID, set a.Set) ([]a.Code, error) {
	_ = []a.Code(codes)      // want "GOVO002: direct extraction from protected type Codes"
	_ = [4]byte(id)          // want "GOVO002: direct extraction from protected type ID"
	_ = map[string]bool(set) // want "GOVO002: direct extraction from protected type Set"

	var raw []a.Code = codes // want "GOVO002: implicit extraction from protected type Codes"
	raw = codes              // want "GOVO002: implicit extraction from protected type Codes"
	takeRaw(codes)           // want "GOVO002: implicit extraction from protected type Codes"
	takeVariadic(codes)      // want "GOVO002: implicit extraction from protected type Codes"
	_ = [][]a.Code{codes}    // want "GOVO002: implicit extraction from protected type Codes"

	var m map[string]bool = set // want "GOVO002: implicit extraction from protected type Set"
	_ = m

	var bytes [4]byte
	_ = bytes == id // want "GOVO001: implicit construction of protected type ID"

	var err error
	raw, err = codesPair()           // want "GOVO002: implicit extraction from protected type Codes"
	var r2, e2 = codesPair()         // declared with the result types
	var r3, r4 []a.Code = twoCodes() // want "GOVO002: implicit extraction from protected type Codes" "GOVO002: implicit extraction from protected type Codes"
	_, _, _, _, _ = r2, e2, r3, r4, err

	byName := map[string]a.Codes{}
	var ok bool
	raw, ok = byName["x"] // want "GOVO002: implicit extraction from protected type Codes"
	_ = ok

	//govo:ignore GOVO002 // approved extraction
	raw = codes

	_ = raw

	// Conversions to interfaces and to the same protected type are permitted.
	var v any = codes
	_ = v
	_ = a.Codes(codes)

	return codes, nil // want "GOVO002: implicit extraction from protected type Codes"
}

func compositeTupleReturn() ([]a.Code, error) {
	return codesPair() // want "GOVO002: implicit extraction from protected type Codes"
}

// Element operations are outside the rules, except that untyped constants
// used as a protected element or key type are still reported.
func compositeElements(codes a.Codes, id a.ID, set a.Set, dst []a.Code, raw []a.Code) {
	_ = codes[0]
	codes[0] = a.NewCode("X")
	codes[1] = "X" // want "GOVO001: untyped constant used as protected type Code"
	_ = len(codes)
	_ = codes[1:]
	_ = id[:]
	id[0] = 1
	set["x"] = true
	delete(set, "x")

	for i, c := range codes {
		_, _ = i, c
	}

	for k, v := range set {
		_, _ = k, v
	}

	codes = append(codes, raw...)
	dst = append(dst, codes...)
	codes = append(codes, "X") // want "GOVO001: untyped constant used as protected type Code"
	copy(dst, codes)
	copy(codes, raw)
	clear(set)

	v, ok := set["x"]
	_, _ = v, ok
	_ = dst
}

func compositeBuiltins(codes a.Codes, lists [][]a.Code, ids map[a.ID]int, bytes [4]byte, dst []a.Code) {
	lists = append(lists, codes)      // want "GOVO002: implicit extraction from protected type Codes"
	lists = append(lists, dst, codes) // want "GOVO002: implicit extraction from protected type Codes"
	delete(ids, bytes)                // want "GOVO001: implicit construction of protected type ID"
	copy(dst, codes)
	dst = append(dst, codes...)
	_, _ = lists, dst
}
