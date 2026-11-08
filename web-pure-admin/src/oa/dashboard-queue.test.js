import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import Queue from "./dashboard-queue.vue";
import Dashboard from "./dashboard.vue";
const { session, navigation, push } = vi.hoisted(() => ({
  session: { account: null, authorized: vi.fn() },
  navigation: {},
  push: vi.fn()
}));
vi.mock("./session", () => ({ useSession: () => session }));
vi.mock("./navigation", () => ({ employeeNavigation: () => navigation }));
vi.mock("vue-router", () => ({ useRouter: () => ({ push }) }));
let wrapper;
beforeEach(() => {
  session.authorized.mockReset();
  push.mockReset();
  Object.keys(navigation).forEach(k => delete navigation[k]);
  session.account = {
    user: { role: "HR_VIEWER", displayName: "测试查阅员" },
    permissions: {}
  };
});
afterEach(() => wrapper?.unmount());
it("reads authorized queues only for the current role and renders real objects", async () => {
  session.authorized.mockImplementation(async path =>
    path.startsWith("/dashboard")
      ? { contractReminders: 1 }
      : {
          list: [
            {
              contractId: 8,
              employeeId: 11,
              employeeName: "测试员工",
              contractNo: "HT-8",
              endDate: "2026-12-01",
              daysRemaining: 23
            }
          ],
          total: 1
        }
  );
  wrapper = mount(Dashboard);
  await flushPromises();
  expect(session.authorized.mock.calls.map(([path]) => path)).toEqual([
    "/dashboard?days=30",
    "/contract-reminders?days=30&page=1&pageSize=5"
  ]);
  expect(wrapper.text()).toContain("测试员工 · HT-8");
  expect(wrapper.text()).not.toContain("需要交接的审批");
});
it("opens one contract intent during repeated activation and preserves another intent on failure", async () => {
  let finish;
  push.mockImplementation(
    () =>
      new Promise(resolve => {
        finish = resolve;
      })
  );
  session.authorized.mockResolvedValue({
    list: [
      {
        contractId: 8,
        employeeId: 11,
        employeeName: "员工",
        contractNo: "HT-8",
        daysRemaining: 2
      }
    ]
  });
  wrapper = mount(Queue, { props: { kind: "contracts", title: "合同" } });
  await flushPromises();
  const button = wrapper.findAll("button").find(b => b.text() === "查看");
  button.element.click();
  button.element.click();
  await flushPromises();
  expect(push).toHaveBeenCalledTimes(1);
  expect(push).toHaveBeenCalledWith("/employees");
  expect(navigation.contractId).toBe(8);
  navigation.contractId = 99;
  navigation.employeeId = 100;
  finish({ type: 8 });
  await flushPromises();
  expect(navigation.contractId).toBe(99);
  expect(navigation.employeeId).toBe(100);
  expect(wrapper.text()).toContain("未能打开详情");
});
it("renders retryable error instead of empty success and recovers with data", async () => {
  session.authorized
    .mockRejectedValueOnce(new Error("没有访问权限"))
    .mockResolvedValueOnce({
      list: [{ id: 7, title: "测试报销", status: "IN_REVIEW" }]
    });
  wrapper = mount(Queue, { props: { kind: "requests", title: "最近的申请" } });
  await flushPromises();
  expect(wrapper.text()).toContain("没有访问权限");
  expect(wrapper.text()).not.toContain("暂无最近的申请");
  await wrapper
    .findAll("button")
    .find(b => b.text() === "重新加载")
    .trigger("click");
  await flushPromises();
  expect(wrapper.text()).toContain("测试报销");
});

it("serializes statistic navigation for a dual-role user and retains the selected view", async () => {
  session.account.permissions = {
    canApproveRequests: true,
    canSubmitRequests: true
  };
  session.authorized.mockImplementation(async path =>
    path.startsWith("/dashboard")
      ? { pendingTasks: 2, myRequests: 3, contractReminders: 0 }
      : { list: [] }
  );
  let finish;
  push.mockImplementation(
    () =>
      new Promise(resolve => {
        finish = resolve;
      })
  );
  wrapper = mount(Dashboard);
  await flushPromises();
  const button = wrapper
    .findAll("button")
    .find(b => b.text() === "查看我的申请");
  button.element.click();
  button.element.click();
  await flushPromises();
  expect(push).toHaveBeenCalledTimes(1);
  expect(push).toHaveBeenCalledWith("/approvals");
  expect(navigation.approvalView).toBe("mine");
  navigation.approvalView = "pending";
  finish({ type: 8 });
  await flushPromises();
  expect(navigation.approvalView).toBe("pending");
  expect(wrapper.text()).toContain("未能打开目标页面");
});
