import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The browser talks to one origin; /api/<service>/... is forwarded to that
// service, so the backend needs no CORS setup. nginx does the same in Docker.
const services = { master: 8081, order: 8082, warehouse: 8083, billing: 8084 };

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      Object.entries(services).map(([name, port]) => [
        `/api/${name}`,
        { target: `http://localhost:${port}`, rewrite: (path: string) => path.replace(`/api/${name}`, "") },
      ]),
    ),
  },
  test: { environment: "node" },
});
