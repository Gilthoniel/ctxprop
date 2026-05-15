package d

import "context"

type Handler struct {
	context.Context
}

func (h Handler) Do(ctx context.Context) error {
	_ = foo(h) // want `function call must inherit the context from the parent; use ctx instead\.`
	_ = h.inner(ctx)
	_ = h.inner(context.Background()) // want `function call must inherit the context from the parent; use ctx instead\.`
	return foo(ctx)
}

func (h Handler) inner(ctx context.Context) error {
	return ctx.Err()
}

func foo(ctx context.Context) error {
	return ctx.Err()
}
