import { fileURLToPath, URL } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: Number(process.env.PORT) || 5174,
    proxy: {
      "/api": {
        target: process.env.VITE_API_TARGET || "http://localhost:8085",
        changeOrigin: true,
      },
    },
  },
});
