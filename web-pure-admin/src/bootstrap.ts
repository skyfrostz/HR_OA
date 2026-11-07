import { createApp } from "vue";
import { MotionPlugin } from "@vueuse/motion";
import VueTippy from "vue-tippy";
import Table from "@pureadmin/table";
import App from "./App.vue";
import router from "./router";
import { setupStore } from "@/store";
import { getPlatformConfig } from "@/config";
import { useI18n } from "@/plugins/i18n";
import { useElementPlus } from "@/plugins/elementPlus";
import { injectResponsiveStorage } from "@/utils/responsive";
import { Ripple } from "@/directives/ripple";
import IconifyIconOffline from "@/components/ReIcon/src/iconifyIconOffline";

export function createOaApp() {
  const app = createApp(App);
  const config = getPlatformConfig(app);
  setupStore(app);
  injectResponsiveStorage(app, config);
  app.component("IconifyIconOffline", IconifyIconOffline);
  app.directive("ripple", Ripple);
  app.use(VueTippy).use(MotionPlugin).use(useI18n).use(useElementPlus).use(Table).use(router);
  return app;
}
