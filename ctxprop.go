package ctxprop

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

var Analyzer = &analysis.Analyzer{
	Name: "ctxprop",
	Doc:  "check whether a context is properly progagated through the functions",
	Run:  run,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
}

func run(pass *analysis.Pass) (any, error) {
	ssa := findSSA(pass)
	if ssa == nil {
		return nil, nil
	}

	e := engine{
		ssa:      ssa,
		ctxIface: findContextInterface(ssa),
		report:   pass.Report,
	}

	e.do()

	return nil, nil
}

func findSSA(pass *analysis.Pass) *buildssa.SSA {
	ssa, ok := pass.ResultOf[buildssa.Analyzer]
	if !ok {
		return nil
	}
	typedSSA, ok := ssa.(*buildssa.SSA)
	if !ok {
		return nil
	}
	return typedSSA
}

func findContextInterface(ssa *buildssa.SSA) *types.Interface {
	if ssa == nil || ssa.Pkg == nil || ssa.Pkg.Prog == nil {
		return nil
	}

	pkg := ssa.Pkg.Prog.ImportedPackage("context")
	if pkg == nil {
		return nil
	}

	ctxType := pkg.Type("Context")
	if ctxType == nil {
		return nil
	}

	return findInterfaceType(ctxType.Type())
}

func findInterfaceType(t types.Type) *types.Interface {
	switch typed := t.(type) {
	case *types.Named:
		return findInterfaceType(typed.Underlying())
	case *types.Interface:
		return typed
	}
	return nil
}

type engine struct {
	ssa      *buildssa.SSA
	ctxIface *types.Interface
	report   func(analysis.Diagnostic)
}

func (e engine) do() {
	if e.ssa == nil || e.ctxIface == nil {
		return
	}

	for _, fn := range e.ssa.SrcFuncs {
		if fn == nil {
			continue
		}
		if !e.hasContextAsFirstParam(fn) {
			continue
		}

		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				e.checkInstruction(instr)
			}
		}
	}
}

func (e engine) checkInstruction(instr ssa.Instruction) {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return
	}

	for _, arg := range call.Call.Args {
		if !e.isContextImpl(arg.Type()) {
			continue
		}
		switch arg.(type) {
		case *ssa.Parameter:
			// TODO: check it is the parent context.
			continue
		default:
			// TODO: send diagnostic
		}
	}
}

func (e engine) hasContextAsFirstParam(fn *ssa.Function) bool {
	if len(fn.Params) == 0 {
		return false
	}
	return e.isContextImpl(fn.Params[0].Type())
}

func (e engine) isContextImpl(t types.Type) bool {
	return types.Implements(t, e.ctxIface)
}
