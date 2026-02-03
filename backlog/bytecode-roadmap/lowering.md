# AST Lowering Work

This lists the remaining lowering tasks to fully decouple VM backends from checker.Type.

## High-priority nodes

These drive most runtime type assertions today:

- InstanceMethod dispatch
  - Precompute method kind for primitives, collections, Result/Maybe
  - Precompute struct/enum method target (direct pointer to method closure)
- Match/Pattern nodes
  - Precompute variant tags, enum discriminants, and match arm metadata
- Collection literals
  - Ensure list/map element types are carried directly on the node
- Function calls
  - Embed resolved function references (local, module, extern)
  - Embed resolved generics (specialized signature)

## Suggested lowering fields

- For method nodes:
  - MethodKind enum and receiver kind
  - Pre-resolved callee signature
- For pattern matching:
  - Normalized pattern representation
  - Pre-validated exhaustiveness metadata
- For module calls:
  - Direct module path and symbol reference

## Audit targets

- vm/interpret.go: eliminate all Type() assertions
- checker/checker.go: ensure all derived metadata is stored on nodes

## Regression suite

- Add a "lowered execution" test set that runs existing vm tests against a node traversal that does not call Type().
