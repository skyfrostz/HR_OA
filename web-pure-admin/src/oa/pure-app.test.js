import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { flushPromises } from "@vue/test-utils";
import { createOaApp } from "../bootstrap";
import router from "../router";
import { useUserStoreHook } from "../store/modules/user";

const employee = {
  id: 11, employeeNo: "SIM-011", name: "合成员工", department: "合成部门",
  birthDate: "1995-06-01", hireDate: "2021-07-01", age: 31, tenureYears: 5
};
const salaryCents = 600009;
const contract = {
  id: 21, employeeId: 11, contractNo: "SIM-C21", startDate: "2025-01-01",
  endDate: "2027-12-31", status: "SIGNED", previousContractId: null, files: []
};

const json = (data, status = 200) => new Response(JSON.stringify({ code: status < 400 ? 0 : 7, data }), {
  status, headers: { "Content-Type": "application/json" }
});
const page = list => ({ list, total: list.length, page: 1, pageSize: 20 });

async function settle() {
  await flushPromises();
  await new Promise(resolve => setTimeout(resolve, 360));
  await flushPromises();
}

function input(id, value) {
  const field = document.querySelector("#" + id);
  expect(field).not.toBeNull();
  field.value = value;
  field.dispatchEvent(new Event("input", { bubbles: true }));
}

function button(label) {
  const control = [...document.querySelectorAll("button")].find(
    element => element.textContent.trim() === label
  );
  expect(control, label).toBeDefined();
  return control;
}

