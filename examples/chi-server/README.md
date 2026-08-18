# chi Graceful-Shutdown Server

A pure-Ard port of chi's [graceful-shutdown example](https://github.com/go-chi/chi/blob/master/_examples/graceful/main.go):
an HTTP server built on [chi](https://github.com/go-chi/chi) that finishes in-flight
requests before exiting on `SIGINT` or `SIGTERM`.

There is no Go shim — everything is direct `use go:` interop:

- `chi::NewRouter()` returned as `mut chi::Mux` and passed where `http::Handler`
  is expected (Go interface satisfaction across packages)
- chi middleware passed by reference: `router.Use(middleware::Logger)`
  (Go functions as first-class values)
- Ard closures as `http.HandlerFunc` route handlers, with value-producing
  bodies discarded for the void callback
- a keyed `http::Server` struct literal with omitted fields, stored as an
  explicit reference for pointer-receiver methods (`ListenAndServe`, `Shutdown`)
- OS signals through a built-in channel: `Chan::new<os::Signal>()` narrowed
  with `.sender()` for `signal::Notify`
- `async::start` for the background serve goroutine
- Go errors handled as identity-preserving Ard results (`Void!Error`)

## Adaptations from the Go original

- `signal.NotifyContext` and `context.WithTimeout` return non-error value
  pairs, which Ard's Go interop does not map; the port registers `SIGINT` and
  `SIGTERM` through `signal.Notify`, waits on a channel, and shuts down with
  `context.Background()`.
- Go error identity is preserved, so `errors.Is(error, http.ErrServerClosed)`
  checks the server sentinel directly.

## Run

```sh
ard run main.ard
```

Then, in another terminal:

```sh
curl http://localhost:3333/        # "sup"
curl http://localhost:3333/slow &  # takes 5 seconds
kill -INT <server pid>             # SIGTERM also shuts down gracefully
```

The server logs `shutting down`, the in-flight `/slow` request finishes with
`all done.`, and the process exits with `goodbye`.
