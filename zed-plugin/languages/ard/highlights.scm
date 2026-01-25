;; imports
(import (module_path) @string.special)

; ;; Basic types
(primitive_type) @type.builtin
(type (identifier)) @type.builtin
; (generic_type) @type
; (generic_type name: (identifier) @type)
; (result_type) @type.builtin
; (list_type) @type.builtin
(list_type element_type: (type (identifier))) @type.builtin
; (map_type) @type.builtin

; ;; Identifiers
(identifier) @variable
(wildcard) @variable.special
; "@" @variable.special

; Assume uppercase names are types/enum-constructors
((identifier) @type
 (#match? @type "^[A-Z]"))

(trait_definition name: (identifier) @type.interface)
(struct_definition name: (identifier) @type)
(struct_instance name: (identifier) @type)
(enum_definition name: (identifier) @type)
(function_definition name: (identifier) @function.definition)
(param_def name: (identifier) @variable.parameter)
(anonymous_parameter name: (identifier) @variable.parameter)
(trait_function name: (identifier) @function.definition)

(function_call target: (identifier) @function)
(member_access (function_call) @function)
(member_access (identifier) @property)

;; Named arguments
(named_argument name: (identifier) @variable.parameter)

; ;; Attributes
(struct_property name: (identifier) @property)
(struct_prop_pair name: (identifier) @property)

;; Values
[
  (string)
  (string_content)
] @string
; (escape_sequence) @string.escape
(number) @number
(boolean) @boolean

;; Operators
(assign) @operator.assignment
(increment) @operator.assignment
(decrement) @operator.assignment
(plus) @operator
(minus) @operator
(multiply) @operator
(divide) @operator
(modulo) @operator
(equal) @operator
(not_equal) @operator
(less_than) @operator
(greater_than) @operator
(less_than_or_equal) @operator
(greater_than_or_equal) @operator
(inclusive_range) @operator
"=>" @operator

;; Keywords
[
  "if"
  "else"
  "while"
  "for"
  "in"
  (break)
  "match"
  "let"
  "mut"
  "use"
  "fn"
  "struct"
  "enum"
  "impl"
  "type"
  "trait"
  (private)
 ] @keyword

[
  (and)
  (not)
  (or)
] @operator


; ;; Punctuation
[
  (period)
  ";"
  ","
  (double_colon)
  ":"
] @punctuation.delimiter

[
  "?"
  "$"
] @punctuation.special

[
  "("
  ")"
  "{"
  "}"
  "["
  "]"
  "<"
  ">"
] @punctuation.bracket

(comment) @comment
