import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ElDatePicker } from "element-plus";
import { flushPromises, mount } from "@vue/test-utils";
import WorkflowSettings from "./workflow-settings.vue";
import ApprovedLeaves from "./approved-leaves.vue";
import WorkflowExceptions from "./workflow-exceptions.vue";
import LeaveLedger from "./leave-ledger.vue";
import Dashboard from "./dashboard.vue";
const { session, push, navigation } = vi.hoisted(() => ({
  session: {
    account: {
      user: { id: 1, role: "HR_ADMIN", employeeId: 11 },
      permissions: {}
    },
    authorized: vi.fn()
  },
  push: vi.fn(),
  navigation: {}
}));
vi.mock("./session", () => ({ useSession: () => session }));
vi.mock("./navigation", () => ({ employeeNavigation: () => navigation }));
vi.mock("vue-router", () => ({ useRouter: () => ({ push }) }));
vi.mock("./use-route-write-lock", () => ({ useRouteWriteLock: vi.fn() }));
let wrapper;
const page = list => ({ list, total: list.length, page: 1, pageSize: 20 });
function button(label) {
  const found = wrapper.findAll("button").find(item => item.text() === label);
  expect(found, label).toBeDefined();
  return found;
}
beforeEach(() => {
  session.authorized.mockReset();
  push.mockReset();
  session.account.user.role = "HR_ADMIN";
});
afterEach(() => {
  wrapper?.unmount();
  vi.restoreAllMocks();
});

it("keeps calendar editing locked after an unknown write until reading the saved version", async () => {
  let calendar = {
    version: 4,
    workingWeekdays: [1, 2, 3, 4, 5],
    exceptions: []
  };
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/leave-types") return { list: [] };
    if (path === "/leave-calendar" && options.method === "PUT") {
      calendar = { ...options.data, version: 5 };
      throw new TypeError("lost response");
    }
    if (path === "/leave-calendar") return calendar;
    throw new Error(path);
  });
  wrapper = mount(WorkflowSettings);
  await flushPromises();
  await button("工作日历").trigger("click");
  await flushPromises();
  await button("编辑工作日历").trigger("click");
  wrapper
    .getComponent(ElDatePicker)
    .vm.$emit("update:modelValue", "2027-01-01");
  await flushPromises();
  await button("添加日期例外").trigger("click");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(button("审批规则").attributes("disabled")).toBeDefined();
  expect(button("取消").attributes("disabled")).toBeDefined();
  expect(
    session.authorized.mock.calls.filter(([, opts]) => opts.method === "PUT")
  ).toHaveLength(1);
  await button("重新读取并核对").trigger("click");
  await flushPromises();
  expect(wrapper.text()).toContain("版本 5");
  expect(wrapper.text()).toContain("2027-01-01");
  expect(button("审批规则").attributes("disabled")).toBeUndefined();
});

it("submits an ordered rule with exact cents and preserves it on a rule conflict", async () => {
  session.authorized.mockImplementation(async (path, options) => {
    if (path === "/leave-types") return { list: [] };
    if (path === "/departments/tree")
      return { list: [{ id: 8, name: "测试部门" }] };
    if (path.startsWith("/approval-rules/assignees"))
      return page([{ id: 2, displayName: "测试审批人" }]);
    if (path.startsWith("/approval-rules?")) return page([]);
    if (path === "/approval-rules" && options.method === "POST")
      throw { status: 409, errorCode: "RULE_CONFLICT" };
    throw new Error(path);
  });
  wrapper = mount(WorkflowSettings);
  await flushPromises();
  await button("审批规则").trigger("click");
  await flushPromises();
  await button("新增审批规则").trigger("click");
  await flushPromises();
  const labels = () => wrapper.findAll("label");
  const field = name =>
    labels()
      .find(item => item.text().startsWith(name))
      .find("input,select");
  await field("规则名称").setValue("报销复核");
  await field("申请类型").setValue("EXPENSE");
  await field("直属部门").setValue("8");
  await field("金额下限").setValue("12.01");
  await field("金额上限").setValue("99.99");
  await field("步骤名称").setValue("负责人复核");
  await field("审批人").setValue("2");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  const write = session.authorized.mock.calls.find(
    ([path]) => path === "/approval-rules"
  );
  expect(write[1].data).toMatchObject({
    departmentId: 8,
    minAmountCents: 1201,
    maxAmountCents: 9999,
    steps: [{ name: "负责人复核", assigneeId: 2 }]
  });
  expect(wrapper.text()).toContain("启用规则有重叠");
  expect(field("规则名称").element.value).toBe("报销复核");
});

