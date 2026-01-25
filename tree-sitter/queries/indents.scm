;; Indent
(function_definition body: (block) @indent)
(if_statement body: (block) @indent)
(else_clause body: (block) @indent)
(for_loop body: (block) @indent)
(for_in_loop body: (block) @indent)
(while_loop body: (block) @indent)
(anonymous_function body: (block) @indent)
(match_expression) @indent
(match_case body: (block) @indent)
(struct_definition) @indent
(enum_definition) @indent

;; Outdent
("}") @outdent