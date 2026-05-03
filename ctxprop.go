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
				e.checkInstruction(block, instr)
			}
		}
	}
}

func (e engine) checkInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return
	}

	parentCtxVar := e.extractParentContextVariable(block)
	if parentCtxVar == nil {
		return
	}

	for _, arg := range call.Call.Args {
		if !e.isContextImpl(arg.Type()) {
			continue
		}
		if !e.checkIfInheritParentCtx(arg, parentCtxVar) {
			e.report(analysis.Diagnostic{
				Pos:     instr.Pos(),
				Message: "function must inherit the context from the parent",
			})
		}
	}
}

func (e engine) checkIfInheritParentCtx(arg ssa.Value, parentCtxVar types.Object) bool {
	// At this point we know that the argument implements <context.Context>
	// so we need to verify if it inherits from the parent context.
	switch a := arg.(type) {
	case *ssa.Parameter:
		return areIdenticalVariable(a.Object(), parentCtxVar)

	case *ssa.MakeInterface:
		return e.checkIfInheritParentCtx(a.X, parentCtxVar)

	case *ssa.Call:
		for _, arg := range a.Call.Args {
			if e.checkIfInheritParentCtx(arg, parentCtxVar) {
				return true
			}
		}
	}
	return false
}

func (e engine) extractParentContextVariable(block *ssa.BasicBlock) types.Object {
	parent := block.Parent()
	if parent == nil {
		return nil
	}

	for _, param := range parent.Params {
		if e.isContextImpl(param.Type()) {
			return param.Object()
		}
	}

	return nil
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

func areIdenticalVariable(a, b types.Object) bool {
	aVar, aOk := a.(*types.Var)
	bVar, bOk := b.(*types.Var)
	return aOk && bOk && aVar == bVar
}
