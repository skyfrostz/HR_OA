import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import EmployeeWorkspace from "./employee-workspace.vue";
const { session, viewport } = vi.hoisted(() => ({
  viewport: { narrow: true },
  session: {
    canWrite: false,
    canReadSalary: true,
    account: { user: { role: "HR_VIEWER" }, permissions: {} },
    authorized: vi.fn()
  }
}));
vi.mock("./session", () => ({ useSession: () => session }));
vi.mock("./navigation", () => ({
  employeeNavigation: () => ({ employeeId: null, contractId: null })
}));
vi.mock("./use-route-write-lock", () => ({ useRouteWriteLock: vi.fn() }));
vi.mock("@vueuse/core", async original => ({
  ...(await original()),
  useMediaQuery: () => ({ value: viewport.narrow, __v_isRef: true })
}));
vi.mock("@/components/RePureTableBar", () => ({
  PureTableBar: {
    props: ["columns"],
    methods: { exitFullscreen() {} },
    template: '<div><slot :dynamicColumns="columns" size="default" /></div>'
  }
}));
let wrapper;
beforeEach(() => {
  session.authorized.mockReset();
  session.canWrite = false;
  viewport.narrow = true;
});
afterEach(() => {
  wrapper?.unmount();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});
it.each([true, false])(
  "keeps mobile facts and authorized salary readable, salary permission=%s",
  async canReadSalary => {
    session.canReadSalary = canReadSalary;
    session.authorized.mockImplementation(async path =>
      path === "/departments/tree"
        ? { list: [] }
        : {
            total: 1,
            page: 1,
            pageSize: 20,
            list: [
              {
                id: 11,
                employeeNo: "SIM-011",
                name: "长中文员工姓名用于布局检查",
                department: "长中文组织部门名称",
                tenureYears: 3,
                salaryCents: 601200,
                employmentStatus: "ACTIVE"
              }
            ]
          }
    );
    wrapper = mount(EmployeeWorkspace, {
      global: { stubs: { ElDrawer: true } }
    });
    await flushPromises();
    const list = wrapper.get('ul[aria-label="员工列表"]');
    expect(list.text()).toContain("长中文员工姓名用于布局检查");
    expect(list.text()).toContain("SIM-011");
    expect(list.text()).toContain("长中文组织部门名称");
    expect(list.text()).toContain("3 年");
    expect(list.text()).toContain("在职");
    expect(list.text().includes("6,012.00")).toBe(canReadSalary);
    expect(list.get("button").text()).toBe("查看档案");
    expect(list.get("button").attributes("aria-describedby")).toBe(
      "oa-employee-name-11"
    );
    expect(wrapper.find(".oa-org-tree").exists()).toBe(false);
  }
);

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
const page = list => ({ total: list.length, page: 1, pageSize: 20, list });
function button(label) {
  return wrapper.findAll("button").find(item => item.text() === label);
}
async function saveAndCloseEmployee() {
  let searchCount = 0;
  let finishList;
  session.canWrite = true;
  session.authorized.mockImplementation((path, options) => {
    if (path === "/departments/tree") return Promise.resolve({ list: [] });
    if (path === "/employees/search") {
      if (++searchCount === 1) return Promise.resolve(page([employee]));
      return new Promise(resolve => (finishList = resolve));
    }
    if (path === "/employees/11")
      return Promise.resolve({
        ...employee,
        version: options?.method === "PUT" ? 2 : 1
      });
    throw new Error(`Unexpected request: ${path}`);
  });
  wrapper = mount(EmployeeWorkspace, {
    attachTo: document.body,
    global: {
      stubs: {
        ElDrawer: {
          props: ["modelValue"],
          emits: ["closed"],
          watch: {
            modelValue(open) {
              if (!open) this.$nextTick(() => this.$emit("closed"));
            }
          },
          template:
            '<div v-if="modelValue" class="oa-employee-drawer"><slot /></div>'
        },
        PureTable: {
          props: ["data"],
          template:
            '<div><template v-for="row in data"><slot name="actions" :row="row" /></template></div>'
        },
        ContractLedger: true,
        EmployeeAccount: true,
        WeComBinding: true,
        AnnualLeave: true,
        AuditHistory: true
      }
    }
  });
  await flushPromises();
  const original = wrapper.get('button[data-employee-id="11"]').element;
  original.focus();
  await button("查看档案").trigger("click");
  await flushPromises();
  await button("编辑员工").trigger("click");
  await wrapper.get(".oa-inline-form form").trigger("submit");
  await flushPromises();
  expect(session.authorized).toHaveBeenCalledWith(
    "/employees/11",
    expect.objectContaining({ method: "PUT" })
  );
  expect(original.isConnected).toBe(false);
  await button("返回员工列表").trigger("click");
  await flushPromises();
  return { original, finishList };
}

it.each([false, true])(
  "restores the same employee action after save, close and delayed list readback, narrow=%s",
  async narrow => {
    viewport.narrow = narrow;
    const { original, finishList } = await saveAndCloseEmployee();
    expect(document.activeElement).not.toBe(
      wrapper.get("#employees-heading").element
    );
    finishList(page([{ ...employee, version: 2 }]));
    await flushPromises();
    const replacement = wrapper.get('button[data-employee-id="11"]').element;
    expect(replacement).not.toBe(original);
    expect(document.activeElement).toBe(replacement);
  }
);

it("returns to the directory heading when the saved employee no longer matches the list", async () => {
  const { finishList } = await saveAndCloseEmployee();
  finishList(page([]));
  await flushPromises();
  expect(document.activeElement).toBe(
    wrapper.get("#employees-heading").element
  );
});

it.each(["new employee", "search focus"])(
  "does not steal focus after delayed readback when the user proceeds to %s",
  async nextTask => {
    const { finishList } = await saveAndCloseEmployee();
    if (nextTask === "new employee") {
      await button("新增员工").trigger("click");
      await flushPromises();
    } else wrapper.get("#oa-employee-search").element.focus();
    const active = document.activeElement;
    expect(active).not.toBe(document.body);
    finishList(page([{ ...employee, version: 2 }]));
    await flushPromises();
    expect(document.activeElement).toBe(active);
  }
);
