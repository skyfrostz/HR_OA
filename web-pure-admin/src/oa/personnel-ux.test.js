import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { ElDatePicker } from "element-plus";
import AuditHistory from "./audit-history.vue";
import ContractForm from "./contract-form.vue";
import EmployeeDetail from "./employee-detail.vue";

const { session, routing, navigation } = vi.hoisted(() => ({
  session: {
    canWrite: true,
    canReadSalary: true,
    account: {
      user: { role: "HR_ADMIN" },
      permissions: { canManageDepartments: false }
    },
    authorized: vi.fn()
  },
  routing: { current: null },
  navigation: { employeeId: null, contractId: null }
}));
vi.mock("./session", () => ({ useSession: () => session }));
vi.mock("./navigation", () => ({ employeeNavigation: () => navigation }));
vi.mock("vue-router", async original => ({
  ...(await original()),
  useRouter: () => routing.current
}));
let wrapper;
const page = list => ({ list, page: 1, pageSize: 20, total: list.length });
function button(label) {
  return wrapper.findAll("button").find(x => x.text() === label);
}
beforeEach(() => {
  session.authorized.mockReset();
  session.canWrite = true;
  session.canReadSalary = true;
  session.account.user.role = "HR_ADMIN";
  navigation.employeeId = navigation.contractId = null;
});
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

const employee = {
  id: 11,
  employeeNo: "SIM-011",
  name: "合成员工",
  department: "人事部",
  birthDate: "1990-01-01",
  hireDate: "2020-01-01",
  salaryCents: 600000,
  age: 36,
  tenureYears: 6,
  employmentStatus: "ACTIVE",
  version: 1
};
function mountEmployeeDetail() {
  wrapper = mount(EmployeeDetail, {
    attachTo: document.body,
    props: { employeeId: 11 },
    global: {
      stubs: {
        ContractLedger: true,
        EmployeeAccount: true,
        WeComBinding: true,
        AnnualLeave: true,
        AuditHistory: true
      }
    }
  });
}

it("returns save focus to the enabled employee edit action after delayed readback", async () => {
  let finishReadback;
  session.authorized
    .mockResolvedValueOnce(employee)
    .mockResolvedValueOnce({ ...employee, version: 2 })
    .mockImplementationOnce(
      () => new Promise(resolve => (finishReadback = resolve))
    );
  mountEmployeeDetail();
  await flushPromises();
  await button("编辑员工").trigger("click");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(session.authorized).toHaveBeenCalledWith(
    "/employees/11",
    expect.objectContaining({ method: "PUT" })
  );
  expect(button("编辑员工").element.disabled).toBe(true);
  finishReadback({ ...employee, version: 2 });
  await flushPromises();
  expect(button("编辑员工").element.disabled).toBe(false);
  expect(document.activeElement).toBe(button("编辑员工").element);
});

it.each(["readback failure", "lost write permission"])(
  "returns save focus to the selected business entry after %s",
  async scenario => {
    session.authorized
      .mockResolvedValueOnce(employee)
      .mockResolvedValueOnce({ ...employee, version: 2 })
      .mockImplementationOnce(() => {
        if (scenario === "readback failure")
          return Promise.reject(new Error("读取暂时失败"));
        session.canWrite = false;
        session.account.user.role = "READ_ONLY";
        return Promise.resolve({ ...employee, version: 2 });
      });
    mountEmployeeDetail();
    await flushPromises();
    await button("年假记录").trigger("click");
    await button("编辑员工").trigger("click");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(document.activeElement).toBe(button("年假记录").element);
    expect(button("年假记录").attributes("aria-pressed")).toBe("true");
    expect(button("年假记录").element.disabled).toBe(false);
    if (scenario === "readback failure")
      expect(wrapper.text()).toContain("读取暂时失败");
    else expect(button("编辑员工")).toBeUndefined();
  }
);

it("returns cancel focus directly to employee edit without saving or refetching", async () => {
  session.authorized.mockResolvedValue(employee);
  mountEmployeeDetail();
  await flushPromises();
  await button("编辑员工").trigger("click");
  await button("取消").trigger("click");
  await flushPromises();
  expect(document.activeElement).toBe(button("编辑员工").element);
  expect(session.authorized).toHaveBeenCalledTimes(1);
});

