// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

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
            { label: "Functions", slug: "guide/functions" },
            { label: "Control Flow", slug: "guide/control-flow" },
            { label: "Structs", slug: "guide/structs" },
            { label: "Enums", slug: "guide/enums" },
            { label: "Error Handling", slug: "guide/error-handling" },
            { label: "Pattern Matching", slug: "guide/pattern-matching" },
            { label: "Modules", slug: "guide/modules" },
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
