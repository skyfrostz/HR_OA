import { fileURLToPath, URL } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import vueJsx from "@vitejs/plugin-vue-jsx";
import VueI18nPlugin from "@intlify/unplugin-vue-i18n/vite";
import Icons from "unplugin-icons/vite";
import svgLoader from "vite-svg-loader";
import { defineConfig } from "vitest/config";

const path = (value: string) => fileURLToPath(new URL(value, import.meta.url));

export default defineConfig({
  base: "./",
  appType: "mpa",
  publicDir: false,
  resolve: { alias: { "@": path("./src"), "@build": path("./build") } },
  plugins: [
    tailwindcss(),
    vue(),
    vueJsx(),
    VueI18nPlugin({ include: [path("./locales/**")] }),
    svgLoader(),
    Icons({ compiler: "vue3", autoInstall: false })
  ],
  server: {
    host: "127.0.0.1",
    port: 8081,
    strictPort: true,
    proxy: {
      "^/api/oa/v1(?:/|$)": {
        target: process.env.OA_API_TARGET || "http://127.0.0.1:8888",
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: "dist",
    assetsDir: "pure-assets",
    sourcemap: false,
    rolldownOptions: { input: path("./oa.html") }
  },
  define: {
    "import.meta.env.VITE_HIDE_HOME": JSON.stringify("true")
  },
  test: {
    include: ["src/oa/**/*.test.{js,ts}"],
    environment: "jsdom"
  }
});
