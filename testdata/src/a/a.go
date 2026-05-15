package a

import (
	"context"
	"strings"
)

func _(ctx context.Context) error {
	newCtx := Wrap(ctx)
	_ = bar(ctx)
	_ = bar(newCtx)
	_ = bar(context.Background()) // want `function call must inherit the context from the parent; use newCtx instead.`

	anotherCtx, err := NewContext(ctx)
	if err != nil {
		return err
	}

	_ = bar(anotherCtx)

	return nil
}

func _(ctx context.Context) error {
	anotherCtx, err := NewContext(ctx)
	if err != nil {
		return err
	}

	_ = foo(anotherCtx, anotherCtx.IsAuthenticated)
	_ = bar(anotherCtx)
	_ = bar(anotherCtx.Context)
	_ = bar(Wrap(ctx).Context)
	return nil
}

func _(ctx context.Context) error {
	_ = bar(MyContext{Context: ctx})
	return nil
}

func foo(ctx context.Context, _ bool) error {
	return ctx.Err()
}

func bar(ctx context.Context) error {
	return ctx.Err()
}

type MyContext struct {
	context.Context
	IsAuthenticated bool
}

func NewContext(ctx context.Context) (MyContext, error) {
	return MyContext{Context: ctx, IsAuthenticated: true}, ctx.Err()
}

func Wrap(ctx context.Context) MyContext {
	return MyContext{Context: ctx}
}

func (MyContext) Err() error {
	return nil
}

func _(ctx context.Context, opts ...Option) context.Context {
	for _, opt := range opts {
		ctx = opt(ctx)
	}
	return ctx
}

type Option func(context.Context) context.Context

type Service struct{}

func (s Service) Hello(ctx AuthContext, name string) string {
	name = s.ToLowerCase(ctx, name)
	return name
}

func (s Service) ToLowerCase(ctx context.Context, name string) string {
	return strings.ToLower(name)
}

type AuthContext interface {
	context.Context
}

type ExtendedAuthContext interface {
	context.Context
	IsAuthenticated() bool
}

func _(ctx ExtendedAuthContext) error {
	return bar(ctx)
}

// ssa.TypeAssert (comma-ok form lowers to Extract -> TypeAssert)
func _(ctx context.Context) error {
	if c, ok := ctx.(ExtendedAuthContext); ok {
		_ = bar(c)
	}
	if c := ctx.(ExtendedAuthContext); c.IsAuthenticated() {
		_ = bar(c)
	}
	return nil
}

// ssa.TypeAssert on a non-parent value: should still be reported.
func _(ctx context.Context, other any) error {
	if c, ok := other.(context.Context); ok {
		_ = bar(c) // want `function call must inherit the context from the parent; use ctx instead.`
	}
	return nil
}

// ssa.IndexAddr + ssa.Slice on a ssa.Alloc backing array (slice literal).
func _(ctx context.Context) error {
	ctxs := []context.Context{ctx}
	return bar(ctxs[0])
}

// ssa.IndexAddr on a ssa.Alloc'd array (no Slice).
func _(ctx context.Context) error {
	var arr [1]context.Context
	arr[0] = ctx
	return bar(arr[0])
}

// ssa.IndexAddr where the stored value does not derive from the parent ctx.
func _(ctx context.Context) error {
	ctxs := []context.Context{context.Background()}
	return bar(ctxs[0]) // want `function call must inherit the context from the parent; use ctx instead.`
}

// ssa.Lookup + ssa.MakeMap with ssa.MapUpdate carrying the parent ctx.
func _(ctx context.Context) error {
	m := map[string]context.Context{"k": ctx}
	return bar(m["k"])
}

// ssa.Lookup where the map's MapUpdate does not derive from the parent ctx.
func _(ctx context.Context) error {
	m := map[string]context.Context{"k": context.Background()}
	return bar(m["k"]) // want `function call must inherit the context from the parent; use ctx instead.`
}

// ssa.MakeMap with no MapUpdate referrers (empty map literal).
func _(ctx context.Context) error {
	m := map[string]context.Context{}
	return bar(m["k"]) // want `function call must inherit the context from the parent; use ctx instead.`
}

// ssa.IndexAddr guard: array element type is `any`, not context.
func _(ctx context.Context) error {
	arr := [1]any{ctx}
	return bar(arr[0].(context.Context)) // want `function call must inherit the context from the parent; use ctx instead.`
}

// ssa.MapUpdate guard: map value type is `any`, not context.
func _(ctx context.Context) error {
	m := map[string]any{"k": ctx}
	return bar(m["k"].(context.Context)) // want `function call must inherit the context from the parent; use ctx instead.`
}

// ssa.FreeVar: closure captures the parent ctx and propagates it.
func _(ctx context.Context) error {
	go func() {
		_ = bar(ctx)
	}()
	return nil
}

// ssa.FreeVar: closure captures the parent ctx but also issues a call with
// a fresh context — the bad call must be flagged.
func _(ctx context.Context) error {
	go func() {
		_ = bar(ctx)
		_ = bar(context.Background()) // want `function call must inherit the context from the parent; use ctx instead.`
	}()
	return nil
}

// Closure does not capture ctx; outer ctx is still in scope and the call
// with a fresh context must be flagged.
func _(ctx context.Context) error {
	go func() {
		_ = bar(context.Background()) // want `function call must inherit the context from the parent; use ctx instead.`
	}()
	return nil
}

// Nested closure: inner closure propagates ctx through the middle closure's
// FreeVar capture.
func _(ctx context.Context) error {
	func() {
		func() {
			_ = bar(ctx)
			_ = bar(context.Background()) // want `function call must inherit the context from the parent; use ctx instead.`
		}()
	}()
	return nil
}

func _(ctx context.Context, n int) error {
	named := ctx
	for i := 0; i < n; i++ {
		named = Wrap(named).Context
	}
	_ = bar(context.Background()) // want `function call must inherit the context from the parent; use named instead\.`
	return nil
}

func _(ctx context.Context) error {
	named := (Wrap(ctx))
	_ = named
	_ = bar(context.Background()) // want `function call must inherit the context from the parent; use named instead\.`
	return nil
}

func _(ctx context.Context) error {
	a, b := ctx, ctx
	_ = a
	_ = b
	_ = bar(context.Background()) // want `function call must inherit the context from the parent; use ctx instead\.`
	return nil
}
