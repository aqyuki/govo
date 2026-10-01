package b

import "a"

type Ints = a.List[int]

func useGeneric(xs []int, l a.List[int], u a.RefID[a.User], o a.RefID[a.Order]) {
	_ = a.NewList(xs)
	_ = l.Raw()
	_, _ = a.ParseID[a.User]("u")

	_ = a.List[int](xs)    // want `GOV001: direct construction of protected type List\[int\]`
	_ = Ints(xs)           // want `GOV001: direct construction of protected type List\[int\]`
	_ = []int(l)           // want `GOV002: direct extraction from protected type List\[int\]`
	_ = a.RefID[a.User](o) // want `GOV001: direct construction of protected type RefID\[User\]`
	_ = u == "x"           // want `GOV003: untyped constant used as protected type RefID\[User\]`
}
