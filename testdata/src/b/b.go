package b

import (
	"context"
	"net/http"
)

func _() {
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

func _(ctx context.Context) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Here we expect the context from the request.
		foo(r.Context())
		foo(ctx) // want `function must inherit the context from the parent`
	})
}

func _(r *http.Request) {
	func() {
		foo(r.Context())
		foo(context.Background()) // want `function must inherit the context from the parent`
	}()
}

func _(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if v := r.Header.Get("Authorization"); v != "" {
			ctx = context.WithValue(ctx, struct{}{}, v)
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func _(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if r.Header.Get("Reset") != "" {
			ctx = context.Background()
		}
		h.ServeHTTP(w, r.WithContext(ctx)) // want `function must inherit the context from the parent`
	})
}

func _(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		for i := 0; i < 3; i++ {
			ctx = context.WithValue(ctx, struct{}{}, i)
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func foo(ctx context.Context) error {
	return ctx.Err()
}
