package a

func genericOther(xs []int, l List[int], u RefID[User], o RefID[Order], m Money[Yen], s Strings) {
	_ = List[int](xs) // want `GOV001: direct construction of protected type List\[int\]`
	_ = []int(l)      // want `GOV002: direct extraction from protected type List\[int\]`
	_ = List[int]{1}  // want `GOV001: direct construction of protected type List\[int\]`
	_ = List[int]{}
	_ = Strings{"x"} // want `GOV001: direct construction of protected type List\[string\]`
	_ = []string(s)  // want `GOV002: direct extraction from protected type List\[string\]`

	var raw []int = l // want `GOV002: implicit extraction from protected type List\[int\]`
	l = xs            // want `GOV001: implicit construction of protected type List\[int\]`
	_ = raw

	// Instances of one type are distinct types.
	_ = RefID[User](o)     // want `GOV001: direct construction of protected type RefID\[User\]`
	_ = (*RefID[User])(&o) // want `GOV001: direct construction of protected type RefID\[User\]`
	_ = RefID[User](u)

	_ = u == "root" // want `GOV003: untyped constant used as protected type RefID\[User\]`
	_ = m * 2       // want `GOV003: untyped constant scales protected type Money\[Yen\]`
	m++             // want `GOV003: \+\+ applies an untyped constant to protected type Money\[Yen\]`

	// Element operations are not checked.
	l[0] = 1
	_ = l[0] + 1
}

// Typed constants of an instance are declared in the type declaration file.
const OtherID RefID[User] = "other" // want "GOV001: protected constant OtherID declared outside its type file"
