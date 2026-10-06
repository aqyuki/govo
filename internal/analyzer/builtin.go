package analyzer

import (
	"go/ast"
	"go/types"
)

// builtinCopiesElements reports whether the i-th argument of the built-in
// call c only supplies elements to copy, as the arguments of copy and the
// argument spread by append(s, x...) do. Element operations are outside the rules,
// so such an argument is not reported merely because its type shares the
// underlying type of the parameter. Other arguments, such as the value in
// append(lists, codes), are converted as a whole and checked as usual.
//
//declscope:shared // walk.go checks such arguments for untyped values only
func builtinCopiesElements(builtin *types.Builtin, c *ast.CallExpr, i int) bool {
	if builtin == nil {
		return false
	}

	switch builtin.Name() {
	case "copy":
		return true
	case "append":
		return c.Ellipsis.IsValid() && i == len(c.Args)-1
	}

	return false
}

// builtin returns the built-in function that fun denotes, if any, including
// those of package unsafe.
//
//declscope:shared // walk.go and untyped.go recognize built-in calls with it
func (s *state) builtin(fun ast.Expr) *types.Builtin {
	var id *ast.Ident

	switch f := ast.Unparen(fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return nil
	}

	builtin, _ := s.pass.TypesInfo.Uses[id].(*types.Builtin)

	return builtin
}
