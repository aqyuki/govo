package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

func (s *analyzerState) analyzeFile(f *ast.File) {
	for _, decl := range f.Decls {
		s.grant = nil
		if fn, ok := decl.(*ast.FuncDecl); ok {
			s.grant = s.grants[fn]
		}

		s.analyzeNode(decl, nil)
	}

	s.grant = nil
}

func (s *analyzerState) analyzeNode(root ast.Node, signature *types.Signature) {
	if fn, ok := root.(*ast.FuncDecl); ok {
		sig, _ := s.pass.TypesInfo.TypeOf(fn.Name).(*types.Signature)
		signature = sig
	}

	ast.Inspect(root, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			if sig, ok := s.pass.TypesInfo.TypeOf(n).(*types.Signature); ok {
				s.analyzeNode(n.Body, sig)
				return false
			}

		case *ast.CallExpr:
			s.call(n)

		case *ast.GenDecl:
			if n.Tok == token.CONST || n.Tok == token.VAR {
				s.valueDecl(n)
			}

			if n.Tok == token.CONST {
				// The type declaration file may declare protected constants
				// with explicit conversions, as in const A = Code("A").
				s.inConst = true
				for _, spec := range n.Specs {
					s.analyzeNode(spec, signature)
				}

				s.inConst = false

				return false
			}

		case *ast.AssignStmt:
			switch n.Tok {
			case token.ASSIGN, token.DEFINE:
				if len(n.Lhs) > 1 && len(n.Rhs) == 1 {
					s.implicitTuple(n.Rhs[0], s.typesOf(n.Lhs))
					break
				}

				for i, rhs := range n.Rhs {
					if i < len(n.Lhs) {
						s.implicit(rhs, s.pass.TypesInfo.TypeOf(n.Lhs[i]), ruleConstruction)
					}
				}
			case token.SHL_ASSIGN, token.SHR_ASSIGN:
				// A shift count never takes the type of the shifted operand.
			default:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
					s.implicit(n.Rhs[0], s.pass.TypesInfo.TypeOf(n.Lhs[0]), ruleOperation)
				}
			}

		case *ast.IncDecStmt:
			s.incDec(n)

		case *ast.ReturnStmt:
			if signature != nil {
				results := signature.Results()
				if results.Len() > 1 && len(n.Results) == 1 {
					targets := make([]types.Type, results.Len())
					for i := range targets {
						targets[i] = results.At(i).Type()
					}

					s.implicitTuple(n.Results[0], targets)

					break
				}

				for i, expr := range n.Results {
					if i < signature.Results().Len() {
						s.implicit(expr, signature.Results().At(i).Type(), ruleConstruction)
					}
				}
			}

		case *ast.CompositeLit:
			s.composite(n)

		case *ast.SendStmt:
			if t := s.pass.TypesInfo.TypeOf(n.Chan); t != nil {
				if ch, ok := t.Underlying().(*types.Chan); ok {
					s.implicit(n.Value, ch.Elem(), ruleConstruction)
				}
			}

		case *ast.IndexExpr:
			if t := s.pass.TypesInfo.TypeOf(n.X); t != nil {
				if m, ok := t.Underlying().(*types.Map); ok {
					s.implicit(n.Index, m.Key(), ruleConstruction)
				}
			}

		case *ast.BinaryExpr:
			s.binary(n)

		case *ast.SwitchStmt:
			s.valueSwitch(n)
		}

		return true
	})
}

