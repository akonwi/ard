package ir

import (
	"strings"
	"testing"
)

func TestValidateModuleValid(t *testing.T) {
	module := &Module{
		Path:        "demo/main",
		PackageName: "main",
		Decls: []Decl{
			&StructDecl{
				Name: "Person",
				Fields: []Field{
					{Name: "name", Type: StrType},
					{Name: "age", Type: IntType},
				},
			},
			&EnumDecl{
				Name: "Color",
				Values: []EnumValue{
					{Name: "Red", Value: 0},
					{Name: "Blue", Value: 1},
				},
			},
			&UnionDecl{
				Name:  "Value",
				Types: []Type{IntType, StrType},
			},
			&ExternTypeDecl{
				Name: "Handle",
			},
			&VarDecl{
				Name:  "AppName",
				Type:  StrType,
				Value: &LiteralExpr{Kind: "str", Value: "demo"},
			},
			&FuncDecl{
				Name:          "read_name",
				Params:        []Param{{Name: "id", Type: IntType}},
				Return:        StrType,
				IsExtern:      true,
				ExternBinding: "ReadName",
			},
			&FuncDecl{
				Name:   "main",
				Params: []Param{},
				Return: Void,
				Body: &Block{
					Stmts: []Stmt{
						&ExprStmt{
							Value: &CallExpr{
								Callee: &IdentExpr{Name: "print"},
								Args:   []Expr{&LiteralExpr{Kind: "str", Value: "ok"}},
							},
						},
						&MemberAssignStmt{
							Subject: &IdentExpr{Name: "self"},
							Field:   "value",
							Value:   &LiteralExpr{Kind: "int", Value: "1"},
						},
						&ForIntRangeStmt{
							Cursor: "item",
							Start:  &LiteralExpr{Kind: "int", Value: "0"},
							End:    &LiteralExpr{Kind: "int", Value: "2"},
							Body: &Block{
								Stmts: []Stmt{
									&ExprStmt{Value: &IdentExpr{Name: "item"}},
								},
							},
						},
						&ForLoopStmt{
							InitName:  "j",
							InitValue: &LiteralExpr{Kind: "int", Value: "0"},
							Cond: &CallExpr{
								Callee: &IdentExpr{Name: "int_lt"},
								Args: []Expr{
									&IdentExpr{Name: "j"},
									&LiteralExpr{Kind: "int", Value: "3"},
								},
							},
							Update: &AssignStmt{
								Target: "j",
								Value: &CallExpr{
									Callee: &IdentExpr{Name: "int_add"},
									Args: []Expr{
										&IdentExpr{Name: "j"},
										&LiteralExpr{Kind: "int", Value: "1"},
									},
								},
							},
							Body: &Block{
								Stmts: []Stmt{
									&ExprStmt{Value: &IdentExpr{Name: "j"}},
								},
							},
						},
						&ForInStrStmt{
							Cursor: "char",
							Index:  "idx",
							Value:  &LiteralExpr{Kind: "str", Value: "ab"},
							Body: &Block{
								Stmts: []Stmt{
									&ExprStmt{Value: &IdentExpr{Name: "idx"}},
								},
							},
						},
						&ForInListStmt{
							Cursor: "value",
							List:   &IdentExpr{Name: "values"},
							Body: &Block{
								Stmts: []Stmt{
									&ExprStmt{Value: &IdentExpr{Name: "value"}},
								},
							},
						},
						&ForInMapStmt{
							Key:   "k",
							Value: "v",
							Map:   &IdentExpr{Name: "items"},
							Body: &Block{
								Stmts: []Stmt{
									&ExprStmt{Value: &IdentExpr{Name: "v"}},
								},
							},
						},
						&ExprStmt{
							Value: &ListLiteralExpr{
								Type: &ListType{Elem: IntType},
								Elements: []Expr{
									&LiteralExpr{Kind: "int", Value: "1"},
								},
							},
						},
						&ExprStmt{
							Value: &MapLiteralExpr{
								Type: &MapType{Key: StrType, Value: IntType},
								Entries: []MapEntry{
									{
										Key:   &LiteralExpr{Kind: "str", Value: "a"},
										Value: &LiteralExpr{Kind: "int", Value: "1"},
									},
								},
							},
						},
						&ExprStmt{
							Value: &StructLiteralExpr{
								Type: &NamedType{Name: "Person"},
								Fields: []StructFieldValue{
									{Name: "name", Value: &LiteralExpr{Kind: "str", Value: "ari"}},
									{Name: "age", Value: &LiteralExpr{Kind: "int", Value: "1"}},
								},
							},
						},
						&ExprStmt{
							Value: &EnumVariantExpr{
								Type:         &NamedType{Name: "Color"},
								Discriminant: 1,
							},
						},
						&ExprStmt{
							Value: &IfExpr{
								Cond: &LiteralExpr{Kind: "bool", Value: "true"},
								Then: &Block{Stmts: []Stmt{
									&ExprStmt{Value: &LiteralExpr{Kind: "int", Value: "1"}},
								}},
								Else: &Block{Stmts: []Stmt{
									&ExprStmt{Value: &LiteralExpr{Kind: "int", Value: "0"}},
								}},
								Type: IntType,
							},
						},
						&ExprStmt{
							Value: &CopyExpr{
								Value: &IdentExpr{Name: "values"},
								Type:  &ListType{Elem: IntType},
							},
						},
						&ExprStmt{
							Value: &UnionMatchExpr{
								Subject: &IdentExpr{Name: "input"},
								Cases: []UnionMatchCase{
									{
										Type:    IntType,
										Pattern: "num",
										Body: &Block{
											Stmts: []Stmt{
												&AssignStmt{Target: "_", Value: &IdentExpr{Name: "num"}},
											},
										},
									},
									{
										Type:    StrType,
										Pattern: "text",
										Body: &Block{
											Stmts: []Stmt{
												&AssignStmt{Target: "_", Value: &IdentExpr{Name: "text"}},
											},
										},
									},
								},
								CatchAll: &Block{
									Stmts: []Stmt{
										&ExprStmt{Value: &LiteralExpr{Kind: "void", Value: "()"}},
									},
								},
								Type: Void,
							},
						},
						&ExprStmt{
							Value: &TryExpr{
								Kind:     "result",
								Subject:  &IdentExpr{Name: "res"},
								CatchVar: "err",
								Catch: &Block{
									Stmts: []Stmt{
										&ReturnStmt{Value: &LiteralExpr{Kind: "int", Value: "0"}},
									},
								},
								Type: IntType,
							},
						},
						&ExprStmt{
							Value: &TryExpr{
								Kind:    "result",
								Subject: &IdentExpr{Name: "res"},
								Catch:   nil,
								Type:    IntType,
							},
						},
						&WhileStmt{
							Cond: &LiteralExpr{Kind: "bool", Value: "false"},
							Body: &Block{
								Stmts: []Stmt{
									&BreakStmt{},
									&ReturnStmt{},
								},
							},
						},
						&ReturnStmt{},
					},
				},
			},
		},
	}

	if err := ValidateModule(module); err != nil {
		t.Fatalf("expected valid module, got error: %v", err)
	}
}

