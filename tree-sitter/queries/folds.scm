;; Fold nodes
(block) @fold
(function_definition body: (block) @fold)
(if_statement body: (block) @fold)
(else_clause body: (block) @fold)
(for_loop body: (block) @fold)
(for_in_loop body: (block) @fold)
(while_loop body: (block) @fold)
(anonymous_function body: (block) @fold)
(match_expression) @fold
(struct_definition) @fold
(enum_definition) @fold
(comment) @fold