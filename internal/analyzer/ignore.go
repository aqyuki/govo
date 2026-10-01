package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"slices"
	"strings"
)

// ignoreDirective is a //govo:ignore directive and the rules it suppressed.
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

// ignoreList holds the ignore directives of a package.
//
//declscope:package // state holds the list
type ignoreList []*ignoreDirective

// parseIgnore validates an ignore directive, whose rule IDs are ids, and
// records it.
//
//declscope:package // directive.go hands ignore directives to it
func (s *state) parseIgnore(file *ast.File, comment *ast.Comment, ids, reason string) {
	ignore := &ignoreDirective{
		pos:      comment.Pos(),
		line:     s.pass.Fset.Position(comment.Pos()).Line,
		trailing: s.trailingIgnore(file, comment),
		file:     file,
		rules:    make(map[string]bool),
		used:     make(map[string]bool),
		reason:   reason,
	}

	if ids == "" {
		ignore.all = true
	} else {
		for id := range strings.SplitSeq(ids, ",") {
			id = strings.TrimSpace(id)
			switch {
			case id == ruleUnusedIgnore:
				// Unused ignore directives are found after suppression.
				s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("ignore rule %s cannot be suppressed", id), file)
			case slices.Contains(knownRules, id):
				ignore.rules[id] = true
			default:
				s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("unknown ignore rule %q", id), file)
			}
		}
	}

	if !ignore.trailing {
		ignore.target = nextIgnoreTarget(file, comment)
		if ignore.target == nil {
			s.issue(comment.Pos(), ruleInvalidDirective, "ignore is not followed by a statement or declaration", file)
			return
		}
	}

	if ignore.reason == "" && s.config.Ignore.MissingReason == configMissingReasonError {
		s.issue(comment.Pos(), ruleMissingReason, "ignore directive has no reason", file)
	}

	s.ignores = append(s.ignores, ignore)
}

// trailingIgnore reports whether code precedes the ignore directive comment
// on its line.
func (s *state) trailingIgnore(f *ast.File, comment *ast.Comment) bool {
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
				list = ignoreCandidates(n.Specs)
			}
		case *ast.BlockStmt:
			list = ignoreCandidates(n.List)
		case *ast.CaseClause:
			list = ignoreCandidates(n.Body)
		case *ast.CommClause:
			list = ignoreCandidates(n.Body)
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

// ignoreCandidates converts list to the candidates for an ignore target.
func ignoreCandidates[N ast.Node](list []N) []ast.Node {
	out := make([]ast.Node, len(list))
	for i, n := range list {
		out[i] = n
	}

	return out
}

// applyIgnores reports whether an ignore directive suppresses problem, and
// records the use in every directive that does.
//
//declscope:package // analyzer.go asks it before reporting each issue
func (s *state) applyIgnores(problem issue) bool {
	suppressed := false

	for _, ignore := range s.ignores {
		if ignore.file != problem.file || (!ignore.all && !ignore.rules[problem.rule]) {
			continue
		}

		if !s.ignoreApplies(ignore, problem.pos) {
			continue
		}

		ignore.used[problem.rule] = true
		suppressed = true
	}

	return suppressed
}

// reportUnusedIgnores reports the ignore directives and rules that
// suppressed nothing. It runs after every issue has been applied.
//
//declscope:package // analyzer.go runs it after reporting the issues
func (s *state) reportUnusedIgnores() {
	for _, ignore := range s.ignores {
		if s.skipFile(ignore.file) {
			continue
		}

		if ignore.all && len(ignore.used) == 0 {
			s.pass.Reportf(ignore.pos, "%s: unused ignore directive", ruleUnusedIgnore)
		}

		for _, rule := range slices.Sorted(maps.Keys(ignore.rules)) {
			if !ignore.used[rule] {
				s.pass.Reportf(ignore.pos, "%s: unused ignore rule %s", ruleUnusedIgnore, rule)
			}
		}
	}
}

func (s *state) ignoreApplies(ignore *ignoreDirective, pos token.Pos) bool {
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
	return inIgnoreTargetHeader(target, pos) && !inIgnoreTargetFuncLitBody(target, pos)
}

// inIgnoreTargetHeader reports whether pos, which is inside target, is
// outside the body of a statement or declaration that has one.
func inIgnoreTargetHeader(target ast.Node, pos token.Pos) bool {
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

// inIgnoreTargetFuncLitBody reports whether pos is inside the body of a
// function literal in root, an ignore target.
func inIgnoreTargetFuncLitBody(root ast.Node, pos token.Pos) bool {
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
