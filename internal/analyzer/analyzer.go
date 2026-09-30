// Package analyzer implements checks for protected Go value objects.
package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer checks conversions and untyped constants involving //govo:protect types.
var Analyzer = newAnalyzer()

const (
	ruleConstruction = "GOVO001"
	ruleExtraction   = "GOVO002"
	ruleOperation    = "GOVO003"
	ruleDirective    = "GOVO004"
)

var knownRules = []string{ruleConstruction, ruleExtraction, ruleOperation, ruleDirective}

type protectedFact struct{}

func (*protectedFact) AFact() {}

func newAnalyzer() *analysis.Analyzer {
	a := &analysis.Analyzer{
		Name:      "govo",
		Doc:       "check protected value object construction and extraction",
		Run:       run,
		FactTypes: []analysis.Fact{new(protectedFact)},
	}
	registerConfigFlag(&a.Flags)

	return a
}

type protectedType struct {
	name *types.TypeName
	file *ast.File // nil for an imported protected type
}

type fileInfo struct {
	name      string
	generated bool
	protected []*protectedType // protected types declared in this file
	src       []byte
	srcErr    error
	srcLoaded bool
}

type analyzerState struct {
	pass   *analysis.Pass
	config Config
	files  map[*ast.File]*fileInfo
	local  map[*types.TypeName]*protectedType
	// imported caches fact lookups; a nil value records a type that is not protected.
	imported map[*types.TypeName]*protectedType
	// protectComments holds //govo:protect comments attached to a type declaration.
	protectComments map[*ast.Comment]bool
	issues          []issue
	ignore          []*ignoreDirective
	current         *ast.File
}

type issue struct {
	pos     token.Pos
	rule    string
	message string
	file    *ast.File
}

func run(pass *analysis.Pass) (any, error) {
	if excludedPackage(pass) {
		return nil, nil
	}

	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	s := &analyzerState{
		pass:            pass,
		config:          config,
		files:           make(map[*ast.File]*fileInfo),
		local:           make(map[*types.TypeName]*protectedType),
		imported:        make(map[*types.TypeName]*protectedType),
		protectComments: make(map[*ast.Comment]bool),
	}

	for _, f := range pass.Files {
		s.files[f] = &fileInfo{
			name:      pass.Fset.PositionFor(f.Pos(), false).Filename,
			generated: ast.IsGenerated(f),
		}
		s.collectProtect(f)
	}

	for _, f := range pass.Files {
		s.current = f
		s.collectDirectives(f)

		if s.skipFile(f) {
			continue
		}

		s.analyzeFile(f)
	}

	s.reportIssues()

	return nil, nil
}

func excludedPackage(pass *analysis.Pass) bool {
	// Only packages in the main module contribute facts or diagnostics.
	if pass.Module == nil || pass.Module.Path == "" || externalModule(pass.Module) {
		return true
	}

	if len(pass.Files) == 0 {
		return true
	}

	return vendoredPackage(pass)
}

// vendoredPackage reports whether pass analyzes a copy of a dependency in
// a vendor directory. In module mode the import path of a vendored package
// is the dependency's own path, and its files are in vendor/<import path>.
// Matching the whole suffix avoids excluding a main-module package that
// merely has a directory named vendor in its path.
func vendoredPackage(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	filename := pass.Fset.PositionFor(pass.Files[0].Pos(), false).Filename
	dir := filepath.ToSlash(filepath.Dir(filename))

	return strings.HasSuffix(dir, "/vendor/"+pass.Pkg.Path())
}

// externalModule reports whether a module with a non-empty path is a
// dependency rather than the main module.
func externalModule(module *analysis.Module) bool {
	if module.Main {
		return false
	}

	// go vet before Go 1.27 supplies Path, Version, and GoVersion but not
	// Main. The main module then has no version; versioned dependencies do.
	return module.Version != "" || module.Dir != "" || module.GoMod != "" || module.Replace != nil
}

func (s *analyzerState) skipFile(f *ast.File) bool {
	info := s.files[f]
	if info.generated {
		return true
	}

	return !s.config.Tests && strings.HasSuffix(info.name, "_test.go")
}

func (s *analyzerState) collectProtect(f *ast.File) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)

			if !s.markProtect(typeSpecDoc(gd, ts)) {
				continue
			}

			obj, ok := s.pass.TypesInfo.Defs[ts.Name].(*types.TypeName)
			if !ok || obj.IsAlias() {
				s.issue(ts.Name.Pos(), ruleDirective, "protect requires a defined type", f)
				continue
			}

			if !basicUnderlying(obj.Type()) {
				s.issue(ts.Name.Pos(), ruleDirective, "protect requires a basic underlying type", f)
				continue
			}

			p := &protectedType{name: obj, file: f}

			s.local[obj] = p
			s.files[f].protected = append(s.files[f].protected, p)

			// Unexported types need facts too: their values can reach other
			// packages through exported functions, fields, and aliases.
			s.pass.ExportObjectFact(obj, new(protectedFact))
		}
	}
}

// typeSpecDoc returns the doc comment that applies to ts. A declaration's
// doc comment applies to its spec only when the declaration is not grouped.
func typeSpecDoc(gd *ast.GenDecl, ts *ast.TypeSpec) *ast.CommentGroup {
	if ts.Doc == nil && gd.Lparen == token.NoPos {
		return gd.Doc
	}

	return ts.Doc
}

// markProtect records every protect directive in group as attached and
// reports whether one of them is valid. collectDirectives reports arguments.
func (s *analyzerState) markProtect(group *ast.CommentGroup) bool {
	if group == nil {
		return false
	}

	protect := false

	for _, c := range group.List {
		command, args, ok := parseDirective(c.Text)
		if !ok || command != directiveProtect {
			continue
		}

		s.protectComments[c] = true
		protect = protect || args == ""
	}

	return protect
}

func basicUnderlying(t types.Type) bool {
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && b.Info()&types.IsUntyped == 0 && b.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
}

func (s *analyzerState) protected(t types.Type) *protectedType {
	if t == nil {
		return nil
	}

	name, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return nil
	}

	obj := name.Obj()
	if p := s.local[obj]; p != nil {
		return p
	}

	if obj.Pkg() == nil || obj.Pkg() == s.pass.Pkg {
		return nil
	}

	if p, ok := s.imported[obj]; ok {
		return p
	}

	var p *protectedType
	if s.pass.ImportObjectFact(obj, new(protectedFact)) {
		p = &protectedType{name: obj}
	}

	s.imported[obj] = p

	return p
}

func (s *analyzerState) issue(pos token.Pos, rule, msg string, f *ast.File) {
	s.issues = append(s.issues, issue{pos: pos, rule: rule, message: msg, file: f})
}
