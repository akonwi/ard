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
        src: "./src/assets/logo.png",
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
