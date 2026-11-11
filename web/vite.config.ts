import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The dashboard calls core through /api so the browser never needs to know core's address.
const coreUrl = process.env.PADDOCK_CORE_URL ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      "/api": {
        target: coreUrl,
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ""),
      },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["src/test-setup.ts"],
  },
});
