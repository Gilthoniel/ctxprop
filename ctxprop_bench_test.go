package ctxprop

import (
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func BenchmarkAnalyzer(b *testing.B) {
	for _, pkg := range []string{"a", "b", "c", "d"} {
		b.Run(pkg, func(b *testing.B) {
			ssaResult := buildBenchSSA(b, pkg)
			pass := &analysis.Pass{
				Analyzer: Analyzer,
				ResultOf: map[*analysis.Analyzer]any{buildssa.Analyzer: ssaResult},
				Report:   func(analysis.Diagnostic) {},
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := Analyzer.Run(pass); err != nil {
					b.Fatalf("Run: %v", err)
				}
			}
		})
	}
}

// buildBenchSSA loads the testdata package and its dependencies, builds SSA
// once, and returns a buildssa.SSA scoped to the project package only — so the
// benchmark times the analyzer's traversal of project code, not buildssa over
// the stdlib closure.
func buildBenchSSA(b *testing.B, pattern string) *buildssa.SSA {
	b.Helper()

	dir, err := filepath.Abs("testdata")
	if err != nil {
		b.Fatalf("abs testdata: %v", err)
	}
	cfg := &packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   dir,
		Tests: false,
		Env:   append(os.Environ(), "GOPATH="+dir, "GO111MODULE=off", "GOWORK=off"),
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		b.Fatalf("load %q: %v", pattern, err)
	}
	if len(pkgs) != 1 {
		b.Fatalf("expected one package for %q, got %d", pattern, len(pkgs))
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		b.Fatalf("package %s has errors: %v", pkg.PkgPath, pkg.Errors)
	}

	prog, ssapkgs := ssautil.AllPackages(pkgs, 0)
	prog.Build()
	ssapkg := ssapkgs[0]

	var funcs []*ssa.Function
	var addAnons func(*ssa.Function)
	addAnons = func(f *ssa.Function) {
		funcs = append(funcs, f)
		for _, anon := range f.AnonFuncs {
			addAnons(anon)
		}
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			fn, _ := pkg.TypesInfo.Defs[fd.Name].(*types.Func)
			if fn == nil {
				continue
			}
			if f := prog.FuncValue(fn); f != nil {
				addAnons(f)
			}
		}
	}

	return &buildssa.SSA{Pkg: ssapkg, SrcFuncs: funcs}
}
