package a

import "context"

func foo(ctx context.Context) error {
	bar(ctx)
	bar(context.Background())
	return ctx.Err()
}

func bar(ctx context.Context) error {
	return ctx.Err()
}
