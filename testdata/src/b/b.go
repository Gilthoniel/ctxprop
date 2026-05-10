package b

import (
	"context"
	"net/http"
)

func serve() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		foo(r.Context())
		foo(context.Background()) // want `function must inherit the context from the parent`
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		foo(ctx)
		foo(context.Background()) // want `function must inherit the context from the parent`
	})
}

func addHandler(ctx context.Context) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Here we expect the context from the request.
		foo(r.Context())
		foo(ctx) // want `function must inherit the context from the parent`
	})
}

func foo(ctx context.Context) error {
	return ctx.Err()
}
