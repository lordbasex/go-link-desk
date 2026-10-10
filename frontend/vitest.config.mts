// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { searchForWorkspaceRoot } from "vite";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // The cross-language test vectors live in protocol/, outside frontend/.
  server: { fs: { allow: [searchForWorkspaceRoot(process.cwd()), "../protocol"] } },
  test: {
    environment: "jsdom",
    include: ["packages/*/test/**/*.test.ts", "apps/*/src/**/*.test.tsx"],
    setupFiles: ["./vitest.setup.ts"],
  },
});
