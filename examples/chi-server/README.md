# chi Graceful-Shutdown Server

A pure-Ard port of chi's [graceful-shutdown example](https://github.com/go-chi/chi/blob/master/_examples/graceful/main.go):
an HTTP server built on [chi](https://github.com/go-chi/chi) that finishes in-flight
requests before exiting on `SIGINT`.

There is no Go shim — everything is direct `use go:` interop:

- `chi::NewRouter()` returned as `mut chi::Mux` and passed where `http::Handler`
  is expected (Go interface satisfaction across packages)
- chi middleware (`RequestID`, `Logger`) applied through `router.Use`
- Ard closures as `http.HandlerFunc` route handlers, with value-producing
  bodies discarded for the void callback
- a keyed `http::Server` struct literal with omitted fields, and
  pointer-receiver methods (`ListenAndServe`, `Shutdown`) on the `mut` binding
- OS signals through a built-in channel: `Chan::new<os::Signal>()` narrowed
  with `.sender()` for `signal::Notify`
- `async::start` for the background serve goroutine
- Go `(T, error)` returns handled as Ard results (`Void!Str`)

## Adaptations from the Go original

- `signal.NotifyContext` and `context.WithTimeout` return non-error value
  pairs, which Ard's Go interop does not map; the port waits on a signal
  channel and shuts down with `context.Background()`.
- Go variadic parameters take one Ard argument, so the server listens for
  `SIGINT` (the original also registers `SIGTERM`).
- Go errors are stringified, so `errors.Is(err, http.ErrServerClosed)` becomes
  a message comparison.
- Go functions are not yet first-class values
  ([#263](https://github.com/akonwi/ard/issues/263)), so middleware is wrapped
  in closures instead of passed by reference.

## Run

```sh
ard run main.ard
```

Then, in another terminal:

```sh
curl http://localhost:3333/        # "sup"
curl http://localhost:3333/slow &  # takes 5 seconds
kill -INT <server pid>             # graceful: /slow still completes
```

The server logs `shutting down`, the in-flight `/slow` request finishes with
`all done.`, and the process exits with `goodbye`.
