; Ard outline query (top-level + impl methods)

; Types (top-level)
(source_file (struct_declaration name: (identifier) @name) @item)
(source_file (enum_declaration name: (identifier) @name) @item)
(source_file (trait_declaration name: (identifier) @name) @item)

; Functions (top-level)
(source_file (function_declaration name: (identifier) @name) @item)
(source_file (function_declaration name: (qualified_identifier) @name) @item)
(source_file (extern_function name: (identifier) @name) @item)

; Impl blocks (as containers)
(source_file (impl_block target: (qualified_identifier) @name) @item)
(source_file (impl_block target: (identifier) @name) @item)
(source_file
  (impl_block
    target: (qualified_identifier) @name
    for_type: (identifier) @context) @item)
(source_file
  (impl_block
    target: (identifier) @name
    for_type: (identifier) @context) @item)

; Methods inside impl blocks
(impl_block (impl_body (function_declaration name: (identifier) @name) @item))
(impl_block (impl_body (function_declaration name: (qualified_identifier) @name) @item))
