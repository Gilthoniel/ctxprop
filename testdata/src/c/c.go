package c

import "context"

func _(ctx context.Context, auth AuthContext) error {
	if err := bar(ctx, auth); err != nil {
		return err
	}
	return ctx.Err()
}

func bar(_ context.Context, auth AuthContext) error {
	return auth.Err()
}

type AuthContext interface {
	context.Context
	IsAuthenticated() bool
}
