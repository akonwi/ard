# Chained Comparisons

Ard supports eloquent relative conditions—chaining multiple comparison operators in a single expression for improved readability.

## Syntax

Instead of writing:
```ard
if 200 <= status && status <= 300 {
    // ...
}
```

You can write:
```ard
if 200 <= status <= 300 {
    // ...
}
```

## Semantics

A chained comparison `a op1 b op2 c` is equivalent to `(a op1 b) && (b op2 c)`.

The key property: **middle operands are evaluated exactly once**, even though they appear in multiple logical comparisons.

## Operators

Only relative comparison operators can be chained:
- `<` (less than)
- `<=` (less than or equal)
- `>` (greater than)
- `>=` (greater than or equal)

Equality operators (`==` and `!=`) cannot be chained. This is a semantic constraint validated by the checker with the error: `"equality operators cannot be chained"`.

## Examples

### Basic range check
```ard
let status = 201
if 200 <= status <= 300 {
    io::print("Success range")
}
```

### Mixed operators
```ard
let x = 150
if 100 < x <= 200 {
    io::print("x is between 100 (exclusive) and 200 (inclusive)")
}
```

### Multiple chains
```ard
let y = 75
if 50 < y <= 100 <= 150 {
    // y is between 50 and 100, and 100 is less than or equal to 150
}
```

### With side effects
```ard
fn get_value() Int { /* ... */ }

// The function is called once, result reused in both comparisons
if 0 < get_value() <= 100 {
    // ...
}
```

## Type Requirements

Each pairwise comparison must operate on the same types. The checker validates this for each segment:

```ard
// Valid: all comparisons are Int
if 0 < x <= 100 {
    // ...
}

// Invalid: mixing types across a comparison
if 0 < "hello" <= 100 {
    // error: Cannot compare different types
}
```

## Precedence

Chained comparisons bind tighter than logical operators:

```ard
// Parsed as: (200 <= status <= 300) && (status != 250)
if 200 <= status <= 300 && status != 250 {
    // ...
}
```

## Implementation Details

- **Parser**: Recognizes consecutive relative comparison operators and constructs a `ChainedComparison` AST node
- **Checker**: Validates operator compatibility and transforms the chain into nested `BinaryExpression` nodes joined by `And` operators
- **VM**: Evaluates the desugared logical expression; operand reuse is handled at the checker level