func (s *analyzerState) call(c *ast.CallExpr) {
	if tv, ok := s.pass.TypesInfo.Types[c.Fun]; ok && tv.IsType() {
		if len(c.Args) == 1 {
			s.conversion(c, tv.Type)
		}

		return
	}

	// For built-in functions such as append, go/types records a signature
	// specific to the call site, so they need no special handling.
	sig, ok := s.pass.TypesInfo.TypeOf(c.Fun).(*types.Signature)
	if !ok {
		return
	}

	params := sig.Params()
	paramType := func(i int, ellipsis bool) types.Type {
		index := i
		if sig.Variadic() && i >= params.Len()-1 {
			index = params.Len() - 1
		}

		if index < 0 || index >= params.Len() {
			return nil
		}

		t := params.At(index).Type()
		if sig.Variadic() && index == params.Len()-1 && !ellipsis {
			if sl, ok := t.(*types.Slice); ok {
				t = sl.Elem()
			}
		}

		return t
	}

	builtin := s.builtin(c.Fun)

	if len(c.Args) == 1 && builtin == nil {
		if tuple, ok := s.pass.TypesInfo.TypeOf(c.Args[0]).(*types.Tuple); ok {
			targets := make([]types.Type, tuple.Len())
			for i := range targets {
				targets[i] = paramType(i, false)
			}

			s.implicitTuple(c.Args[0], targets)

			return
		}
	}

	for i, arg := range c.Args {
		t := paramType(i, c.Ellipsis.IsValid())
		if t == nil {
			continue
		}

		if elementCopy(builtin, c, i) {
			s.implicitUntyped(arg, t, ruleConstruction)
		} else {
			s.implicit(arg, t, ruleConstruction)
		}
	}
}

func (s *analyzerState) conversion(c *ast.CallExpr, dest types.Type) {
	source := s.pass.TypesInfo.TypeOf(c.Args[0])

	if from, to, ok := aliasing(source, dest); ok {
		s.aliasingConversion(ast.Unparen(c.Fun).Pos(), from, to)
		return
	}

	pd, ps := s.protected(dest), s.protected(source)
	if pd != nil && ps != nil && types.Identical(dest, source) && !s.untyped(c.Args[0]) {
		return
	}

	if pd != nil && !s.mayConstruct(pd) {
		s.constructionIssue(c.Fun.Pos(), "direct", pd)
		return
	}

	// go/types records the contextual type of an untyped operand, as in
	// Code("ABC"), which extracts nothing.
	if ps == nil || s.untyped(c.Args[0]) || s.mayExtract(ps) {
		return
	}

	if _, ok := dest.Underlying().(*types.Interface); !ok {
		s.extractionIssue(c.Fun.Pos(), "direct", ps)
	}
}

// aliasing reports whether a conversion from source to dest yields a pointer
// that shares storage with the operand under another type: a conversion
// between pointer types, as in (*string)(&code), or from a slice to an
// array pointer, as in (*ID)(raw). It returns the types of that storage as
// seen before and after the conversion.
func aliasing(source, dest types.Type) (from, to types.Type, ok bool) {
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

// aliasingConversion checks a conversion that makes the storage of a value
// of type from accessible as type to. Reads and writes through the result
// cross the type boundary in both directions, so each protected type
// involved needs both a factory and a converter. Construction is reported
// first, as for other conversions.
func (s *analyzerState) aliasingConversion(pos token.Pos, from, to types.Type) {
	if types.Identical(from, to) {
		return
	}

	involved := []*protectedType{s.protected(to), s.protected(from)}
	for _, p := range involved {
		if p != nil && !s.mayConstruct(p) {
			s.constructionIssue(pos, "direct", p)
			return
		}
	}

	for _, p := range involved {
		if p != nil && !s.mayExtract(p) {
			s.extractionIssue(pos, "direct", p)
			return
		}
	}
}

func (s *analyzerState) valueDecl(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		v := spec.(*ast.ValueSpec)

		if d.Tok == token.VAR && len(v.Names) > 1 && len(v.Values) == 1 {
			targets := make([]types.Type, len(v.Names))
			for i, name := range v.Names {
				if obj := s.pass.TypesInfo.Defs[name]; obj != nil {
					targets[i] = obj.Type()
				}
			}

			s.implicitTuple(v.Values[0], targets)

			continue
		}

		for i, name := range v.Names {
			var t types.Type
			if obj := s.pass.TypesInfo.Defs[name]; obj != nil {
				t = obj.Type()
			}

			if p := s.protected(t); d.Tok == token.CONST && p != nil {
				if p.file != s.current && (i >= len(v.Values) || !s.directConversionTo(v.Values[i], t)) {
					s.issue(name.Pos(), ruleConstruction, fmt.Sprintf("protected constant %s declared outside its type file", name.Name), s.current)
				}

				continue
			}

			if d.Tok == token.VAR && i < len(v.Values) {
				s.implicit(v.Values[i], t, ruleConstruction)
			}
		}
	}
}

