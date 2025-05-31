# Generics in Ardlang

## Syntax

Generic types do not need explicit declarations like in other languages. Instead, the begin with `$` in normal declarations.

In this example, the `map` function accepts a list of generic type `$A` and returns a list of another generic type `$B`.

```ard
fn map(list: [$A]) [$B] {
  // ...
}
```

When calling the function, the generics can be constrained by using type arguments for all the declared generics.

```ard
let floats = map<Int, Float>(ints)
```

The order of type arguments will correspond to the order of Generics in the signature.

If type arguments are not provided, the compiler will attempt to infer the generics based on the provided values and their usage.

```ard
let ints = [1,2,3]
let floats: [Float] = map(ints) // the compiler knows ints is [Int] and the result must be [Float]
```

If the compiler doesn't have enough information to infer types, it will show errors and require type arguments or explicit declarations.

## Checker algorithm

* `Type.equal()` - a comparator method required by all Types
  - usually called as `actual.equal(expected)`
    - this allows `actual` to "refine" `expected`, when `expected` is a generic type
    - once a generic is refined, it is closed to matching other types
* in functions and methods
  - open generics are collected
    a. if there are type arguments, refine generics with provided type arguments
    b. refine generic parameters + return with inference of provided values
