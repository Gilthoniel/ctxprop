package d

import "context"

type Handler struct {
	context.Context
}

func (h Handler) Do(ctx context.Context) error {
	_ = foo(h) // want `function must inherit the context from the parent`
	return foo(ctx)
}

func foo(ctx context.Context) error {
	return ctx.Err()
}
