import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The browser talks to one origin: /api/... goes through nginx (:3000, from
// `docker compose up`) to the KrakenD gateway, exactly as in production.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: { "/api": "http://localhost:3000" },
  },
  test: { environment: "node" },
});
