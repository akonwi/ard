const identifier = '[A-Za-z_][A-Za-z0-9_]*'
const qualifiedIdentifier = `${identifier}(?:::${identifier})*`
const capitalizedIdentifier = '[A-Z][A-Za-z0-9_]*'
const qualifiedType = `${capitalizedIdentifier}(?:::${capitalizedIdentifier})*`

export default [
  {
    name: 'ard',
    displayName: 'Ard',
    scopeName: 'source.ard',
    fileTypes: ['ard'],
    patterns: [{ include: '#main' }],
    repository: {
      main: {
        patterns: [
          { include: '#comments' },
          { include: '#strings' },
          { include: '#imports' },
          { include: '#typeDeclarations' },
          { include: '#implBlocks' },
          { include: '#functionDeclarations' },
          { include: '#externFunctionDeclarations' },
          { include: '#variableDeclarations' },
          { include: '#enumVariants' },
          { include: '#genericParameters' },
          { include: '#builtinTypes' },
          { include: '#userTypes' },
          { include: '#booleansAndSpecials' },
          { include: '#numbers' },
          { include: '#propertiesAndLabels' },
          { include: '#keywords' },
          { include: '#operators' },
          { include: '#punctuation' },
        ],
      },

      comments: {
        patterns: [
          {
            name: 'comment.line.double-slash.ard',
            match: '//.*$',
          },
        ],
      },

      strings: {
        patterns: [
          {
            name: 'string.quoted.double.ard',
            begin: '"',
            beginCaptures: {
              0: { name: 'punctuation.definition.string.begin.ard' },
            },
            end: '"',
            endCaptures: {
              0: { name: 'punctuation.definition.string.end.ard' },
            },
            patterns: [
              {
                name: 'constant.character.escape.ard',
                match: '\\\\.',
              },
              {
                name: 'meta.interpolation.ard',
                begin: '\\{',
                beginCaptures: {
                  0: { name: 'punctuation.section.interpolation.begin.ard' },
                },
                end: '\\}',
                endCaptures: {
                  0: { name: 'punctuation.section.interpolation.end.ard' },
                },
                patterns: [{ include: '#main' }],
              },
            ],
          },
        ],
      },

      imports: {
        patterns: [
          {
            match: `\\b(use)\\s+([A-Za-z0-9_][A-Za-z0-9_\\/.-]*)(?:\\s+(as)\\s+(${identifier}))?`,
            captures: {
              1: { name: 'keyword.control.import.ard' },
              2: { name: 'entity.name.namespace.ard' },
              3: { name: 'keyword.control.import.ard' },
              4: { name: 'entity.name.namespace.ard' },
            },
          },
        ],
      },

      typeDeclarations: {
        patterns: [
          {
            match: `\\b(private)\\s+(struct|enum|trait)\\s+(${identifier})`,
            captures: {
              1: { name: 'storage.modifier.ard' },
              2: { name: 'keyword.declaration.type.ard' },
              3: { name: 'entity.name.type.ard' },
            },
          },
          {
            match: `\\b(struct|enum|trait)\\s+(${identifier})`,
            captures: {
              1: { name: 'keyword.declaration.type.ard' },
              2: { name: 'entity.name.type.ard' },
            },
          },
        ],
      },

      implBlocks: {
        patterns: [
          {
            match: `\\b(impl)\\s+(${qualifiedIdentifier})(?:\\s+(for)\\s+(${qualifiedIdentifier}))?(?:\\s+(as)\\s+(${identifier}))?`,
            captures: {
              1: { name: 'keyword.declaration.implementation.ard' },
              2: { name: 'entity.name.type.ard' },
              3: { name: 'keyword.control.ard' },
              4: { name: 'entity.name.type.ard' },
              5: { name: 'keyword.control.ard' },
              6: { name: 'variable.parameter.ard' },
            },
          },
        ],
      },

      functionDeclarations: {
        patterns: [
          {
            match: `\\b(private)\\s+(fn)\\s+(${qualifiedIdentifier})`,
            captures: {
              1: { name: 'storage.modifier.ard' },
              2: { name: 'keyword.declaration.function.ard' },
              3: { name: 'entity.name.function.ard' },
            },
          },
          {
            match: `\\b(fn)\\s+(${qualifiedIdentifier})`,
            captures: {
              1: { name: 'keyword.declaration.function.ard' },
              2: { name: 'entity.name.function.ard' },
            },
          },
        ],
      },

      externFunctionDeclarations: {
        patterns: [
          {
            match: `\\b(private)\\s+(extern)\\s+(fn)\\s+(${identifier})`,
            captures: {
              1: { name: 'storage.modifier.ard' },
              2: { name: 'storage.modifier.ard' },
              3: { name: 'keyword.declaration.function.ard' },
              4: { name: 'entity.name.function.ard' },
            },
          },
          {
            match: `\\b(extern)\\s+(fn)\\s+(${identifier})`,
            captures: {
              1: { name: 'storage.modifier.ard' },
              2: { name: 'keyword.declaration.function.ard' },
              3: { name: 'entity.name.function.ard' },
            },
          },
        ],
      },

      variableDeclarations: {
        patterns: [
          {
            match: `\\b(let|mut)\\s+(${identifier})`,
            captures: {
              1: { name: 'keyword.declaration.variable.ard' },
              2: { name: 'variable.other.definition.ard' },
            },
          },
        ],
      },

      enumVariants: {
        patterns: [
          {
            match: `\\b(${capitalizedIdentifier})\\b(?=\\s*(?:=|,|\\}))`,
            name: 'constant.other.enum-value.ard',
          },
        ],
      },

      genericParameters: {
        patterns: [
          {
            name: 'entity.name.type.parameter.ard',
            match: '\\$[A-Za-z_][A-Za-z0-9_]*',
          },
        ],
      },

      builtinTypes: {
        patterns: [
          {
            name: 'support.type.builtin.ard',
            match: '\\b(Int|Float|Str|Bool|Void)\\b',
          },
        ],
      },

      userTypes: {
        patterns: [
          {
            name: 'entity.name.type.ard',
            match: `\\b${qualifiedType}\\b`,
          },
        ],
      },

      booleansAndSpecials: {
        patterns: [
          {
            name: 'constant.language.boolean.ard',
            match: '\\b(true|false)\\b',
          },
          {
            name: 'variable.language.ard',
            match: '\\b(self|it)\\b',
          },
          {
            name: 'variable.language.wildcard.ard',
            match: '\\b_\\b',
          },
        ],
      },

      numbers: {
        patterns: [
          {
            name: 'constant.numeric.ard',
            match: '\\b[0-9][0-9_]*(?:\\.[0-9_]+)?\\b',
          },
        ],
      },

      propertiesAndLabels: {
        patterns: [
          {
            match: `\\b(${identifier})\\b(?=\\s*:)`,
            captures: {
              1: { name: 'variable.other.member.ard' },
            },
          },
          {
            match: `(?<=\\.)(${identifier})\\b`,
            captures: {
              1: { name: 'variable.other.member.ard' },
            },
          },
          {
            match: `(?<=::)(${identifier})\\b`,
            captures: {
              1: { name: 'entity.name.function.ard' },
            },
          },
        ],
      },

      keywords: {
        patterns: [
          {
            name: 'keyword.control.ard',
            match: '\\b(break|if|else|while|for|in|match|try|private)\\b',
          },
          {
            name: 'keyword.operator.logical.ard',
            match: '\\b(and|or|not)\\b',
          },
        ],
      },

      operators: {
        patterns: [
          {
            name: 'keyword.operator.ard',
            match: '(::|=>|->|\\.\\.|=\\+|=-|==|<=|>=|=|\\+|-|\\*|/|%|<|>|!|\\?)',
          },
        ],
      },

      punctuation: {
        patterns: [
          {
            name: 'punctuation.separator.key-value.ard',
            match: ':',
          },
          {
            name: 'punctuation.separator.comma.ard',
            match: ',',
          },
          {
            name: 'punctuation.bracket.angle.ard',
            match: '[<>]',
          },
          {
            name: 'punctuation.section.group.begin.ard',
            match: '[({\\[]',
          },
          {
            name: 'punctuation.section.group.end.ard',
            match: '[)}\\]]',
          },
        ],
      },
    },
  },
]
