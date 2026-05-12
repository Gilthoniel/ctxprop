[![codecov](https://codecov.io/github/Gilthoniel/ctxprop/graph/badge.svg?token=6XXTUHLZ51)](https://codecov.io/github/Gilthoniel/ctxprop)
[![Go Reference](https://pkg.go.dev/badge/github.com/Gilthoniel/ctxprop.svg)](https://pkg.go.dev/github.com/Gilthoniel/ctxprop)

# ctxprop

`ctxprop` is a Golang static linter that will report unproper propogation of contexts.

## Installation

Linter can be installed as a standalone binary.

```Shell
go install github.com/Gilthoniel/ctxprop/cmd/ctxprop@latest
```

Or built locally:

```Shell
make build
make install
```

## Usage

It can be invoked as a single linter but it is recommended to run it through golangci-lint.

```Shell
ctxprop [-flag] [package]
```

## What it checks

`ctxprop` reports calls that pass a `context.Context` that is **not derived from
the parent context** already in scope. The parent is found by walking the
enclosing function (and its parents, for closures) and looking for either:

- a parameter implementing `context.Context`, or
- a parameter exposing a `Context() context.Context` method (a *context
  provider*, e.g. `*http.Request`).

A context "inherits" from the parent if it is the parent itself, a value
derived from it (wrappers like `context.WithCancel`, struct embedding, type
assertions, slice/map lookups storing it, …), or a value returned by the
provider's `Context()` method.

## Examples

```go
func handle(ctx context.Context) error {
    return work(context.Background()) // want: must inherit the context from the parent — use ctx instead
}

func handle(ctx context.Context) error {
    sub, cancel := context.WithCancel(ctx)
    defer cancel()
    return work(sub) // ok
}

func serve() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        foo(r.Context())              // ok
        foo(context.Background())     // want: use r.Context() instead
    })
}

func addHandler(ctx context.Context) {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        foo(r.Context()) // ok
        foo(ctx)         // want: use r.Context() instead
    })
}

func run(ctx context.Context) {
    go func() {
        _ = work(ctx)                 // ok
        _ = work(context.Background()) // want: use ctx instead
    }()
}

type MyContext struct {
    context.Context
    IsAuthenticated bool
}

func handle(ctx context.Context) {
    mc := MyContext{Context: ctx}
    _ = work(mc)         // ok
    _ = work(mc.Context) // ok
}
```
