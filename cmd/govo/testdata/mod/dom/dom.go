package dom

//govo:protect
type Code string

//govo:factory Code
func NewCode(s string) Code { return Code(s) }

//govo:protect
type secret string

func Secret() secret { return secret("s") }
