import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { shallowRef } from "vue";
import OrganizationChart from "./organization-chart.vue";
import {
  buildOrganization,
  layoutOrganization,
  visibleDepartments
} from "./organization-model";

const { flow, resize } = vi.hoisted(() => ({
  flow: {
    viewport: null,
    setViewport: vi.fn(),
    setCenter: vi.fn(),
    zoomIn: vi.fn(),
    zoomOut: vi.fn()
  },
  resize: { callback: null }
}));
vi.mock("@vueuse/core", async original => ({
  ...(await original<typeof import("@vueuse/core")>()),
  useResizeObserver: (_target, callback) => {
    resize.callback = callback;
  }
}));
vi.mock("@vue-flow/core", () => ({
  Position: { Top: "top", Bottom: "bottom" },
  Handle: { template: "<span />" },
  useVueFlow: () => flow,
  VueFlow: {
    name: "VueFlow",
    props: [
      "nodes",
      "edges",
      "fitViewOnInit",
      "nodesDraggable",
      "nodesConnectable"
    ],
    emits: ["init"],
    mounted() {
      this.$emit("init");
    },
    template:
      '<div data-flow><slot v-for="node in nodes" name="node-department" :data="node.data" /></div>'
  }
}));
vi.mock("./organization-model", async original => {
  const actual = await original<typeof import("./organization-model")>();
  return { ...actual, layoutOrganization: vi.fn(actual.layoutOrganization) };
});
let wrapper: VueWrapper;
let size: { width: number; height: number };
let frames: Map<number, FrameRequestCallback>;
let frameId = 0;
const rows = Array.from({ length: 100 }, (_, index) => ({
  id: index + 1,
  parentId: index > 1 ? 2 : null,
  name: `部门${index + 1}`,
  employeeCount: index,
  version: 1
}));
const model = buildOrganization(rows);
const items = visibleDepartments(model, new Set(model.byId.keys()));
async function frame() {
  const pending = [...frames.values()];
  frames.clear();
  pending.forEach(callback => callback(0));
  await flushPromises();
}
beforeEach(() => {
  frames = new Map();
  size = { width: 900, height: 560 };
  flow.viewport = shallowRef({ x: 0, y: 0, zoom: 1 });
  flow.setViewport.mockReset().mockImplementation(value => {
    flow.viewport.value = value;
  });
  flow.setCenter.mockReset();
  vi.mocked(layoutOrganization).mockClear();
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(
    () => size.width
  );
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(
    () => size.height
  );
  vi.stubGlobal("requestAnimationFrame", callback => {
    frames.set(++frameId, callback);
    return frameId;
  });
  vi.stubGlobal("cancelAnimationFrame", id => frames.delete(id));
});
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("keeps every graph control reachable by Tab, reveals offscreen keyboard focus without native scrolling, and leaves pointer focus unchanged", async () => {
  wrapper = mount(OrganizationChart, {
    attachTo: document.body,
    props: { items, selectedId: null, expanded: new Set([1, 2]), active: true }
  });
  await flushPromises();
  await frame();
  const rect = (left: number, top: number, width: number, height: number) =>
    ({
      left,
      top,
      right: left + width,
      bottom: top + height,
      width,
      height
    }) as DOMRect;
  vi.spyOn(
    wrapper.get(".oa-org-canvas").element,
    "getBoundingClientRect"
  ).mockReturnValue(rect(50, 120, 900, 560));
  const controls = wrapper.findAll(".oa-org-node button");
  controls.forEach(control =>
    vi
      .spyOn(control.element, "getBoundingClientRect")
      .mockReturnValue(rect(1500, 850, 226, 44))
  );
  const first = controls[0].element as HTMLButtonElement;
  const focus = vi.spyOn(first, "focus");
  const reset = wrapper
    .findAll("button")
    .find(button => button.text() === "恢复视图")!;
  (reset.element as HTMLButtonElement).focus();
  flow.setViewport.mockClear();
  const before = { ...flow.viewport.value };
  const tab = new KeyboardEvent("keydown", {
    key: "Tab",
    bubbles: true,
    cancelable: true
  });
  reset.element.dispatchEvent(tab);
  expect(tab.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(first);
  expect(focus).toHaveBeenCalledWith({ preventScroll: true });
  expect(flow.viewport.value).toEqual({
    ...before,
    x: before.x - 788,
    y: before.y - 226
  });

  // All rendered node controls stay in the sequence, including expansion buttons.
  for (let index = 1; index < 12; index++) {
    document.activeElement?.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "Tab",
        bubbles: true,
        cancelable: true
      })
    );
    expect(document.activeElement).toBe(controls[index].element);
  }
  document.activeElement?.dispatchEvent(
    new KeyboardEvent("keydown", {
      key: "Tab",
      shiftKey: true,
      bubbles: true,
      cancelable: true
    })
  );
  expect(document.activeElement).toBe(controls[10].element);

  window.dispatchEvent(new Event("pointerdown"));
  flow.setViewport.mockClear();
  first.focus();
  await controls[0].trigger("click");
  expect(flow.setViewport).not.toHaveBeenCalled();
  expect(wrapper.emitted("select")?.[0][0]).toBe(1);
  const last = controls.at(-1)!.element as HTMLButtonElement;
  last.focus();
  const exit = new KeyboardEvent("keydown", {
    key: "Tab",
    bubbles: true,
    cancelable: true
  });
  last.dispatchEvent(exit);
  expect(exit.defaultPrevented).toBe(false);
});

