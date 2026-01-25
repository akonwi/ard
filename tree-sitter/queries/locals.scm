;; Definitions
(function_definition name: (identifier) @local.definition.function)
(param_def name: (identifier) @local.definition.parameter)
(anonymous_parameter name: (identifier) @local.definition.parameter)
(variable_definition name: (identifier) @local.definition.var)
(for_in_loop cursor: (identifier) @local.definition.var)

;; References
(identifier) @local.reference

;; Scopes
(block) @local.scope
(function_definition) @local.scope
(for_loop) @local.scope
(for_in_loop) @local.scope
(while_loop) @local.scope
(if_statement) @local.scope
(anonymous_function) @local.scope