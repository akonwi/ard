; Minimal Ard highlights for bootstrap grammar (tree-sitter CLI)

; Keywords (node-based, minimal)
(break_statement) @keyword
(if_statement) @keyword
(while_loop) @keyword
(for_loop) @keyword
(for_in_loop) @keyword
(match_expression) @keyword
(try_expression) @keyword
(import_statement) @keyword
(variable_declaration) @keyword
(function_declaration) @keyword
(extern_function) @keyword
(struct_declaration) @keyword
(enum_declaration) @keyword
(trait_declaration) @keyword
(impl_block) @keyword

; Types
(primitive_type) @type.builtin
(type_parameter) @type.parameter
(type_arguments "<" @punctuation.bracket ">" @punctuation.bracket)
(generic_type (qualified_identifier) @type)
(generic_type (identifier) @type)
(qualified_identifier) @type

; Declarations
(struct_declaration name: (identifier) @type)
(enum_declaration name: (identifier) @type)
(trait_declaration name: (identifier) @type.interface)
(function_declaration name: (identifier) @function.definition)
(function_declaration name: (qualified_identifier) @function.definition)
(extern_function name: (identifier) @function.definition)
(enum_variant name: (identifier) @constant)

; Parameters
(parameter name: (identifier) @variable.parameter)
(named_argument name: (identifier) @variable.parameter)

; Variables and properties
(variable_declaration name: (identifier) @variable)
(struct_field name: (identifier) @property)
(struct_literal_field name: (identifier) @property)
(member_expression "." (identifier) @property)
(member_expression "::" (identifier) @type
  (#match? @type "^[A-Z]"))
(member_expression "::" (identifier) @function
  (#match? @function "^[a-z_]"))
(call_expression
  (member_expression "." (identifier) @function))
(call_expression
  (member_expression "::" (identifier) @function))
(self_expression "@" @keyword (identifier) @property)

; Calls
(call_expression
  (member_expression
    (primary_expression (identifier) @function))
  (argument_list))

; Literals
(number) @number
(string) @string
(boolean) @boolean
(void) @constant.builtin
(wildcard) @variable.special

; Comments
(comment) @comment

; Punctuation
[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
  ","
  ":"
] @punctuation.delimiter

[
  "."
  "::"
  "=>"
  "->"
] @punctuation.special

; Operators
[
  "="
  "=+"
  "=-"
  "+"
  "-"
  "*"
  "/"
  "%"
  "<"
  "<="
  ">"
  ">="
  "=="
  "!"
  "?"
  ".."
] @operator
