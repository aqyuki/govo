package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"slices"
	"strings"
	"unicode"
)

const govoDirectivePrefix = "//govo:"

const (
	directiveProtect   = "protect"
	directiveFactory   = "factory"
	directiveConverter = "converter"
	directiveIgnore    = "ignore"
)

type ignoreDirective struct {
	pos      token.Pos
	line     int
	trailing bool
	target   ast.Node
	rules    map[string]bool
	used     map[string]bool
	all      bool
	reason   string
	file     *ast.File
}

// parseDirective splits a //govo: comment into its command and arguments.
func parseDirective(text string) (command, args string, ok bool) {
	body, ok := strings.CutPrefix(text, govoDirectivePrefix)
	if !ok {
		return "", "", false
	}

	body = strings.TrimSpace(body)
	if i := strings.IndexFunc(body, unicode.IsSpace); i >= 0 {
		return body[:i], strings.TrimSpace(body[i:]), true
	}

	return body, "", true
}

func (s *analyzerState) collectDirectives(f *ast.File) {
	for _, group := range f.Comments {
		for _, comment := range group.List {
			command, args, ok := parseDirective(comment.Text)
			if !ok {
				continue
			}

			switch command {
			case directiveProtect:
				if args != "" {
					s.issue(comment.Pos(), ruleDirective, "protect does not accept arguments", f)
				}

				if !s.protectComments[comment] {
					s.issue(comment.Pos(), ruleDirective, "protect is not attached to a type declaration", f)
				}
			case directiveFactory, directiveConverter:
				s.validateAPI(f, comment, command, args)
			case directiveIgnore:
				s.parseIgnore(f, comment, args)
			default:
				s.issue(comment.Pos(), ruleDirective, fmt.Sprintf("unknown govo directive %q", command), f)
			}
		}
	}
}

func commentInGroup(group *ast.CommentGroup, comment *ast.Comment) bool {
	return group != nil && slices.Contains(group.List, comment)
}

func (s *analyzerState) validateAPI(f *ast.File, comment *ast.Comment, command, args string) {
	var fn *ast.FuncDecl

	for _, decl := range f.Decls {
		candidate, ok := decl.(*ast.FuncDecl)
		if ok && commentInGroup(candidate.Doc, comment) {
			fn = candidate
			break
		}
	}

	if fn == nil || !fn.Name.IsExported() {
		s.issue(comment.Pos(), ruleDirective, command+" requires an exported function or method", f)
		return
	}

	protected := s.files[f].protected

	var names []string

	if args != "" {
		names = strings.Fields(args)
	} else if len(protected) == 1 {
		names = []string{protected[0].name.Name()}
	} else {
		s.issue(comment.Pos(), ruleDirective, command+" requires a type name unless this file declares exactly one protected type", f)
		return
	}

	sig, _ := s.pass.TypesInfo.TypeOf(fn.Name).(*types.Signature)

	for _, name := range names {
		i := slices.IndexFunc(protected, func(p *protectedType) bool { return p.name.Name() == name })
		if i < 0 {
			s.issue(comment.Pos(), ruleDirective, fmt.Sprintf("%s: %s is not a protected type declared in this file", command, name), f)
			continue
		}

		if sig == nil {
			continue
		}

		t := protected[i].name.Type()

		if command == directiveFactory {
			if !returnsType(sig, t) {
				s.issue(comment.Pos(), ruleDirective, fmt.Sprintf("%s: %s is not returned by this API", command, name), f)
			}
		} else if !acceptsType(sig, t) {
			s.issue(comment.Pos(), ruleDirective, fmt.Sprintf("%s: %s is not accepted by this API", command, name), f)
		}
	}
}

func returnsType(sig *types.Signature, t types.Type) bool {
	for v := range sig.Results().Variables() {
		if types.Identical(v.Type(), t) {
			return true
		}
	}

	return false
}

func acceptsType(sig *types.Signature, t types.Type) bool {
	if sig.Recv() != nil && apiInputMatches(sig.Recv().Type(), t) {
		return true
	}

	for v := range sig.Params().Variables() {
		if apiInputMatches(v.Type(), t) {
			return true
		}
	}

	return false
}

func apiInputMatches(input, protected types.Type) bool {
	if types.Identical(input, protected) {
		return true
	}

	ptr, ok := input.(*types.Pointer)

	return ok && types.Identical(ptr.Elem(), protected)
}

func (s *analyzerState) parseIgnore(file *ast.File, comment *ast.Comment, args string) {
	ids, reason, _ := strings.Cut(args, "//")
	ids = strings.TrimSpace(ids)

	ignore := &ignoreDirective{
		pos:      comment.Pos(),
		line:     s.pass.Fset.Position(comment.Pos()).Line,
		trailing: s.commentTrailing(file, comment),
		file:     file,
		rules:    make(map[string]bool),
		used:     make(map[string]bool),
		reason:   strings.TrimSpace(reason),
	}

	if ids == "" {
		ignore.all = true
	} else {
		for id := range strings.SplitSeq(ids, ",") {
			id = strings.TrimSpace(id)
			if slices.Contains(knownRules, id) {
				ignore.rules[id] = true
			} else {
				s.issue(comment.Pos(), ruleDirective, fmt.Sprintf("unknown ignore rule %q", id), file)
			}
		}
	}

	if !ignore.trailing {
		ignore.target = nextIgnoreTarget(file, comment)
		if ignore.target == nil {
			s.issue(comment.Pos(), ruleDirective, "ignore is not followed by a statement or declaration", file)
			return
		}
	}

	if ignore.reason == "" && s.config.Ignore.MissingReason == missingReasonError {
		s.issue(comment.Pos(), ruleDirective, "ignore directive has no reason", file)
	}

	s.ignore = append(s.ignore, ignore)
}

