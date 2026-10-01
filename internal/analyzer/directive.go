package analyzer

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"
	"unicode"
)

const directivePrefix = "//govo:"

// Commands of //govo: directives.
//
//declscope:package // protect.go finds protect, and diagnostics name the markers
const (
	directiveProtect   = "protect"
	directiveFactory   = "factory"
	directiveConverter = "converter"
	//declscope:private
	directiveOp     = "op"
	directiveScalar = "scalar"
	//declscope:private
	directiveIgnore = "ignore"
)

// parseDirective splits a //govo: comment into its command and arguments.
//
//declscope:package // protect.go finds protect directives with it
func parseDirective(text string) (command, args string, ok bool) {
	body, ok := strings.CutPrefix(text, directivePrefix)
	if !ok {
		return "", "", false
	}

	body = strings.TrimSpace(body)
	if i := strings.IndexFunc(body, unicode.IsSpace); i >= 0 {
		return body[:i], strings.TrimSpace(body[i:]), true
	}

	return body, "", true
}

// collectDirectives validates the directives of f and records the
// permissions and ignore directives they declare.
//
//declscope:package // analyzer.go runs it for each file
func (s *state) collectDirectives(f *ast.File) {
	// A scalar directive depends on the factory permission that the markers
	// of its function grant, wherever they appear in the doc comment.
	var scalars []*ast.Comment

	for _, group := range f.Comments {
		for _, comment := range group.List {
			command, args, ok := parseDirective(comment.Text)
			if !ok {
				continue
			}

			switch command {
			case directiveProtect:
				if args != "" {
					s.issue(comment.Pos(), ruleInvalidDirective, "protect does not accept arguments", f)
				}

				if !s.protectAttached(comment) {
					s.issue(comment.Pos(), ruleInvalidDirective, "protect is not attached to a type declaration", f)
				}
			case directiveFactory, directiveConverter, directiveOp:
				s.validateMarkerDirective(f, comment, command, args)
			case directiveScalar:
				scalars = append(scalars, comment)
			case directiveIgnore:
				s.parseIgnore(f, comment, args)
			default:
				s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("unknown govo directive %q", command), f)
			}
		}
	}

	for _, comment := range scalars {
		_, args, _ := parseDirective(comment.Text)
		s.validateScalarDirective(f, comment, args)
	}
}

// directiveTargets returns the function that a function directive is
// attached to and the protected types that it names. It reports the
// directive and returns a nil function when either cannot be determined.
// A name that is not a protected type declared in f is reported and left
// out of the result.
func (s *state) directiveTargets(f *ast.File, comment *ast.Comment, command, args string) (*ast.FuncDecl, []*protectedType) {
	var fn *ast.FuncDecl

	for _, decl := range f.Decls {
		candidate, ok := decl.(*ast.FuncDecl)
		if ok && candidate.Doc != nil && slices.Contains(candidate.Doc.List, comment) {
			fn = candidate
			break
		}
	}

	if fn == nil {
		s.issue(comment.Pos(), ruleInvalidDirective, command+" requires a function or method", f)
		return nil, nil
	}

	protected := s.protectedIn(f)

	var names []string

	if args != "" {
		names = strings.Fields(args)
	} else if len(protected) == 1 {
		names = []string{protected[0].name.Name()}
	} else {
		s.issue(comment.Pos(), ruleInvalidDirective, command+" requires a type name unless this file declares exactly one protected type", f)
		return nil, nil
	}

	var targets []*protectedType

	for _, name := range names {
		i := slices.IndexFunc(protected, func(p *protectedType) bool { return p.name.Name() == name })
		if i < 0 {
			s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("%s: %s is not a protected type declared in this file", command, name), f)
			continue
		}

		targets = append(targets, protected[i])
	}

	return fn, targets
}

// validateMarkerDirective checks the shape of a function marked as a factory,
// converter, or op of each named type and grants the permission for each
// type that passes. An op both constructs and extracts, so it must pass
// the checks of both a factory and a converter.
func (s *state) validateMarkerDirective(f *ast.File, comment *ast.Comment, command, args string) {
	fn, targets := s.directiveTargets(f, comment, command, args)
	if fn == nil {
		return
	}

	sig, _ := s.pass.TypesInfo.TypeOf(fn.Name).(*types.Signature)
	if sig == nil {
		return
	}

	for _, p := range targets {
		if command != directiveFactory && !directiveFuncAccepts(sig, p) {
			s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("%s: %s is not accepted by this API", command, p.name.Name()), f)
			continue
		}

		if command != directiveConverter && !directiveFuncReturns(sig, p) {
			s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("%s: %s is not returned by this API", command, p.name.Name()), f)
			continue
		}

		g := s.grants[fn]
		if g == nil {
			g = &grant{
				construct: make(map[*protectedType]bool),
				extract:   make(map[*protectedType]bool),
				scale:     make(map[*protectedType]bool),
			}
			s.grants[fn] = g
		}

		g.construct[p] = g.construct[p] || command != directiveConverter
		g.extract[p] = g.extract[p] || command != directiveFactory
	}
}

// validateScalarDirective checks a scalar directive, which permits untyped constants
// as scalars that multiply or divide the named numeric types. Scaling
// yields a new value, so the function must be permitted to construct each
// type, as a factory or op of it.
func (s *state) validateScalarDirective(f *ast.File, comment *ast.Comment, args string) {
	fn, targets := s.directiveTargets(f, comment, directiveScalar, args)
	if fn == nil {
		return
	}

	for _, p := range targets {
		if basic, ok := p.name.Type().Underlying().(*types.Basic); !ok || basic.Info()&types.IsNumeric == 0 {
			s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("%s: %s does not have a numeric underlying type", directiveScalar, p.name.Name()), f)
			continue
		}

		g := s.grants[fn]
		if g == nil || !g.construct[p] {
			s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("%s: this API is not a factory or op of %s", directiveScalar, p.name.Name()), f)
			continue
		}

		g.scale[p] = true
	}
}

// directiveFuncReturns reports whether the function a directive is attached
// to, with signature sig, returns p or an instance of it.
func directiveFuncReturns(sig *types.Signature, p *protectedType) bool {
	for v := range sig.Results().Variables() {
		if directiveNamesType(v.Type(), p) {
			return true
		}
	}

	return false
}

// directiveFuncAccepts reports whether the function a directive is attached
// to, with signature sig, takes p or *p, or an instance of it, as its
// receiver or a parameter.
func directiveFuncAccepts(sig *types.Signature, p *protectedType) bool {
	if sig.Recv() != nil && directiveFuncInput(sig.Recv().Type(), p) {
		return true
	}

	for v := range sig.Params().Variables() {
		if directiveFuncInput(v.Type(), p) {
			return true
		}
	}

	return false
}

// directiveFuncInput reports whether a receiver or parameter of type input
// takes a value of p, directly or through a pointer.
func directiveFuncInput(input types.Type, p *protectedType) bool {
	if directiveNamesType(input, p) {
		return true
	}

	ptr, ok := types.Unalias(input).(*types.Pointer)

	return ok && directiveNamesType(ptr.Elem(), p)
}

// directiveNamesType reports whether t is p or an instance of it. Any
// instance counts, whether its type arguments are type parameters of the
// function, as in List[T], or concrete types, as in List[int].
func directiveNamesType(t types.Type, p *protectedType) bool {
	named, ok := types.Unalias(t).(*types.Named)

	return ok && named.Origin().Obj() == p.name
}