async function login() {
  input("oa-username", "fixture");
  input("oa-password", "fixture-password");
  await flushPromises();
  document.querySelector("form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  // The first navigation compiles both lazy-loaded Pure layout and employee page in Vitest.
  await vi.waitFor(() => {
    expect(router.currentRoute.value.name).toBe("OaEmployees");
    expect(document.querySelector(".sidebar-container")).not.toBeNull();
    expect(document.querySelector("#employees-heading")).not.toBeNull();
    expect(document.querySelector('table[aria-label="员工列表"]')).not.toBeNull();
  }, { timeout: 8_000 });
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  vi.stubGlobal("matchMedia", vi.fn(() => ({
    matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}
  })));
  vi.stubGlobal("ResizeObserver", class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("mounts the Pure layout, renders the allowed menus and enforces the memory session across roles", async () => {
  const appRoot = document.createElement("div");
  document.body.append(appRoot);
  const probe = "fixture-token-pure";
  let role = "HR_VIEWER";
  let expired = false;
  const fetchMock = vi.fn(async (url, options) => {
    if (url.endsWith("/auth/login")) {
      return json({
        token: probe, expiresAt: new Date(Date.now() + 60_000).toISOString(),
        user: { id: 9, username: "fixture", displayName: "合成账号", role, employeeId: 11 },
        permissions: { canWriteEmployees: role === "HR_ADMIN", canReadSalary: role !== "HR_VIEWER" }
      });
    }
    if (expired) return json({ errorCode: "UNAUTHORIZED" }, 401);
    const row = role === "HR_VIEWER" ? employee : { ...employee, salaryCents };
    if (url.endsWith("/employees/search")) return json(page([row]));
    if (url.includes("/departments?")) return json(page([]));
    if (url.endsWith("/employees/11")) return json(row);
    if (url.endsWith("/employees/11/contracts?page=1&pageSize=20")) return json(page([contract]));
    return json({ errorCode: "NOT_FOUND" }, 404);
  });
  vi.stubGlobal("fetch", fetchMock);

  const app = createOaApp();
  await router.isReady();
  app.mount(appRoot);
  try {
    await settle();
    expect(router.currentRoute.value.name).toBe("Login");
    expect(router.hasRoute("Welcome")).toBe(false);
    expect(router.hasRoute("AccountSettings")).toBe(false);
    await login();
    expect(router.currentRoute.value.name).toBe("OaEmployees");
    expect(appRoot.querySelector('.sidebar-container a[aria-label="员工档案"]')).not.toBeNull();
    expect(appRoot.querySelector('.sidebar-container a[aria-label="移动审批入口"]')).not.toBeNull();
    expect(appRoot.querySelector("table").textContent).toContain(employee.name);
    expect(appRoot.textContent).not.toContain("约定工资");
    expect(appRoot.textContent).not.toContain("新增员工");

    const employeeRead = fetchMock.mock.calls.find(([url]) => url.endsWith("/employees/search"));
    expect(employeeRead[1].headers.get("x-token")).toBe(probe);
    expect(employeeRead[1]).toMatchObject({ credentials: "omit", cache: "no-store" });
    menu("移动审批入口");
    await settle();
    expect(appRoot.querySelector("a[href='./h5.html']")).not.toBeNull();
    expect(appRoot.querySelector(".oa-mobile-entry.oa-pure.oa-surface")).not.toBeNull();
    expect(appRoot.querySelector(".tags-view").textContent).toContain("移动审批入口");
    expect(activeMenuLabels()).toEqual(["移动审批入口"]);
    tag("员工档案");
    await settle();
    expect(router.currentRoute.value.name).toBe("OaEmployees");
    expect(activeMenuLabels()).toEqual(["员工档案"]);

    button("退出登录").click();
    await settle();
    expect(router.currentRoute.value.name).toBe("Login");
    expect(appRoot.textContent).not.toContain("合成账号");
    role = "APPROVER";
    await login();
    expect(appRoot.textContent).toContain("审批人");
    expect(appRoot.querySelector("#oa-department-filter")).toBeNull();
    expect(appRoot.textContent).toContain("¥6,000.09");
    expect(appRoot.textContent).not.toContain("新增员工");
    button("查看档案").click();
    await settle();
    expect(appRoot.textContent).toContain("SIM-C21");
    expect(appRoot.textContent).not.toContain("新建合同");
    expect(router.currentRoute.value.fullPath).toBe("/employees");

    expired = true;
    button("返回员工列表").click();
    await settle();
    expect(router.currentRoute.value.name).toBe("Login");
    expect(appRoot.textContent).toContain("登录已失效");
    expect(document.cookie).toBe("");
    expect(sessionStorage.length).toBe(0);
    const persisted = Object.entries(localStorage);
    expect(persisted.every(([key]) => key.startsWith("hr-oa-pure-"))).toBe(true);
    for (const value of [probe, "fixture-password", "合成账号", employee.name, employee.employeeNo, "600009"]) {
      expect(JSON.stringify(persisted)).not.toContain(value);
    }
  } finally {
    if (useUserStoreHook().account) useUserStoreHook().logOut();
    await flushPromises();
    app.unmount();
    appRoot.remove();
  }
}, 20_000);

function submit() {
  document.querySelector("form").dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
}

function menu(label) {
  const link = [...document.querySelectorAll(".sidebar-container a")].find(
    element => element.getAttribute("aria-label") === label
  );
  expect(link, label).toBeDefined();
  link.querySelector('[role="menuitem"]').click();
}

function activeMenuLabels() {
  return [...document.querySelectorAll(".sidebar-container .el-menu-item.is-active")].map(
    item => item.closest("a")?.getAttribute("aria-label")
  );
}

function tag(label) {
  const item = [...document.querySelectorAll(".tags-view .tag-title")].find(
    title => title.textContent.trim() === label
  );
  expect(item, label).toBeDefined();
  item.click();
}

function chooseWorkbook(name) {
  const input = document.querySelector("#oa-import-file");
  const file = new File(["fixture-xlsx"], name, {
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
  });
  Object.defineProperty(input, "files", { configurable: true, value: [file] });
  input.dispatchEvent(new Event("change", { bubbles: true }));
}

it("integrates the Pure menus with employee versions, department edits, half-day leave, reminders and Excel replay", async () => {
  const appRoot = document.createElement("div");
  document.body.append(appRoot);
  let row = { ...employee, departmentId: 5, salaryCents, version: 1 };
  let department = { id: 5, name: employee.department, parentId: null, version: 3 };
  let releaseWrite;
  let writeAttempts = 0;
  let leave = [];
  let importAttempts = 0;
  const confirmations = [];
  const downloads = [];
  const originalClick = HTMLAnchorElement.prototype.click;
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function () {
    if (this.href === "blob:fixture-workbook") downloads.push(this.download);
    else originalClick.call(this);
  });
  vi.stubGlobal("URL", class extends URL {
    static createObjectURL = () => "blob:fixture-workbook";
    static revokeObjectURL = vi.fn();
  });
  const validId = "fixture-preview-0123456789";
  const preview = {
    id: validId, status: "PREVIEW", totalRows: 1, validRows: 1, errorRows: 0, canConfirm: true,
    rows: [{
      row: 2, values: { employeeNo: "SIM-X1", name: "导入演示", department: employee.department,
        birthDate: "1995-06-01", hireDate: "2021-07-01", salary: "6000.09" }, errors: []
    }]
  };
  const fetchMock = vi.fn(async (url, options) => {
    const path = url.replace("/api/oa/v1", "");
    if (path === "/auth/login") return json({
      token: "fixture-admin-token", expiresAt: new Date(Date.now() + 60_000).toISOString(),
      user: { id: 3, username: "fixture", displayName: "模拟人事管理员", role: "HR_ADMIN", employeeId: null },
      permissions: {
        canWriteEmployees: true, canReadSalary: true, canManageDepartments: true,
        canManageAnnualLeave: true, canImportEmployees: true, canExportEmployees: true
      }
    });
    if (path === "/employees/export.xlsx" || path === "/employees/import-template.xlsx") {
      return new Response("fixture-workbook", {
        headers: { "Content-Disposition": "attachment; filename=fixture.xlsx" }
      });
    }
    if (path === "/employees/search") return json(page([row]));
    if (path.startsWith("/departments?")) return json(page([department]));
    if (path === "/departments/5" && options.method === "PUT") {
      department = { ...department, ...JSON.parse(options.body), version: 4 };
      return json(department);
    }
    if (path === "/employees/11" && options.method === "PUT") {
      if (++writeAttempts === 1) {
        row = { ...row, version: 2 };
        return new Promise(resolve => { releaseWrite = () => resolve(json({ errorCode: "STALE_RECORD" }, 409)); });
      }
      row = { ...row, ...JSON.parse(options.body), version: 3 };
      return json(row);
    }
    if (path === "/employees/11") return json(row);
    if (path.startsWith("/employees/11/contracts?")) return json(page([contract]));
    if (path.startsWith("/employees/11/annual-leave?")) return json(page(leave));
    if (path.startsWith("/employees/11/annual-leave/")) {
      const data = JSON.parse(options.body);
      leave = [{ id: 16, employeeId: 11, year: Number(path.split("/").pop()), ...data,
        remainingHalfDays: data.entitledHalfDays - data.usedHalfDays, version: 1 }];
      return json(leave[0]);
    }
    if (path.startsWith("/contract-reminders?")) return json(page([{
      contractId: 21, contractNo: "SIM-C21", employeeId: 11, employeeNo: employee.employeeNo,
      employeeName: row.name, endDate: "2026-11-05", daysRemaining: -2, status: "SIGNED"
    }]));
    if (path === "/employee-imports/preview") {
      if (options.body.get("file").name === "invalid.xlsx") {
        return json({
          ...preview, validRows: 0, errorRows: 1, canConfirm: false,
          rows: [{ ...preview.rows[0], errors: [{ field: "employeeNo", message: "工号已存在" }] }]
        }, 201);
      }
      return json(preview, 201);
    }
    if (path === "/employee-imports/" + validId) return json({ ...preview, status: "COMMITTED", canConfirm: false });
    if (path === "/employee-imports/" + validId + "/confirm") {
      confirmations.push({ path, body: options.body });
      if (++importAttempts === 1) throw new TypeError("fixture response loss");
      return json({ previewId: validId, importedCount: 1, employeeIds: [19] });
    }
    return json({ errorCode: "NOT_FOUND" }, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  const app = createOaApp();
  await router.isReady();
  app.mount(appRoot);
  try {
    await settle();
    await login();
    button("按当前筛选导出").click();
    await flushPromises();
    expect(downloads).toEqual(["员工档案.xlsx"]);
    const exportRead = fetchMock.mock.calls.find(([url]) => url.endsWith("/employees/export.xlsx"));
    expect(exportRead[1].headers.get("x-token")).toBe("fixture-admin-token");
    button("查看档案").click();
    await settle();
    button("编辑员工").click();
    await flushPromises();
    input("employee-name", "编辑中的合成员工");
    submit();
    await vi.waitFor(() => expect(releaseWrite).toBeTypeOf("function"));
    menu("部门管理");
    await flushPromises();
    expect(router.currentRoute.value.name).toBe("OaEmployees");
    expect(activeMenuLabels()).toEqual(["员工档案"]);
    releaseWrite();
    await settle();
    expect(appRoot.textContent).toContain("请重新读取后再提交");
    menu("部门管理");
    await flushPromises();
    expect(router.currentRoute.value.name).toBe("OaEmployees");
    expect(activeMenuLabels()).toEqual(["员工档案"]);
    button("重新读取员工").click();
    await settle();
    input("employee-name", "已更新的合成员工");
    submit();
    await settle();
    expect(appRoot.textContent).toContain("员工档案已更新");
    const employeeWrites = fetchMock.mock.calls.filter(([url, options]) =>
      url.endsWith("/employees/11") && options.method === "PUT"
    );
    expect(employeeWrites.map(([, options]) => JSON.parse(options.body).version)).toEqual([1, 2]);

    button("新增年假记录").click();
    await flushPromises();
    const leaveInputs = [...document.querySelectorAll("form input")];
    for (const [field, value] of [[leaveInputs[1], "10.5"], [leaveInputs[2], "2"]]) {
      field.value = value;
      field.dispatchEvent(new Event("input", { bubbles: true }));
    }
    submit();
    await settle();
    const leaveWrite = fetchMock.mock.calls.find(([url, options]) => url.includes("/annual-leave/") && options.method === "PUT");
    expect(JSON.parse(leaveWrite[1].body)).toEqual({ entitledHalfDays: 21, usedHalfDays: 4, note: "", version: 0 });
    expect(appRoot.textContent).toContain("8.5 天");

    menu("部门管理");
    await settle();
    button("编辑部门").click();
    await flushPromises();
    const departmentName = document.querySelector("form input");
    departmentName.value = "合成部门改名";
    departmentName.dispatchEvent(new Event("input", { bubbles: true }));
    menu("员工档案");
    tag("员工档案");
    await flushPromises();
    expect(router.currentRoute.value.name).toBe("OaDepartments");
    expect(activeMenuLabels()).toEqual(["部门管理"]);
    expect(button("刷新").disabled).toBe(true);
    button("刷新").click();
    expect(document.querySelector("form input").value).toBe("合成部门改名");
    submit();
    await settle();
    expect(appRoot.textContent).toContain("合成部门改名");
    const deptWrite = fetchMock.mock.calls.find(([url, options]) => url.endsWith("/departments/5") && options.method === "PUT");
    expect(JSON.parse(deptWrite[1].body)).toEqual({ name: "合成部门改名", parentId: null, version: 3 });

    menu("合同到期提醒");
    await settle();
    expect(appRoot.textContent).toContain("已逾期 2 天");
    button("查看合同").click();
    await settle();
    expect(appRoot.textContent).toContain("SIM-C21");
    expect(router.currentRoute.value.fullPath).toBe("/employees");

    menu("Excel 导入");
    await settle();
    button("下载导入模板").click();
    await flushPromises();
    expect(downloads).toEqual(["员工档案.xlsx", "员工导入模板.xlsx"]);
    chooseWorkbook("invalid.xlsx");
    submit();
    await settle();
    expect(appRoot.textContent).toContain("工号已存在");
    expect(button("确认导入").disabled).toBe(true);
    chooseWorkbook("valid.xlsx");
    submit();
    await settle();
    button("确认导入").click();
    await settle();
    expect(appRoot.textContent).toContain("暂时无法确认导入结果");
    menu("员工档案");
    await flushPromises();
    expect(router.currentRoute.value.name).toBe("OaEmployeeImport");
    const employeeTag = [...appRoot.querySelectorAll(".tags-view .tag-title")].find(
      tag => tag.textContent.trim() === "员工档案"
    );
    expect(employeeTag).toBeDefined();
    employeeTag.click();
    await flushPromises();
    expect(router.currentRoute.value.name).toBe("OaEmployeeImport");
    button("查询预览状态").click();
    await settle();
    expect(appRoot.textContent).toContain("预览已提交");
    button("重试同一预览").click();
    await settle();
    expect(appRoot.textContent).toContain("已导入 1 名员工");
    expect(confirmations).toEqual(Array(2).fill({ path: "/employee-imports/" + validId + "/confirm", body: "{}" }));
    expect(router.currentRoute.value.fullPath).toBe("/employee-import");
    expect(JSON.stringify(Object.entries(localStorage))).not.toContain(validId);

    button("退出登录").click();
    await settle();
    expect(router.currentRoute.value.name).toBe("Login");
  } finally {
    if (useUserStoreHook().account) useUserStoreHook().logOut();
    await flushPromises();
    app.unmount();
    appRoot.remove();
  }
}, 25_000);

it("keeps applied search and export filters in request bodies and ignores superseded results", async () => {
  const appRoot = document.createElement("div");
  document.body.append(appRoot);
  let finishSlow;
  let slowSignal;
  const actualClick = HTMLAnchorElement.prototype.click;
  const filenames = [];
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function () {
    if (this.href === "blob:fixture-filtered-export") filenames.push(this.download);
    else actualClick.call(this);
  });
  vi.stubGlobal("URL", class extends URL {
    static createObjectURL = () => "blob:fixture-filtered-export";
    static revokeObjectURL = vi.fn();
  });
  const fetchMock = vi.fn(async (url, options) => {
    if (url.endsWith("/auth/login")) return json({
      token: "fixture-viewer", expiresAt: new Date(Date.now() + 60_000).toISOString(),
      user: { id: 9, username: "fixture", displayName: "合成账号", role: "HR_VIEWER", employeeId: null },
      permissions: { canWriteEmployees: false, canReadSalary: false, canExportEmployees: true }
    });
    if (url.includes("/departments?")) return json(page([{ id: 5, name: "合成部门", parentId: null, version: 1 }]));
    if (url.endsWith("/employees/export.xlsx")) {
      return new Response("fixture-workbook", { headers: { "Content-Disposition": "attachment; filename=fixture.xlsx" } });
    }
    if (url.endsWith("/employees/search")) {
      const body = JSON.parse(options.body);
      const result = (list, total = 1) => json({ list, total, page: body.page, pageSize: body.pageSize });
      if (body.query === "慢查询") {
        slowSignal = options.signal;
        return new Promise(resolve => {
          finishSlow = () => resolve(result([{ ...employee, name: "旧响应" }]));
        });
      }
      if (body.query === "没有匹配") return result([], 0);
      return result([{ ...employee, name: body.query === "新查询" ? "最新响应" : employee.name }], 101);
    }
    return json({ errorCode: "NOT_FOUND" }, 404);
  });
  vi.stubGlobal("fetch", fetchMock);
  const search = () => fetchMock.mock.calls.filter(([url]) => url.endsWith("/employees/search"));
  const lastBody = () => JSON.parse(search().at(-1)[1].body);
  const select = (id, value) => {
    const field = document.getElementById(id);
    field.value = String(value);
    field.dispatchEvent(new Event("change", { bubbles: true }));
  };
  const query = (value) => input("oa-employee-search", value);

  const app = createOaApp();
  await router.isReady();
  app.mount(appRoot);
  try {
    await vi.waitFor(() => expect(document.getElementById("oa-username")).not.toBeNull(), { timeout: 8_000 });
    await login();
    expect(lastBody()).toEqual({ query: "", departmentId: null, page: 1, pageSize: 20 });
    button("下一页").click();
    await flushPromises();
    expect(lastBody().page).toBe(2);

    select("oa-department-filter", 5);
    query("  合成%_  ");
    await flushPromises();
    expect(lastBody().page).toBe(2);
    submit();
    await flushPromises();
    expect(lastBody()).toEqual({ query: "合成%_", departmentId: 5, page: 1, pageSize: 20 });
    button("下一页").click();
    await flushPromises();
    select("oa-employee-page-size", 50);
    await flushPromises();
    expect(lastBody()).toEqual({ query: "合成%_", departmentId: 5, page: 1, pageSize: 50 });

    query("未应用条件");
    await flushPromises();
    button("按当前筛选导出").click();
    await flushPromises();
    const exportCall = fetchMock.mock.calls.find(([url]) => url.endsWith("/employees/export.xlsx"));
    expect(exportCall[1].method).toBe("POST");
    expect(JSON.parse(exportCall[1].body)).toEqual({ query: "合成%_", departmentId: 5 });
    expect(filenames).toEqual(["员工档案.xlsx"]);
    expect(appRoot.querySelector("table").textContent).not.toContain("约定工资");

    query("慢查询");
    await flushPromises();
    submit();
    await flushPromises();
    query("新查询");
    await flushPromises();
    submit();
    await flushPromises();
    expect(slowSignal.aborted).toBe(true);
    finishSlow();
    await flushPromises();
    expect(appRoot.querySelector("table").textContent).toContain("最新响应");
    expect(appRoot.textContent).not.toContain("旧响应");

    query("没有匹配");
    await flushPromises();
    submit();
    await flushPromises();
    expect(appRoot.textContent).toContain("没有符合当前筛选条件的员工");
    button("清空").click();
    await flushPromises();
    expect(lastBody()).toEqual({ query: "", departmentId: null, page: 1, pageSize: 50 });
    expect(appRoot.querySelector("#oa-employee-search").value).toBe("");
    expect(router.currentRoute.value.fullPath).toBe("/employees");
    expect(JSON.stringify(Object.entries(localStorage))).not.toMatch(/合成%_|未应用条件|慢查询|新查询/);
    expect(sessionStorage.length).toBe(0);
    // Let the download URL's cleanup timer run before restoring the URL mock.
    await new Promise(resolve => setTimeout(resolve, 1_000));
  } finally {
    if (useUserStoreHook().account) useUserStoreHook().logOut();
    await flushPromises();
    app.unmount();
    appRoot.remove();
  }
}, 20_000);
