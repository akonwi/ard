;; All blocks increase indentation and end at closing brace
(block) @indent

;; Type/data definitions with braces
(struct_definition) @indent
(enum_definition) @indent
(trait_definition) @indent
(implements_definition) @indent
(trait_implementation) @indent

;; Collections
(list_value) @indent
(map_value) @indent

;; Patterns with brackets/braces/parens
(_ "[" "]" @end) @indent
(_ "{" "}" @end) @indent
(_ "<" ">" @end) @indent
(_ "(" ")" @end) @indent
