// This file is the package's API and the analysis state that the other
// files read and update.
//declscope:core

// Package analyzer implements checks for protected Go value objects.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer checks conversions and untyped constants involving //govo:protect types.
var Analyzer = newAnalyzer()

// Rule IDs for protected types have the GOV prefix and rule IDs for
// directives the GOVD prefix, so each category is numbered independently.
//
//declscope:package // every check reports under these IDs
const (
	ruleConstruction = "GOV001"
	//declscope:private
	ruleExtraction = "GOV002"
	ruleOperation  = "GOV003"

	ruleInvalidDirective   = "GOVD001"
	ruleUnusedIgnore       = "GOVD002"
	ruleMissingReason      = "GOVD003"
	ruleRedundantDirective = "GOVD004"
)

// knownRules lists the rule IDs that an ignore directive may name.
//
//declscope:package // ignore.go validates ignore directives against it
var knownRules = []string{
	ruleConstruction, ruleExtraction, ruleOperation,
	ruleInvalidDirective, ruleUnusedIgnore, ruleMissingReason, ruleRedundantDirective,
}

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

type fileInfo struct {
	name      string
	generated bool
	src       []byte
	srcErr    error
	srcLoaded bool
}

// state is the state of one analysis pass.
//
//declscope:package // the methods of every check are declared on it
type state struct {
	pass     *analysis.Pass
	config   Config
	protects protectedTypes
	// grants holds the protected types that valid factory, converter, op,
	// and scalar directives permit each function to construct, extract, or
	// scale.
	grants  map[*ast.FuncDecl]*grant
	ignores ignoreList
	current *ast.File
	// grant is the permission of the function declaration being analyzed,
	// or nil outside a marked function.
	grant *grant
	// inConst is set while a const declaration is analyzed.
	inConst bool
	//declscope:private
	files map[*ast.File]*fileInfo
	//declscope:private
	issues []issue
}

// grant records the protected types that a marked function may construct
// (factory or op) or extract (converter or op) with direct conversions, and
// those that it may scale by untyped constants (scalar).
//
//declscope:package // directive.go grants it and the checks consult it
type grant struct {
	construct map[*protectedType]bool
	extract   map[*protectedType]bool
	scale     map[*protectedType]bool
}

// issue is a diagnostic waiting for ignore directives to be applied.
//
//declscope:package // ignore.go matches issues against ignore directives
type issue struct {
	pos  token.Pos
	rule string
	file *ast.File
	//declscope:private
	message string
}

func run(pass *analysis.Pass) (any, error) {
	if excludedPackage(pass) {
		return nil, nil
	}

	config, err := loadConfig()
	if err != nil {
		return nil, err
	}

	s := &state{
		pass:     pass,
		config:   config,
		files:    make(map[*ast.File]*fileInfo),
		protects: newProtectedTypes(),
		grants:   make(map[*ast.FuncDecl]*grant),
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

		s.walkFile(f)
	}

	s.reportIssues()

	return nil, nil
}

// reportIssues reports every issue that no ignore directive suppresses, and
// then the ignore directives that suppressed nothing.
func (s *state) reportIssues() {
	for _, problem := range s.issues {
		if s.skipFile(problem.file) || s.applyIgnores(problem) {
			continue
		}

		s.report(problem.pos, problem.rule, problem.message)
	}

	s.reportUnusedIgnores()
}

// report reports a diagnostic of rule at pos. The rule ID is the diagnostic's
// category and also prefixes its message for plain-text output.
//
//declscope:package // ignore.go reports unused ignore directives through it
func (s *state) report(pos token.Pos, rule, message string) {
	s.pass.Report(analysis.Diagnostic{
		Pos:      pos,
		Category: rule,
		Message:  rule + ": " + message,
	})
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

// skipFile reports whether diagnostics in f are left out.
//
//declscope:package // ignore.go leaves out unused ignores in the same files
func (s *state) skipFile(f *ast.File) bool {
	info := s.files[f]
	if info.generated {
		return true
	}

	return !s.config.Tests && strings.HasSuffix(info.name, "_test.go")
}

// mayConstruct reports whether the code being analyzed may construct p
// directly: in a function marked as a factory of p, or in a const
// declaration in the file that declares p.
//
//declscope:package // the conversion checks consult it
func (s *state) mayConstruct(p *protectedType) bool {
	if p.file != s.current {
		return false
	}

	return s.inConst || s.grant != nil && s.grant.construct[p]
}

// mayExtract reports whether the code being analyzed may extract the
// underlying representation of p directly: in a function marked as a
// converter of p.
//
//declscope:package // the conversion checks consult it
func (s *state) mayExtract(p *protectedType) bool {
	return p.file == s.current && s.grant != nil && s.grant.extract[p]
}

// mayScale reports whether the code being analyzed may multiply or divide a
// value of p by an untyped constant: in a function with a scalar directive
// for p.
//
//declscope:package // operation.go consults it for scaled operands
func (s *state) mayScale(p *protectedType) bool {
	return p.file == s.current && s.grant != nil && s.grant.scale[p]
}

// source returns the contents of f, reading the file at most once.
//
//declscope:package // ignore.go reads the line of an ignore directive
func (s *state) source(f *ast.File) ([]byte, error) {
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

// issue records a diagnostic, which reportIssues reports unless an ignore
// directive suppresses it.
//
//declscope:package // every check records its diagnostics through it
func (s *state) issue(pos token.Pos, rule, msg string, f *ast.File) {
	s.issues = append(s.issues, issue{pos: pos, rule: rule, message: msg, file: f})
}

// constructionIssue reports a construction of the protected type t outside
// its factories and points to the marker that permits it.
//
//declscope:package // conversions and composite literals construct values
func (s *state) constructionIssue(pos token.Pos, kind string, t types.Type) {
	s.issue(pos, ruleConstruction, fmt.Sprintf("%s construction of protected type %s; use a //govo:%s function", kind, typeString(t), directiveFactory), s.current)
}

// extractionIssue reports an extraction from the protected type t outside
// its converters and points to the marker that permits it.
//
//declscope:package // conversion.go reports extractions
func (s *state) extractionIssue(pos token.Pos, kind string, t types.Type) {
	s.issue(pos, ruleExtraction, fmt.Sprintf("%s extraction from protected type %s; use a //govo:%s function", kind, typeString(t), directiveConverter), s.current)
}

// typeString returns the protected type t as diagnostics name it: the type
// that an alias denotes, with the type arguments of an instance and without
// package qualifiers, as in Code or List[int].
//
//declscope:package // the checks name protected types in their diagnostics
func typeString(t types.Type) string {
	return types.TypeString(types.Unalias(t), func(*types.Package) string { return "" })
}
