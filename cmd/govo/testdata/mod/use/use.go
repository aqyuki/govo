package use

import "example.com/mod/dom"

func Use(s string) {
	_ = dom.Code(s)
	_ = dom.Secret() == "X"
	_ = dom.NewCode(s)
}
