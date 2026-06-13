import { includeIgnoreFile } from "@eslint/compat"
import eslint from "@eslint/js"
import eslintConfigPrettier from "eslint-config-prettier"
import globals from "globals"
import path from "node:path"
import { fileURLToPath } from "node:url"
import tseslint from "typescript-eslint"

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const gitignorePath = path.resolve(__dirname, ".gitignore")

export default tseslint.config(
  eslint.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  includeIgnoreFile(gitignorePath),
  {
    // The vendored prosemirror-model submodule and the build output are not ours to lint.
    ignores: ["prosemirror/**", "dist/**", "model/**", "coverage/**"],
  },
  {
    files: ["**/*.ts"],
    rules: {
      "no-unused-vars": "off",
      "no-undef": "off",
      "@typescript-eslint/switch-exhaustiveness-check": "error",
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          args: "none",
          caughtErrors: "none",
        },
      ],
    },
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      globals: {
        ...globals.browser,
        ...globals.node,
        ...globals.es2025,
      },
      parserOptions: {
        parser: tseslint.parser,
        project: ["./tsconfig.eslint.json"],
        tsconfigRootDir: __dirname,
      },
    },
  },
  eslintConfigPrettier,
)
