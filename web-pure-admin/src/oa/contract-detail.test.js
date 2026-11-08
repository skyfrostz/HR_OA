import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { ElDatePicker, ElMessageBox } from "element-plus";
import ContractDetail from "./contract-detail.vue";
import ContractLedger from "./contract-ledger.vue";
import { findCreatedContract } from "./use-contract-record";

const { session } = vi.hoisted(() => ({
  session: {
    canWrite: true,
    account: { user: { role: "HR_ADMIN" } },
    authorized: vi.fn()
  }
}));
vi.mock("./session", () => ({ useSession: () => session }));
let wrapper;
const page = (list, current = 1, total = list.length) => ({
  list,
  page: current,
  pageSize: 100,
  total
});
const contract = (id = 21, overrides = {}) => ({
  id,
  employeeId: 11,
  contractNo: `SIM-C${id}`,
  status: "DRAFT",
  version: 1,
  startDate: "2026-01-01",
  endDate: "2026-12-31",
  files: [],
  ...overrides
});
const file = {
  id: 7,
  version: 1,
  classification: "RESTRICTED",
  originalName: "合成附件.pdf"
};
function button(text) {
  return wrapper.findAll("button").find(item => item.text() === text);
}
function mountDetail(props = {}) {
  wrapper = mount(ContractDetail, {
    attachTo: document.body,
    props: {
      contractId: 21,
      expectedEmployeeId: 11,
      onSelect: id => wrapper.setProps({ contractId: id }),
      ...props
    },
    global: { stubs: { AuditHistory: true } }
  });
}
async function renewInput() {
  await button("续约").trigger("click");
  await wrapper.get("#contract-number").setValue("SIM-RENEW");
  const dates = wrapper.findAllComponents(ElDatePicker);
  dates[0].vm.$emit("update:modelValue", "2027-01-01");
  dates[1].vm.$emit("update:modelValue", "2027-12-31");
  await wrapper.get("textarea").setValue("合成续约原因");
}
beforeEach(() => {
  session.authorized.mockReset();
  session.canWrite = true;
  session.account.user.role = "HR_ADMIN";
});
afterEach(() => {
  wrapper?.unmount();
  wrapper = undefined;
  document.body.innerHTML = "";
  vi.restoreAllMocks();
});

it("opens only the target contract and never moves focus when the read completes", async () => {
  const anchor = document.createElement("button");
  document.body.append(anchor);
  anchor.focus();
  session.authorized.mockResolvedValue(contract());
  mountDetail();
  await flushPromises();
  expect(session.authorized.mock.calls.map(([path]) => path)).toEqual([
    "/contracts/21"
  ]);
  expect(wrapper.get("#oa-focused-contract").text()).toBe("SIM-C21");
  expect(document.activeElement).toBe(anchor);
});

it("cancels old reads and rejects out of order responses after changing contract", async () => {
  const reads = [];
  session.authorized.mockImplementation(
    (path, { signal }) =>
      new Promise(resolve => reads.push({ path, signal, resolve }))
  );
  mountDetail();
  await wrapper.setProps({ contractId: 22 });
  expect(reads[0].signal.aborted).toBe(true);
  reads[1].resolve(contract(22));
  await flushPromises();
  reads[0].resolve(contract(21));
  await flushPromises();
  expect(wrapper.text()).toContain("SIM-C22");
  expect(wrapper.text()).not.toContain("SIM-C21");
  expect(wrapper.emitted("loaded")).toHaveLength(1);
});

