; SQL string injections

((string (string_content) @injection.content)
  (#match? @injection.content "(?i)^\\s*(select|insert|update|delete|with|create|drop|alter|pragma)")
  (#set! injection.language "sql"))
