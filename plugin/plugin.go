package plugin

import (
	"github.com/Gilthoniel/ctxprop"
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("ctxprop", New)
}

func New(_ any) (register.LinterPlugin, error) {
	return Plugin{}, nil
}

type Plugin struct{}

func (p Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{ctxprop.Analyzer}, nil
}

func (p Plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}
