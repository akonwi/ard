;; Comment injections for documentation or embedded languages
((comment) @injection.content
 (#match? @injection.content "^/\*.*\*/$")
 (#set! injection.language "markdown"))

;; String interpolation
(string_interpolation
  expression: (_) @injection.content
  (#set! injection.language "ard")
)