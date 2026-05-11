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
	buildSSA := findSSA(pass)
	if buildSSA == nil {
		return nil, nil
	}

	e := engine{
		ssa:    buildSSA,
		report: pass.Report,
	}

	e.do()

	return nil, nil
}

func findSSA(pass *analysis.Pass) *buildssa.SSA {
	buildSSA, ok := pass.ResultOf[buildssa.Analyzer]
	if !ok {
		return nil
	}
	typedSSA, ok := buildSSA.(*buildssa.SSA)
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
	for _, param := range dropFuncReceiver(fn) {
		if e.isContextImpl(param.Type()) {
			return originVariable
		}
		if e.isContextProvider(param.Type()) {
			return originProvider
		}
	}
	return originNone
}

// dropFuncReceiver returns fn.Params with the receiver dropped. Embedded
// contexts on a receiver are not treated as a propagation source — they're an
// antipattern (Go discourages storing a context on a struct).
func dropFuncReceiver(fn *ssa.Function) []*ssa.Parameter {
	if fn.Signature != nil && fn.Signature.Recv() != nil && len(fn.Params) > 0 {
		return fn.Params[1:]
	}
	return fn.Params
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

	for _, arg := range dropCallReceiver(call) {
		if !e.isContextImpl(arg.Type()) {
			continue
		}
		if !e.checkIfInheritParentCtx(arg, candidates, nil) {
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

// dropCallReceiver returns call.Call.Args with the implicit receiver dropped for
// static method calls. We must not treat that slot as a propagation candidate —
// for the same reason dropFuncReceiver excludes the parent function's receiver.
func dropCallReceiver(call *ssa.Call) []ssa.Value {
	args := call.Call.Args
	callee := call.Call.StaticCallee()
	if callee == nil || callee.Signature == nil || callee.Signature.Recv() == nil {
		return args
	}
	if len(args) == 0 {
		return args
	}
	return args[1:]
}

func (e *engine) checkIfInheritParentCtx(value ssa.Value, candidates Candidates, stack []ssa.Value) bool {
	if slices.Contains(stack, value) {
		// When infinite recursivity is detected, we consider this branch to be ok.
		return true
	}

	// At this point we know that the argument implements <context.Context>
	// so we need to verify if it inherits from the parent context.
	switch a := value.(type) {
	case *ssa.Parameter:
		return candidates.MatchAny(a.Object())

	case *ssa.MakeInterface:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.ChangeType:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.ChangeInterface:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.TypeAssert:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.Extract:
		return e.checkIfInheritParentCtx(a.Tuple, candidates, append(stack, value))
	case *ssa.UnOp:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.FieldAddr:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.Field:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.IndexAddr:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.Slice:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))
	case *ssa.Lookup:
		return e.checkIfInheritParentCtx(a.X, candidates, append(stack, value))

	case *ssa.Call:
		for _, arg := range a.Call.Args {
			if e.checkIfInheritParentCtx(arg, candidates, append(stack, value)) {
				return true
			}
		}

	case *ssa.Phi:
		// Since a PHI node indicates represents a potential value, we need to check that
		// all edges will inherit. We however need to make sure to stop when an object
		// has already been verified.

		match := true
		for _, edge := range a.Edges {
			match = match && e.checkIfInheritParentCtx(edge, candidates, append(stack, value))
		}
		return match

	case *ssa.Alloc:
		values := e.collectStoredCtxValues(a)
		if len(values) == 0 {
			return false
		}
		match := true
		for _, v := range values {
			match = match && e.checkIfInheritParentCtx(v, candidates, append(stack, value))
		}
		return match

	case *ssa.MakeMap:
		values := e.collectStoredCtxValues(a)
		if len(values) == 0 {
			return false
		}
		match := true
		for _, v := range values {
			match = match && e.checkIfInheritParentCtx(v, candidates, append(stack, value))
		}
		return match
	}
	return false
}

// collectStoredCtxValues returns the values written through Store referrers of
// addr, including values stored into any context-typed field reached via a
// FieldAddr. This covers both `var x = ctx` and `T{Ctx: ctx}` patterns.
func (e *engine) collectStoredCtxValues(addr ssa.Value) []ssa.Value {
	if addr.Referrers() == nil {
		return nil
	}
	var values []ssa.Value
	for _, ref := range *addr.Referrers() {
		switch r := ref.(type) {
		case *ssa.Store:
			values = append(values, r.Val)
		case *ssa.FieldAddr:
			ptr, ok := r.Type().(*types.Pointer)
			if !ok || !e.isContextImpl(ptr.Elem()) {
				continue
			}
			if r.Referrers() == nil {
				continue
			}
			for _, fieldRef := range *r.Referrers() {
				if store, ok := fieldRef.(*ssa.Store); ok {
					values = append(values, store.Val)
				}
			}
		case *ssa.IndexAddr:
			ptr, ok := r.Type().(*types.Pointer)
			if !ok || !e.isContextImpl(ptr.Elem()) {
				continue
			}
			if r.Referrers() == nil {
				continue
			}
			for _, elemRef := range *r.Referrers() {
				if store, ok := elemRef.(*ssa.Store); ok {
					values = append(values, store.Val)
				}
			}
		case *ssa.MapUpdate:
			if !e.isContextImpl(r.Value.Type()) {
				continue
			}
			values = append(values, r.Value)
		}
	}
	return values
}

func (e *engine) extractParentContextVariable(block *ssa.BasicBlock) (candidates []types.Object) {
	parent := block.Parent()
	if parent == nil {
		return nil
	}

	for _, param := range dropFuncReceiver(parent) {
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

	for _, arg := range dropCallReceiver(call) {
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

	for _, param := range dropFuncReceiver(parent) {
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
