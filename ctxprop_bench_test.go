package ctxprop

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/packages"
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
	if len(pkgs[0].Errors) > 0 {
		b.Fatalf("package %s has errors: %v", pkgs[0].PkgPath, pkgs[0].Errors)
	}

	graph, err := checker.Analyze([]*analysis.Analyzer{buildssa.Analyzer}, pkgs, nil)
	if err != nil {
		b.Fatalf("analyze: %v", err)
	}
	return graph.Roots[0].Result.(*buildssa.SSA)
}
