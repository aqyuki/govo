package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"
)

// protectedFact marks a protected type for the packages that import it.
//
//declscope:package // analyzer.go declares it as the analyzer's fact type
type protectedFact struct{}

func (*protectedFact) AFact() {}

// protectedType is a type marked with //govo:protect, declared in this
// package or imported.
//
//declscope:package // every check judges values against it
type protectedType struct {
	name *types.TypeName
	file *ast.File // nil for an imported protected type
}

// protectedTypes holds the protected types that a pass knows of.
//
//declscope:package // state holds it
type protectedTypes struct {
	// local holds the protected types declared in this package.
	//
	//declscope:private
	local map[*types.TypeName]*protectedType
	// byFile holds the protected types declared in each file.
	//
	//declscope:private
	byFile map[*ast.File][]*protectedType
	// imported caches fact lookups; a nil value records a type that is not protected.
	//
	//declscope:private
	imported map[*types.TypeName]*protectedType
	// comments holds //govo:protect comments attached to a type declaration.
	//
	//declscope:private
	comments map[*ast.Comment]bool
}

// newProtectedTypes returns an empty protectedTypes.
//
//declscope:package // analyzer.go creates the protectedTypes of each pass
func newProtectedTypes() protectedTypes {
	return protectedTypes{
		local:    make(map[*types.TypeName]*protectedType),
		byFile:   make(map[*ast.File][]*protectedType),
		imported: make(map[*types.TypeName]*protectedType),
		comments: make(map[*ast.Comment]bool),
	}
}

// collectProtect records the protected types that f declares, and exports a
// fact for each.
//
//declscope:package // analyzer.go collects every file before the checks run
func (s *state) collectProtect(f *ast.File) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)

			if !s.markProtect(protectedTypeDoc(gd, ts)) {
				continue
			}

			obj, ok := s.pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
			if !ok || obj.IsAlias() {
				s.issue(ts.Name.Pos(), ruleInvalidDirective, "protect requires a defined type", f)
				continue
			}

			if !protectableUnderlying(obj.Type()) {
				s.issue(ts.Name.Pos(), ruleInvalidDirective, "protect requires a basic, array, slice, or map underlying type", f)
				continue
			}

			p := &protectedType{name: obj, file: f}

			s.protects.local[obj] = p
			s.protects.byFile[f] = append(s.protects.byFile[f], p)

			// Unexported types need facts too: their values can reach other
			// packages through exported functions, fields, and aliases.
			s.pass.ExportObjectFact(obj, new(protectedFact))
		}
	}
}

// protectedTypeDoc returns the doc comment that applies to ts. A declaration's
// doc comment applies to its spec only when the declaration is not grouped.
func protectedTypeDoc(gd *ast.GenDecl, ts *ast.TypeSpec) *ast.CommentGroup {
	if ts.Doc == nil && gd.Lparen == token.NoPos {
		return gd.Doc
	}

	return ts.Doc
}

// markProtect records every protect directive in group as attached and
// reports whether one of them is valid. collectDirectives reports arguments.
func (s *state) markProtect(group *ast.CommentGroup) bool {
	if group == nil {
		return false
	}

	protect := false

	for _, c := range group.List {
		command, args, _, ok := parseDirective(c.Text)
		if !ok || command != directiveProtect {
			continue
		}

		s.protects.comments[c] = true
		protect = protect || args == ""
	}

	return protect
}

func protectableUnderlying(t types.Type) bool {
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsUntyped == 0 && u.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
	case *types.Array, *types.Slice, *types.Map:
		return true
	}

	return false
}

// protected returns the protected type that t names, or nil.
//
//declscope:package // every check asks it whether a type is protected
func (s *state) protected(t types.Type) *protectedType {
	if t == nil {
		return nil
	}

	name, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return nil
	}

	obj := name.Obj()
	if p := s.protects.local[obj]; p != nil {
		return p
	}

	if obj.Pkg() == nil || obj.Pkg() == s.pass.Pkg {
		return nil
	}

	if p, ok := s.protects.imported[obj]; ok {
		return p
	}

	var p *protectedType
	if s.pass.ImportObjectFact(obj, new(protectedFact)) {
		p = &protectedType{name: obj}
	}

	s.protects.imported[obj] = p

	return p
}

// protectAttached reports whether comment is a //govo:protect directive
// attached to a type declaration.
//
//declscope:package // directive.go reports protect directives attached to nothing
func (s *state) protectAttached(comment *ast.Comment) bool {
	return s.protects.comments[comment]
}

// protectedIn returns the protected types declared in f.
//
//declscope:package // directive.go resolves the type names of markers in it
func (s *state) protectedIn(f *ast.File) []*protectedType {
	return s.protects.byFile[f]
}
