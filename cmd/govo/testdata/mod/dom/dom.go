package dom

//govo:protect
type Code string

//govo:factory Code
func NewCode(s string) Code { return Code(s) }

//govo:protect
type secret string

//govo:factory secret
func Secret() secret { return secret("s") }
