import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    server: { deps: { inline: ["next-auth"] } }
  },
  resolve: {
    alias: {
      "@": new URL(".", import.meta.url).pathname,
      "server-only": new URL("./test/server-only.ts", import.meta.url).pathname
    }
  }
});
