/**
 * @file Ard grammar for tree-sitter
 * @author Akonwi Ngoh
 * @license MIT
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

// match rule at least once, with comma separator
const sepByComma1 = (rule) => seq(rule, repeat(seq(",", optional(rule))));

// match 0 or more of rule, with comma separator
const sepByComma = (rule) => optional(sepByComma1(rule));

// match rule at least once, with pipe separator
const sepByPipe = (rule) => seq(rule, repeat(seq("|", rule)));

// underscore prefix is used to hide rules from the syntax tree
module.exports = grammar({
  name: "ard",

  precedences: ($) => [
    [
      "unary",
      "member",
      "call",
      "multiply",
      "divide",
      "plus",
      "minus",
      "modulo",
      "comparison",
      "range",
      "and",
      "or",
      "assignment",
    ],
    [$.member_access, $.expression],
    ["function_call", "expression"],
    [$.expression, $.struct_instance],
    [$.type, $.function_type],
  ],

  conflicts: ($) => [
    [$.function_call, $.expression],
    [$._expression_statement, $.binary_expression],
    [$.binary_expression, $.variable_definition],
    [$.anonymous_parameter, $.expression],
    [$.anonymous_function, $.parameters],
    [$.anonymous_parameter, $.param_def],
  ],

  extras: ($) => [/\s/, $.comment],

  rules: {
    program: ($) => seq(repeat($.import), repeat($.statement)),

    //// imports
    import: ($) => seq("use", field("path", $.module_path)),

    //// Statements
    statement: ($) =>
      choice(
        $.comment,
        $.break,
        $.while_loop,
        $.if_statement,
        $.for_loop,
        $.for_in_loop,
        $.variable_definition,
        $.function_definition,
        $.reassignment,
        $._expression_statement,
        $.struct_definition,
        $.implements_definition,
        $.enum_definition,
        $.type_declaration,
        $.trait_definition,
        $.trait_implementation,
      ),

    //// definitions
    type_declaration: ($) =>
      seq(
        optional($.private),
        "type",
        field("name", $.identifier),
        $.assign,
        $.type_union,
      ),

    type_union: ($) => sepByPipe($.type),

    struct_definition: ($) =>
      seq(
        optional($.private),
        $._struct,
        field("name", $.identifier),
        $._left_brace,
        sepByComma(field("field", $.struct_property)),
        $._right_brace,
      ),

    struct_property: ($) =>
      seq(field("name", $.identifier), $._colon, field("type", $.type)),

    implements_definition: ($) => seq("impl", $.parameters, $.block),

    trait_definition: ($) =>
      seq(
        optional($.private),
        "trait",
        field("name", $.identifier),
        $._left_brace,
        repeat(field("function", $.trait_function)),
        $._right_brace,
      ),

    trait_function: ($) =>
      seq(
        $._fn,
        field("name", $.identifier),
        field("parameters", $.parameters),
        field("type", $.type),
      ),

    trait_implementation: ($) =>
      seq(
        "impl",
        field("trait", choice($.identifier, $.member_access)),
        "for",
        field("for", $.identifier),
        $._left_brace,
        repeat(field("function", $.trait_implementation_function)),
        $._right_brace,
      ),

    trait_implementation_function: ($) =>
      seq(
        $._fn,
        field("name", $.identifier),
        field("parameters", $.parameters),
        field("type", $.type),
        field("body", $.block),
      ),

    enum_definition: ($) =>
      seq(
        optional($.private),
        $._enum,
        field("name", $.identifier),
        $._left_brace,
        sepByComma(field("variant", $.enum_variant)),
        $._right_brace,
      ),

    enum_variant: ($) => field("variant", choice($.identifier)),

    variable_definition: ($) =>
      seq(
        field("binding", $.variable_binding),
        field("name", $.identifier),
        optional(seq($._colon, field("type", $.type))),
        $.assign,
        field("value", $.expression),
      ),

    variable_binding: ($) => choice("let", "mut"),

    function_definition: ($) =>
      seq(
        optional($.private),
        $._fn,
        field("name", $.identifier),
        field("parameters", $.parameters),
        field("return", optional($.type)),
        field("body", $.block),
      ),

    block: ($) =>
      seq($._left_brace, optional(repeat($.statement)), $._right_brace),

    while_loop: ($) =>
      seq("while", field("condition", $.expression), field("body", $.block)),

    for_loop: ($) =>
      seq(
        "for",
        field("cursor", $.variable_definition),
        $._semi_colon,
        field("condition", $.expression),
        $._semi_colon,
        field("step", $.statement),
        field("body", $.block),
      ),

    for_in_loop: ($) =>
      seq(
        "for",
        field("cursor", $.identifier),
        "in",
        field("range", $.expression),
        field("body", $.block),
      ),

    if_statement: ($) =>
      seq(
        "if",
        field("condition", $.expression),
        field("body", $.block),
        optional(field("else", $.else_clause)),
      ),

    else_clause: ($) =>
      seq("else", choice(field("if", $.if_statement), field("body", $.block))),

    reassignment: ($) =>
      prec(
        "assignment",
        seq(
          field("target", choice($.identifier, $.member_access)),
          field("operator", choice($.assign, $.increment, $.decrement)),
          field("value", $.expression),
        ),
      ),

    _expression_statement: ($) => $.expression,

    //// Expressions
    expression: ($) =>
      choice(
        $.identifier,
        $.primitive_value,
        $.list_value,
        $.map_value,
        $.unary_expression,
        $.binary_expression,
        $.member_access,
        $.function_call,
        $.struct_instance,
        $.paren_expression,
        $.match_expression,
        $.anonymous_function,
        $.range_expression,
        $.wildcard,
        $.instance_property,
      ),

    instance_property: ($) => seq("@", field("name", $.identifier)),

    range_expression: ($) =>
      prec.left(
        "range",
        seq(
          field("left", $.expression),
          field("operator", $.inclusive_range),
          field("right", $.expression),
        ),
      ),

    paren_expression: ($) =>
      seq($._left_paren, field("expr", $.expression), $._right_paren),

    anonymous_function: ($) =>
      choice(
        seq(
          seq(
            $._left_paren,
            sepByComma(field("parameter", $.anonymous_parameter)),
            $._right_paren,
          ),
          field("return", optional($.type)),
          field("body", $.block),
        ),
        seq(
          $._fn,
          seq(
            $._left_paren,
            sepByComma(field("parameter", $.anonymous_parameter)),
            $._right_paren,
          ),
          field("return", optional($.type)),
          field("body", $.block),
        ),
      ),

    anonymous_parameter: ($) =>
      choice(
        seq(field("name", $.identifier), $._colon, field("type", $.type)),
        field("name", $.identifier),
      ),

    function_call: ($) =>
      prec(
        "function_call",
        seq(
          field("target", $.identifier),
          field("arguments", $.paren_arguments),
        ),
      ),

    paren_arguments: ($) =>
      seq(
        $._left_paren,
        sepByComma(field("argument", choice($.named_argument, $.expression))),
        $._right_paren,
      ),

    named_argument: ($) =>
      seq(
        field("name", $.identifier),
        $._colon,
        field("value", $.expression),
      ),

    parameters: ($) =>
      seq(
        $._left_paren,
        sepByComma(field("parameter", $.param_def)),
        $._right_paren,
      ),

    param_def: ($) =>
      seq(
        optional(field("binding", "mut")),
        field("name", $.identifier),
        $._colon,
        field("type", choice($.type, $.member_access_type)),
      ),

    member_access_type: ($) =>
      seq(
        field("target", choice($.identifier, $.primitive_type)),
        field("operator", $.double_colon),
        field("member", $.identifier),
      ),

    match_expression: ($) =>
      seq(
        $._match,
        field("expr", $.expression),
        $._left_brace,
        sepByComma(field("case", $.match_case)),
        $._right_brace,
      ),

    match_case: ($) =>
      seq(
        field(
          "pattern",
          choice(
            $.member_access,
            $.primitive_value,
            $.identifier,
            $.wildcard,
            $.function_call,
            $.range_expression,
          ),
        ),
        $._fat_arrow,
        field("body", choice($.block, $.expression)),
      ),

    wildcard: ($) => "_",

    struct_instance: ($) =>
      seq(
        field("name", $.identifier),
        $._left_brace,
        sepByComma(field("field", $.struct_prop_pair)),
        $._right_brace,
      ),

    member_access: ($) =>
      prec.right(
        "member",
        seq(
          field("target", $.expression),
          field("operator", choice($.period, $.double_colon)),
          field(
            "member",
            choice($.member_access, $.identifier, $.function_call),
          ),
        ),
      ),

    unary_expression: ($) =>
      prec(
        "unary",
        choice(
          seq(field("operator", $.minus), field("operand", $.expression)),
          seq(field("operator", $.not), field("operand", $.expression)),
        ),
      ),

    binary_expression: ($) =>
      choice(
        ...[
          [$.multiply, "multiply"],
          [$.divide, "divide"],
          [$.plus, "plus"],
          [$.minus, "minus"],
          [$.modulo, "modulo"],
          [$.greater_than, "comparison"],
          [$.greater_than_or_equal, "comparison"],
          [$.less_than, "comparison"],
          [$.less_than_or_equal, "comparison"],
          [$.equal, "comparison"],
          [$.not_equal, "comparison"],
          [$.or, "or"],
          [$.and, "and"],
        ].map(([operator, precedence]) =>
          prec.left(
            // @ts-expect-error precedence is definitely a string
            precedence,
            seq(
              field("left", $.expression),
              field("operator", operator),
              field("right", $.expression),
            ),
          ),
        ),
      ),

    //// operators
    multiply: ($) => "*",
    divide: ($) => "/",
    plus: ($) => "+",
    minus: ($) => "-",
    modulo: ($) => "%",
    greater_than: ($) => ">",
    greater_than_or_equal: ($) => ">=",
    less_than: ($) => "<",
    less_than_or_equal: ($) => "<=",
    equal: ($) => "==",
    not_equal: ($) => "!=",
    and: ($) => "and",
    or: ($) => "or",
    bang: ($) => "!",
    inclusive_range: ($) => "..",

    // assignment operators
    increment: ($) => "=+",
    decrement: ($) => "=-",

    ///// types
    type: ($) =>
      prec.left(
        1,
        seq(
          choice(
            $.generic_type,
            $.map_type,
            $.list_type,
            $.primitive_type,
            $.identifier,
            $.result_type,
            $.function_type,
          ),
          field("optional", optional($._question)),
        ),
      ),

    function_type: ($) =>
      prec(
        2,
        seq("fn", $._left_paren, sepByComma($.type), $._right_paren, $.type),
      ),

    generic_type: ($) => seq($._dollar, field("name", $.identifier)),

    result_type: ($) =>
      seq(
        "Result",
        $._left_angle,
        field("type", $.type),
        $._comma,
        field("type", $.type),
        $._right_angle,
      ),

    list_type: ($) =>
      seq($._left_bracket, field("element_type", $.type), $._right_bracket),

    map_type: ($) =>
      seq(
        $._left_bracket,
        field("key", $.type),
        $._colon,
        field("value", $.type),
        $._right_bracket,
      ),
    primitive_type: ($) => choice($.str, $.bool, $.int, $.float, $.void),

    ///// values
    list_value: ($) =>
      seq($._left_bracket, sepByComma($.expression), $._right_bracket),

    map_value: ($) =>
      choice(
        seq($._left_bracket, $._colon, $._right_bracket),
        seq(
          $._left_bracket,
          sepByComma1(field("entry", $.map_pair)),
          $._right_bracket,
        ),
      ),
    map_pair: ($) =>
      seq(field("key", $.expression), $._colon, field("value", $.expression)),
    struct_prop_pair: ($) =>
      seq(field("name", $.identifier), $._colon, field("value", $.expression)),
    primitive_value: ($) =>
      field("primitive", choice($.string, $.number, $.boolean)),
    identifier: ($) => /[a-zA-Z_][a-zA-Z0-9_]*/,
    module_path: ($) => /[a-zA-Z][a-zA-Z0-9_\-\.\/]*/,
    string: ($) =>
      seq(
        '"',
        repeat(
          field(
            "chunk",
            choice(
              alias(/[^"{\\]+/, $.string_content),
              $.string_interpolation,
              $.escape_sequence,
            ),
          ),
        ),
        '"',
      ),
    string_interpolation: ($) =>
      seq($._left_brace, field("expression", $.expression), $._right_brace),
    escape_sequence: ($) => token.immediate(seq("\\", /[tnr"'\\]/)),
    /// comments
    comment: ($) =>
      token(
        choice(seq("//", /[^\n]*/), seq("/*", /[^*]*\*+([^/*][^*]*\*+)*\//)),
      ),
    number: ($) => /\d+(\.\d+)?/,

    /// keywords
    boolean: ($) => choice("true", "false"),
    str: ($) => "Str",
    int: ($) => "Int",
    float: ($) => "Float",
    bool: ($) => "Bool",
    void: ($) => "Void",
    _enum: ($) => "enum",
    _struct: ($) => "struct",
    _match: ($) => "match",
    _fn: ($) => "fn",
    break: ($) => "break",
    not: ($) => "not",
    private: ($) => "private",

    /// symbols + punctuation
    _colon: ($) => ":",
    _semi_colon: ($) => ";",
    double_colon: ($) => "::",
    assign: ($) => "=",
    _dollar: ($) => "$",
    _question: ($) => "?",
    _left_paren: ($) => "(",
    _right_paren: ($) => ")",
    _left_brace: ($) => "{",
    _right_brace: ($) => "}",
    _left_bracket: ($) => "[",
    _right_bracket: ($) => "]",
    _left_angle: ($) => "<",
    _right_angle: ($) => ">",
    _comma: ($) => ",",
    period: ($) => ".",
    _fat_arrow: ($) => "=>",
    _pipe: ($) => "|",
  },
});
