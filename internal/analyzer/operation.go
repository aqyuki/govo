package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// checkBinaryOperation checks the operands of b against the protected type
// of the other operand.
//
//declscope:shared // walk.go checks every binary expression with it
func (s *state) checkBinaryOperation(b *ast.BinaryExpr) {
	if b.Op == token.SHL || b.Op == token.SHR {
		// A shift count never takes the type of the shifted operand, and an
		// untyped constant shifted by a protected count takes its type from
		// the context, which the enclosing use site checks.
		return
	}

	// An untyped operand, as in (1 << u) + 1, has the protected type only
	// because of the context. The enclosing use site reports the whole
	// expression, so only a genuinely protected operand is checked here.
	// Only a multiplier or a divisor scales the protected operand; a
	// dividend, as in 100 / n, does not.
	x, y := s.pass.TypesInfo.TypeOf(b.X), s.pass.TypesInfo.TypeOf(b.Y)
	if s.protected(x) != nil && !s.untyped(b.X) {
		s.checkOperationOperand(b.Y, x, b.Op == token.MUL || b.Op == token.QUO)
	}

	if s.protected(y) != nil && !s.untyped(b.Y) {
		s.checkOperationOperand(b.X, y, b.Op == token.MUL)
	}
}

// checkOperationOperand checks expr, an operand of an operation with a value of the
// protected type target. With scaling set, expr multiplies or divides that
// value, so an untyped constant is a dimensionless scalar rather than a
// value of the type, which a function with a scalar directive may use.
//
//declscope:shared // walk.go checks compound assignments with it
func (s *state) checkOperationOperand(expr ast.Expr, target types.Type, scaling bool) {
	p := s.protected(target)
	if !scaling || p == nil || !s.untyped(expr) || s.pass.TypesInfo.Types[expr].Value == nil {
		s.checkImplicitConversion(expr, target, ruleOperation)
		return
	}

	if !s.mayScale(p) {
		s.issue(expr.Pos(), ruleOperation, fmt.Sprintf("untyped constant scales protected type %s; use a //govo:%s function", typeString(target), directiveScalar), s.current)
	}
}

// checkIncDecOperation reports x++ and x--, which add the untyped constant 1 to x.
//
//declscope:shared // walk.go checks every x++ and x-- with it
func (s *state) checkIncDecOperation(stmt *ast.IncDecStmt) {
	t := s.pass.TypesInfo.TypeOf(stmt.X)
	if s.protected(t) == nil {
		return
	}

	s.issue(stmt.TokPos, ruleOperation, fmt.Sprintf("%s applies an untyped constant to protected type %s", stmt.Tok, typeString(t)), s.current)
}

// checkSwitchOperation checks the case values of a switch on a protected
// value, which are compared with it.
//
//declscope:shared // walk.go checks every expression switch with it
func (s *state) checkSwitchOperation(sw *ast.SwitchStmt) {
	if sw.Tag == nil {
		return
	}

	t := s.pass.TypesInfo.TypeOf(sw.Tag)
	if s.protected(t) == nil {
		return
	}

	for _, stmt := range sw.Body.List {
		cc := stmt.(*ast.CaseClause)
		for _, expr := range cc.List {
			s.checkImplicitConversion(expr, t, ruleOperation)
		}
	}
}