it("cancels at close, preserves settled exit content, and refreshes a reopened target", async () => {
  const reads = [];
  session.authorized.mockImplementation(
    (path, { signal }) =>
      new Promise(resolve => reads.push({ signal, resolve }))
  );
  mountDetail();
  await wrapper.setProps({ active: false });
  expect(reads[0].signal.aborted).toBe(true);
  reads[0].resolve(contract());
  await flushPromises();
  expect(wrapper.find("#oa-focused-contract").exists()).toBe(false);
  await wrapper.setProps({ active: true });
  reads[1].resolve(contract());
  await flushPromises();
  await wrapper.setProps({ active: false });
  expect(wrapper.get("#oa-focused-contract").text()).toBe("SIM-C21");
  expect(wrapper.get("section").attributes("inert")).toBeDefined();
  await wrapper.setProps({ active: true });
  expect(wrapper.find("#oa-focused-contract").exists()).toBe(false);
  reads[2].resolve(contract(21, { version: 2 }));
  await flushPromises();
  expect(wrapper.text()).toContain("SIM-C21");
});

it.each([
  [403, "无权查看"],
  [404, "不存在或已不可访问"],
  [500, "读取失败"],
  [0, "检查网络"]
])(
  "clears the previous contract and distinguishes read error %s",
  async (status, message) => {
    session.authorized
      .mockResolvedValueOnce(contract())
      .mockRejectedValueOnce({ status });
    mountDetail();
    await flushPromises();
    await wrapper.setProps({ contractId: 22 });
    await flushPromises();
    expect(wrapper.text()).toContain(message);
    expect(wrapper.text()).not.toContain("SIM-C21");
    expect(button("重新加载")).toBeDefined();
  }
);

it("does not expose a response for another employee", async () => {
  session.authorized.mockResolvedValue(contract(21, { employeeId: 99 }));
  mountDetail();
  await flushPromises();
  expect(wrapper.text()).toContain("与当前员工不匹配");
  expect(wrapper.text()).not.toContain("SIM-C21");
  expect(wrapper.emitted("loaded")).toBeUndefined();
});

it("retains permitted files but omits write actions for a reader", async () => {
  session.canWrite = false;
  session.account.user.role = "HR_VIEWER";
  session.authorized.mockResolvedValue(
    contract(21, { files: [{ ...file, classification: "STANDARD" }] })
  );
  mountDetail();
  await flushPromises();
  expect(wrapper.text()).toContain("合成附件.pdf");
  expect(button("下载")).toBeDefined();
  expect(button("续约")).toBeUndefined();
  expect(button("访问范围")).toBeUndefined();
});

it("keeps edit input on a rejected write and honors cancel versus discard confirmation", async () => {
  session.authorized.mockImplementation((_, options) =>
    options?.method
      ? Promise.reject({ status: 422, message: "日期不合法" })
      : Promise.resolve(contract())
  );
  mountDetail();
  await flushPromises();
  await button("编辑草稿").trigger("click");
  await wrapper.get("#contract-number").setValue("用户尚未保存");
  await wrapper.get("textarea").setValue("合成原因");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(wrapper.get("#contract-number").element.value).toBe("用户尚未保存");
  expect(wrapper.text()).toContain("日期不合法");
  expect(wrapper.findComponent({ name: "AuditHistory" }).exists()).toBe(false);
  const confirm = vi
    .spyOn(ElMessageBox, "confirm")
    .mockRejectedValueOnce("cancel")
    .mockResolvedValueOnce("confirm");
  expect(await wrapper.vm.requestClose()).toBe(false);
  expect(wrapper.get("#contract-number").element.value).toBe("用户尚未保存");
  expect(await wrapper.vm.requestClose()).toBe(true);
  expect(confirm).toHaveBeenCalledTimes(2);
});

it("prevents close and duplicate writes during submission, then reads only the saved target", async () => {
  let finish;
  session.authorized.mockImplementation((_, options) =>
    options?.method
      ? new Promise(resolve => {
          finish = resolve;
        })
      : Promise.resolve(contract())
  );
  mountDetail();
  await flushPromises();
  await button("编辑草稿").trigger("click");
  await wrapper.get("textarea").setValue("合成保存");
  await wrapper.get("form").trigger("submit");
  await wrapper.get("form").trigger("submit");
  expect(await wrapper.vm.requestClose()).toBe(false);
  expect(
    session.authorized.mock.calls.filter(([, options]) => options?.method)
  ).toHaveLength(1);
  finish(contract(21, { version: 2 }));
  await flushPromises();
  expect(wrapper.find("form").exists()).toBe(false);
  expect(
    session.authorized.mock.calls
      .filter(([, options]) => !options?.method)
      .map(([path]) => path)
  ).toEqual(["/contracts/21", "/contracts/21"]);
});

