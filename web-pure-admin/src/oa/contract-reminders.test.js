import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defineComponent, nextTick } from "vue";
import { flushPromises, mount } from "@vue/test-utils";
import ContractReminders from "./contract-reminders.vue";
import ContractDrawer from "./contract-drawer.vue";

const { session, detail, routing } = vi.hoisted(() => ({
  session: { authorized: vi.fn() },
  detail: { close: vi.fn(), cancel: vi.fn(), mounted: vi.fn() },
  routing: { leave: null }
}));
vi.mock("./session", () => ({ useSession: () => session }));
vi.mock("vue-router", () => ({
  onBeforeRouteLeave: guard => {
    routing.leave = guard;
  }
}));
vi.mock("./contract-detail.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      name: "ContractDetail",
      props: ["contractId", "expectedEmployeeId", "active"],
      emits: ["select", "changed", "state-change"],
      setup(props, { expose }) {
        detail.mounted(props.contractId);
        expose({ requestClose: detail.close, cancelPending: detail.cancel });
        return () =>
          h(
            "div",
            { "data-detail": props.contractId },
            "目标合同 " + props.contractId
          );
      }
    })
  };
});
const drawerStub = defineComponent({
  name: "ElDrawer",
  props: ["modelValue", "beforeClose"],
  emits: ["closed", "open-auto-focus"],
  template:
    '<div v-show="modelValue" role="dialog"><slot name="header" /><slot /></div>'
});
const tableStub = {
  props: ["data"],
  template:
    '<div><template v-for="row in data" :key="row.contractId"><slot name="actions" :row="row" /></template></div>'
};
const row = {
  contractId: 21,
  employeeId: 11,
  employeeName: "合成员工",
  employeeNo: "SIM-011",
  contractNo: "SIM-021",
  endDate: "2026-11-20",
  daysRemaining: 12
};
const page = (list, number = 1, total = list.length) => ({
  list,
  page: number,
  pageSize: 20,
  total
});
let wrapper;
let host;
beforeEach(() => {
  session.authorized.mockReset();
  detail.close.mockReset().mockResolvedValue(true);
  detail.cancel.mockReset();
  detail.mounted.mockReset();
  host = document.createElement("div");
  host.className = "el-scrollbar__wrap";
  document.body.append(host);
});
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
});
async function mountPage() {
  wrapper = mount(ContractReminders, {
    attachTo: host,
    global: { stubs: { ElDrawer: drawerStub, PureTable: tableStub } }
  });
  await flushPromises();
}
function button(text) {
  return wrapper.findAll("button").find(node => node.text() === text);
}
function shared() {
  return wrapper.findComponent({ name: "ContractDetail" });
}
async function close() {
  await button("关闭").trigger("click");
  await flushPromises();
  wrapper.findComponent({ name: "ElDrawer" }).vm.$emit("closed");
  await flushPromises();
}

it("opens once immediately on rapid double click, preserves the list and uses no employee navigation", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  const trigger = button("查看合同");
  trigger.element.click();
  trigger.element.click();
  await nextTick();
  expect(detail.mounted).toHaveBeenCalledTimes(1);
  expect(shared().props()).toMatchObject({
    contractId: 21,
    expectedEmployeeId: 11,
    active: true
  });
  expect(wrapper.findComponent(ContractDrawer).props("modelValue")).toBe(true);
  expect(trigger.exists()).toBe(true);
  expect(wrapper.get(".oa-contract-drawer-employee").text()).toContain(
    "合成员工"
  );
  expect(wrapper.get(".oa-contract-drawer-employee").text()).toContain(
    "SIM-011"
  );
  expect(session.authorized.mock.calls.map(call => call[0])).toEqual([
    "/contract-reminders?page=1&pageSize=20&days=30"
  ]);
});

it("honors rejected unsaved-close and route leave, then cancels immediately on accepted close", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  await button("查看合同").trigger("click");
  detail.close.mockResolvedValue(false);
  await button("关闭").trigger("click");
  await flushPromises();
  expect(shared().props("active")).toBe(true);
  expect(await routing.leave()).toBe(false);
  detail.close.mockResolvedValue(true);
  await button("关闭").trigger("click");
  await flushPromises();
  expect(shared().props("active")).toBe(false);
  expect(detail.cancel).toHaveBeenCalled();
});

it("restores the trigger and real inner scroll position, including when the trigger was replaced", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  host.scrollTop = 240;
  host.scrollLeft = 12;
  button("查看合同").element.focus();
  await button("查看合同").trigger("click");
  shared().vm.$emit("changed", { contractId: 21, kind: "sign" });
  await flushPromises();
  host.scrollTop = 0;
  await close();
  expect(document.activeElement).toBe(button("查看合同").element);
  expect(host.scrollTop).toBe(240);
  expect(host.scrollLeft).toBe(12);
  expect(wrapper.get("select").element.value).toBe("30");
});

it("preserves rows while refresh is pending and restores heading when a terminated contract disappears", async () => {
  let finish;
  session.authorized.mockResolvedValueOnce(page([row])).mockImplementationOnce(
    () =>
      new Promise(resolve => {
        finish = resolve;
      })
  );
  await mountPage();
  await button("查看合同").trigger("click");
  shared().vm.$emit("changed", {
    contractId: 21,
    kind: "terminate",
    contract: { id: 21, employeeId: 11, status: "TERMINATED" }
  });
  await nextTick();
  expect(button("查看合同")).toBeDefined();
  expect(wrapper.text()).toContain("不再符合到期提醒条件");
  await close();
  finish(page([]));
  await flushPromises();
  expect(document.activeElement).toBe(wrapper.get("h1").element);
  expect(wrapper.text()).toContain("暂无到期合同");
});

