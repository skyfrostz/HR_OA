<script setup lang="ts">
import { useTemplateRef } from "vue";
import type { Organization, OrganizationItem } from "./organization-model";
defineProps<{
  items: OrganizationItem[];
  model: Organization;
  expanded: Set<number>;
  selectedId: number | null;
}>();
const emit = defineEmits<{
  select: [id: number, event: MouseEvent];
  toggle: [id: number];
}>();
const tree = useTemplateRef("tree");
function locate(id: number) {
  const button = tree.value?.querySelector<HTMLButtonElement>(
    `button[data-department-id="${id}"]`
  );
  button?.focus({ preventScroll: true });
  button?.scrollIntoView?.({ block: "nearest", behavior: "instant" });
}
defineExpose({ locate });
</script>

<template>
  <ul ref="tree" class="oa-org-tree-list" aria-label="部门层级列表">
    <li
      v-for="item in items"
      :key="item.department.id"
      :style="{ '--org-indent': Math.min(item.depth - 1, 4) }"
      :class="{
        'has-parent': item.parentId !== null,
        'is-current': selectedId === item.department.id
      }"
    >
      <div class="oa-org-tree-row">
        <button
          v-if="item.childCount"
          type="button"
          class="oa-org-tree-toggle"
          :aria-expanded="expanded.has(item.department.id)"
          :aria-label="
            (expanded.has(item.department.id) ? '收起' : '展开') +
            item.department.name +
            '的下级部门'
          "
          @click="emit('toggle', item.department.id)"
        >
          {{ expanded.has(item.department.id) ? "−" : "＋" }}
        </button>
        <span v-else class="oa-org-tree-leaf" aria-hidden="true">·</span>
        <button
          type="button"
          class="oa-org-tree-name"
          :data-department-id="item.department.id"
          :aria-label="'查看部门 ' + item.department.name"
          :aria-pressed="selectedId === item.department.id"
          @click="emit('select', item.department.id, $event)"
        >
          <strong>{{ item.department.name }}</strong>
          <span
            >直属员工 {{ item.department.employeeCount ?? "—" }} · 下级
            {{ item.childCount }}</span
          >
          <span v-if="item.depth > 4"
            >第 {{ item.depth }} 层 · 上级：{{
              model.byId.get(item.parentId!)?.name
            }}</span
          >
        </button>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.oa-org-tree-list {
  padding: 12px;
  margin: 0;
  list-style: none;
}

.oa-org-tree-list li {
  position: relative;
  padding-inline-start: calc(var(--org-indent) * 12px);
}

.oa-org-tree-list .has-parent::before {
  position: absolute;
  top: 0;
  bottom: 50%;
  left: calc(var(--org-indent) * 12px - 6px);
  width: 12px;
  content: "";
  border-bottom: 1px solid var(--oa-muted);
  border-left: 1px solid var(--oa-muted);
}

.oa-org-tree-row {
  display: flex;
  align-items: center;
  min-width: 0;
  border-bottom: 1px solid var(--oa-line);
}

.oa-org-tree-toggle,
.oa-org-tree-leaf {
  display: grid;
  flex: 0 0 44px;
  place-items: center;
  width: 44px;
  height: 44px;
  color: var(--oa-accent-text);
  background: transparent;
  border: 0;
}

.oa-org-tree-toggle {
  cursor: pointer;
}

.oa-org-tree-name {
  display: grid;
  flex: 1;
  gap: 4px;
  min-width: 0;
  min-height: 64px;
  padding: 12px 8px;
  color: var(--oa-text);
  text-align: start;
  overflow-wrap: anywhere;
  cursor: pointer;
  background: transparent;
  border: 0;
}

.oa-org-tree-name strong {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.5;
}

.oa-org-tree-name span {
  font-size: 12px;
  line-height: 1.5;
  color: var(--oa-muted);
}

.is-current > .oa-org-tree-row {
  background: var(--oa-tint);
  border-radius: 8px;
}
</style>
