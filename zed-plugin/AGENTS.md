# AGENTS.md

This file provides guidance to a coding agent when working with code in this repository.

## Project Overview

This is a Zed editor extension that provides language support for the Ard programming language. It includes syntax highlighting, bracket matching, indentation configuration, and other editor features through Tree-sitter grammar integration.

## Project Structure

- `extension.toml`: Extension metadata and configuration, including the grammar repository reference
- `grammars/`: Grammar files (linked from tree-sitter-ard)
- `languages/ard/`: Language configuration for Ard
  - `config.toml`: Language metadata (name, file extensions, comment syntax)
  - `highlights.scm`: Syntax highlighting rules using Tree-sitter query syntax
  - `brackets.scm`: Bracket matching rules
  - `indents.scm`: Indentation rules

## Development

For comprehensive guidance on developing Zed extensions, see [Zed's extension development documentation](https://zed.dev/docs/extensions/developing-extensions?highlight=extens#developing-an-extension-locally).

### Modifying Highlighting Rules

Edit `languages/ard/highlights.scm` to define syntax highlighting. Rules map Tree-sitter nodes to Zed's built-in highlight scopes. Common scopes:
- `keyword`, `comment`, `string`, `number`
- `function`, `variable`, `type`, `property`
- `operator`, `punctuation`

### Modifying Brackets

Edit `languages/ard/brackets.scm` to define bracket pairs for matching and navigation.

### Modifying Indentation

Edit `languages/ard/indents.scm` to define indentation rules based on Tree-sitter patterns.

## Integration with Tree-sitter

The extension uses the Tree-sitter grammar defined in `../tree-sitter/`. Update `extension.toml` if the grammar path or commit changes.
