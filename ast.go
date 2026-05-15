package ctxprop

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/ssa"
)

// suggestions implements an analyzer of the AST to find the proper replacement
// suggestion of a bad context inheritance since SSA is not sufficient to find
// out.
type suggestions struct {
	engine     *engine
	instr      ssa.Instruction
	scope      *ssa.Function
	candidates []parentCandidate
}

func (s suggestions) Find() (suggestion Suggestion, ok bool) {
	if s.scope == nil || s.scope.Pos() == token.NoPos {
		return Suggestion{}, false
	}
	block := s.findBody(s.scope.Pos())
	if block == nil {
		return Suggestion{}, false
	}

	ast.PreorderStack(block, nil, func(n ast.Node, stack []ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign {
			return true
		}
		if assign.Pos() >= s.instr.Pos() {
			return false
		}
		name, matched := s.tryFindSuggestion(assign)
		if matched {
			suggestion = Suggestion{Name: name}
			ok = true
			return false
		}
		return true
	})
	return
}

func (s suggestions) findBody(pos token.Pos) *ast.BlockStmt {
	for _, f := range s.engine.pass.Files {
		if pos < f.FileStart || pos > f.FileEnd {
			continue
		}
		path, _ := astutil.PathEnclosingInterval(f, pos, pos)
		for _, node := range path {
			switch fn := node.(type) {
			case *ast.FuncDecl:
				return fn.Body
			case *ast.FuncLit:
				return fn.Body
			}
		}
	}
	return nil
}

func (s suggestions) tryFindSuggestion(assign *ast.AssignStmt) (string, bool) {
	for i, expr := range assign.Lhs {
		name, valid := extractIdent(expr)
		if !valid || !s.hasValidInheritance(assign, i, expr) {
			continue
		}
		return name, true
	}
	return "", false
}

func (s suggestions) hasValidInheritance(assign *ast.AssignStmt, idx int, expr ast.Expr) bool {
	if named := s.findNamedContextAt(expr.Pos()); named != nil {
		return anyInherits(s.candidates, named, nil) == fullInheritance
	}

	if len(assign.Rhs) != 1 {
		return false
	}
	val := s.findValueAt(rhsValuePos(assign.Rhs[0]))
	if val == nil {
		return false
	}
	exprType := val.Type()
	if tuple, isTuple := val.Type().(*types.Tuple); isTuple {
		if idx >= tuple.Len() {
			return false
		}
		exprType = tuple.At(idx).Type()
	}
	return s.engine.isContextImpl(exprType) && anyInherits(s.candidates, val, nil) == fullInheritance
}

func (s suggestions) findNamedContextAt(pos token.Pos) ssa.Value {
	for _, block := range s.scope.Blocks {
		for _, instr := range block.Instrs {
			switch v := instr.(type) {
			case *ssa.Alloc:
				if v.Pos() != pos {
					continue
				}
				ptr, ok := v.Type().(*types.Pointer)
				if ok && s.engine.isContextImpl(ptr.Elem()) {
					return v
				}

			case *ssa.Phi:
				if v.Pos() == pos && s.engine.isContextImpl(v.Type()) {
					return v
				}
			}
		}
	}
	return nil
}

func (s suggestions) findValueAt(pos token.Pos) ssa.Value {
	for _, block := range s.scope.Blocks {
		for _, instr := range block.Instrs {
			if v, ok := instr.(ssa.Value); ok && v.Pos() == pos {
				return v
			}
		}
	}
	return nil
}

func rhsValuePos(expr ast.Expr) token.Pos {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return rhsValuePos(e.X)
	case *ast.CallExpr:
		return e.Lparen
	case *ast.TypeAssertExpr:
		return e.Lparen
	default:
	}
	return expr.Pos()
}

func extractIdent(e ast.Expr) (string, bool) {
	id, ok := e.(*ast.Ident)
	if !ok || id.Name == "_" {
		return "", false
	}
	return id.Name, true
}

type Suggestion struct {
	Name string
}
