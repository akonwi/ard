// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import ardGrammar from "./src/shiki/ard.mjs";

// https://astro.build/config
export default defineConfig({
  integrations: [
    starlight({
      title: "Ard Language",
      description:
        "A modern, statically-typed programming language designed for clarity, safety, and ease.",
      favicon: "/favicon.svg",
      logo: {
        src: "./src/assets/logo-bordered.png",
      },
      customCss: ["./src/styles/custom.css"],
      expressiveCode: {
        shiki: {
          langs: [ardGrammar],
        },
        customizeTheme: (theme) => {
          const isDark = theme.type === "dark";
          const palette = isDark
            ? {
                bg: "#181818",
                fg: "#E6E6E6",
                border: "#202127",
                comment: "#46474F",
                keyword: "#8B8B8B",
                punctuation: "#5C6974",
                string: "#C2C2C2",
                embedded: "#9592A4",
                constant: "#F9B98C",
                type: "#D8C6AA",
                namespace: "#E6E7A3",
                variable: "#E6E6E6",
              }
            : {
                bg: "#F5F1EB",
                fg: "#2B2A28",
                border: "#D7D0C7",
                comment: "#9B9389",
                keyword: "#7F786F",
                punctuation: "#938A80",
                string: "#5C5752",
                embedded: "#8A8398",
                constant: "#B97046",
                type: "#8D6F57",
                namespace: "#8D9146",
                variable: "#2B2A28",
              };

          theme.bg = palette.bg;
          theme.fg = palette.fg;
          theme.colors["editor.background"] = palette.bg;
          theme.colors["editor.foreground"] = palette.fg;
          theme.colors["panel.background"] = palette.bg;
          theme.colors["titleBar.activeBackground"] = palette.bg;
          theme.colors["tab.activeBackground"] = palette.bg;
          theme.colors["tab.inactiveBackground"] = palette.bg;
          theme.colors["titleBar.border"] = palette.border;
          theme.colors["editorGroupHeader.tabsBorder"] = palette.border;

          theme.settings.push(
            {
              name: "Ard comments",
              scope: ["comment", "comment.line.double-slash.ard"],
              settings: { foreground: palette.comment },
            },
            {
              name: "Ard keywords",
              scope: [
                "keyword",
                "keyword.control",
                "keyword.control.ard",
                "keyword.control.import.ard",
                "keyword.declaration.function.ard",
                "keyword.declaration.type.ard",
                "keyword.declaration.variable.ard",
                "keyword.declaration.implementation.ard",
                "storage.modifier.ard",
                "keyword.operator.logical.ard",
              ],
              settings: { foreground: palette.keyword },
            },
            {
              name: "Ard operators and punctuation",
              scope: [
                "keyword.operator",
                "keyword.operator.ard",
                "punctuation",
                "punctuation.separator.key-value.ard",
                "punctuation.separator.comma.ard",
                "punctuation.bracket",
                "punctuation.bracket.angle.ard",
                "punctuation.section.group.begin.ard",
                "punctuation.section.group.end.ard",
                "punctuation.definition.string.begin.ard",
                "punctuation.definition.string.end.ard",
                "punctuation.section.interpolation.begin.ard",
                "punctuation.section.interpolation.end.ard",
              ],
              settings: { foreground: palette.punctuation },
            },
            {
              name: "Ard strings",
              scope: ["string", "string.quoted.double.ard"],
              settings: { foreground: palette.string },
            },
            {
              name: "Ard escapes and embedded",
              scope: [
                "constant.character.escape",
                "constant.character.escape.ard",
                "meta.interpolation.ard",
                "variable.language.ard",
              ],
              settings: { foreground: palette.embedded },
            },
            {
              name: "Ard numbers booleans and constants",
              scope: [
                "constant",
                "constant.numeric",
                "constant.numeric.ard",
                "constant.language.boolean",
                "constant.language.boolean.ard",
                "constant.other.enum-value.ard",
                "variable.language.wildcard.ard",
              ],
              settings: { foreground: palette.constant },
            },
            {
              name: "Ard functions",
              scope: ["entity.name.function", "entity.name.function.ard"],
              settings: { foreground: palette.constant },
            },
            {
              name: "Ard types",
              scope: [
                "entity.name.type",
                "entity.name.type.ard",
                "entity.name.type.parameter.ard",
                "support.type.builtin.ard",
              ],
              settings: { foreground: palette.type },
            },
            {
              name: "Ard namespaces",
              scope: ["entity.name.namespace.ard"],
              settings: { foreground: palette.namespace },
            },
            {
              name: "Ard variables and properties",
              scope: [
                "variable",
                "variable.other.definition.ard",
                "variable.other.member.ard",
                "variable.parameter",
                "variable.parameter.ard",
                "variable.language",
              ],
              settings: { foreground: palette.variable },
            }
          );

          return theme;
        },
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/akonwi/ard",
        },
      ],
      sidebar: [
        {
          label: "Getting Started",
          items: [
            { label: "Introduction", slug: "getting-started/introduction" },
            { label: "Installation", slug: "getting-started/installation" },
            // { label: 'Your First Program', slug: 'getting-started/first-program' },
          ],
        },
        {
          label: "Language Guide",
          items: [
            { label: "Types", slug: "guide/types" },
            { label: "Variables", slug: "guide/variables" },
            { label: "Formatting", slug: "guide/formatting" },
            { label: "Functions", slug: "guide/functions" },
            { label: "Control Flow", slug: "guide/control-flow" },
            { label: "Structs", slug: "guide/structs" },
            { label: "Enums", slug: "guide/enums" },
            { label: "Error Handling", slug: "guide/error-handling" },
            { label: "Pattern Matching", slug: "guide/pattern-matching" },
            { label: "Modules", slug: "guide/modules" },
            { label: "Testing", slug: "guide/testing" },
          ],
        },
        {
          label: "Advanced Topics",
          items: [
            { label: "Traits", slug: "advanced/traits" },
            { label: "Generics", slug: "advanced/generics" },
            { label: "Async Programming", slug: "advanced/async" },
            { label: "Using External Data", slug: "advanced/data-decoding" },
          ],
        },
        {
          label: "Standard Library",
          items: [
            { label: "ard/argv", slug: "stdlib/argv" },
            { label: "ard/async", slug: "stdlib/async" },
            { label: "ard/chrono", slug: "stdlib/chrono" },
            { label: "ard/crypto", slug: "stdlib/crypto" },
            { label: "ard/dates", slug: "stdlib/dates" },
            { label: "ard/decode", slug: "stdlib/decode" },
            { label: "ard/duration", slug: "stdlib/duration" },
            { label: "ard/dynamic", slug: "stdlib/dynamic" },
            { label: "ard/encode", slug: "stdlib/encode" },
            { label: "ard/env", slug: "stdlib/env" },
            { label: "ard/float", slug: "stdlib/float" },
            { label: "ard/fs", slug: "stdlib/fs" },
            { label: "ard/http", slug: "stdlib/http" },
            { label: "ard/int", slug: "stdlib/int" },
            { label: "ard/io", slug: "stdlib/io" },
            { label: "ard/json", slug: "stdlib/json" },
            { label: "ard/list", slug: "stdlib/list" },
            { label: "ard/map", slug: "stdlib/map" },
            { label: "ard/maybe", slug: "stdlib/maybe" },
            { label: "ard/result", slug: "stdlib/result" },
            { label: "ard/sql", slug: "stdlib/sql" },
            { label: "ard/string", slug: "stdlib/string" },
            { label: "ard/testing", slug: "stdlib/testing" },
          ],
        },
        {
          label: "Examples",
          items: [{ label: "Code Samples", slug: "examples/samples" }],
        },
      ],
    }),
  ],
});
