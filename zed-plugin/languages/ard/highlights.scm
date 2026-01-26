;; imports
(import (module_path) @string.special)

;; Basic types
(primitive_type) @type.builtin
(type (identifier)) @type.builtin
(generic_type name: (identifier) @type)
(result_type) @type.builtin
(list_type element_type: (type (identifier))) @type.builtin
(map_type) @type.builtin

;; Result type error annotation (Type!ErrorType)
(type error_type: (identifier) @type)
(type error_type: (member_access_type) @type)

; ;; Identifiers
(identifier) @variable
(wildcard) @variable.special
(instance_property name: (identifier) @property)

;; Try expressions
(try_expression catch_var: (identifier) @variable.parameter)

; Assume uppercase names are types/enum-constructors
((identifier) @type
 (#match? @type "^[A-Z]"))

(trait_definition name: (identifier) @type.interface)
(struct_definition name: (identifier) @type)
(struct_instance name: (identifier) @type)
(enum_definition name: (identifier) @type)
(function_definition name: (identifier) @function.definition)
(external_function name: (identifier) @function.definition)
;; Qualified function names (Namespace::function_name)
(function_definition (double_colon) (identifier) @function.definition)
(param_def name: (identifier) @variable.parameter)
(anonymous_parameter name: (identifier) @variable.parameter)
(trait_function name: (identifier) @function.definition)
(trait_implementation_function name: (identifier) @function.definition)

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
"!" @operator

;; Keywords/operators that have grammar nodes
(break) @keyword
(private) @keyword
(and) @operator
(not) @operator
(or) @operator

;; Punctuation
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