// commentTrailing reports whether code precedes comment on its line.
func (s *analyzerState) commentTrailing(f *ast.File, comment *ast.Comment) bool {
	src, err := s.source(f)
	if err != nil {
		return false
	}

	tf := s.pass.Fset.File(comment.Pos())
	start := tf.Offset(tf.LineStart(tf.Line(comment.Pos())))

	end := tf.Offset(comment.Pos())
	if end > len(src) {
		return false
	}

	return strings.TrimSpace(string(src[start:end])) != ""
}

// source returns the contents of f, reading the file at most once.
func (s *analyzerState) source(f *ast.File) ([]byte, error) {
	info := s.files[f]
	if !info.srcLoaded {
		if s.pass.ReadFile != nil {
			info.src, info.srcErr = s.pass.ReadFile(info.name)
		} else {
			info.src, info.srcErr = os.ReadFile(info.name)
		}

		info.srcLoaded = true
	}

	return info.src, info.srcErr
}

// nextIgnoreTarget returns the declaration, spec, or statement that
// immediately follows comment in the innermost declaration list, spec
// group, block, or case clause containing it. It returns nil when the list
// has no such element or comment is inside an element, such as between the
// arguments of a call or at the end of a block.
func nextIgnoreTarget(f *ast.File, comment *ast.Comment) ast.Node {
	list := make([]ast.Node, 0, len(f.Decls))
	for _, decl := range f.Decls {
		list = append(list, decl)
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return false
		}

		if _, ok := n.(*ast.File); !ok && (comment.Pos() < n.Pos() || n.End() < comment.End()) {
			return false
		}

		switch n := n.(type) {
		case *ast.GenDecl:
			if n.Lparen.IsValid() {
				list = nodes(n.Specs)
			}
		case *ast.BlockStmt:
			list = nodes(n.List)
		case *ast.CaseClause:
			list = nodes(n.Body)
		case *ast.CommClause:
			list = nodes(n.Body)
		}

		return true
	})

	for _, n := range list {
		if n.Pos() > comment.End() {
			return n
		}

		if n.End() > comment.Pos() {
			return nil
		}
	}

	return nil
}

func nodes[N ast.Node](list []N) []ast.Node {
	out := make([]ast.Node, len(list))
	for i, n := range list {
		out[i] = n
	}

	return out
}

func (s *analyzerState) reportIssues() {
	for _, problem := range s.issues {
		if s.skipFile(problem.file) {
			continue
		}

		suppressed := false

		for _, ignore := range s.ignore {
			if ignore.file != problem.file || (!ignore.all && !ignore.rules[problem.rule]) {
				continue
			}

			if !s.ignoreApplies(ignore, problem.pos) {
				continue
			}

			ignore.used[problem.rule] = true
			suppressed = true
		}

		if !suppressed {
			s.pass.Reportf(problem.pos, "%s: %s", problem.rule, problem.message)
		}
	}

	for _, ignore := range s.ignore {
		if s.skipFile(ignore.file) {
			continue
		}

		if ignore.all && len(ignore.used) == 0 {
			s.pass.Reportf(ignore.pos, "%s: unused ignore directive", ruleDirective)
		}

		for _, rule := range slices.Sorted(maps.Keys(ignore.rules)) {
			if !ignore.used[rule] {
				s.pass.Reportf(ignore.pos, "%s: unused ignore rule %s", ruleDirective, rule)
			}
		}
	}
}

func (s *analyzerState) ignoreApplies(ignore *ignoreDirective, pos token.Pos) bool {
	if ignore.trailing {
		return s.pass.Fset.Position(pos).Line == ignore.line
	}

	target := ignore.target
	if target == nil || pos < target.Pos() || pos > target.End() {
		return false
	}

	// A label does not change which statement the directive covers.
	for {
		labeled, ok := target.(*ast.LabeledStmt)
		if !ok {
			break
		}

		target = labeled.Stmt
	}

	// Bodies of function literals in the target are blocks, too.
	return inHeader(target, pos) && !inFuncLitBody(target, pos)
}

// inHeader reports whether pos, which is inside target, is outside the
// body of a statement or declaration that has one.
func inHeader(target ast.Node, pos token.Pos) bool {
	var body *ast.BlockStmt

	switch n := target.(type) {
	case *ast.CaseClause:
		return pos < n.Colon
	case *ast.CommClause:
		return pos < n.Colon
	case *ast.IfStmt:
		body = n.Body
	case *ast.ForStmt:
		body = n.Body
	case *ast.RangeStmt:
		body = n.Body
	case *ast.SwitchStmt:
		body = n.Body
	case *ast.TypeSwitchStmt:
		body = n.Body
	case *ast.SelectStmt:
		body = n.Body
	case *ast.FuncDecl:
		body = n.Body
	case *ast.BlockStmt:
		return false
	default:
		return true
	}

	return body == nil || pos < body.Lbrace
}

// inFuncLitBody reports whether pos is inside the body of a function
// literal in root.
func inFuncLitBody(root ast.Node, pos token.Pos) bool {
	found := false

	ast.Inspect(root, func(n ast.Node) bool {
		if found || n == nil || pos < n.Pos() || n.End() <= pos {
			return false
		}

		if lit, ok := n.(*ast.FuncLit); ok && pos >= lit.Body.Lbrace {
			found = true
		}

		return !found
	})

	return found
}
