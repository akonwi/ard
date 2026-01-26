;; Top-level definitions
(function_definition name: (identifier) @name) @item
(struct_definition name: (identifier) @name) @item
(trait_definition name: (identifier) @name) @item

;; Qualified function names (Namespace::function_name)
(function_definition
  (double_colon)
  (identifier) @name) @item