it("preserves a version-conflicted draft until an explicit target-only reread", async () => {
  session.authorized.mockImplementation((_, options) =>
    options?.method
      ? Promise.reject({ status: 409, errorCode: "STALE_RECORD" })
      : Promise.resolve(contract(21, { version: 2 }))
  );
  mountDetail();
  await flushPromises();
  await button("编辑草稿").trigger("click");
  await wrapper.get("#contract-number").setValue("CONFLICT-INPUT");
  await wrapper.get("textarea").setValue("保留冲突原因");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(wrapper.get("#contract-number").element.value).toBe("CONFLICT-INPUT");
  expect(wrapper.get("textarea").element.value).toBe("保留冲突原因");
  expect(wrapper.get("#contract-number").element.disabled).toBe(true);
  await button("重新读取合同").trigger("click");
  await flushPromises();
  expect(wrapper.find("form").exists()).toBe(false);
  expect(
    session.authorized.mock.calls.filter(([, options]) => options?.method)
  ).toHaveLength(1);
  expect(
    session.authorized.mock.calls
      .filter(([, options]) => !options?.method)
      .map(([path]) => path)
  ).toEqual(["/contracts/21", "/contracts/21"]);
});

it("creates a linked renewal draft and selects only its returned id", async () => {
  const original = contract(21, { status: "SIGNED", files: [file] });
  const renewed = contract(22, {
    contractNo: "SIM-RENEW",
    previousContractId: 21
  });
  session.authorized.mockImplementation((path, options) =>
    Promise.resolve(
      options?.method ? renewed : path === "/contracts/22" ? renewed : original
    )
  );
  mountDetail();
  await flushPromises();
  await renewInput();
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(session.authorized).toHaveBeenCalledWith(
    "/employees/11/contracts",
    expect.objectContaining({
      method: "POST",
      data: expect.objectContaining({ status: "DRAFT", previousContractId: 21 })
    })
  );
  expect(wrapper.emitted("select")).toEqual([[22]]);
  expect(wrapper.emitted("changed")[0][0]).toMatchObject({
    contractId: 22,
    employeeId: 11,
    kind: "renew",
    contract: renewed
  });
  expect(original.files).toEqual([file]);
  expect(session.authorized.mock.calls.map(([path]) => path)).toEqual([
    "/contracts/21",
    "/employees/11/contracts",
    "/contracts/22"
  ]);
});

it.each(["not found", "read failure", "wrong relation", "wrong dates"])(
  "retains an unknown renewal without repeating POST when explicit reconciliation is %s",
  async outcome => {
    session.authorized.mockImplementation((path, options) => {
      if (options?.method === "POST") return Promise.reject({ status: 0 });
      if (path.startsWith("/employees/11/contracts?")) {
        if (outcome === "read failure")
          return Promise.reject(new Error("读取失败"));
        return Promise.resolve(
          page(
            ["wrong relation", "wrong dates"].includes(outcome)
              ? [contract(22, { contractNo: "SIM-RENEW" })]
              : []
          )
        );
      }
      return Promise.resolve(
        path === "/contracts/22"
          ? contract(22, {
              contractNo: "SIM-RENEW",
              previousContractId: outcome === "wrong relation" ? 99 : 21
            })
          : contract()
      );
    });
    mountDetail();
    await flushPromises();
    await renewInput();
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(
      session.authorized.mock.calls.some(([path]) => path.includes("page="))
    ).toBe(false);
    await button("重新读取合同").trigger("click");
    await flushPromises();
    expect(wrapper.get("#contract-number").element.value).toBe("SIM-RENEW");
    expect(wrapper.get("#contract-number").element.disabled).toBe(true);
    expect(button("重新读取合同")).toBeDefined();
    expect(
      session.authorized.mock.calls.filter(
        ([, options]) => options?.method === "POST"
      )
    ).toHaveLength(1);
    expect(wrapper.emitted("changed")).toBeUndefined();
  }
);

