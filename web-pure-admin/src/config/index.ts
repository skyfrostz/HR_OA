import type { App } from "vue";
import platform from "../../public/platform-config.json";

const config: PlatformConfigs = { ...platform, TooltipEffect: "light" };

export function getConfig(): PlatformConfigs;
export function getConfig<K extends keyof PlatformConfigs>(key: K): PlatformConfigs[K];
export function getConfig(key?: keyof PlatformConfigs) {
  return key ? config[key] : config;
}

export function getPlatformConfig(app: App) {
  app.config.globalProperties.$config = config;
  return config;
}

export const responsiveStorageNameSpace = () => config.ResponsiveStorageNameSpace;
