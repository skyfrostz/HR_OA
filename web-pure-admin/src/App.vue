<script setup lang="ts">
import { computed } from "vue";
import { useGlobal } from "@pureadmin/utils";
import en from "element-plus/es/locale/lang/en";
import zhCn from "element-plus/es/locale/lang/zh-cn";

const { $storage } = useGlobal<GlobalPropertiesApi>();
const locale = computed(() => $storage.locale?.locale === "en" ? en : zhCn);
</script>

<template>
  <el-config-provider :locale="locale">
    <router-view />
  </el-config-provider>
</template>
