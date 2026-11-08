<script setup lang="ts">
import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import {
  computed,
  onBeforeUnmount,
  shallowRef,
  useId,
  useTemplateRef,
  watch
} from "vue";
import { useEventListener, useResizeObserver } from "@vueuse/core";
import { ElButton } from "element-plus";
import {
  Handle,
  Position,
  VueFlow,
  useVueFlow,
  type Edge,
  type Node
} from "@vue-flow/core";
import {
  layoutOrganization,
  type OrganizationItem
} from "./organization-model";

const props = defineProps<{
  items: OrganizationItem[];
  expanded: Set<number>;
  selectedId: number | null;
  active: boolean;
}>();
const emit = defineEmits<{
  select: [id: number, event: MouseEvent];
  toggle: [id: number];
}>();
const id = "oa-organization-" + useId();
const chart = useTemplateRef("chart");
const container = useTemplateRef("container");
const { setViewport, setCenter, zoomIn, zoomOut, viewport } = useVueFlow(id);
const nodes = shallowRef<Node[]>([]);
const edges = shallowRef<Edge[]>([]);
const zoom = computed(() => Math.round(viewport.value.zoom * 100));
const signature = computed(() =>
  JSON.stringify(
    props.items.map(item => [
      item.department.id,
      item.department.name,
      item.department.employeeCount,
      item.parentId,
      item.childCount
    ])
  )
);
let ready = false;
let initialized = false;
let frame: number | undefined;
let previousLayout = "";
let pendingLocation: number | "start" | null = null;
let geometry: ReturnType<typeof layoutOrganization> = [];
let keyboardTab = false;

useEventListener(
  window,
  "keydown",
  event => {
    keyboardTab = event.key === "Tab";
  },
  { capture: true }
);
useEventListener(
  window,
  "pointerdown",
  () => {
    keyboardTab = false;
  },
  { capture: true }
);

function revealKeyboardTarget(event: FocusEvent) {
  const target = event.target;
  const canvas = container.value;
  if (
    !keyboardTab ||
    !props.active ||
    !canvas ||
    !(target instanceof HTMLElement) ||
    !target.closest(".oa-org-node")
  )
    return;
  let clip = canvas.getBoundingClientRect();
  // The graph may be below the page fold. Scroll its viewport, never its transformed nodes.
  if (Math.min(clip.bottom, window.innerHeight) - Math.max(clip.top, 0) < 88) {
    canvas.scrollIntoView?.({
      block: "center",
      inline: "nearest",
      behavior: "instant"
    });
    clip = canvas.getBoundingClientRect();
  }
  const bounds = {
    left: Math.max(clip.left, 0) + 12,
    right: Math.min(clip.right, window.innerWidth) - 12,
    top: Math.max(clip.top, 0) + 12,
    bottom: Math.min(clip.bottom, window.innerHeight) - 12
  };
  const rect = target.getBoundingClientRect();
  if (bounds.right <= bounds.left || bounds.bottom <= bounds.top) return;
  const shift = (start: number, end: number, low: number, high: number) => {
    if (end - start > high - low) return (low + high - start - end) / 2;
    if (start < low) return low - start;
    if (end > high) return high - end;
    return 0;
  };
  const x = shift(rect.left, rect.right, bounds.left, bounds.right);
  const y = shift(rect.top, rect.bottom, bounds.top, bounds.bottom);
  if (x || y)
    void setViewport({
      ...viewport.value,
      x: viewport.value.x + x,
      y: viewport.value.y + y
    });
}

function navigateChart(event: KeyboardEvent) {
  if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey)
    return;
  const controls = Array.from(
    chart.value?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)") ||
      []
  );
  const current = controls.indexOf(event.target as HTMLButtonElement);
  const next = controls[current + (event.shiftKey ? -1 : 1)];
  if (current < 0 || !next?.closest(".oa-org-node")) return;
  event.preventDefault();
  keyboardTab = true;
  next.focus({ preventScroll: true });
}

function positionView() {
  if (
    !ready ||
    !props.active ||
    !container.value?.clientWidth ||
    !geometry.length
  )
    return;
  if (!initialized || pendingLocation === "start") {
    const first =
      geometry.find(item => item.parentId === null && item.childCount > 0) ||
      geometry[0];
    void setViewport({
      x:
        Math.max(16, (container.value.clientWidth - first.width) / 2) -
        first.position.x,
      y: 24 - first.position.y,
      zoom: 1
    });
    initialized = true;
  } else if (pendingLocation !== null) {
    const node = geometry.find(item => item.department.id === pendingLocation);
    if (!node) return;
    void setCenter(
      node.position.x + node.width / 2,
      node.position.y + node.height / 2,
      { zoom: Math.max(1, viewport.value.zoom) }
    );
  }
  pendingLocation = null;
}

function scheduleLayout() {
  if (frame !== undefined) return;
  frame = requestAnimationFrame(() => {
    frame = undefined;
    const element = container.value;
    if (!props.active || !element?.clientWidth || !element.clientHeight) return;
    const key = `${signature.value}|${element.clientWidth}|${element.clientHeight}`;
    if (key !== previousLayout) {
      previousLayout = key;
      geometry = layoutOrganization(props.items);
      nodes.value = geometry.map(item => ({
        id: String(item.department.id),
        type: "department",
        position: item.position,
        data: item,
        style: { width: `${item.width}px`, height: `${item.height}px` },
        draggable: false,
        connectable: false,
        selectable: false,
        focusable: false
      }));
      const visible = new Set(geometry.map(item => item.department.id));
      edges.value = geometry
        .filter(item => item.parentId !== null && visible.has(item.parentId))
        .map(item => ({
          id: `${item.parentId}-${item.department.id}`,
          source: String(item.parentId),
          target: String(item.department.id),
          type: "smoothstep",
          selectable: false,
          focusable: false,
          updatable: false
        }));
    }
    positionView();
  });
}
watch([signature, () => props.active], scheduleLayout, {
  flush: "post",
  immediate: true
});
useResizeObserver(container, scheduleLayout);
onBeforeUnmount(() => {
  if (frame !== undefined) cancelAnimationFrame(frame);
});

