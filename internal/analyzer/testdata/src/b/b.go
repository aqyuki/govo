package b

import "a"

const (
	Foreign   a.Code = "X" // want "GOVO001"
	Inherited              // want "GOVO001"
)

func use(c a.Code, s string) {
	_ = a.Code(s)  // want "GOVO001"
	_ = a.Alias(s) // want "GOVO001"

	_ = string(c) // want "GOVO002"

	_ = c == "X" // want "GOVO003"

	_ = []a.Code{"X"}              // want "GOVO001"
	_ = map[a.Code]int{"X": 1}     // want "GOVO001"
	_ = struct{ C a.Code }{C: "X"} // want "GOVO001"
	_ = append([]a.Code{}, "X")    // want "GOVO001"

	var ch chan a.Code
	ch <- "X" // want "GOVO001"

	var p *a.Code
	*p = "X" // want "GOVO001"

	_ = func() a.Code { return "X" }() // want "GOVO001"

	switch c {
	case "X": // want "GOVO003"
	}

	c += "X" // want "GOVO003"

	//govo:ignore GOVO001 // test exception
	_ = a.Code("X")

	_ = any(c)

	_ = a.NewCode("X")

	take(
		"X", //govo:ignore GOVO001 // line scoped exception
	)

	//govo:ignore GOVO003 // only the header
	if c == "X" {
		_ = c == "Y" // want "GOVO003"
	}

	//govo:ignore GOVO001,GOVO002 // both conversions
	_ = a.Code(string(c))
	_ = a.Code(string(c)) //govo:ignore GOVO001,GOVO002 // same line
}

func take(a.Code) {}

func call() { take("X") } // want "GOVO001"

func builtins(c a.Code, m map[a.Code]int) {
	delete(m, "X")  // want "GOVO001"
	_ = min(c, "X") // want "GOVO001"
	_ = max("X", c) // want "GOVO001"

	var f a.Flag = true // want "GOVO001"
	_ = f

	append := func(c a.Code, other a.Code) {}
	append(c, "X") // want "GOVO001"
}

func constants(c a.Code) {
	var imported a.Code = a.Raw // want "GOVO001"
	_ = imported
	_ = c == a.Raw   // want "GOVO003"
	_ = c == (a.Raw) // want "GOVO003"

	var n a.Number = min(1, 2) // want "GOVO001"
	_ = n
	_ = n == max(1, 2) // want "GOVO003"

	var typed a.Code = a.Local
	_ = typed

	var s string = a.Raw
	_ = s
	_ = len(s) == len("X")
}

func shifts(n a.Number, u uint) {
	_ = n << 1
	_ = n >> u
	n <<= 2
	n >>= 1
	_ = n + 1 // want "GOVO003"
	n++       // want "GOVO003"
	n--       // want "GOVO003"

	var i int
	i++
}

func unexported(h a.Holder) {
	_ = a.Hidden() == "X" // want "GOVO003"
	_ = a.Holder{H: "X"}  // want "GOVO001"
	_ = string(h.H)       // want "GOVO002"
	_ = a.Hidden() == h.H
}

func ignoreScope(c a.Code, ch chan a.Code) {
	switch c {
	//govo:ignore GOVO003 // only the case expressions
	case "A":
		_ = c == "B" // want "GOVO003"
	}

	//govo:ignore GOVO003 // only the loop header
L:
	for c == "A" {
		_ = c == "B" // want "GOVO003"
		break L
	}

	select {
	//govo:ignore GOVO001 // only the communication
	case ch <- "A":
		ch <- "B" // want "GOVO001"
	}

	take(
		//govo:ignore GOVO001 // want "GOVO004"
		"X", // want "GOVO001"
	)

	//govo:ignore GOVO003 // want "GOVO004"
}

var afterDangling a.Code = "Z" // want "GOVO001"

func untypedExpressions(u uint, x, y int, f, g a.Flag, n, m a.Number) {
	var typedCompare a.Flag = int(1) == int(1) // want "GOVO001: untyped constant used as protected type Flag"
	_ = typedCompare
	_ = a.Flag(int(1) == int(1)) // want "GOVO001: direct construction"
	_ = a.Flag(x == y)           // want "GOVO001: direct construction"
	_ = a.Number(1 << u)         // want "GOVO001: direct construction"

	var shifted a.Number = 1 << u // want "GOVO001: untyped expression used as protected type Number"
	_ = shifted
	var compared a.Flag = x == y // want "GOVO001: untyped expression used as protected type Flag"
	_ = compared
	_ = f == (x == y) // want "GOVO003: untyped expression"
	_ = n + (1 << u)  // want "GOVO003: untyped expression"

	_ = a.Flag(f)
	_ = n << u
	var i int = 1 << u
	_ = i
	var ok bool = n == m
	_ = ok
	_ = f == !g
}

type S struct{ N a.Number }

func elided() {
	_ = []*S{{N: 1}}               // want "GOVO001"
	_ = map[string]*S{"a": {N: 2}} // want "GOVO001"
}

func funcLit() {
	//govo:ignore GOVO001 // only the argument, not the function literal body
	defer func(n a.Number) {
		var m a.Number = 1 // want "GOVO001"
		_ = m + n
	}(2)
}

// One diagnostic per untyped expression, even when go/types gives its
// operands the protected type of the context.
func noDuplicates(u uint, x, y int, n a.Number) {
	var constant a.Number = 1 + 2 // want "GOVO001: untyped constant"
	var shifted a.Number = (1 << u) + 1 // want "GOVO001: untyped expression"
	var logical a.Flag = x == y && x > 0 // want "GOVO001: untyped expression"
	_ = n == 1+2 // want "GOVO003: untyped constant"
	_, _, _ = constant, shifted, logical
}
