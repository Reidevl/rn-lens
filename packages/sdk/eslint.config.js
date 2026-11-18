import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import prettier from "eslint-config-prettier";

export default tseslint.config(
  { ignores: ["dist", "node_modules"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    // Editors run one ESLint process for the whole workspace; without an
    // explicit root, typescript-eslint sees both packages and refuses to
    // parse. No `files`: the TypeScript parser also reads .js/.mjs/.cjs.
    languageOptions: { parserOptions: { tsconfigRootDir: import.meta.dirname } },
  },
  reactHooks.configs.flat.recommended,
  prettier,
  {
    files: ["scripts/**/*.mjs"],
    languageOptions: { globals: { ...globals.node } },
  },
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      globals: { ...globals.browser, ...globals.node, __DEV__: "readonly" },
    },
    rules: {
      "@typescript-eslint/consistent-type-imports": "error",
      "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }],
    },
  },
);