func (s *analyzerState) directConversionTo(expr ast.Expr, target types.Type) bool {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	if !ok {
		return false
	}

	tv, ok := s.pass.TypesInfo.Types[call.Fun]

	return ok && tv.IsType() && types.Identical(tv.Type, target)
}

func (s *analyzerState) composite(c *ast.CompositeLit) {
	t := s.pass.TypesInfo.TypeOf(c)
	if t == nil {
		return
	}

	// go/types records *T for a literal whose &T is elided, as in []*T{{...}}.
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}

	// An empty literal holds no elements, like the zero value or make.
	if p := s.protected(t); p != nil && !s.mayConstruct(p) && len(c.Elts) > 0 {
		pos := c.Lbrace
		if c.Type != nil {
			pos = c.Type.Pos()
		}

		s.constructionIssue(pos, "direct", p)
	}

	switch u := t.Underlying().(type) {
	case *types.Slice:
		s.elements(c.Elts, u.Elem())
	case *types.Array:
		s.elements(c.Elts, u.Elem())
	case *types.Map:
		for _, elt := range c.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				s.implicit(kv.Key, u.Key(), ruleConstruction)
				s.implicit(kv.Value, u.Elem(), ruleConstruction)
			}
		}
	case *types.Struct:
		for i, elt := range c.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				// go/types records struct literal keys as uses of the field.
				if id, ok := kv.Key.(*ast.Ident); ok {
					if field, ok := s.pass.TypesInfo.Uses[id].(*types.Var); ok {
						s.implicit(kv.Value, field.Type(), ruleConstruction)
					}
				}
			} else if i < u.NumFields() {
				s.implicit(elt, u.Field(i).Type(), ruleConstruction)
			}
		}
	}
}

// elements checks slice or array literal elements, which may be keyed by index.
func (s *analyzerState) elements(elts []ast.Expr, elem types.Type) {
	for _, elt := range elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}

		s.implicit(elt, elem, ruleConstruction)
	}
}

func (s *analyzerState) binary(b *ast.BinaryExpr) {
	if b.Op == token.SHL || b.Op == token.SHR {
		// A shift count never takes the type of the shifted operand, and an
		// untyped constant shifted by a protected count takes its type from
		// the context, which the enclosing use site checks.
		return
	}

	// An untyped operand, as in (1 << u) + 1, has the protected type only
	// because of the context. The enclosing use site reports the whole
	// expression, so only a genuinely protected operand is checked here.
	x, y := s.pass.TypesInfo.TypeOf(b.X), s.pass.TypesInfo.TypeOf(b.Y)
	if s.protected(x) != nil && !s.untyped(b.X) {
		s.implicit(b.Y, x, ruleOperation)
	}

	if s.protected(y) != nil && !s.untyped(b.Y) {
		s.implicit(b.X, y, ruleOperation)
	}
}

// incDec reports x++ and x--, which add the untyped constant 1 to x.
func (s *analyzerState) incDec(stmt *ast.IncDecStmt) {
	p := s.protected(s.pass.TypesInfo.TypeOf(stmt.X))
	if p == nil {
		return
	}

	s.issue(stmt.TokPos, ruleOperation, fmt.Sprintf("%s applies an untyped constant to protected type %s", stmt.Tok, p.name.Name()), s.current)
}

func (s *analyzerState) valueSwitch(sw *ast.SwitchStmt) {
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
			s.implicit(expr, t, ruleOperation)
		}
	}
}

// implicit checks expr where its value is implicitly converted to target.
func (s *analyzerState) implicit(expr ast.Expr, target types.Type, rule string) {
	s.implicitUntyped(expr, target, rule)
	s.assignment(expr.Pos(), s.pass.TypesInfo.TypeOf(expr), target)
}

