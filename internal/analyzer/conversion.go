package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// checkExplicitConversion checks the explicit conversion c to dest.
//
//declscope:package // walk.go checks each conversion call with it
func (s *state) checkExplicitConversion(c *ast.CallExpr, dest types.Type) {
	source := s.pass.TypesInfo.TypeOf(c.Args[0])

	if from, to, ok := conversionAliases(source, dest); ok {
		s.checkAliasingConversion(ast.Unparen(c.Fun).Pos(), from, to)
		return
	}

	pd, ps := s.protected(dest), s.protected(source)
	if pd != nil && ps != nil && types.Identical(dest, source) && !s.untyped(c.Args[0]) {
		return
	}

	if pd != nil && !s.mayConstruct(pd) {
		s.constructionIssue(c.Fun.Pos(), "direct", dest)
		return
	}

	// go/types records the contextual type of an untyped operand, as in
	// Code("ABC"), which extracts nothing.
	if ps == nil || s.untyped(c.Args[0]) || s.mayExtract(ps) {
		return
	}

	if _, ok := dest.Underlying().(*types.Interface); !ok {
		s.extractionIssue(c.Fun.Pos(), "direct", source)
	}
}

// conversionAliases reports whether a conversion from source to dest yields a pointer
// that shares storage with the operand under another type: a conversion
// between pointer types, as in (*string)(&code), or from a slice to an
// array pointer, as in (*ID)(raw). It returns the types of that storage as
// seen before and after the conversion.
func conversionAliases(source, dest types.Type) (from, to types.Type, ok bool) {
	if source == nil {
		return nil, nil, false
	}

	ptr, ok := dest.Underlying().(*types.Pointer)
	if !ok {
		return nil, nil, false
	}

	switch u := source.Underlying().(type) {
	case *types.Pointer:
		return u.Elem(), ptr.Elem(), true
	case *types.Slice:
		return source, ptr.Elem(), true
	}

	return nil, nil, false
}

// checkAliasingConversion checks a conversion that makes the storage of a value
// of type from accessible as type to. Reads and writes through the result
// cross the type boundary in both directions, so each protected type
// involved needs both a factory and a converter. Construction is reported
// first, as for other conversions.
func (s *state) checkAliasingConversion(pos token.Pos, from, to types.Type) {
	if types.Identical(from, to) {
		return
	}

	involved := []types.Type{to, from}
	for _, t := range involved {
		if p := s.protected(t); p != nil && !s.mayConstruct(p) {
			s.constructionIssue(pos, "direct", t)
			return
		}
	}

	for _, t := range involved {
		if p := s.protected(t); p != nil && !s.mayExtract(p) {
			s.extractionIssue(pos, "direct", t)
			return
		}
	}
}

// directConversionTo reports whether expr is an explicit conversion to target.
//
//declscope:package // walk.go accepts protected constants declared this way
func (s *state) directConversionTo(expr ast.Expr, target types.Type) bool {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok {
		return false
	}

	tv, ok := s.pass.TypesInfo.Types[call.Fun]

	return ok && tv.IsType() && types.Identical(tv.Type, target)
}

// checkImplicitConversion checks expr where its value is implicitly converted to target.
//
//declscope:package // walk.go and operation.go check implicitly converted values with it
func (s *state) checkImplicitConversion(expr ast.Expr, target types.Type, rule string) {
	s.checkUntypedConversion(expr, target, rule)
	s.checkAssignedConversion(expr.Pos(), s.pass.TypesInfo.TypeOf(expr), target)
}

// checkTupleConversion checks the values of a multi-valued expression, such as a
// call or a comma-ok expression, where they are assigned to targets.
//
//declscope:package // walk.go checks multi-valued assignments, calls, and returns
func (s *state) checkTupleConversion(expr ast.Expr, targets []types.Type) {
	tuple, ok := s.pass.TypesInfo.TypeOf(expr).(*types.Tuple)
	if !ok {
		return
	}

	for i := 0; i < tuple.Len() && i < len(targets); i++ {
		s.checkAssignedConversion(expr.Pos(), tuple.At(i).Type(), targets[i])
	}
}

// checkAssignedConversion checks the implicit conversion of a value of type source to
// target. A value of an unnamed array, slice, or map type is assignable to
// a defined type with the same underlying type and vice versa, so it
// constructs or extracts a protected type without a conversion. As with an
// explicit conversion, marked functions are trusted.
func (s *state) checkAssignedConversion(pos token.Pos, source, target types.Type) {
	if source == nil || target == nil || types.Identical(source, target) || !types.Identical(source.Underlying(), target.Underlying()) {
		return
	}

	if p := s.protected(target); p != nil {
		if !s.mayConstruct(p) {
			s.constructionIssue(pos, "implicit", target)
		}

		return
	}

	if p := s.protected(source); p != nil && !s.mayExtract(p) {
		s.extractionIssue(pos, "implicit", source)
	}
}

// checkUntypedConversion reports expr when it is untyped and target is a
// protected type, so that its value becomes a value of that type.
//
//declscope:package // walk.go checks arguments that copy elements with it
func (s *state) checkUntypedConversion(expr ast.Expr, target types.Type, rule string) {
	p := s.protected(target)
	if p == nil || !s.untyped(expr) {
		return
	}

	kind := "expression"
	if s.pass.TypesInfo.Types[expr].Value != nil {
		kind = "constant"
	}

	s.issue(expr.Pos(), rule, fmt.Sprintf("untyped %s used as protected type %s", kind, typeString(target)), s.current)
}
