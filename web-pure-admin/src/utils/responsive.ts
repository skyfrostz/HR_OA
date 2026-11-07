// 响应式storage
import type { App } from "vue";
import Storage from "responsive-storage";
import { responsiveStorageNameSpace } from "@/config";

export const injectResponsiveStorage = (app: App, config: PlatformConfigs) => {
  const nameSpace = responsiveStorageNameSpace();
  const configObj = {
      // 国际化 默认中文zh
      locale: Storage.getData("locale", nameSpace) ?? {
        locale: config.Locale ?? "zh"
      },
      // layout模式以及主题
      layout: {
        theme: config.Theme ?? "light",
        darkMode: config.DarkMode ?? false,
        sidebarStatus: config.SidebarStatus ?? true,
        epThemeColor: config.EpThemeColor ?? "#409EFF",
        themeColor: config.Theme ?? "light", // 主题色（对应系统配置中的主题色，与theme不同的是它不会受到浅色、深色主题模式切换的影响，只会在手动点击主题色时改变）
        themeMode: config.ThemeMode ?? "light",
        ...Storage.getData("layout", nameSpace),
        layout: "vertical"
      },
      // 系统配置-界面显示
      configure: Storage.getData("configure", nameSpace) ?? {
        grey: config.Grey ?? false,
        weak: config.Weak ?? false,
        hideTabs: config.HideTabs ?? false,
        hideFooter: config.HideFooter ?? true,
        showLogo: config.ShowLogo ?? true,
        watermark: config.Watermark ?? false,
        watermarkText: config.WatermarkText ?? "",
        tagsStyle: config.TagsStyle ?? "chrome",
        multiTagsCache: config.MultiTagsCache ?? false,
        stretch: config.Stretch ?? false
      }
  };

  app.use(Storage, { nameSpace, memory: configObj });
};
