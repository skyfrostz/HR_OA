import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { ElMessageBox } from "element-plus";
import Departments from "../departments.vue";
import type { DepartmentTreeNode } from "../business";

const { session, screen, writeLock, chart } = vi.hoisted(() => ({
  session: {
    account: {
      user: { role: "HR_ADMIN" },
      permissions: { canManageDepartments: true }
    },
    authorized: vi.fn()
  },
  screen: { narrow: true },
  writeLock: vi.fn(),
  chart: { locate: vi.fn(), resetView: vi.fn() }
}));
vi.mock("../session", () => ({ useSession: () => session }));
vi.mock("../use-route-write-lock", () => ({ useRouteWriteLock: writeLock }));
vi.mock("@vueuse/core", async original => ({
  ...(await original<typeof import("@vueuse/core")>()),
  useMediaQuery: () => ({ value: screen.narrow, __v_isRef: true })
}));
const drawerStub = {
  props: ["modelValue", "beforeClose"],
  emits: ["close", "closed"],
  watch: {
    modelValue(open: boolean) {
      if (!open) this.$nextTick(() => this.$emit("closed"));
    }
  },
  template:
    '<div v-if="modelValue" role="dialog"><button aria-label="关闭部门详情" @click="beforeClose()">关闭</button><slot /></div>'
};
const chartStub = {
  props: ["items", "expanded", "selectedId", "active"],
  methods: { locate: chart.locate, resetView: chart.resetView },
  template:
    '<div class="chart-stub" :data-active="active"><span v-for="item in items" :key="item.department.id">{{ item.department.name }}</span></div>'
};
let wrapper: VueWrapper;
let rows: DepartmentTreeNode[];
const node = (
  id: number,
  parentId: number | null = null,
  name = `部门${id}`
): DepartmentTreeNode => ({
  id,
  parentId,
  name,
  version: 1,
  employeeCount: id
});
function button(text: string) {
  return wrapper.findAll("button").find(item => item.text() === text)!;
}
async function render() {
  wrapper = mount(Departments, {
    attachTo: document.body,
    global: {
      stubs: {
        ElDrawer: drawerStub,
        OrganizationChart: chartStub,
        AuditHistory: {
          props: ["departmentId"],
          template: "<p data-history>部门历史 {{ departmentId }}</p>"
        }
      }
    }
  });
  await flushPromises();
}
async function open(id: number) {
  await wrapper
    .get(`.oa-org-tree-list button[data-department-id="${id}"]`)
    .trigger("click");
  await flushPromises();
}
beforeEach(() => {
  session.authorized.mockReset();
  writeLock.mockClear();
  chart.locate.mockClear();
  session.account.permissions.canManageDepartments = true;
  screen.narrow = true;
  rows = [node(1), node(2, 1), node(3, 2), node(4)];
  session.authorized.mockImplementation(async path => {
    if (path === "/departments/tree") return { list: structuredClone(rows) };
    throw new Error("Unexpected " + path);
  });
});
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

it("loads the complete tree, defaults to two levels on mobile and locates hidden descendants without opening a drawer", async () => {
  await render();
  expect(session.authorized).toHaveBeenCalledTimes(1);
  expect(session.authorized.mock.calls[0][0]).toBe("/departments/tree");
  expect(
    wrapper.find('.oa-org-tree-list [data-department-id="3"]').exists()
  ).toBe(false);
  await wrapper.get("#department-search").setValue("部门3");
  await wrapper.get("form[role=search]").trigger("submit");
  await flushPromises();
  expect(
    wrapper
      .get('.oa-org-tree-list [data-department-id="3"]')
      .attributes("aria-pressed")
  ).toBe("true");
  expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
  await button("刷新").trigger("click");
  await flushPromises();
  expect(
    wrapper.find('.oa-org-tree-list [data-department-id="3"]').exists()
  ).toBe(true);
  await button("收起至两层").trigger("click");
  expect(
    wrapper.find('.oa-org-tree-list [data-department-id="3"]').exists()
  ).toBe(false);
});

it("keeps mobile browsing tools in a closed native disclosure while search and the department tree remain available", async () => {
  await render();
  const tools = wrapper.get("details.oa-org-mobile-tools");
  expect((tools.element as HTMLDetailsElement).open).toBe(false);
  expect(tools.get("summary").text()).toContain("浏览选项");
  expect(tools.get("summary").text()).toContain("层级树");
  expect(tools.find("#department-search").exists()).toBe(false);
  expect(wrapper.findAll("#organization-root")).toHaveLength(1);
  expect(wrapper.find(".oa-org-expansion").exists()).toBe(false);
  expect(wrapper.get(".oa-org-tree-list").text()).toContain("部门1");
  await tools.get("summary").trigger("click");
  expect((tools.element as HTMLDetailsElement).open).toBe(true);
  await button("展开全部").trigger("click");
  expect(
    wrapper.find('.oa-org-tree-list [data-department-id="3"]').exists()
  ).toBe(true);
  await button("列表").trigger("click");
  expect(tools.get("summary").text()).toContain("列表");
  expect(wrapper.get(".oa-department-list").text()).toContain("部门3");
});