it("reveals native keyboard entry from outside the chart but preserves an already visible control and drawer focus restoration", async () => {
  wrapper = mount(OrganizationChart, {
    attachTo: document.body,
    props: { items, selectedId: null, expanded: new Set([1, 2]), active: true }
  });
  await flushPromises();
  await frame();
  vi.spyOn(
    wrapper.get(".oa-org-canvas").element,
    "getBoundingClientRect"
  ).mockReturnValue({ left: 0, top: 100, right: 900, bottom: 660 } as DOMRect);
  const control = wrapper.get(".oa-org-node button")
    .element as HTMLButtonElement;
  const position = vi.spyOn(control, "getBoundingClientRect").mockReturnValue({
    left: 40,
    top: 140,
    right: 266,
    bottom: 184
  } as DOMRect);
  flow.setViewport.mockClear();
  window.dispatchEvent(
    new KeyboardEvent("keydown", { key: "Tab", shiftKey: true })
  );
  control.focus();
  expect(flow.setViewport).not.toHaveBeenCalled();
  control.blur();
  position.mockReturnValue({
    left: -300,
    top: 140,
    right: -74,
    bottom: 184
  } as DOMRect);
  control.focus();
  expect(flow.setViewport).toHaveBeenCalledOnce();
  flow.setViewport.mockClear();
  control.blur();
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
  control.focus({ preventScroll: true });
  expect(flow.setViewport).not.toHaveBeenCalled();
});

it("initializes at 100% on a real root with children and does not relayout or fit on selection, drawer or unchanged refresh", async () => {
  wrapper = mount(OrganizationChart, {
    props: { items, selectedId: null, expanded: new Set([1, 2]), active: true }
  });
  await flushPromises();
  await frame();
  expect(layoutOrganization).toHaveBeenCalledTimes(1);
  expect(flow.setViewport).toHaveBeenCalledTimes(1);
  expect(flow.viewport.value.zoom).toBe(1);
  const view = { ...flow.viewport.value };
  await wrapper.setProps({ selectedId: 4 });
  await wrapper.setProps({ items: structuredClone(items) });
  await frame();
  expect(layoutOrganization).toHaveBeenCalledTimes(1);
  expect(flow.viewport.value).toEqual(view);
  expect(flow.setCenter).not.toHaveBeenCalled();
  const canvas = wrapper.findComponent({ name: "VueFlow" });
  expect(canvas.props("fitViewOnInit")).toBe(false);
  expect(canvas.props("nodesDraggable")).toBe(false);
  expect(canvas.props("nodesConnectable")).toBe(false);
});

it("coalesces resize, ignores unchanged dimensions, preserves pan/zoom on changed data, and locates only on explicit request", async () => {
  wrapper = mount(OrganizationChart, {
    props: { items, selectedId: 2, expanded: new Set([1, 2]), active: true }
  });
  await flushPromises();
  await frame();
  flow.viewport.value = { x: -400, y: -100, zoom: 1.2 };
  resize.callback();
  resize.callback();
  resize.callback();
  await frame();
  expect(layoutOrganization).toHaveBeenCalledTimes(1);
  size.width = 760;
  resize.callback();
  resize.callback();
  await frame();
  expect(layoutOrganization).toHaveBeenCalledTimes(2);
  expect(flow.viewport.value).toEqual({ x: -400, y: -100, zoom: 1.2 });
  const updated = structuredClone(items);
  updated[1].department.name = "保存后的部门名称";
  await wrapper.setProps({ items: updated });
  await frame();
  expect(layoutOrganization).toHaveBeenCalledTimes(3);
  expect(flow.setViewport).toHaveBeenCalledTimes(1);
  wrapper.vm.locate(2);
  await frame();
  expect(flow.setCenter).toHaveBeenCalledOnce();
  wrapper.vm.resetView();
  await frame();
  expect(flow.setViewport).toHaveBeenCalledTimes(2);
  expect(flow.viewport.value.zoom).toBe(1);
  resize.callback();
  wrapper.unmount();
  expect(frames.size).toBe(0);
});
