---
title: HTTP Operations with ard/http
description: Make HTTP requests and handle responses using the ard/http module.
---

The `ard/http` module provides APIs for making outbound HTTP requests and serving inbound HTTP endpoints.

The module includes:
- **HTTP methods** (`GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `OPTIONS`)
- **Request structures** for outbound and inbound requests
- **Response structures** for handling responses
- **HTTP client** functionality for outbound requests
- **HTTP server** functionality for route handlers

## Quick start

```ard
use ard/http
use ard/io

fn main() {
  let req = http::Request{
    method: http::Method::Get,
    url: "https://example.com",
    headers: [:],
  }

  match http::send(req, 30) {
    ok(response) => io::print("Status: {response.status.to_str()}"),
    err(e) => io::print("Error: {e}")
  }
}
```

In `http::send(req, timeout)`:
- the call-site `timeout` wins if provided
- otherwise `req.timeout` is used
- if neither is set, no request timeout is applied by Ard

## API

### `enum Method`

HTTP methods for requests.

- **`Get`** - GET request
- **`Post`** - POST request
- **`Put`** - PUT request
- **`Del`** - DELETE request
- **`Patch`** - PATCH request
- **`Options`** - OPTIONS request

The `Method` enum implements `ToString`:

```ard
use ard/http
use ard/io

let method = http::Method::Post
io::print(method) // "POST"
```

### `struct Request`

Represents an HTTP request.

- **`method: Method`** - The HTTP method
- **`url: Str`** - The request URL
- **`headers: [Str:Str]`** - Request headers as a map
- **`body: Dynamic?`** - Optional request body
- **`timeout: Int?`** - Optional request timeout in seconds

`Request.body` is dynamic so you can support text, JSON payloads, and other shapes without forcing a string-only API. For server handlers, decode `req.body` into the expected shape with `ard/decode`.

Timeouts are expressed in whole seconds. You can pass a timeout directly to `http::send(req, timeout)` without any duration conversion.

#### Methods

##### `fn path() Str?`

Get the path from an inbound request.

```ard
use ard/http

fn handler(req: http::Request, mut res: http::Response) {
  match req.path() {
    path => res.body = "Path: {path}",
    _ => {
      res.status = 404
      res.body = "Not Found"
    }
  }
}
```

##### `fn path_param(name: Str) Str`

Get a path parameter value from an inbound request by name.

```ard
use ard/http

fn handler(req: http::Request, mut res: http::Response) {
  let id = req.path_param("id")
  res.body = "User: {id}"
}
```

##### `fn query_param(name: Str) Str`

Get a query parameter value from an inbound request by name.

```ard
use ard/http

fn handler(req: http::Request, mut res: http::Response) {
  let page = req.query_param("page")
  res.body = "Page: {page}"
}
```

### `struct Response`

Represents an HTTP response.

- **`status: Int`** - HTTP status code
- **`headers: [Str:Str]`** - Response headers as a map
- **`body: Str`** - Response body

##### `fn Response::new(status: Int, body: Str) Response`

Create a new response with the given status code and body.

```ard
use ard/http

let response = http::Response::new(200, "OK")
```

##### `fn is_ok() Bool`

Check if the response status indicates success (2xx status code).

```ard
use ard/http

if response.is_ok() {
  // Handle successful response
}
```

### `fn send(req: Request, timeout: Int?) Response!Str`

Send an HTTP request and return the response or an error message.

The optional `timeout` argument overrides `req.timeout`.

```ard
use ard/http
use ard/io

let req = http::Request{
  method: http::Method::Get,
  url: "https://api.example.com/users",
  headers: [:],
}

match http::send(req, 60) {
  ok(response) => io::print(response.body),
  err(error) => io::print("Request failed: {error}")
}
```

### `fn serve(port: Int, handlers: [Str:fn(Request, mut Response)]) Void!Str`

Start an HTTP server on the given port with route handlers.

The handlers map keys are route paths and values are handler functions that take a request and a mutable response object. Handlers modify the response object directly.

```ard
use ard/http

fn main() {
  let handlers: [Str:fn(http::Request, mut http::Response)] = [
    "/": fn(req: http::Request, mut res: http::Response) {
      res.body = "Hello, World!"
      res.status = 200
    }
  ]

  http::serve(8080, handlers).expect("Failed to start server")
}
```

## Examples

### Make a GET request

```ard
use ard/http
use ard/io

fn main() {
  let req = http::Request{
    method: http::Method::Get,
    url: "https://jsonplaceholder.typicode.com/posts/1",
    headers: [:],
  }

  match http::send(req) {
    ok(response) => {
      if response.is_ok() {
        io::print(response.body)
      } else {
        io::print("Error: {response.status.to_str()}")
      }
    },
    err(error) => io::print("Request failed: {error}")
  }
}
```

### Make a POST request

```ard
use ard/http
use ard/io
use ard/json

fn main() {
  let body_data = ["name": "Alice", "age": 30]
  let body_json = json::encode(body_data).expect("Failed to encode")

  let req = http::Request{
    method: http::Method::Post,
    url: "https://api.example.com/users",
    headers: ["Content-Type": "application/json"],
    body: body_json,
  }

  match http::send(req) {
    ok(response) => io::print(response.body),
    err(error) => io::print("Request failed: {error}")
  }
}
```

### Long-running request with timeout override

```ard
use ard/http

fn main() {
  let req = http::Request{
    method: http::Method::Post,
    url: "https://api.openai.com/v1/responses",
    headers: ["content-type": "application/json"],
    body: "{\"model\":\"gpt-4.1\"}",
  }

  let res = http::send(req, 90)
}
```

### Decode an inbound JSON body

```ard
use ard/http
use ard/decode

fn main() {
  let handlers: [Str:fn(http::Request, mut http::Response)] = [
    "/api/auth/sign-up": fn(req: http::Request, mut res: http::Response) {
      let raw_body = try req.body -> _ {
        res.status = 400
        res.body = "Missing request body"
      }

      let body_text = try decode::run(raw_body, decode::string) -> errs {
        res.status = 400
        res.body = "Body must be text: {decode::flatten(errs)}"
      }

      let payload = try decode::from_json(body_text) -> err {
        res.status = 400
        res.body = "Invalid JSON: {err}"
      }

      let email = try decode::run(payload, decode::field("email", decode::string)) -> errs {
        res.status = 400
        res.body = "Missing email: {decode::flatten(errs)}"
      }

      res.status = 201
      res.body = "Created user with email {email}"
    }
  ]

  http::serve(8000, handlers).expect("Failed to start server")
}
```

### Simple HTTP server

```ard
use ard/http

fn main() {
  let handlers: [Str:fn(http::Request, mut http::Response)] = [
    "/": fn(req: http::Request, mut res: http::Response) {
      res.body = "Welcome!"
    },
    "/about": fn(req: http::Request, mut res: http::Response) {
      res.body = "About page"
    },
    "/users/:id": fn(req: http::Request, mut res: http::Response) {
      let id = req.path_param("id")
      res.body = "User ID: {id}"
    }
  ]

  http::serve(3000, handlers).expect("Failed to start server")
}
```

### HTTP server with query parameters

```ard
use ard/http

fn main() {
  let handlers: [Str:fn(http::Request, mut http::Response)] = [
    "/search": fn(req: http::Request, mut res: http::Response) {
      let query = req.query_param("q")
      res.body = "Results for: {query}"
    }
  ]

  http::serve(8080, handlers).expect("Failed to start server")
}
```