// implicitTuple checks the values of a multi-valued expression, such as a
// call or a comma-ok expression, where they are assigned to targets.
func (s *analyzerState) implicitTuple(expr ast.Expr, targets []types.Type) {
	tuple, ok := s.pass.TypesInfo.TypeOf(expr).(*types.Tuple)
	if !ok {
		return
	}

	for i := 0; i < tuple.Len() && i < len(targets); i++ {
		s.assignment(expr.Pos(), tuple.At(i).Type(), targets[i])
	}
}

// assignment checks the implicit conversion of a value of type source to
// target. A value of an unnamed array, slice, or map type is assignable to
// a defined type with the same underlying type and vice versa, so it
// constructs or extracts a protected type without a conversion. As with an
// explicit conversion, marked functions are trusted.
func (s *analyzerState) assignment(pos token.Pos, source, target types.Type) {
	if source == nil || target == nil || types.Identical(source, target) || !types.Identical(source.Underlying(), target.Underlying()) {
		return
	}

	if p := s.protected(target); p != nil {
		if !s.mayConstruct(p) {
			s.constructionIssue(pos, "implicit", p)
		}

		return
	}

	if p := s.protected(source); p != nil && !s.mayExtract(p) {
		s.extractionIssue(pos, "implicit", p)
	}
}

// constructionIssue reports a construction of p outside its factories and
// points to the marker that permits it.
func (s *analyzerState) constructionIssue(pos token.Pos, kind string, p *protectedType) {
	s.issue(pos, ruleConstruction, fmt.Sprintf("%s construction of protected type %s; use a //govo:%s function", kind, p.name.Name(), directiveFactory), s.current)
}

// extractionIssue reports an extraction from p outside its converters and
// points to the marker that permits it.
func (s *analyzerState) extractionIssue(pos token.Pos, kind string, p *protectedType) {
	s.issue(pos, ruleExtraction, fmt.Sprintf("%s extraction from protected type %s; use a //govo:%s function", kind, p.name.Name(), directiveConverter), s.current)
}

func (s *analyzerState) typesOf(exprs []ast.Expr) []types.Type {
	ts := make([]types.Type, len(exprs))
	for i, expr := range exprs {
		ts[i] = s.pass.TypesInfo.TypeOf(expr)
	}

	return ts
}

func (s *analyzerState) implicitUntyped(expr ast.Expr, target types.Type, rule string) {
	p := s.protected(target)
	if p == nil || !s.untyped(expr) {
		return
	}

	kind := "expression"
	if s.pass.TypesInfo.Types[expr].Value != nil {
		kind = "constant"
	}

	s.issue(expr.Pos(), rule, fmt.Sprintf("untyped %s used as protected type %s", kind, p.name.Name()), s.current)
}

// untyped reports whether expr is untyped before it is converted to the
// type of its context. go/types records that contextual type, which may be
// a protected type, so the expression's ingredients are inspected instead.
// Besides untyped constants, comparisons and shifts of untyped constants by
// non-constant counts produce untyped values.
func (s *analyzerState) untyped(expr ast.Expr) bool {
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
		return s.untypedConstantObject(e)
	case *ast.SelectorExpr:
		// A named constant declared in another package.
		return s.untypedConstantObject(e.Sel)
	case *ast.CallExpr:
		return s.untypedBuiltinCall(e)
	}

	return false
}

func (s *analyzerState) untypedConstantObject(id *ast.Ident) bool {
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
func (s *analyzerState) untypedBuiltinCall(c *ast.CallExpr) bool {
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

// elementCopy reports whether the i-th argument of the built-in call c only
// supplies elements to copy, as the arguments of copy and the argument
// spread by append(s, x...) do. Element operations are outside the rules,
// so such an argument is not reported merely because its type shares the
// underlying type of the parameter. Other arguments, such as the value in
// append(lists, codes), are converted as a whole and checked as usual.
func elementCopy(builtin *types.Builtin, c *ast.CallExpr, i int) bool {
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
func (s *analyzerState) builtin(fun ast.Expr) *types.Builtin {
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
