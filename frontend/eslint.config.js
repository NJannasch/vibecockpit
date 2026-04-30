import js from "@eslint/js";
import svelte from "eslint-plugin-svelte";
import svelteParser from "svelte-eslint-parser";
import globals from "globals";

export default [
  js.configs.recommended,
  ...svelte.configs.recommended,
  {
    languageOptions: {
      globals: {
        ...globals.browser,
      },
    },
  },
  {
    files: ["**/*.svelte"],
    languageOptions: {
      parser: svelteParser,
    },
    rules: {
      // Keyed each blocks are best practice but not required for correctness
      // in lists that don't reorder. Enable gradually.
      "svelte/require-each-key": "warn",
      // new Date() and new Set() inside $derived are fine — SvelteDate/SvelteSet
      // are optional and the codebase doesn't need fine-grained reactivity on them
      "svelte/prefer-svelte-reactivity": "off",
      // $state + $effect vs writable $derived is a style choice
      "svelte/prefer-writable-derived": "off",
      // {@html} is used for trusted server-rendered content (markdown previews)
      "svelte/no-at-html-tags": "warn",
    },
  },
  {
    ignores: ["node_modules/", "../internal/web/static/"],
  },
];
