package ctxprop

import (
	"go/token"
	"go/types"
	"slices"

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
		ssa:    ssa,
		report: pass.Report,
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

type engine struct {
	ssa              *buildssa.SSA
	ctxIface         *types.Interface
	ctxProviderIFace *types.Interface
	report           func(analysis.Diagnostic)
}

func (e *engine) init() {
	e.initCtxInterfaces()
}

func (e *engine) initCtxInterfaces() {
	if e.ssa == nil || e.ssa.Pkg == nil || e.ssa.Pkg.Prog == nil {
		return
	}
	pkg := e.ssa.Pkg.Prog.ImportedPackage("context")
	if pkg == nil {
		return
	}
	ctxType := pkg.Type("Context")
	if ctxType == nil {
		return
	}

	e.ctxIface = findInterfaceType(ctxType.Type())

	methods := []*types.Func{
		types.NewFunc(token.NoPos, nil, "Context", types.NewSignatureType(
			nil,
			nil,
			nil,
			types.NewTuple(),
			types.NewTuple(types.NewVar(token.NoPos, ctxType.Package().Pkg, "", ctxType.Type())),
			false,
		)),
	}
	e.ctxProviderIFace = types.NewInterfaceType(methods, nil)
	e.ctxProviderIFace.Complete()
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

func (e *engine) do() {
	e.init()

	if e.ctxIface == nil || e.ctxProviderIFace == nil {
		return
	}

	for _, fn := range e.ssa.SrcFuncs {
		if fn == nil {
			continue
		}

		switch e.hasContextInParams(fn) {
		case originVariable:
			for _, block := range fn.Blocks {
				for _, instr := range block.Instrs {
					e.checkInstruction(block, instr)
				}
			}

		case originProvider:
			for _, block := range fn.Blocks {
				for _, instr := range block.Instrs {
					e.checkInstructionForProvider(block, instr)
				}
			}

		case originNone:
		}

	}
}

func (e *engine) hasContextInParams(fn *ssa.Function) origin {
	for _, param := range fn.Params {
		if e.isContextImpl(param.Type()) {
			return originVariable
		}
		if e.isContextProvider(param.Type()) {
			return originProvider
		}
	}
	return originNone
}

// origin indicates if the parent context is provided from a variable or from a
// provider.
type origin int

const (
	originNone = iota
	originVariable
	originProvider
)

func (e *engine) checkInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return
	}

	candidates := e.extractParentContextVariable(block)
	if len(candidates) == 0 {
		return
	}

	for _, arg := range call.Call.Args {
		if !e.isContextImpl(arg.Type()) {
			continue
		}
		if !e.checkIfInheritParentCtx(arg, candidates) {
			e.report(analysis.Diagnostic{
				Pos:     instr.Pos(),
				Message: "function must inherit the context from the parent",
				Related: []analysis.RelatedInformation{{
					Pos:     arg.Pos(),
					Message: "Use " + candidates[0].Name() + " instead",
				}},
			})
		}
	}
}

func (e *engine) checkIfInheritParentCtx(arg ssa.Value, candidates Candidates) bool {
	// At this point we know that the argument implements <context.Context>
	// so we need to verify if it inherits from the parent context.
	switch a := arg.(type) {
	case *ssa.Parameter:
		return candidates.MatchAny(a.Object())

	case *ssa.MakeInterface:
		return e.checkIfInheritParentCtx(a.X, candidates)

	case *ssa.Call:
		for _, arg := range a.Call.Args {
			if e.checkIfInheritParentCtx(arg, candidates) {
				return true
			}
		}
	}
	return false
}

func (e *engine) extractParentContextVariable(block *ssa.BasicBlock) (candidates []types.Object) {
	parent := block.Parent()
	if parent == nil {
		return nil
	}

	for _, param := range parent.Params {
		if e.isContextImpl(param.Type()) {
			candidates = append(candidates, param.Object())
		}
	}

	return candidates
}

func (e *engine) checkInstructionForProvider(block *ssa.BasicBlock, instr ssa.Instruction) {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return
	}

	parentCtxProvider := e.extractParentContextProvider(block)
	if parentCtxProvider == nil {
		return
	}

	for _, arg := range call.Call.Args {
		if !e.isContextImpl(arg.Type()) {
			continue
		}
		if !e.checkIfCtxProvided(arg, parentCtxProvider) {
			e.report(analysis.Diagnostic{
				Pos:     instr.Pos(),
				Message: "function must inherit the context from the parent",
				Related: []analysis.RelatedInformation{{
					Pos:     parentCtxProvider.Pos(),
					Message: "Use " + parentCtxProvider.Name() + ".Context() instead",
				}},
			})
		}
	}
}

func (e *engine) extractParentContextProvider(block *ssa.BasicBlock) types.Object {
	parent := block.Parent()
	if parent == nil {
		return nil
	}

	for _, param := range parent.Params {
		if e.isContextProvider(param.Type()) {
			return param.Object()
		}
	}

	return nil
}

func (e *engine) checkIfCtxProvided(arg ssa.Value, parentCtxProvider types.Object) bool {
	switch a := arg.(type) {
	case *ssa.Call:
		for _, arg := range a.Call.Args {
			switch typedArg := arg.(type) {
			case *ssa.Parameter:
				if !areIdenticalVariable(typedArg.Object(), parentCtxProvider) {
					return false
				}

				return types.Identical(e.ctxProviderIFace.Method(0).Signature(), a.Call.Signature())

			default:
				return e.checkIfCtxProvided(arg, parentCtxProvider)
			}
		}

		return false

	case *ssa.MakeInterface:
		return e.checkIfCtxProvided(a.X, parentCtxProvider)

	case *ssa.Extract:
		return e.checkIfCtxProvided(a.Tuple, parentCtxProvider)
	}
	return false
}

func (e *engine) isContextImpl(t types.Type) bool {
	return types.Implements(t, e.ctxIface)
}

func (e *engine) isContextProvider(t types.Type) bool {
	return types.Implements(t, e.ctxProviderIFace)
}

type Candidates []types.Object

func (c Candidates) MatchAny(obj types.Object) bool {
	return slices.ContainsFunc(c, func(candidate types.Object) bool {
		return areIdenticalVariable(obj, candidate)
	})
}

func areIdenticalVariable(a, b types.Object) bool {
	aVar, aOk := a.(*types.Var)
	bVar, bOk := b.(*types.Var)
	return aOk && bOk && aVar == bVar
}
