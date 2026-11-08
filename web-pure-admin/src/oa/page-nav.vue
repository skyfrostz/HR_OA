<script setup>
import { computed, shallowRef, watch } from "vue";
import { ElButton } from "element-plus";

const props = defineProps({
  page: { type: Number, required: true },
  total: { type: Number, required: true },
  pageSize: { type: Number, required: true },
  label: { type: String, required: true },
  disabled: Boolean,
  jump: Boolean
});
const emit = defineEmits(["change"]);
const pages = computed(() =>
  Math.max(1, Math.ceil(props.total / props.pageSize))
);
const target = shallowRef(props.page);
watch(
  () => props.page,
  page => {
    target.value = page;
  }
);
function jumpTo() {
  if (
    Number.isInteger(target.value) &&
    target.value >= 1 &&
    target.value <= pages.value
  )
    emit("change", target.value);
}
</script>

<template>
  <nav
    v-if="total"
    class="oa-pagination"
    :class="{ 'oa-pagination-single': pages === 1 }"
    :aria-label="label"
  >
    <ElButton
      v-if="pages > 1"
      :disabled="disabled || page <= 1"
      @click="emit('change', page - 1)"
      >上一页</ElButton
    >
    <span aria-live="polite"
      ><template v-if="pages > 1">第 {{ page }} / {{ pages }} 页 · </template>共
      {{ total }} 条</span
    >
    <ElButton
      v-if="pages > 1"
      :disabled="disabled || page >= pages"
      @click="emit('change', page + 1)"
      >下一页</ElButton
    >
    <form
      v-if="jump && pages > 1"
      class="oa-page-jump"
      @submit.prevent="jumpTo"
    >
      <label
        >页码
        <input
          v-model.number="target"
          type="number"
          min="1"
          :max="pages"
          :disabled="disabled"
          required
      /></label>
      <ElButton native-type="submit" :disabled="disabled">跳转</ElButton>
    </form>
  </nav>
</template>
