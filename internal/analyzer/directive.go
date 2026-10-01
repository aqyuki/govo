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

// parseDirective splits a //govo: comment into its command, its arguments,
// and the reason that follows them in the golangci-lint "// reason" form.
//
//declscope:package // protect.go finds protect directives with it
func parseDirective(text string) (command, args, reason string, ok bool) {
	body, ok := strings.CutPrefix(text, directivePrefix)
	if !ok {
		return "", "", "", false
	}

	body, reason, _ = strings.Cut(body, "//")
	reason = strings.TrimSpace(reason)

	body = strings.TrimSpace(body)
	if i := strings.IndexFunc(body, unicode.IsSpace); i >= 0 {
		return body[:i], strings.TrimSpace(body[i:]), reason, true
	}

	return body, "", reason, true
}

// collectDirectives validates the directives of f and records the
// permissions and ignore directives they declare.
//
//declscope:package // analyzer.go runs it for each file
func (s *state) collectDirectives(f *ast.File) {
	// A scalar directive depends on the factory permission that the markers
	// of its function grant, wherever they appear in the doc comment.
	type scalar struct {
		fn      *ast.FuncDecl
		comment *ast.Comment
	}

	var (
		scalars []scalar
		markers []directiveMarker
	)

	funcs := directiveFuncDocs(f)

	for _, group := range f.Comments {
		fn := funcs[group]

		for _, comment := range group.List {
			command, args, reason, ok := parseDirective(comment.Text)
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
				markers = append(markers, s.validateMarkerDirective(f, fn, comment, command, args)...)
			case directiveScalar:
				scalars = append(scalars, scalar{fn: fn, comment: comment})
			case directiveIgnore:
				s.parseIgnore(f, comment, args, reason)
			default:
				s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("unknown govo directive %q", command), f)
			}
		}
	}

	for _, sc := range scalars {
		_, args, _, _ := parseDirective(sc.comment.Text)
		markers = append(markers, s.validateScalarDirective(f, sc.fn, sc.comment, args)...)
	}

	s.reportRedundantDirectives(f, markers)
}

// directiveMarker is a protected type that a valid factory, converter, op,
// or scalar directive names for its function.
type directiveMarker struct {
	fn      *ast.FuncDecl
	comment *ast.Comment
	command string
	p       *protectedType
}

// reportRedundantDirectives reports the types that a directive names for a
// function whose other directives already grant the same permission: a
// marker or scalar that repeats the same type, and a factory or converter
// of a type that an op of the function names. The markers of each command
// are in source order, so the later of two repetitions is reported.
func (s *state) reportRedundantDirectives(f *ast.File, markers []directiveMarker) {
	type named struct {
		comment *ast.Comment
		p       *protectedType
	}

	reported := make(map[named]bool)

	for i, m := range markers {
		if reported[named{m.comment, m.p}] {
			continue
		}

		same := func(command string) func(directiveMarker) bool {
			return func(o directiveMarker) bool { return o.fn == m.fn && o.p == m.p && o.command == command }
		}

		var msg string

		switch {
		case (m.command == directiveFactory || m.command == directiveConverter) && slices.ContainsFunc(markers, same(directiveOp)):
			msg = fmt.Sprintf("%s: %s is already permitted by op %[2]s", m.command, m.p.name.Name())
		case slices.ContainsFunc(markers[:i], same(m.command)):
			msg = fmt.Sprintf("%s: %s is named more than once for this function", m.command, m.p.name.Name())
		default:
			continue
		}

		reported[named{m.comment, m.p}] = true
		s.issue(m.comment.Pos(), ruleRedundantDirective, msg, f)
	}
}

// directiveFuncDocs maps the doc comment of each function declared in f
// to the function, so that a directive finds the function it is attached
// to from its comment group.
func directiveFuncDocs(f *ast.File) map[*ast.CommentGroup]*ast.FuncDecl {
	funcs := make(map[*ast.CommentGroup]*ast.FuncDecl)

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Doc != nil {
			funcs[fn.Doc] = fn
		}
	}

	return funcs
}

// directiveTargets returns the protected types that a function directive
// names. fn is the function that the directive is attached to, or nil when
// its comment is not a function's doc comment. It reports the directive and
// returns false when either cannot be determined. A name that is not a
// protected type declared in f is reported and left out of the result.
func (s *state) directiveTargets(f *ast.File, fn *ast.FuncDecl, comment *ast.Comment, command, args string) ([]*protectedType, bool) {
	if fn == nil {
		s.issue(comment.Pos(), ruleInvalidDirective, command+" requires a function or method", f)
		return nil, false
	}

	protected := s.protectedIn(f)

	var names []string

	if args != "" {
		names = strings.Fields(args)
	} else if len(protected) == 1 {
		names = []string{protected[0].name.Name()}
	} else {
		s.issue(comment.Pos(), ruleInvalidDirective, command+" requires a type name unless this file declares exactly one protected type", f)
		return nil, false
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

	return targets, true
}

// validateMarkerDirective checks the shape of a function marked as a factory,
// converter, or op of each named type and grants the permission for each
// type that passes. An op both constructs and extracts, so it must pass
// the checks of both a factory and a converter. It returns the types that
// pass.
func (s *state) validateMarkerDirective(f *ast.File, fn *ast.FuncDecl, comment *ast.Comment, command, args string) []directiveMarker {
	targets, ok := s.directiveTargets(f, fn, comment, command, args)
	if !ok {
		return nil
	}

	sig, _ := s.pass.TypesInfo.TypeOf(fn.Name).(*types.Signature)
	if sig == nil {
		return nil
	}

	var markers []directiveMarker

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

		markers = append(markers, directiveMarker{fn: fn, comment: comment, command: command, p: p})
	}

	return markers
}

// validateScalarDirective checks a scalar directive, which permits untyped constants
// as scalars that multiply or divide the named numeric types. Scaling
// yields a new value, so the function must be permitted to construct each
// type, as a factory or op of it. It returns the types that pass.
func (s *state) validateScalarDirective(f *ast.File, fn *ast.FuncDecl, comment *ast.Comment, args string) []directiveMarker {
	targets, ok := s.directiveTargets(f, fn, comment, directiveScalar, args)
	if !ok {
		return nil
	}

	var markers []directiveMarker

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
		markers = append(markers, directiveMarker{fn: fn, comment: comment, command: directiveScalar, p: p})
	}

	return markers
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
