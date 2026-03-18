package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
)

func TestExternType(t *testing.T) {
	run(t, []test{
		{
			name: "extern type can be declared",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
			}, "\n"),
		},
		{
			name: "private extern type can be declared",
			input: strings.Join([]string{
				`private extern type ConnectionPtr`,
			}, "\n"),
		},
		{
			name: "extern type can be used in extern fn signatures",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr = "SqlConnect"`,
				`extern fn close(db: ConnectionPtr) Void = "SqlClose"`,
			}, "\n"),
		},
		{
			name: "extern type can be used as struct field",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`struct Database {`,
				`  ptr: ConnectionPtr,`,
				`  path: Str,`,
				`}`,
			}, "\n"),
		},
		{
			name: "extern type can be wrapped in Maybe",
			input: strings.Join([]string{
				`extern type RawRequest`,
				`extern fn get_req(r: RawRequest?) Str = "GetReq"`,
			}, "\n"),
		},
		{
			name: "extern type can be wrapped in Result",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr!Str = "SqlConnect"`,
			}, "\n"),
		},
		{
			name: "extern type can be used in lists",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn get_all() [ConnectionPtr] = "GetAll"`,
			}, "\n"),
		},
		{
			name: "extern type cannot be compared with ==",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr = "SqlConnect"`,
				`let a = connect("a")`,
				`let b = connect("b")`,
				`let same = a == b`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid: ConnectionPtr == ConnectionPtr"},
			},
		},
		{
			name: "duplicate extern type declaration",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern type ConnectionPtr`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Duplicate declaration: ConnectionPtr"},
			},
		},
		{
			name: "different extern types are not interchangeable",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern type TransactionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr = "SqlConnect"`,
				`extern fn close(db: TransactionPtr) Void = "SqlClose"`,
				`let conn = connect("x")`,
				`close(conn)`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected TransactionPtr, got ConnectionPtr"},
			},
		},
	})
}
