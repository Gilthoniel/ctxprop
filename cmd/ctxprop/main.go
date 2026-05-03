package main

import (
	"github.com/Gilthoniel/ctxprop"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(ctxprop.Analyzer)
}