function locate(departmentId: number) {
  pendingLocation = departmentId;
  scheduleLayout();
}
function resetView() {
  pendingLocation = "start";
  scheduleLayout();
}
function onReady() {
  ready = true;
  scheduleLayout();
}
defineExpose({ locate, resetView });
</script>

<template>
  <section
    ref="chart"
    class="oa-org-chart"
    aria-label="部门组织结构图"
    @keydown.capture="navigateChart"
    @focusin="revealKeyboardTarget"
  >
    <div class="oa-org-chart-tools">
      <p class="oa-muted">拖动画布浏览；部门关系通过表单维护。</p>
      <div class="oa-actions" aria-label="组织图缩放">
        <ElButton
          aria-label="缩小组织图"
          :disabled="zoom <= 85"
          @click="zoomOut()"
          >−</ElButton
        >
        <output aria-label="当前缩放">{{ zoom }}%</output>
        <ElButton
          aria-label="放大组织图"
          :disabled="zoom >= 160"
          @click="zoomIn()"
          >＋</ElButton
        >
        <ElButton @click="resetView">恢复视图</ElButton>
      </div>
    </div>
    <div ref="container" class="oa-org-canvas">
      <VueFlow
        :id="id"
        :nodes="nodes"
        :edges="edges"
        :min-zoom="0.85"
        :max-zoom="1.6"
        :nodes-draggable="false"
        :nodes-connectable="false"
        :nodes-focusable="false"
        :edges-updatable="false"
        :edges-focusable="false"
        :elements-selectable="false"
        :select-nodes-on-drag="false"
        :zoom-on-scroll="false"
        :zoom-on-double-click="false"
        :prevent-scrolling="false"
        :delete-key-code="null"
        :fit-view-on-init="false"
        @init="onReady"
      >
        <template #node-department="{ data }">
          <article
            class="oa-org-node"
            :class="{ 'is-current': selectedId === data.department.id }"
          >
            <Handle
              type="target"
              :position="Position.Top"
              :connectable="false"
            />
            <button
              type="button"
              class="oa-org-node-title nodrag nopan"
              :data-department-id="data.department.id"
              :aria-label="'查看部门 ' + data.department.name"
              :aria-pressed="selectedId === data.department.id"
              @click="emit('select', data.department.id, $event)"
            >
              {{ data.department.name }}
            </button>
            <div class="oa-org-node-counts">
              <span
                >直属员工
                <strong>{{
                  data.department.employeeCount ?? "—"
                }}</strong></span
              >
              <button
                v-if="data.childCount"
                type="button"
                class="oa-org-branch nodrag nopan"
                :aria-label="
                  (expanded.has(data.department.id) ? '收起' : '展开') +
                  data.department.name +
                  '的下级部门'
                "
                :aria-expanded="expanded.has(data.department.id)"
                @click="emit('toggle', data.department.id)"
              >
                {{ expanded.has(data.department.id) ? "−" : "＋" }} 下级
                {{ data.childCount }}
              </button>
              <span v-else>下级 0</span>
            </div>
            <Handle
              type="source"
              :position="Position.Bottom"
              :connectable="false"
            />
          </article>
        </template>
      </VueFlow>
    </div>
  </section>
</template>

<style scoped>
.oa-org-chart-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--oa-line);
}

.oa-org-chart-tools p {
  margin: 0;
  font-size: 12px;
}

.oa-org-chart-tools output {
  min-width: 3rem;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.oa-org-canvas {
  width: 100%;
  height: clamp(420px, 62vh, 720px);
  background: var(--oa-canvas);
}

.oa-org-node {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  width: 100%;
  height: 100%;
  padding: 12px 16px;
  color: var(--oa-text);
  pointer-events: auto;
  background: var(--oa-surface);
  border: 1px solid var(--oa-line);
  border-radius: 8px;
}

.oa-org-node.is-current {
  border-color: var(--oa-accent-text);
  box-shadow: 0 0 0 1px var(--oa-accent-text);
}

.oa-org-node-title {
  display: block;
  width: 100%;
  min-height: 44px;
  padding: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
  color: var(--oa-text);
  text-align: start;
  overflow-wrap: anywhere;
  cursor: pointer;
  background: transparent;
  border: 0;
}

.oa-org-node-title:hover {
  color: var(--oa-accent-text);
}

.oa-org-node-counts {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--oa-muted);
}

.oa-org-node-counts strong {
  color: var(--oa-text);
}

.oa-org-branch {
  min-height: 44px;
  padding: 0 4px;
  color: var(--oa-accent-text);
  cursor: pointer;
  background: transparent;
  border: 0;
}

.oa-org-node :deep(.vue-flow__handle) {
  pointer-events: none;
  opacity: 0;
}

.oa-org-canvas :deep(.vue-flow__edge-path) {
  stroke: var(--oa-muted);
  stroke-width: 1.5;
}

.oa-org-canvas :deep(.vue-flow__node) {
  padding: 0;
}

.oa-org-canvas :deep(.vue-flow) {
  overflow: clip;
}

@media (width <= 38rem) {
  .oa-org-chart-tools {
    padding: 12px;
  }

  .oa-org-chart-tools .oa-actions {
    justify-content: flex-end;
    width: 100%;
  }

  .oa-org-canvas {
    height: 460px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .oa-org-node {
    transition: none;
  }
}
</style>
