import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

const remote = process.env.VITE_API_TARGET;
const api = remote || "http://localhost:8000";
const ws = remote ? remote.replace(/^http/, "ws") : "ws://localhost:8000";
const blob = remote || "http://localhost:9000";

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      "/api": { target: api, changeOrigin: true },
      "/ws": {
        target: ws,
        ws: true,
        changeOrigin: true,
        headers: remote ? { Origin: remote } : undefined,
      },
      "/avatars": { target: api, changeOrigin: true },
      "/blog": { target: blob, changeOrigin: true },
    },
  },
});
