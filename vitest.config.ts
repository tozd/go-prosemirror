import { defineConfig } from "vitest/config"

// https://vitest.dev/config/
export default defineConfig({
  test: {
    coverage: {
      provider: "v8",
      reporter: ["text", "cobertura", "html"],
      include: ["src/**/*.ts"],
      all: true,
    },
  },
})
