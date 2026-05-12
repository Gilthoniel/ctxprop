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

	visited := make([]ssa.Value, 0, 16)
	candidates := make([]Candidate, 0, 4)
	for _, fn := range e.ssa.SrcFuncs {
		if fn == nil {
			continue
		}
		candidates = e.collectCandidates(fn, candidates[:0])
		if len(candidates) == 0 {
			continue
		}
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				e.checkInstruction(candidates, instr, visited)
			}
		}
	}
}

func (e *engine) checkInstruction(candidates []Candidate, instr ssa.Instruction, visited []ssa.Value) {
	call, ok := instr.(*ssa.Call)
	if !ok {
		return
	}

	for _, arg := range dropCallReceiver(call) {
		if !e.isContextImpl(arg.Type()) {
			continue
		}
		if anyInherits(candidates, arg, visited) {
			continue
		}
		e.report(analysis.Diagnostic{
			Pos:     instr.Pos(),
			Message: "function must inherit the context from the parent",
			Related: []analysis.RelatedInformation{{
				Pos:     candidates[0].Pos(),
				Message: "Use " + candidates[0].ReplacementName() + " instead",
			}},
		})
	}
}

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

func anyInherits(cs []Candidate, v ssa.Value, visited []ssa.Value) bool {
	for _, c := range cs {
		if c.Inherits(v, visited) {
			return true
		}
	}
	return false
}

func (e *engine) collectCandidates(fn *ssa.Function, out []Candidate) []Candidate {
	for f := fn; f != nil; f = f.Parent() {
		for _, p := range dropFuncReceiver(f) {
			switch {
			case e.isContextImpl(p.Type()):
				out = append(out, &variableCandidate{obj: p.Object(), e: e})
			case e.isContextProvider(p.Type()):
				out = append(out, &providerCandidate{obj: p.Object(), e: e})
			}
		}

		// As soon as at least one candidate is found for this scope, we stop and return
		// since we want to privilege the nearest parent contexts (e.g. a closure taking
		// a context as parameter).
		if len(out) > 0 {
			return out
		}
	}
	return out
}

func dropFuncReceiver(fn *ssa.Function) []*ssa.Parameter {
	if fn.Signature != nil && fn.Signature.Recv() != nil && len(fn.Params) > 0 {
		return fn.Params[1:]
	}
	return fn.Params
}

type Candidate interface {
	Inherits(v ssa.Value, visited []ssa.Value) bool

	Pos() token.Pos

	ReplacementName() string
}

type variableCandidate struct {
	obj types.Object
	e   *engine
}

func (c *variableCandidate) Pos() token.Pos {
	return c.obj.Pos()
}

func (c *variableCandidate) ReplacementName() string {
	return c.obj.Name()
}

func (c *variableCandidate) Inherits(value ssa.Value, visited []ssa.Value) bool {
	if slices.Contains(visited, value) {
		// When infinite recursivity is detected, we consider this branch to be ok.
		return true
	}
	visited = append(visited, value)

	switch a := value.(type) {
	case *ssa.Parameter:
		return areIdenticalVariable(a.Object(), c.obj)

	case *ssa.FreeVar:
		if binding := freeVarBinding(a); binding != nil {
			return c.Inherits(binding, visited)
		}

	case *ssa.MakeInterface:
		return c.Inherits(a.X, visited)
	case *ssa.ChangeType:
		return c.Inherits(a.X, visited)
	case *ssa.ChangeInterface:
		return c.Inherits(a.X, visited)
	case *ssa.TypeAssert:
		return c.Inherits(a.X, visited)
	case *ssa.Extract:
		return c.Inherits(a.Tuple, visited)
	case *ssa.UnOp:
		return c.Inherits(a.X, visited)
	case *ssa.FieldAddr:
		return c.Inherits(a.X, visited)
	case *ssa.Field:
		return c.Inherits(a.X, visited)
	case *ssa.IndexAddr:
		return c.Inherits(a.X, visited)
	case *ssa.Slice:
		return c.Inherits(a.X, visited)
	case *ssa.Lookup:
		return c.Inherits(a.X, visited)

	case *ssa.Call:
		for _, arg := range a.Call.Args {
			if c.Inherits(arg, visited) {
				return true
			}
		}

	case *ssa.Phi:
		// Since a PHI node indicates represents a potential value, we need to check that
		// all edges will inherit. We however need to make sure to stop when an object
		// has already been verified.
		match := true
		for _, edge := range a.Edges {
			match = match && c.Inherits(edge, visited)
		}
		return match

	case *ssa.Alloc:
		return c.matchStoredValues(a, visited)

	case *ssa.MakeMap:
		return c.matchStoredValues(a, visited)
	}
	return false
}

