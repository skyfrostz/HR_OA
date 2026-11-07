<script setup lang="ts">
import { computed } from "vue";
import { isUrl } from "@pureadmin/utils";
import { transformI18n } from "@/plugins/i18n";
import { menuType } from "@/layout/types";

const props = defineProps<{
  to: menuType;
}>();

const isExternalLink = computed(() => isUrl(props.to.name));
const getLinkProps = (item: menuType) => {
  if (isExternalLink.value) {
    return {
      href: item.name,
      target: "_blank",
      rel: "noopener"
    };
  }
  return {
    to: item
  };
};
</script>

<template>
  <component
    :is="isExternalLink ? 'a' : 'router-link'"
    v-bind="getLinkProps(to)"
    :aria-label="transformI18n(to.meta?.title)"
  >
    <slot />
  </component>
</template>
