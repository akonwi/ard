package formatter

import "testing"

// ADR 0073 pointer syntax round-trips through the formatter.

func TestFormatPointerTypes(t *testing.T) {
	input := `struct Node {
value:Int,
parent:*Node,
}
fn rename(user:*mut User,name:Str) *User {
user
}
fn maybe(user:(*User)?,inner:*User?,items:*mut [Int],result:(*mut User)!Str) {}
let callback:fn(*mut User) *User=rename
`
	want := `struct Node {
  value: Int,
  parent: *Node,
}
fn rename(user: *mut User, name: Str) *User {
  user
}
fn maybe(user: (*User)?, inner: *User?, items: *mut [Int], result: (*mut User)!Str) {}
let callback: fn(*mut User) *User = rename
`

	assertDerefFormat(t, input, want)
}

func TestFormatAddressOfAndPointerDeref(t *testing.T) {
	input := `fn main() {
let reader=&user
let writer=&mut user
let fresh=&mut User{name:"Ada"}
let field=&user.profile
let same=&a==&b
let value=writer.*
let name=writer.*.name
let loaded=load().*
let doubled=writer.* *2
let roundtrip=(&value).*
writer.*=User{name:"Grace"}
&mut user
print(items...)
print(&mut items...)
}
`
	want := `fn main() {
  let reader = &user
  let writer = &mut user
  let fresh = &mut User{name: "Ada"}
  let field = &user.profile
  let same = &a == &b
  let value = writer.*
  let name = writer.*.name
  let loaded = load().*
  let doubled = writer.* * 2
  let roundtrip = (&value).*
  writer.* = User{name: "Grace"}
  &mut user
  print(items...)
  print(&mut items...)
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatLegacyReferenceSyntaxIsPreserved(t *testing.T) {
	input := `fn update(user:mut User) {}
fn main() {
let reference=mut user
let snapshot=reference.@
}
`
	want := `fn update(user: mut User) {}
fn main() {
  let reference = mut user
  let snapshot = reference.@
}
`

	assertDerefFormat(t, input, want)
}
