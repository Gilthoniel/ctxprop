package a

import (
	"context"
	"strings"
)

func foo(ctx context.Context) error {
	newCtx := Wrap(ctx)
	_ = bar(ctx)
	_ = bar(newCtx)
	_ = bar(context.Background()) // want `function must inherit the context from the parent`

	anotherCtx, err := NewContext(ctx)
	_ = bar(anotherCtx)
	return err
}

func bar(ctx context.Context) error {
	return ctx.Err()
}

type MyContext struct {
	context.Context
}

func NewContext(ctx context.Context) (MyContext, error) {
	return MyContext{ctx}, ctx.Err()
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
