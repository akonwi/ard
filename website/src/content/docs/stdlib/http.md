---
title: HTTP Operations with ard/http
description: Make HTTP requests and handle responses using the ard/http module.
---

The `ard/http` module provides functions for making HTTP requests and serving HTTP endpoints.

The http module provides:
- **HTTP methods** (GET, POST, PUT, DELETE, PATCH, OPTIONS)
- **Request structures** for building and customizing HTTP requests
- **Response structures** for handling HTTP responses
- **HTTP client** functionality for making outbound requests
- **HTTP server** functionality for handling inbound requests

```ard
use ard/http
use ard/io

fn main() {
  let req = http::Request {
    method: http::Method::Get,
    url: "https://example.com",
    headers: [:],
    body: Void?
  }
  
  match http::send(req) {
    ok(response) => io::print("Status: {response.status.to_str()}"),
    err(e) => io::print("Error: {e}")
  }
}
```

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

let method = http::Method::Post
io::print(method)  // "POST"
```

### `struct Request`

Represents an HTTP request.

- **`method: Method`** - The HTTP method
- **`url: Str`** - The request URL
- **`headers: [Str:Str]`** - Request headers as a map
- **`body: Str?`** - Optional request body

#### Methods

##### `fn path() Str?`

Get the path from an inbound request (only available on server-side requests).

```ard
use ard/http

fn handler(req: http::Request) http::Response {
  match req.path() {
    path => {
      // handle specific path
      http::Response::new(200, "OK")
    },
    _ => http::Response::new(404, "Not Found")
  }
}
```

##### `fn path_param(name: Str) Str`

Get a path parameter value from an inbound request by name.

```ard
use ard/http

fn handler(req: http::Request) http::Response {
  let id = req.path_param("id")
  http::Response::new(200, "User: {id}")
}
```

##### `fn query_param(name: Str) Str`

Get a query parameter value from an inbound request by name.

```ard
use ard/http

fn handler(req: http::Request) http::Response {
  let page = req.query_param("page")
  http::Response::new(200, "Page: {page}")
}
```

### `struct Response`

Represents an HTTP response.

- **`status: Int`** - HTTP status code
- **`headers: [Str:Str]`** - Response headers as a map
- **`body: Str`** - Response body

#### Methods

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

### `fn send(req: Request) Response!Str`

Send an HTTP request and return the response or an error message.

```ard
use ard/http

let req = http::Request {
  method: http::Method::Get,
  url: "https://api.example.com/users",
  headers: [:],
  body: Void?
}

match http::send(req) {
  ok(response) => io::print(response.body),
  err(error) => io::print("Request failed: {error}")
}
```

### `fn serve(port: Int, handlers: [Str:fn(Request) Response]) Void!Str`

Start an HTTP server on the given port with route handlers.

The handlers map keys are route paths and values are handler functions that take a request and return a response.

```ard
use ard/http
use ard/io

fn main() {
  let handlers: [Str:fn(http::Request) http::Response] = [
    "/": fn(req: http::Request) http::Response {
      http::Response::new(200, "Hello, World!")
    }
  ]
  
  http::serve(8080, handlers).expect("Failed to start server")
}
```

## Examples

### Make a GET Request

```ard
use ard/http
use ard/io

fn main() {
  let req = http::Request {
    method: http::Method::Get,
    url: "https://jsonplaceholder.typicode.com/posts/1",
    headers: [:],
    body: Void?
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

### Make a POST Request

```ard
use ard/http
use ard/io
use ard/json

fn main() {
  let body_data = ["name": "Alice", "age": 30]
  let body_json = json::encode(body_data).expect("Failed to encode")
  
  let req = http::Request {
    method: http::Method::Post,
    url: "https://api.example.com/users",
    headers: ["Content-Type": "application/json"],
    body: body_json
  }
  
  match http::send(req) {
    ok(response) => io::print(response.body),
    err(error) => io::print("Request failed: {error}")
  }
}
```

### Simple HTTP Server

```ard
use ard/http
use ard/io

fn main() {
  let handlers: [Str:fn(http::Request) http::Response] = [
    "/": fn(req: http::Request) http::Response {
      http::Response::new(200, "Welcome!")
    },
    "/about": fn(req: http::Request) http::Response {
      http::Response::new(200, "About page")
    },
    "/users/:id": fn(req: http::Request) http::Response {
      let id = req.path_param("id")
      http::Response::new(200, "User ID: {id}")
    }
  ]
  
  http::serve(3000, handlers).expect("Failed to start server")
}
```

### HTTP Server with Query Parameters

```ard
use ard/http

fn main() {
  let handlers: [Str:fn(http::Request) http::Response] = [
    "/search": fn(req: http::Request) http::Response {
      let query = req.query_param("q")
      let results = "Results for: {query}"
      http::Response::new(200, results)
    }
  ]
  
  http::serve(8080, handlers).expect("Failed to start server")
}
```
