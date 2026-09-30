package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testAnalyzer := *Analyzer
	testAnalyzer.Run = func(pass *analysis.Pass) (any, error) {
		pass.Module = &analysis.Module{Path: "example.com/test", Main: true}
		return run(pass)
	}

	analysistest.Run(t, analysistest.TestData(), &testAnalyzer, "a", "b")
}

func TestAnalyzerMissingReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "govo.yaml")
	if err := os.WriteFile(path, []byte("ignore:\n  missing-reason: error\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldPath := configPath
	configPath = path

	t.Cleanup(func() { configPath = oldPath })

	testAnalyzer := *Analyzer
	testAnalyzer.Run = func(pass *analysis.Pass) (any, error) {
		pass.Module = &analysis.Module{Path: "example.com/test", Main: true}
		return run(pass)
	}

	analysistest.Run(t, analysistest.TestData(), &testAnalyzer, "c")
}

func TestExcludedPackagesDoNotLoadConfig(t *testing.T) {
	oldPath := configPath
	configPath = filepath.Join(t.TempDir(), "missing.yaml")
	t.Cleanup(func() { configPath = oldPath })

	fset := token.NewFileSet()
	stdlibFile := fset.AddFile(filepath.Join(t.TempDir(), "fmt", "print.go"), -1, 1)
	stdlib := &ast.File{Package: stdlibFile.Pos(0)}

	for _, pass := range []*analysis.Pass{
		{Module: &analysis.Module{Path: "example.com/dependency", Version: "v1.0.0"}},
		{Module: &analysis.Module{Path: "example.com/workspace-dependency", Dir: t.TempDir()}},
		{Fset: fset, Files: []*ast.File{stdlib}},
		{}, // Packages such as unsafe have no source files or module metadata.
	} {
		if _, err := run(pass); err != nil {
			t.Fatalf("excluded package loaded config: %v", err)
		}
	}
}

func TestMainModulePackageIsAnalyzed(t *testing.T) {
	fset := token.NewFileSet()
	file := fset.AddFile(filepath.Join(t.TempDir(), "code.go"), -1, 1)

	pass := &analysis.Pass{
		Module: &analysis.Module{Path: "example.com/main", Main: true},
		Fset:   fset,
		Files:  []*ast.File{{Package: file.Pos(0)}},
	}
	if excludedPackage(pass) {
		t.Fatal("main module package was excluded")
	}

	pass.Module = &analysis.Module{Path: "example.com/main"} // Go 1.26 vet protocol.
	if excludedPackage(pass) {
		t.Fatal("main module package using the Go 1.26 vet protocol was excluded")
	}
}

func TestVendoredPackage(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct {
		name string
		pkg  string
		dir  string
		want bool
	}{
		{"vendored dependency", "example.com/dep/sub", "vendor/example.com/dep/sub", true},
		{"main module directory named vendor", "example.com/main/internal/vendor/foo", "internal/vendor/foo", false},
		{"main module package named vendor", "example.com/main/vendor", "vendor", false},
		{"ordinary package", "example.com/main/foo", "foo", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file := fset.AddFile(filepath.Join(root, filepath.FromSlash(tc.dir), "code.go"), -1, 1)

			pass := &analysis.Pass{
				Module: &analysis.Module{Path: "example.com/main", Main: true},
				Pkg:    types.NewPackage(tc.pkg, path.Base(tc.pkg)),
				Fset:   fset,
				Files:  []*ast.File{{Package: file.Pos(0)}},
			}
			if got := excludedPackage(pass); got != tc.want {
				t.Fatalf("excludedPackage() = %v, want %v", got, tc.want)
			}
		})
	}
}
