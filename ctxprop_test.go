package ctxprop_test

import (
	"testing"

	"github.com/Gilthoniel/ctxprop"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, ctxprop.Analyzer, "a")
	require.Len(t, result, 1)
}

func TestHttpHandler(t *testing.T) {
	testdata := analysistest.TestData()
	result := analysistest.Run(t, testdata, ctxprop.Analyzer, "b")
	require.Len(t, result, 1)
}
