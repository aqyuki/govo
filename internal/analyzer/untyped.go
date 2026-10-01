package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"
)

// untyped reports whether expr is untyped before it is converted to the
// type of its context. go/types records that contextual type, which may be
// a protected type, so the expression's ingredients are inspected instead.
// Besides untyped constants, comparisons and shifts of untyped constants by
// non-constant counts produce untyped values.
//
//declscope:package // the checks of operands and conversions consult it
func (s *state) untyped(expr ast.Expr) bool {
	if _, ok := s.pass.TypesInfo.Types[expr]; !ok {
		return false
	}

	switch e := expr.(type) {
	case *ast.BasicLit:
		return true
	case *ast.ParenExpr:
		return s.untyped(e.X)
	case *ast.UnaryExpr:
		switch e.Op {
		case token.ADD, token.SUB, token.XOR, token.NOT:
			return s.untyped(e.X)
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			// A comparison yields an untyped boolean, even for typed operands.
			return true
		case token.SHL, token.SHR:
			// A shift has the type of its left operand.
			return s.untyped(e.X)
		}

		return s.untyped(e.X) && s.untyped(e.Y)
	case *ast.Ident:
		// Also covers the predeclared true, false, and iota.
		return s.untypedConstantIdent(e)
	case *ast.SelectorExpr:
		// A named constant declared in another package.
		return s.untypedConstantIdent(e.Sel)
	case *ast.CallExpr:
		return s.untypedBuiltinCall(e)
	}

	return false
}

func (s *state) untypedConstantIdent(id *ast.Ident) bool {
	obj, ok := s.pass.TypesInfo.Uses[id].(*types.Const)
	if !ok {
		return false
	}

	basic, ok := obj.Type().(*types.Basic)

	return ok && basic.Info()&types.IsUntyped != 0
}

// untypedBuiltinCall reports whether c is a call of a built-in function
// whose result is an untyped constant when all of its arguments are.
// Other constant calls, such as len or unsafe.Sizeof, have typed results.
func (s *state) untypedBuiltinCall(c *ast.CallExpr) bool {
	if s.pass.TypesInfo.Types[c].Value == nil {
		return false
	}

	builtin := s.builtin(c.Fun)
	if builtin == nil {
		return false
	}

	switch builtin.Name() {
	case "min", "max", "real", "imag", "complex":
	default:
		return false
	}

	for _, arg := range c.Args {
		if !s.untyped(arg) {
			return false
		}
	}

	return len(c.Args) > 0
}