it("stops explicit creation reconciliation at the exact number and validates the target relation", async () => {
  const renewed = contract(24, {
    contractNo: "EXPECTED",
    previousContractId: 21
  });
  const read = vi
    .fn()
    .mockResolvedValueOnce(page([contract(22)], 1, 300))
    .mockResolvedValueOnce(page([renewed], 2, 300))
    .mockResolvedValueOnce(renewed);
  expect(
    await findCreatedContract(read, {
      employeeId: 11,
      contractNo: "EXPECTED",
      startDate: renewed.startDate,
      endDate: renewed.endDate,
      previousContractId: 21
    })
  ).toEqual(renewed);
  expect(read.mock.calls.map(([path]) => path)).toEqual([
    "/employees/11/contracts?page=1&pageSize=100",
    "/employees/11/contracts?page=2&pageSize=100",
    "/contracts/24"
  ]);
});

it.each(["attachment", "classification"])(
  "reports dirty and busy state for %s operations",
  async kind => {
    let failWrite;
    session.authorized.mockImplementation((_, options) =>
      options?.method
        ? new Promise((_, reject) => {
            failWrite = reject;
          })
        : Promise.resolve(contract(21, { files: [file] }))
    );
    mountDetail();
    await flushPromises();
    if (kind === "attachment") {
      await button("添加附件").trigger("click");
      const input = wrapper.get("input[type=file]");
      Object.defineProperty(input.element, "files", {
        value: [new File(["test"], "合成.pdf", { type: "application/pdf" })],
        configurable: true
      });
      await input.trigger("change");
    } else {
      await button("访问范围").trigger("click");
      await wrapper.get("#oa-file-reason").setValue("合成范围原因");
    }
    expect(wrapper.emitted("state-change").at(-1)[0]).toMatchObject({
      editing: true,
      dirty: true,
      busy: false
    });
    await wrapper.get("form").trigger("submit");
    expect(await wrapper.vm.requestClose()).toBe(false);
    failWrite({ status: 422, message: "合成拒绝" });
    await flushPromises();
    expect(wrapper.text()).toContain("合成拒绝");
    expect(wrapper.emitted("state-change").at(-1)[0]).toMatchObject({
      dirty: true,
      busy: false
    });
  }
);

it("focuses the employee ledger detail immediately on explicit selection, independently of a slow read", async () => {
  let finish;
  session.authorized.mockImplementation(path =>
    path.startsWith("/employees/11/contracts?")
      ? Promise.resolve(
          page(Array.from({ length: 20 }, (_, index) => contract(index + 21)))
        )
      : new Promise(resolve => {
          finish = resolve;
        })
  );
  wrapper = mount(ContractLedger, {
    attachTo: document.body,
    props: { employeeId: 11 },
    global: { stubs: { AuditHistory: true } }
  });
  await flushPromises();
  const actions = wrapper
    .findAll("button")
    .filter(item => item.text() === "查看合同详情");
  actions.at(-1).element.focus();
  await actions.at(-1).trigger("click");
  expect(document.activeElement).toBe(
    wrapper.get(".oa-selected-contract").element
  );
  expect(wrapper.find("#oa-focused-contract").exists()).toBe(false);
  finish(contract(40));
  await flushPromises();
  expect(wrapper.get("#oa-focused-contract").text()).toBe("SIM-C40");
  expect(document.activeElement).toBe(
    wrapper.get(".oa-selected-contract").element
  );
});

