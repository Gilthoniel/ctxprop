package a

import "context"

func foo(ctx context.Context) error {
	newCtx := Wrap(ctx)
	bar(ctx)
	bar(newCtx)
	bar(context.Background()) // want `function must inherit the context from the parent`
	return ctx.Err()
}

func bar(ctx context.Context) error {
	return ctx.Err()
}

type MyContext struct {
	context.Context
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