func (c *variableCandidate) matchStoredValues(addr ssa.Value, visited []ssa.Value) bool {
	values := c.e.collectStoredCtxValues(addr)
	if len(values) == 0 {
		return false
	}
	match := true
	for _, v := range values {
		match = match && c.Inherits(v, visited)
	}
	return match
}

type providerCandidate struct {
	obj types.Object
	e   *engine
}

func (c *providerCandidate) Pos() token.Pos {
	return c.obj.Pos()
}

func (c *providerCandidate) ReplacementName() string {
	return c.obj.Name() + ".Context()"
}

func (c *providerCandidate) Inherits(value ssa.Value, visited []ssa.Value) bool {
	if slices.Contains(visited, value) {
		// Prevent an infinite recursion.
		return true
	}
	visited = append(visited, value)

	switch a := value.(type) {
	case *ssa.Call:
		if c.isProvidingContext(a.Call, visited) {
			// This is the call to the provider returning the parent context.
			return true
		}
		for _, v := range a.Call.Args {
			if c.Inherits(v, visited) {
				return true
			}
		}

	case *ssa.MakeInterface:
		return c.Inherits(a.X, visited)

	case *ssa.Extract:
		return c.Inherits(a.Tuple, visited)
	}
	return false
}

func (c *providerCandidate) isProvidingContext(call ssa.CallCommon, visited []ssa.Value) bool {
	if !types.Identical(c.e.ctxProviderIFace.Method(0).Signature(), call.Signature()) {
		return false
	}
	if call.Signature().Recv() == nil || len(call.Args) == 0 {
		return false
	}

	// Only the first argument can match the provider (i.e. the receiver of the
	// function call).
	return c.isTheProvider(call.Args[0], visited)
}

func (c *providerCandidate) isTheProvider(v ssa.Value, visited []ssa.Value) bool {
	if slices.Contains(visited, v) {
		return true
	}
	visited = append(visited, v)

	switch a := v.(type) {
	case *ssa.Parameter:
		return areIdenticalVariable(a.Object(), c.obj)

	case *ssa.FreeVar:
		if binding := freeVarBinding(a); binding != nil {
			return c.isTheProvider(binding, visited)
		}

	case *ssa.UnOp:
		return c.isTheProvider(a.X, visited)

	case *ssa.Alloc:
		for _, ref := range fromReferrers(a.Referrers()) {
			if store, ok := ref.(*ssa.Store); ok && c.isTheProvider(store.Val, visited) {
				return true
			}
		}
	}
	return false
}

func (e *engine) collectStoredCtxValues(addr ssa.Value) (values []ssa.Value) {
	for _, ref := range fromReferrers(addr.Referrers()) {
		switch r := ref.(type) {
		case *ssa.Store:
			values = append(values, r.Val)

		case *ssa.FieldAddr:
			ptr, ok := r.Type().(*types.Pointer)
			if !ok || !e.isContextImpl(ptr.Elem()) {
				continue
			}
			for _, fieldRef := range fromReferrers(r.Referrers()) {
				if store, ok := fieldRef.(*ssa.Store); ok {
					values = append(values, store.Val)
				}
			}

		case *ssa.IndexAddr:
			ptr, ok := r.Type().(*types.Pointer)
			if !ok || !e.isContextImpl(ptr.Elem()) {
				continue
			}
			for _, elemRef := range fromReferrers(r.Referrers()) {
				if store, ok := elemRef.(*ssa.Store); ok {
					values = append(values, store.Val)
				}
			}

		case *ssa.MapUpdate:
			if e.isContextImpl(r.Value.Type()) {
				values = append(values, r.Value)
			}
		}
	}
	return values
}

// freeVarBinding returns the SSA value bound to the free variable in the
// enclosing function's MakeClosure instruction.
func freeVarBinding(fv *ssa.FreeVar) ssa.Value {
	closure := fv.Parent()
	outer := closure.Parent()
	if outer == nil {
		return nil
	}
	idx := slices.Index(closure.FreeVars, fv)
	if idx < 0 {
		return nil
	}
	for _, block := range outer.Blocks {
		for _, instr := range block.Instrs {
			mc, ok := instr.(*ssa.MakeClosure)
			if !ok || mc.Fn != closure || idx >= len(mc.Bindings) {
				continue
			}
			return mc.Bindings[idx]
		}
	}
	return nil
}

func (e *engine) isContextImpl(t types.Type) bool {
	return types.Implements(t, e.ctxIface)
}

func (e *engine) isContextProvider(t types.Type) bool {
	return types.Implements(t, e.ctxProviderIFace)
}

func areIdenticalVariable(a, b types.Object) bool {
	aVar, aOk := a.(*types.Var)
	bVar, bOk := b.(*types.Var)
	return aOk && bOk && aVar == bVar
}

func fromReferrers(referrers *[]ssa.Instruction) []ssa.Instruction {
	if referrers != nil {
		return *referrers
	}
	return nil
}