it("offers each real root as an explicit location and keeps chart selection distinct from opening detail", async () => {
  screen.narrow = false;
  await render();
  expect(wrapper.find("details.oa-org-mobile-tools").exists()).toBe(false);
  expect(
    wrapper
      .get("#organization-root")
      .findAll("option")
      .map(option => option.element.value)
  ).toEqual(["1", "4"]);
  await wrapper.get("#organization-root").setValue("4");
  await flushPromises();
  expect(chart.locate).toHaveBeenCalledWith(4);
  expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
  expect(wrapper.get(".chart-stub").attributes("data-active")).toBe("true");
  await button("列表").trigger("click");
  await wrapper
    .get('.oa-department-list [data-department-id="2"]')
    .trigger("click");
  expect(wrapper.get('[role="dialog"]').text()).toContain("部门2");
});

it("keeps a readonly department drawer and its history without write actions, and does not invent a missing employee count", async () => {
  session.account.permissions.canManageDepartments = false;
  delete (rows[0] as Partial<DepartmentTreeNode>).employeeCount;
  await render();
  await open(1);
  const dialog = wrapper.get('[role="dialog"]');
  expect(dialog.text()).toContain("直属员工—");
  expect(dialog.text()).not.toContain("编辑部门");
  expect(dialog.text()).not.toContain("新增下级");
  expect(button("新增部门")).toBeUndefined();
  await button("变更历史").trigger("click");
  expect(wrapper.get("[data-history]").text()).toBe("部门历史 1");
});

it("excludes self and descendants, preserves a conflict draft, then uses the reread version and keeps selection/expansion", async () => {
  let puts = 0;
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/departments/tree") return { list: structuredClone(rows) };
    if (path === "/departments/2" && options.method === "PUT") {
      if (++puts === 1) {
        rows[1].version = 3;
        rows[1].name = "他人修改";
        throw Object.assign(new Error("记录冲突"), {
          status: 409,
          errorCode: "STALE_RECORD"
        });
      }
      rows[1] = { ...rows[1], ...options.data, version: 4 };
      return structuredClone(rows[1]);
    }
    throw new Error("Unexpected request");
  });
  await render();
  await button("展开全部").trigger("click");
  await open(2);
  await button("编辑部门").trigger("click");
  expect(
    wrapper
      .get("#department-parent")
      .findAll("option")
      .map(option => option.element.value)
  ).toEqual(["无上级部门", "1", "4"]);
  await wrapper.get("#department-name").setValue("保留的编辑内容");
  expect(button("刷新").element.disabled).toBe(true);
  expect(writeLock.mock.calls.at(-1)?.[0].value).toBe(true);
  await wrapper.get('[role="dialog"] form').trigger("submit");
  await flushPromises();
  expect(wrapper.get("#department-name").element.value).toBe("保留的编辑内容");
  expect(wrapper.text()).toContain("输入已保留");
  await button("重新读取部门").trigger("click");
  await flushPromises();
  expect(wrapper.get("#department-name").element.value).toBe("保留的编辑内容");
  await wrapper.get('[role="dialog"] form').trigger("submit");
  await flushPromises();
  const writes = session.authorized.mock.calls.filter(
    ([path, options]) => path === "/departments/2" && options?.method === "PUT"
  );
  expect(writes.map(([, options]) => options.data.version)).toEqual([1, 3]);
  expect(wrapper.get('[role="dialog"]').text()).toContain("部门已保存");
  expect(
    wrapper
      .get('.oa-org-tree-list [data-department-id="2"]')
      .attributes("aria-pressed")
  ).toBe("true");
  expect(
    wrapper.find('.oa-org-tree-list [data-department-id="3"]').exists()
  ).toBe(true);
  expect(writeLock.mock.calls.at(-1)?.[0].value).toBe(false);
});

it("creates a child through the form and retains an unchanged parent, with the complete refreshed tree", async () => {
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/departments/tree") return { list: structuredClone(rows) };
    if (path === "/departments" && options.method === "POST") {
      const created = { ...node(5), ...options.data };
      rows.push(created);
      return created;
    }
    throw new Error("Unexpected request");
  });
  await render();
  await open(2);
  await button("新增下级").trigger("click");
  expect(wrapper.get("#department-parent").element.value).toBe("2");
  await wrapper.get("#department-name").setValue("新增子部门");
  await wrapper.get('[role="dialog"] form').trigger("submit");
  await flushPromises();
  expect(rows[1].name).toBe("部门2");
  expect(wrapper.get('[role="dialog"]').text()).toContain("新增子部门");
  expect(
    wrapper
      .get('.oa-org-tree-list [data-department-id="5"]')
      .attributes("aria-pressed")
  ).toBe("true");
});