it("cancels an approved leave from the authorized summary without fetching private detail", async () => {
  let cancelled = false;
  session.authorized.mockImplementation(async path => {
    if (path.startsWith("/approved-leaves?"))
      return page(
        cancelled
          ? []
          : [
              {
                id: 7,
                applicantName: "测试员工",
                leaveTypeName: "年假",
                startDate: "2026-12-31",
                endDate: "2027-01-01",
                totalHalfDays: 2
              }
            ]
      );
    if (path === "/requests/7/actions") {
      cancelled = true;
      return { id: 7, status: "CANCELLED", title: "" };
    }
    throw new Error(path);
  });
  wrapper = mount(ApprovedLeaves);
  await flushPromises();
  await button("撤销此请假").trigger("click");
  await button("撤销请假").trigger("click");
  await wrapper.get("textarea").setValue("原申请已取消");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(wrapper.text()).toContain("暂无可撤销的已批请假");
  expect(
    session.authorized.mock.calls.some(([path]) => path === "/requests/7")
  ).toBe(false);
  expect(
    session.authorized.mock.calls.find(([path]) => path.endsWith("/actions"))[1]
      .data
  ).toEqual({ action: "CANCEL", reason: "原申请已取消" });
});

it("hands off the future exception using the current task anchor and reconciles an unknown result", async () => {
  let handed = false;
  session.authorized.mockImplementation(async (path, _options) => {
    if (path.startsWith("/workflow-exceptions?"))
      return page(
        handed
          ? []
          : [
              {
                taskId: 9,
                requestId: 7,
                requestType: "GENERAL",
                step: 2,
                stepName: "后续复核",
                assigneeId: 3,
                reason: "FUTURE_UNAVAILABLE",
                version: 4
              }
            ]
      );
    if (path.startsWith("/approval-rules/assignees?"))
      return page([{ id: 8, displayName: "接手人" }]);
    if (path === "/workflow-exceptions/9/handoff") {
      handed = true;
      throw new TypeError("lost response");
    }
    throw new Error(path);
  });
  wrapper = mount(WorkflowExceptions);
  await flushPromises();
  await button("处理异常").trigger("click");
  await flushPromises();
  await wrapper.get("select").setValue("8");
  await wrapper.get("textarea").setValue("修复后续节点");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(button("取消").attributes("disabled")).toBeDefined();
  const write = session.authorized.mock.calls.find(([path]) =>
    path.endsWith("/handoff")
  );
  expect(write[0]).toBe("/workflow-exceptions/9/handoff");
  expect(write[1].data).toEqual({
    version: 4,
    step: 2,
    targetUserId: 8,
    reason: "修复后续节点"
  });
  expect(write[1].idempotencyKey).toBeUndefined();
  await button("重新读取并核对").trigger("click");
  await flushPromises();
  expect(wrapper.text()).toContain("当前没有检测到不可用的审批步骤");
  expect(
    session.authorized.mock.calls.filter(([path]) => path.endsWith("/handoff"))
  ).toHaveLength(1);
});

it("only applies a complete ledger year and never turns keystrokes into requests", async () => {
  session.authorized.mockResolvedValue(page([]));
  wrapper = mount(LeaveLedger, { props: { employeeId: 11 } });
  await flushPromises();
  for (const value of ["2", "20", "202", "2027"])
    await wrapper.get("input").setValue(value);
  expect(session.authorized).toHaveBeenCalledTimes(1);
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(session.authorized.mock.lastCall[0]).toBe(
    "/employees/11/leave-ledger?page=1&pageSize=20&year=2027"
  );
});

it("uses server dashboard counts and does not render admin actions for a viewer", async () => {
  session.account.user.role = "HR_VIEWER";
  session.authorized.mockResolvedValue({
    pendingTasks: 0,
    myRequests: 0,
    contractReminders: 4,
    workflowExceptions: null,
    approvedLeaves: null
  });
  wrapper = mount(Dashboard);
  await flushPromises();
  expect(wrapper.text()).toContain("合同到期");
  expect(wrapper.find(".el-statistic__number").text()).toBe("4");
  expect(wrapper.text()).not.toContain("审批异常");
  expect(wrapper.text()).not.toContain("请假与审批配置");
  await button("查看合同到期").trigger("click");
  expect(push).toHaveBeenCalledWith("/contract-reminders");
});