it("clamps an emptied last page and preserves the chosen day range", async () => {
  const second = { ...row, contractId: 22 };
  session.authorized.mockImplementation(async path => {
    if (path.includes("page=2")) return page([second], 2, 21);
    return page([row], 1, 21);
  });
  await mountPage();
  await wrapper.get("select").setValue("60");
  await flushPromises();
  await button("下一页").trigger("click");
  await flushPromises();
  await button("查看合同").trigger("click");
  session.authorized.mockImplementation(async path =>
    path.includes("page=2") ? page([], 2, 20) : page([row], 1, 20)
  );
  shared().vm.$emit("changed", { contractId: 22, kind: "terminate" });
  await flushPromises();
  expect(session.authorized).toHaveBeenLastCalledWith(
    "/contract-reminders?page=1&pageSize=20&days=60",
    expect.anything()
  );
  expect(wrapper.get("select").element.value).toBe("60");
});

it("a late close confirmation cannot close a newly opened target", async () => {
  let accept;
  detail.close.mockImplementationOnce(
    () =>
      new Promise(resolve => {
        accept = resolve;
      })
  );
  wrapper = mount(ContractDrawer, {
    props: { modelValue: true, contractId: 21, expectedEmployeeId: 11 },
    global: { stubs: { ElDrawer: drawerStub } }
  });
  const pending = wrapper.vm.requestClose();
  await wrapper.setProps({ modelValue: false });
  await wrapper.setProps({ modelValue: true, contractId: 22 });
  accept(true);
  expect(await pending).toBe(false);
  expect(wrapper.emitted("update:modelValue")).toBeUndefined();
  expect(shared().props("contractId")).toBe(22);
});

it("renewal switches only the drawer target and keeps the expected employee", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  await button("查看合同").trigger("click");
  shared().vm.$emit("select", 22);
  await nextTick();
  expect(shared().props()).toMatchObject({
    contractId: 22,
    expectedEmployeeId: 11
  });
  expect(detail.mounted.mock.calls).toEqual([[21], [22]]);
  await close();
  expect(document.activeElement).toBe(button("查看合同").element);
});

it("does not report an obsolete save refresh as failed after the user changes the query", async () => {
  let finish;
  session.authorized
    .mockResolvedValueOnce(page([row]))
    .mockImplementationOnce(
      () =>
        new Promise(resolve => {
          finish = resolve;
        })
    )
    .mockResolvedValue(page([row]));
  await mountPage();
  await button("查看合同").trigger("click");
  shared().vm.$emit("changed", { contractId: 21, kind: "sign" });
  await nextTick();
  await close();
  await wrapper.get("select").setValue("60");
  await flushPromises();
  finish(page([row]));
  await flushPromises();
  expect(wrapper.text()).not.toContain("刷新失败");
  expect(wrapper.get("select").element.value).toBe("60");
});

it("puts visible focus on close after the real Element Plus drawer has opened", async () => {
  wrapper = mount(ContractDrawer, {
    attachTo: host,
    props: { modelValue: false, contractId: 21, expectedEmployeeId: 11 },
    global: { stubs: { transition: false } }
  });
  await wrapper.setProps({ modelValue: true });
  await vi.waitFor(() => {
    const closeButton = [
      ...document.querySelectorAll(".oa-contract-drawer button")
    ].find(node => node.textContent.trim() === "关闭");
    expect(closeButton).toBeDefined();
    expect(document.activeElement).toBe(closeButton);
  });
});

it("keeps both header close and route departure unavailable during submission", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  await button("查看合同").trigger("click");
  shared().vm.$emit("state-change", { busy: true, dirty: true, editing: true });
  await nextTick();
  expect(button("关闭").element.disabled).toBe(true);
  expect(await routing.leave()).toBe(false);
  expect(detail.close).not.toHaveBeenCalled();
  shared().vm.$emit("state-change", {
    busy: false,
    dirty: false,
    editing: false
  });
  await nextTick();
  expect(button("关闭").element.disabled).toBe(false);
});

it("explains the original reminder removal only from a verified signed successor", async () => {
  session.authorized
    .mockResolvedValueOnce(page([row]))
    .mockResolvedValue(page([], 1, 40));
  await mountPage();
  await button("查看合同").trigger("click");
  shared().vm.$emit("changed", {
    kind: "sign",
    contractId: 22,
    contract: {
      id: 22,
      employeeId: 11,
      status: "SIGNED",
      previousContractId: 21,
      endDate: "2099-12-31"
    }
  });
  await flushPromises();
  expect(wrapper.text()).toContain(
    "续约合同已登记签署，原合同不再符合到期提醒条件"
  );
});

it("does not claim termination when an unknown operation is reread as still signed", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  await button("查看合同").trigger("click");
  shared().vm.$emit("changed", {
    kind: "terminate",
    contractId: 21,
    contract: {
      id: 21,
      employeeId: 11,
      status: "SIGNED"
    }
  });
  await flushPromises();
  expect(wrapper.text()).not.toContain("合同已终止");
  expect(wrapper.text()).toContain("到期提醒已刷新");
});

it("holds its scroll lease through exit and releases it before a rapid reopen", async () => {
  session.authorized.mockResolvedValue(page([row]));
  await mountPage();
  await button("查看合同").trigger("click");
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  await button("关闭").trigger("click");
  await flushPromises();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  wrapper.findComponent({ name: "ElDrawer" }).vm.$emit("closed");
  await flushPromises();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    false
  );
  await button("查看合同").trigger("click");
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  await close();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    false
  );
});
