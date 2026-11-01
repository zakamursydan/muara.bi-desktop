import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The bindings Wails generates live in a directory whose name contains dots
// (muara.bi/desktop), which esbuild's dep scanner likes to pre-bundle and then
// fail to resolve. Keeping them out of optimization is what the Wails templates
// do as well.
export default defineConfig({
  plugins: [react()],
  optimizeDeps: {
    exclude: ["../bindings"],
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
});