it.each(["business group", "keyboard group", "return", "drawer closing"])(
  "does not reclaim employee focus after a slow save readback when the user chooses %s",
  async nextTask => {
    let finishReadback;
    session.authorized
      .mockResolvedValueOnce(employee)
      .mockResolvedValueOnce({ ...employee, version: 2 })
      .mockImplementationOnce(
        () => new Promise(resolve => (finishReadback = resolve))
      );
    mountEmployeeDetail();
    await flushPromises();
    await button("编辑员工").trigger("click");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    if (nextTask === "drawer closing")
      await wrapper.setProps({ active: false });
    else {
      const next = button(nextTask === "return" ? "返回员工列表" : "年假记录");
      next.element.focus();
      if (nextTask !== "keyboard group") await next.trigger("click");
    }
    const active = document.activeElement;
    finishReadback({ ...employee, version: 2 });
    await flushPromises();
    expect(document.activeElement).toBe(active);
    expect(document.activeElement).not.toBe(button("编辑员工").element);
  }
);

it("summarizes only changed audit fields and retains unchanged fields in the complete record", async () => {
  session.authorized.mockResolvedValue(
    page([
      {
        id: 1,
        actorId: 1,
        type: "CONTRACT",
        action: "TERMINATE",
        createdAt: "2026-11-08T00:00:00Z",
        reason: "模拟终止",
        before: { contractNo: "SIM-021", status: "SIGNED", version: 2 },
        after: { contractNo: "SIM-021", status: "TERMINATED", version: 3 }
      }
    ])
  );
  wrapper = mount(AuditHistory, { props: { contractId: 21 } });
  await flushPromises();
  const event = wrapper.get(".oa-audit-timeline > li");
  const summary = event.findAll(":scope > dl")[0];
  expect(summary.text()).not.toContain("SIM-021");
  expect(summary.text()).toContain("已签约");
  expect(summary.text()).toContain("已终止");
  expect(event.get("details").text()).toContain("SIM-021");
  expect(event.get("details").attributes("open")).toBeUndefined();
});

it("does not reveal salary audit events to a reader lacking salary permission", async () => {
  session.canReadSalary = false;
  session.authorized.mockResolvedValue(
    page([
      {
        id: 1,
        type: "SALARY",
        action: "UPDATE",
        createdAt: "2026-11-08T00:00:00Z",
        before: { salaryCents: 123456 },
        after: { salaryCents: 987654 }
      }
    ])
  );
  wrapper = mount(AuditHistory);
  await flushPromises();
  expect(wrapper.text()).not.toContain("9876");
  expect(wrapper.find(".oa-audit-timeline").exists()).toBe(false);
});

it("renewal creates an associated draft instead of mutating the original contract or files", async () => {
  const contract = {
    id: 21,
    contractNo: "SIM-021",
    status: "SIGNED",
    version: 4,
    files: [{ id: 9 }]
  };
  session.authorized.mockResolvedValue({ id: 22 });
  wrapper = mount(ContractForm, {
    props: {
      path: "/employees/11/contracts",
      kind: "renew",
      contract,
      refresh: vi.fn()
    }
  });
  await wrapper.get("#contract-number").setValue("SIM-022");
  wrapper
    .findAllComponents(ElDatePicker)[0]
    .vm.$emit("update:modelValue", "2027-01-01");
  wrapper
    .findAllComponents(ElDatePicker)[1]
    .vm.$emit("update:modelValue", "2027-12-31");
  await wrapper.get("textarea").setValue("模拟续约");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(session.authorized).toHaveBeenCalledWith(
    "/employees/11/contracts",
    expect.objectContaining({
      method: "POST",
      data: {
        contractNo: "SIM-022",
        startDate: "2027-01-01",
        endDate: "2027-12-31",
        reason: "模拟续约",
        status: "DRAFT",
        previousContractId: 21
      }
    })
  );
  expect(contract).toEqual({
    id: 21,
    contractNo: "SIM-021",
    status: "SIGNED",
    version: 4,
    files: [{ id: 9 }]
  });
});
