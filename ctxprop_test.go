package ctxprop

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, Analyzer, "a")
	require.Len(t, result, 1)
}

func TestHttpHandler(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, Analyzer, "b")
	require.Len(t, result, 1)
}

func TestAnalyzer_run_skipsOnMissingSSA(t *testing.T) {
	res, err := Analyzer.Run(&analysis.Pass{})
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestAnalyzer_run_skipsOnMalformedResultOfSSA(t *testing.T) {
	res, err := Analyzer.Run(&analysis.Pass{ResultOf: map[*analysis.Analyzer]any{buildssa.Analyzer: nil}})
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestEngine_do_skipsWhenFailingToInit(t *testing.T) {
	e := engine{}
	e.do()
}
