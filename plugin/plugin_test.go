package plugin_test

import (
	"testing"

	"github.com/Gilthoniel/ctxprop"
	"github.com/Gilthoniel/ctxprop/plugin"
	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
)

func Test(t *testing.T) {
	p, err := plugin.New(nil)
	require.NoError(t, err)
	require.Equal(t, register.LoadModeSyntax, p.GetLoadMode())

	analyzers, err := p.BuildAnalyzers()
	require.NoError(t, err)
	require.Len(t, analyzers, 1)
	require.IsType(t, ctxprop.Analyzer, analyzers[0])
}
