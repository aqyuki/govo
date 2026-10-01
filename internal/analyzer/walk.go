package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ast/edge"
	"golang.org/x/tools/go/ast/inspector"
)

// walkedNodes are the node types that walkNode checks.
var walkedNodes = []ast.Node{
	(*ast.FuncLit)(nil),
	(*ast.CallExpr)(nil),
	(*ast.GenDecl)(nil),
	(*ast.AssignStmt)(nil),
	(*ast.IncDecStmt)(nil),
	(*ast.ReturnStmt)(nil),
	(*ast.CompositeLit)(nil),
	(*ast.SendStmt)(nil),
	(*ast.IndexExpr)(nil),
	(*ast.BinaryExpr)(nil),
	(*ast.SwitchStmt)(nil),
}

// walkFile checks the declarations of the file at file, each with the
// permission that its markers grant.
//
//declscope:package // analyzer.go runs it for each file
func (s *state) walkFile(file inspector.Cursor) {
	for decl := range file.Children() {
		if _, ok := decl.Node().(ast.Decl); !ok {
			continue
		}

		s.grant = nil
		if fn, ok := decl.Node().(*ast.FuncDecl); ok {
			s.grant = s.grants[fn]
		}

		s.walkNode(decl, nil)
	}

	s.grant = nil
}

func (s *state) walkNode(root inspector.Cursor, signature *types.Signature) {
	if fn, ok := root.Node().(*ast.FuncDecl); ok {
		sig, _ := s.pass.TypesInfo.TypeOf(fn.Name).(*types.Signature)
		signature = sig
	}

	root.Inspect(walkedNodes, func(cursor inspector.Cursor) bool {
		switch n := cursor.Node().(type) {
		case *ast.FuncLit:
			if sig, ok := s.pass.TypesInfo.TypeOf(n).(*types.Signature); ok {
				s.walkNode(cursor.ChildAt(edge.FuncLit_Body, -1), sig)
				return false
			}

		case *ast.CallExpr:
			s.walkCall(n)

		case *ast.GenDecl:
			if n.Tok == token.CONST || n.Tok == token.VAR {
				s.walkValueDecl(n)
			}

			if n.Tok == token.CONST {
				// The type declaration file may declare protected constants
				// with explicit conversions, as in const A = Code("A").
				s.inConst = true
				for i := range n.Specs {
					s.walkNode(cursor.ChildAt(edge.GenDecl_Specs, i), signature)
				}

				s.inConst = false

				return false
			}

		case *ast.AssignStmt:
			switch n.Tok {
			case token.ASSIGN, token.DEFINE:
				if len(n.Lhs) > 1 && len(n.Rhs) == 1 {
					targets := make([]types.Type, len(n.Lhs))
					for i, lhs := range n.Lhs {
						targets[i] = s.pass.TypesInfo.TypeOf(lhs)
					}

					s.checkTupleConversion(n.Rhs[0], targets)

					break
				}

				for i, rhs := range n.Rhs {
					if i < len(n.Lhs) {
						s.checkImplicitConversion(rhs, s.pass.TypesInfo.TypeOf(n.Lhs[i]), ruleConstruction)
					}
				}
			case token.SHL_ASSIGN, token.SHR_ASSIGN:
				// A shift count never takes the type of the shifted operand.
			default:
				if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
					scaling := n.Tok == token.MUL_ASSIGN || n.Tok == token.QUO_ASSIGN
					s.checkOperationOperand(n.Rhs[0], s.pass.TypesInfo.TypeOf(n.Lhs[0]), scaling)
				}
			}

		case *ast.IncDecStmt:
			s.checkIncDecOperation(n)

		case *ast.ReturnStmt:
			if signature != nil {
				results := signature.Results()
				if results.Len() > 1 && len(n.Results) == 1 {
					targets := make([]types.Type, results.Len())
					for i := range targets {
						targets[i] = results.At(i).Type()
					}

					s.checkTupleConversion(n.Results[0], targets)

					break
				}

				for i, expr := range n.Results {
					if i < signature.Results().Len() {
						s.checkImplicitConversion(expr, signature.Results().At(i).Type(), ruleConstruction)
					}
				}
			}

		case *ast.CompositeLit:
			s.walkComposite(n)

		case *ast.SendStmt:
			if t := s.pass.TypesInfo.TypeOf(n.Chan); t != nil {
				if ch, ok := t.Underlying().(*types.Chan); ok {
					s.checkImplicitConversion(n.Value, ch.Elem(), ruleConstruction)
				}
			}

		case *ast.IndexExpr:
			if t := s.pass.TypesInfo.TypeOf(n.X); t != nil {
				if m, ok := t.Underlying().(*types.Map); ok {
					s.checkImplicitConversion(n.Index, m.Key(), ruleConstruction)
				}
			}

		case *ast.BinaryExpr:
			s.checkBinaryOperation(n)

		case *ast.SwitchStmt:
			s.checkSwitchOperation(n)
		}

		return true
	})
}

func (s *state) walkCall(c *ast.CallExpr) {
	if tv, ok := s.pass.TypesInfo.Types[c.Fun]; ok && tv.IsType() {
		if len(c.Args) == 1 {
			s.checkExplicitConversion(c, tv.Type)
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

			s.checkTupleConversion(c.Args[0], targets)

			return
		}
	}

	for i, arg := range c.Args {
		t := paramType(i, c.Ellipsis.IsValid())
		if t == nil {
			continue
		}

		if builtinCopiesElements(builtin, c, i) {
			s.checkUntypedConversion(arg, t, ruleConstruction)
		} else {
			s.checkImplicitConversion(arg, t, ruleConstruction)
		}
	}
}

func (s *state) walkValueDecl(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		v := spec.(*ast.ValueSpec)

		if d.Tok == token.VAR && len(v.Names) > 1 && len(v.Values) == 1 {
			targets := make([]types.Type, len(v.Names))
			for i, name := range v.Names {
				if obj := s.pass.TypesInfo.Defs[name]; obj != nil {
					targets[i] = obj.Type()
				}
			}

			s.checkTupleConversion(v.Values[0], targets)

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
				s.checkImplicitConversion(v.Values[i], t, ruleConstruction)
			}
		}
	}
}

func (s *state) walkComposite(c *ast.CompositeLit) {
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

		s.constructionIssue(pos, "direct", t)
	}

	switch u := t.Underlying().(type) {
	case *types.Slice:
		s.walkElements(c.Elts, u.Elem())
	case *types.Array:
		s.walkElements(c.Elts, u.Elem())
	case *types.Map:
		for _, elt := range c.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				s.checkImplicitConversion(kv.Key, u.Key(), ruleConstruction)
				s.checkImplicitConversion(kv.Value, u.Elem(), ruleConstruction)
			}
		}
	case *types.Struct:
		for i, elt := range c.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				// go/types records struct literal keys as uses of the field.
				if id, ok := kv.Key.(*ast.Ident); ok {
					if field, ok := s.pass.TypesInfo.Uses[id].(*types.Var); ok {
						s.checkImplicitConversion(kv.Value, field.Type(), ruleConstruction)
					}
				}
			} else if i < u.NumFields() {
				s.checkImplicitConversion(elt, u.Field(i).Type(), ruleConstruction)
			}
		}
	}
}

// walkElements checks slice or array literal elements, which may be keyed by index.
func (s *state) walkElements(elts []ast.Expr, elem types.Type) {
	for _, elt := range elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}

		s.checkImplicitConversion(elt, elem, ruleConstruction)
	}
}
