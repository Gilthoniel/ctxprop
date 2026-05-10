package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test(t *testing.T) {
	dir := t.TempDir()
	outputFile, err := os.CreateTemp(dir, "stdout*")
	require.NoError(t, err)

	defer outputFile.Close()

	defer func() {
		if r := recover(); r != nil {
			data, err := os.ReadFile(outputFile.Name())
			require.NoError(t, err)
			require.Contains(t, string(data), "version devel comments-go-here")
		}
	}()

	stdout := os.Stdout
	os.Stdout = outputFile
	defer func() {
		os.Stdout = stdout
	}()

	os.Args = []string{os.Args[0], "-V=full"}
	main()
}