it("keeps input on an invalid relationship error and requires an explicit discard before closing dirty edits", async () => {
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/departments/tree") return { list: structuredClone(rows) };
    if (options?.method === "PUT")
      throw Object.assign(new Error("部门层级已变化，请核对上级"), {
        status: 400
      });
  });
  const confirm = vi.spyOn(ElMessageBox, "confirm").mockRejectedValue("cancel");
  await render();
  await open(2);
  await button("编辑部门").trigger("click");
  await wrapper.get("#department-name").setValue("未丢失的名称");
  await wrapper.get('[role="dialog"] form').trigger("submit");
  await flushPromises();
  expect(wrapper.get("#department-name").element.value).toBe("未丢失的名称");
  expect(wrapper.text()).toContain("部门层级已变化");
  await wrapper.get('button[aria-label="关闭部门详情"]').trigger("click");
  await flushPromises();
  expect(confirm).toHaveBeenCalledTimes(1);
  expect(wrapper.find("#department-name").exists()).toBe(true);
  confirm.mockResolvedValue("confirm");
  await wrapper.get('button[aria-label="关闭部门详情"]').trigger("click");
  await flushPromises();
  expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
});

it("does not duplicate an uncertain new department after a reread, and retains the draft", async () => {
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/departments/tree") return { list: structuredClone(rows) };
    if (options?.method === "POST") throw new Error("连接中断");
  });
  await render();
  await button("新增部门").trigger("click");
  await wrapper.get("#department-name").setValue("结果待核对的新部门");
  await wrapper.get('[role="dialog"] form').trigger("submit");
  await flushPromises();
  await button("重新读取部门").trigger("click");
  await flushPromises();
  expect(wrapper.get("#department-name").element.value).toBe(
    "结果待核对的新部门"
  );
  expect(wrapper.text()).toContain("不会自动重复新增");
  expect(button("保存部门")).toBeUndefined();
  expect(
    session.authorized.mock.calls.filter(
      ([, options]) => options?.method === "POST"
    )
  ).toHaveLength(1);
});

it("does not reclaim focus while a successful save waits for the tree readback", async () => {
  let finishReadback;
  let reads = 0;
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/departments/tree") {
      if (++reads === 1) return { list: structuredClone(rows) };
      return new Promise(resolve => {
        finishReadback = resolve;
      });
    }
    if (options?.method === "PUT")
      return { ...rows[0], ...options.data, version: 2 };
  });
  await render();
  await open(1);
  await button("编辑部门").trigger("click");
  await wrapper.get("#department-name").setValue("已保存名称");
  await wrapper.get('[role="dialog"] form').trigger("submit");
  await flushPromises();
  const history = button("变更历史").element;
  history.focus();
  finishReadback({ list: rows });
  await flushPromises();
  expect(document.activeElement).toBe(history);
});

it("deduplicates the discard prompt and ignores confirmation after the drawer has gone", async () => {
  let finishConfirmation;
  const confirm = vi.spyOn(ElMessageBox, "confirm").mockImplementation(
    () =>
      new Promise(resolve => {
        finishConfirmation = resolve;
      })
  );
  await render();
  await open(2);
  await button("编辑部门").trigger("click");
  await wrapper.get("#department-name").setValue("尚未保存");
  const close = wrapper.get('button[aria-label="关闭部门详情"]');
  await close.trigger("click");
  await close.trigger("click");
  expect(confirm).toHaveBeenCalledOnce();
  wrapper.unmount();
  finishConfirmation("confirm");
  await flushPromises();
  expect(
    session.authorized.mock.calls.filter(
      ([, options]) => options?.method === "PUT"
    )
  ).toHaveLength(0);
});

it("shows empty and failed states distinctly and preserves the existing tree when refresh fails", async () => {
  rows = [];
  await render();
  expect(wrapper.text()).toContain("暂无部门");
  rows = [node(1)];
  await button("刷新").trigger("click");
  await flushPromises();
  session.authorized.mockRejectedValueOnce(new Error("结构读取失败"));
  await button("刷新").trigger("click");
  await flushPromises();
  expect(wrapper.text()).toContain("结构读取失败");
  expect(
    wrapper.find('.oa-org-tree-list [data-department-id="1"]').exists()
  ).toBe(true);
});