it("reconciles an unknown renewal only after explicit read and selects a matching record without another POST", async () => {
  const renewed = contract(22, {
    contractNo: "SIM-RENEW",
    previousContractId: 21,
    startDate: "2027-01-01",
    endDate: "2027-12-31"
  });
  session.authorized.mockImplementation((path, options) => {
    if (options?.method === "POST") return Promise.reject({ status: 0 });
    if (path.includes("pageSize=100")) return Promise.resolve(page([renewed]));
    return Promise.resolve(path === "/contracts/22" ? renewed : contract());
  });
  mountDetail();
  await flushPromises();
  await renewInput();
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  expect(
    session.authorized.mock.calls.some(([path]) =>
      path.includes("pageSize=100")
    )
  ).toBe(false);
  await button("重新读取合同").trigger("click");
  await flushPromises();
  expect(wrapper.emitted("select")).toEqual([[22]]);
  expect(wrapper.get("#oa-focused-contract").text()).toBe("SIM-RENEW");
  expect(
    session.authorized.mock.calls.filter(
      ([, options]) => options?.method === "POST"
    )
  ).toHaveLength(1);
});

it("retains an unknown employee-ledger creation when an existing same number has different dates", async () => {
  session.authorized.mockImplementation((path, options) => {
    if (options?.method === "POST") return Promise.reject({ status: 0 });
    if (path.includes("pageSize=100"))
      return Promise.resolve(page([contract(22, { contractNo: "CREATION" })]));
    if (path === "/contracts/22")
      return Promise.resolve(contract(22, { contractNo: "CREATION" }));
    return Promise.resolve(page([]));
  });
  wrapper = mount(ContractLedger, {
    attachTo: document.body,
    props: { employeeId: 11 },
    global: { stubs: { AuditHistory: true } }
  });
  await flushPromises();
  await button("新建合同").trigger("click");
  await wrapper.get("#contract-number").setValue("CREATION");
  const dates = wrapper.findAllComponents(ElDatePicker);
  dates[0].vm.$emit("update:modelValue", "2027-01-01");
  dates[1].vm.$emit("update:modelValue", "2027-12-31");
  await wrapper.get("textarea").setValue("合成新建原因");
  await wrapper.get("form").trigger("submit");
  await flushPromises();
  await button("重新读取合同").trigger("click");
  await flushPromises();
  expect(wrapper.text()).toContain("合同期限不一致");
  expect(wrapper.get("#contract-number").element.value).toBe("CREATION");
  expect(wrapper.get("#contract-number").element.disabled).toBe(true);
  expect(wrapper.emitted("changed")).toBeUndefined();
});

it("drops a queued edit when the user changes to view another contract and comes back", async () => {
  const reads = [];
  session.authorized.mockImplementation(path =>
    path.includes("/employees/11/contracts?")
      ? Promise.resolve(page([contract(21), contract(22)]))
      : new Promise(resolve => reads.push({ path, resolve }))
  );
  wrapper = mount(ContractLedger, {
    attachTo: document.body,
    props: { employeeId: 11 },
    global: { stubs: { AuditHistory: true } }
  });
  await flushPromises();
  await wrapper
    .findAll("button")
    .find(item => item.text() === "编辑草稿")
    .trigger("click");
  await button("查看合同详情").trigger("click");
  await button("查看合同详情").trigger("click");
  expect(reads.map(item => item.path)).toEqual([
    "/contracts/21",
    "/contracts/22",
    "/contracts/21"
  ]);
  reads[2].resolve(contract(21));
  await flushPromises();
  reads[0].resolve(contract(21));
  reads[1].resolve(contract(22));
  await flushPromises();
  expect(wrapper.find("form").exists()).toBe(false);
  expect(wrapper.get("#oa-focused-contract").text()).toBe("SIM-C21");
});