func TestValidateModuleInvalid(t *testing.T) {
	tests := []struct {
		name    string
		module  *Module
		wantErr string
	}{
		{
			name:    "nil module",
			module:  nil,
			wantErr: "nil module",
		},
		{
			name: "empty package name",
			module: &Module{
				PackageName: "",
			},
			wantErr: "package name is empty",
		},
		{
			name: "duplicate declaration names",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{Name: "main", Return: Void, Body: &Block{}},
					&FuncDecl{Name: "main", Return: Void, Body: &Block{}},
				},
			},
			wantErr: "duplicate declaration name",
		},
		{
			name: "extern function missing binding",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:     "x",
						Return:   IntType,
						IsExtern: true,
					},
				},
			},
			wantErr: "extern function binding is empty",
		},
		{
			name: "non-extern function nil body",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: IntType,
					},
				},
			},
			wantErr: "non-extern function body is nil",
		},
		{
			name: "invalid assign statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&AssignStmt{Target: "", Value: &LiteralExpr{Kind: "int", Value: "1"}},
							},
						},
					},
				},
			},
			wantErr: "assign target is empty",
		},
		{
			name: "invalid break statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								(*BreakStmt)(nil),
							},
						},
					},
				},
			},
			wantErr: "nil break statement",
		},
		{
			name: "invalid member assign statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&MemberAssignStmt{
									Subject: &IdentExpr{Name: "self"},
									Field:   "",
									Value:   &LiteralExpr{Kind: "int", Value: "1"},
								},
							},
						},
					},
				},
			},
			wantErr: "member assign field is empty",
		},
		{
			name: "invalid for-int-range statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ForIntRangeStmt{
									Cursor: "i",
									End:    &LiteralExpr{Kind: "int", Value: "1"},
									Body:   &Block{},
								},
							},
						},
					},
				},
			},
			wantErr: "for-int-range start is nil",
		},
		{
			name: "invalid while statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&WhileStmt{
									Cond: &LiteralExpr{Kind: "bool", Value: "true"},
								},
							},
						},
					},
				},
			},
			wantErr: "while body is nil",
		},
		{
			name: "invalid for-loop update statement kind",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ForLoopStmt{
									InitName:  "i",
									InitValue: &LiteralExpr{Kind: "int", Value: "0"},
									Cond:      &LiteralExpr{Kind: "bool", Value: "true"},
									Update:    &ExprStmt{Value: &IdentExpr{Name: "i"}},
									Body:      &Block{},
								},
							},
						},
					},
				},
			},
			wantErr: "for-loop update must be assign statement",
		},
		{
			name: "invalid for-in-list statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ForInListStmt{
									Cursor: "",
									List:   &IdentExpr{Name: "values"},
									Body:   &Block{},
								},
							},
						},
					},
				},
			},
			wantErr: "for-in-list cursor is empty",
		},
		{
			name: "invalid for-in-map statement",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ForInMapStmt{
									Key:   "k",
									Value: "v",
									Body:  &Block{},
								},
							},
						},
					},
				},
			},
			wantErr: "for-in-map map is nil",
		},
		{
			name: "invalid list literal expression type",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &ListLiteralExpr{
										Type: Dynamic,
									},
								},
							},
						},
					},
				},
			},
			wantErr: "list literal type must be list type",
		},
		{
			name: "invalid struct literal duplicate fields",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &StructLiteralExpr{
										Type: &NamedType{Name: "Person"},
										Fields: []StructFieldValue{
											{Name: "name", Value: &LiteralExpr{Kind: "str", Value: "a"}},
											{Name: "name", Value: &LiteralExpr{Kind: "str", Value: "b"}},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: "struct literal field[1] duplicate name",
		},
		{
			name: "invalid if expression missing type",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &IfExpr{
										Cond: &LiteralExpr{Kind: "bool", Value: "true"},
										Then: &Block{},
										Else: &Block{},
									},
								},
							},
						},
					},
				},
			},
			wantErr: "if expression type is nil",
		},
		{
			name: "invalid union match expression cases",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &UnionMatchExpr{
										Subject: &IdentExpr{Name: "value"},
										Type:    IntType,
									},
								},
							},
						},
					},
				},
			},
			wantErr: "union match cases are empty",
		},
		{
			name: "invalid try expression kind",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &TryExpr{
										Kind:    "unknown",
										Subject: &IdentExpr{Name: "value"},
										Catch:   &Block{},
										Type:    IntType,
									},
								},
							},
						},
					},
				},
			},
			wantErr: "try expression kind must be result or maybe",
		},
		{
			name: "invalid try expression catch var without catch block",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &TryExpr{
										Kind:     "result",
										Subject:  &IdentExpr{Name: "value"},
										CatchVar: "err",
										Catch:    nil,
										Type:     IntType,
									},
								},
							},
						},
					},
				},
			},
			wantErr: "try expression catch var requires catch block",
		},
		{
			name: "invalid copy expression type",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Return: Void,
						Body: &Block{
							Stmts: []Stmt{
								&ExprStmt{
									Value: &CopyExpr{
										Value: &IdentExpr{Name: "items"},
										Type:  IntType,
									},
								},
							},
						},
					},
				},
			},
			wantErr: "copy expression type must be list type",
		},
		{
			name: "invalid type reference",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&StructDecl{
						Name: "x",
						Fields: []Field{
							{Name: "a", Type: &NamedType{Name: ""}},
						},
					},
				},
			},
			wantErr: "named type name is empty",
		},
		{
			name: "struct decl missing name",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&StructDecl{Name: ""},
				},
			},
			wantErr: "struct name is empty",
		},
		{
			name: "struct decl duplicate field names",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&StructDecl{
						Name: "Box",
						Fields: []Field{
							{Name: "value", Type: IntType},
							{Name: "value", Type: StrType},
						},
					},
				},
			},
			wantErr: "field[1] duplicate name",
		},
		{
			name: "enum decl missing name",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&EnumDecl{Name: "  "},
				},
			},
			wantErr: "enum name is empty",
		},
		{
			name: "enum decl duplicate value names",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&EnumDecl{
						Name: "Color",
						Values: []EnumValue{
							{Name: "Red", Value: 0},
							{Name: "Red", Value: 1},
						},
					},
				},
			},
			wantErr: "value[1] duplicate name",
		},
		{
			name: "union decl missing name",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&UnionDecl{Name: "", Types: []Type{IntType, StrType}},
				},
			},
			wantErr: "union name is empty",
		},
		{
			name: "union decl with no types",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&UnionDecl{Name: "Value"},
				},
			},
			wantErr: "union types are empty",
		},
		{
			name: "extern type decl missing name",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&ExternTypeDecl{Name: ""},
				},
			},
			wantErr: "extern type name is empty",
		},
		{
			name: "extern type decl invalid type arg",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&ExternTypeDecl{
						Name: "Handle",
						Args: []Type{&NamedType{Name: ""}},
					},
				},
			},
			wantErr: "extern type arg[0]",
		},
		{
			name: "function decl missing name",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{Name: "", Return: Void, Body: &Block{}},
				},
			},
			wantErr: "function name is empty",
		},
		{
			name: "function decl duplicate parameter names",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name: "x",
						Params: []Param{
							{Name: "value", Type: IntType},
							{Name: "value", Type: StrType},
						},
						Return: Void,
						Body:   &Block{},
					},
				},
			},
			wantErr: "param[1] duplicate name",
		},
		{
			name: "function decl missing return type",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:   "x",
						Params: []Param{},
						Return: nil,
						Body:   &Block{},
					},
				},
			},
			wantErr: "return type",
		},
		{
			name: "extern function with body",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&FuncDecl{
						Name:          "x",
						Return:        IntType,
						IsExtern:      true,
						ExternBinding: "X",
						Body:          &Block{},
					},
				},
			},
			wantErr: "extern function must not define body",
		},
		{
			name: "var decl missing name",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&VarDecl{
						Name:  "",
						Type:  IntType,
						Value: &LiteralExpr{Kind: "int", Value: "1"},
					},
				},
			},
			wantErr: "variable name is empty",
		},
		{
			name: "var decl missing type",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&VarDecl{
						Name:  "AppName",
						Type:  nil,
						Value: &LiteralExpr{Kind: "str", Value: "demo"},
					},
				},
			},
			wantErr: "variable type",
		},
		{
			name: "var decl missing value",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					&VarDecl{
						Name:  "AppName",
						Type:  StrType,
						Value: nil,
					},
				},
			},
			wantErr: "variable value is nil",
		},
		{
			name: "unsupported declaration type",
			module: &Module{
				PackageName: "main",
				Decls: []Decl{
					unsupportedDecl{},
				},
			},
			wantErr: "unsupported declaration type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateModule(test.module)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected error containing %q, got %q", test.wantErr, err.Error())
			}
		})
	}
}

// unsupportedDecl is a stand-in declaration type used by the validator
// tests to exercise the unsupported-decl branch of ValidateModule
// without polluting the IR model with a real declaration kind.
type unsupportedDecl struct{}

func (unsupportedDecl) declNode() {}
