package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"iter"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/ast/edge"
	"golang.org/x/tools/go/ast/inspector"
)

// ignoreDirective is a //govo:ignore directive and the rules it suppressed.
type ignoreDirective struct {
	pos      token.Pos
	end      token.Pos
	line     int
	trailing bool
	// target is the declaration, spec, or statement that a directive on its
	// own line covers. It is invalid for a trailing directive.
	target inspector.Cursor
	rules  map[string]bool
	used   map[string]bool
	// repeated holds the rule IDs listed more than once, in order.
	repeated []string
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
		end:      comment.End(),
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
				if ignore.rules[id] && !slices.Contains(ignore.repeated, id) {
					ignore.repeated = append(ignore.repeated, id)
				}

				ignore.rules[id] = true
			default:
				s.issue(comment.Pos(), ruleInvalidDirective, fmt.Sprintf("unknown ignore rule %q", id), file)
			}
		}
	}

	if !ignore.trailing {
		target, ok := nextIgnoreTarget(s.fileCursor(file), comment)
		if !ok {
			s.issue(comment.Pos(), ruleInvalidDirective, "ignore is not followed by a statement or declaration", file)
			return
		}

		ignore.target = target
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

// nextIgnoreTarget returns the cursor of the declaration, spec, or
// statement that immediately follows comment in the innermost declaration
// list, spec group, block, or case clause of file containing it. It reports
// false when the list has no such element or comment is inside an element,
// such as between the arguments of a call or at the end of a block.
func nextIgnoreTarget(file inspector.Cursor, comment *ast.Comment) (inspector.Cursor, bool) {
	parent, list := file, edge.File_Decls

	if inner, ok := file.FindByPos(comment.Pos(), comment.End()); ok {
	enclosing:
		for cursor := range inner.Enclosing() {
			switch n := cursor.Node().(type) {
			case *ast.GenDecl:
				if n.Lparen.IsValid() {
					parent, list = cursor, edge.GenDecl_Specs
					break enclosing
				}
			case *ast.BlockStmt:
				parent, list = cursor, edge.BlockStmt_List
				break enclosing
			case *ast.CaseClause:
				parent, list = cursor, edge.CaseClause_Body
				break enclosing
			case *ast.CommClause:
				parent, list = cursor, edge.CommClause_Body
				break enclosing
			}
		}
	}

	for candidate := range ignoreCandidates(parent, list) {
		n := candidate.Node()
		if n.Pos() > comment.End() {
			return candidate, true
		}

		if n.End() > comment.Pos() {
			break
		}
	}

	return inspector.Cursor{}, false
}

// ignoreCandidates returns the children of parent in the list that list
// names: the candidates for an ignore target, in order.
func ignoreCandidates(parent inspector.Cursor, list edge.Kind) iter.Seq[inspector.Cursor] {
	return func(yield func(inspector.Cursor) bool) {
		for child := range parent.Children() {
			if child.ParentEdgeKind() == list && !yield(child) {
				return
			}
		}
	}
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
	for i, ignore := range s.ignores {
		if s.skipFile(ignore.file) {
			continue
		}

		// A redundant directive or rule is reported as redundant, not as
		// unused, since removing it is the fix either way.
		redundantAll, redundantRules := s.redundantIgnore(i)

		if ignore.all && len(ignore.used) == 0 && redundantAll == "" {
			s.report(ignore.pos, ruleUnusedIgnore, "unused ignore directive")
		}

		for _, rule := range slices.Sorted(maps.Keys(ignore.rules)) {
			if !ignore.used[rule] && redundantRules[rule] == "" {
				s.report(ignore.pos, ruleUnusedIgnore, "unused ignore rule "+rule)
			}
		}
	}
}

// reportRedundantIgnores reports the ignore directives, and the rule IDs
// they list, that another ignore directive of the same target already
// suppresses. Each ignore directive would suppress this diagnostic of the
// others, so it is reported directly rather than as an issue.
//
//declscope:package // analyzer.go runs it before reporting the issues
func (s *state) reportRedundantIgnores() {
	for i, ignore := range s.ignores {
		if s.skipFile(ignore.file) {
			continue
		}

		redundantAll, redundantRules := s.redundantIgnore(i)
		if redundantAll != "" {
			s.report(ignore.pos, ruleRedundantDirective, redundantAll)
		}

		for _, rule := range slices.Sorted(maps.Keys(redundantRules)) {
			s.report(ignore.pos, ruleRedundantDirective, redundantRules[rule])
		}
	}
}

// redundantIgnore returns why the i-th ignore directive is redundant as a
// whole, or "" if it is not, and why each of its redundant rule IDs is.
// Listed rule IDs take precedence over targeting all rules, so a directive
// that targets all rules is redundant beside a directive for the same
// target that lists rule IDs, wherever it appears, and after another
// directive for the same target that targets all rules. A listed rule ID
// is redundant after another directive for the same target that lists it,
// and when it is listed more than once.
func (s *state) redundantIgnore(i int) (string, map[string]string) {
	ignore := s.ignores[i]
	rules := make(map[string]string)

	for _, rule := range ignore.repeated {
		rules[rule] = fmt.Sprintf("redundant ignore rule %s; it is listed more than once", rule)
	}

	all := ""

	for j, other := range s.ignores {
		if j == i || ignore.trailing || other.trailing || other.target != ignore.target {
			continue
		}

		if ignore.all {
			switch {
			case !other.all:
				all = "redundant ignore directive; another ignore directive lists the rules to suppress here"
			case j < i && all == "":
				all = "redundant ignore directive; another ignore directive already suppresses all rules here"
			}
		}

		for rule := range ignore.rules {
			if j < i && other.rules[rule] && rules[rule] == "" {
				rules[rule] = fmt.Sprintf("redundant ignore rule %s; another ignore directive already suppresses it here", rule)
			}
		}
	}

	return all, rules
}

func (s *state) ignoreApplies(ignore *ignoreDirective, pos token.Pos) bool {
	if ignore.trailing {
		return s.pass.Fset.Position(pos).Line == ignore.line
	}

	if !ignore.target.Valid() {
		return false
	}

	target := ignore.target
	if pos < ignore.end || pos > target.Node().End() {
		return false
	}

	// Comments between the directive and its target, such as the other
	// directives in a doc comment, are covered, too.
	if pos < target.Node().Pos() {
		return true
	}

	// A label does not change which statement the directive covers.
	for {
		if _, ok := target.Node().(*ast.LabeledStmt); !ok {
			break
		}

		target = target.ChildAt(edge.LabeledStmt_Stmt, -1)
	}

	// Bodies of function literals in the target are blocks, too.
	return inIgnoreTargetHeader(target.Node(), pos) && !inIgnoreTargetFuncLitBody(target, pos)
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
// function literal in target, the cursor of an ignore target.
func inIgnoreTargetFuncLitBody(target inspector.Cursor, pos token.Pos) bool {
	// The range [pos, pos+1) selects the node that holds the token at pos,
	// not one that ends there.
	inner, ok := target.FindByPos(pos, pos+1)
	if !ok {
		return false
	}

	for cursor := range inner.Enclosing((*ast.FuncLit)(nil)) {
		if !target.Contains(cursor) {
			break
		}

		if pos >= cursor.Node().(*ast.FuncLit).Body.Lbrace {
			return true
		}
	}

	return false
}
