import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defineComponent } from "vue";
import { mount } from "@vue/test-utils";
import { useEmployeeImport } from "./use-employee-import";
const { session, save } = vi.hoisted(() => ({
  session: {
    account: { permissions: { canImportEmployees: true } },
    authorized: vi.fn()
  },
  save: vi.fn()
}));
vi.mock("./session", () => ({ useSession: () => session }));
vi.mock("./api", () => ({ saveAttachment: save }));
let wrapper, model;
const row = {
  row: 2,
  values: {
    employeeNo: "SIM-1",
    name: "导入测试",
    department: "测试",
    salary: "7213.25"
  },
  errors: []
};
const preview = (milliseconds = 1800000) => ({
  id: "preview-one",
  status: "PREVIEW",
  expiresAt: new Date(Date.now() + milliseconds).toISOString(),
  totalRows: 1,
  validRows: 1,
  errorRows: 0,
  canConfirm: true,
  rows: [row]
});
function boot() {
  wrapper = mount(
    defineComponent({
      setup() {
        model = useEmployeeImport();
        return () => null;
      }
    })
  );
}
beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
  session.authorized.mockReset();
  save.mockReset();
});
afterEach(() => {
  wrapper?.unmount();
  vi.useRealTimers();
  vi.restoreAllMocks();
});
it("expires rows locally, clears source files and refuses stale confirmation", async () => {
  session.authorized.mockResolvedValue(preview(1000));
  boot();
  model.pick(new File(["fixture"], "employees.xlsx"));
  await model.inspect();
  expect(model.selectedFile.value).toBeNull();
  expect(model.preview.value.rows).toHaveLength(1);
  await vi.advanceTimersByTimeAsync(1001);
  expect(model.preview.value.rows).toEqual([]);
  expect(model.preview.value.status).toBe("EXPIRED");
  await model.confirm();
  expect(session.authorized).toHaveBeenCalledTimes(1);
  wrapper.unmount();
  expect(vi.getTimerCount()).toBe(0);
});
it("recovers unknown confirmation through GET result and never repeats a successful write", async () => {
  let committed = false;
  session.authorized.mockImplementation(async path => {
    if (path.endsWith("/preview")) return preview();
    if (path.endsWith("/confirm")) {
      committed = true;
      throw new TypeError("lost response");
    }
    if (path.endsWith("/result"))
      return { previewId: "preview-one", importedCount: 1, employeeIds: [17] };
    return {
      ...preview(),
      status: committed ? "COMMITTED" : "PREVIEW",
      rows: []
    };
  });
  boot();
  model.pick(new File(["fixture"], "employees.xlsx"));
  await model.inspect();
  await model.confirm();
  expect(model.uncertain.value).toBe(true);
  expect(model.locked.value).toBe(true);
  model.clear();
  expect(model.preview.value.id).toBe("preview-one");
  await model.checkStatus();
  expect(model.uncertain.value).toBe(false);
  expect(model.preview.value.rows).toEqual([]);
  expect(model.result.value.importedCount).toBe(1);
  expect(
    session.authorized.mock.calls.filter(([path]) => path.endsWith("/confirm"))
  ).toHaveLength(1);
});
it("cancelled or invalid selection keeps the current preview and status expiry prevents error downloads", async () => {
  session.authorized.mockResolvedValue(preview());
  boot();
  model.pick(new File(["fixture"], "employees.xlsx"));
  await model.inspect();
  await model.pick();
  await model.pick(new File(["bad"], "bad.csv"));
  expect(model.preview.value.id).toBe("preview-one");
  expect(session.authorized).toHaveBeenCalledTimes(1);
  await model.open("preview-one");
  session.authorized.mockRejectedValue({ status: 410 });
  await model.downloadErrors();
  expect(model.preview.value.rows).toEqual([]);
  expect(model.preview.value.status).toBe("EXPIRED");
  expect(save).not.toHaveBeenCalled();
});
it("reads historical committed result without retaining source values and aborts on leaving", async () => {
  session.authorized.mockImplementation(async path =>
    path.endsWith("/result")
      ? { importedCount: 2 }
      : { ...preview(), status: "COMMITTED", rows: [row] }
  );
  boot();
  await model.open("history-one");
  expect(model.result.value.importedCount).toBe(2);
  expect(model.preview.value.rows).toEqual([]);
  const signal = session.authorized.mock.calls[0][1].signal;
  wrapper.unmount();
  expect(signal.aborted).toBe(true);
  expect(model.preview.value).toBeNull();
});
it("retries the same preview and empty body after unknown confirmation, then clears rows", async () => {
  let attempts = 0;
  session.authorized.mockImplementation(async path => {
    if (path.endsWith("/preview")) return preview();
    if (++attempts === 1) throw new TypeError("lost response");
    return { previewId: "preview-one", importedCount: 1, employeeIds: [17] };
  });
  boot();
  model.pick(new File(["fixture"], "employees.xlsx"));
  await model.inspect();
  await model.confirm();
  await model.confirm();
  const calls = session.authorized.mock.calls.filter(([path]) =>
    path.endsWith("/confirm")
  );
  expect(calls).toHaveLength(2);
  expect(calls[0][0]).toBe(calls[1][0]);
  expect(calls.map(([, opts]) => opts.data)).toEqual([{}, {}]);
  expect(model.preview.value.rows).toEqual([]);
});

it("keeps the selected workbook after preview fails so it can be retried", async () => {
  session.authorized
    .mockRejectedValueOnce(new Error("读取失败"))
    .mockResolvedValueOnce(preview());
  boot();
  const file = new File(["fixture"], "employees.xlsx");
  model.pick(file);
  await model.inspect();
  expect(model.selectedFile.value).toBe(file);
  expect(model.preview.value).toBeNull();
  expect(model.error.value).toBe("读取失败");
  await model.inspect();
  expect(session.authorized.mock.calls[1][1].file).toBe(file);
  expect(model.preview.value.id).toBe("preview-one");
  expect(model.selectedFile.value).toBeNull();
});
it("discards the old preview before switching to a valid replacement and blocks confirmation meanwhile", async () => {
  let finishDiscard;
  session.authorized.mockImplementation(path =>
    path.endsWith("/discard")
      ? new Promise(resolve => {
          finishDiscard = resolve;
        })
      : Promise.resolve(preview())
  );
  boot();
  await model.pick(new File(["first"], "first.xlsx"));
  await model.inspect();
  const replacement = model.pick(new File(["second"], "second.xlsx"));
  expect(model.preview.value.id).toBe("preview-one");
  expect(model.canConfirm.value).toBe(false);
  await model.confirm();
  expect(
    session.authorized.mock.calls.some(([path]) => path.endsWith("/confirm"))
  ).toBe(false);
  finishDiscard({
    ...preview(),
    status: "EXPIRED",
    rows: [],
    canConfirm: false
  });
  await replacement;
  expect(model.preview.value).toBeNull();
  expect(model.selectedFile.value.name).toBe("second.xlsx");
  expect(
    session.authorized.mock.calls.find(([path]) => path.endsWith("/discard"))[1]
  ).toMatchObject({ method: "POST", data: {} });
});
it("keeps the old view locked after an unknown discard and retries the same discard", async () => {
  let attempts = 0;
  session.authorized.mockImplementation(async path => {
    if (path.endsWith("/preview")) return preview();
    if (path.endsWith("/discard")) {
      if (++attempts === 1) throw new TypeError("lost response");
      return { ...preview(), status: "EXPIRED", rows: [], canConfirm: false };
    }
    throw new Error(path);
  });
  boot();
  await model.pick(new File(["first"], "first.xlsx"));
  await model.inspect();
  await model.pick(new File(["second"], "second.xlsx"));
  expect(model.preview.value.id).toBe("preview-one");
  expect(model.replacementPending.value).toBe(true);
  expect(model.locked.value).toBe(true);
  expect(model.canConfirm.value).toBe(false);
  model.clear();
  expect(model.preview.value.id).toBe("preview-one");
  await model.retryReplacement();
  expect(model.replacementPending.value).toBe(false);
  expect(model.selectedFile.value.name).toBe("second.xlsx");
  expect(
    session.authorized.mock.calls
      .filter(([path]) => path.endsWith("/discard"))
      .map(([path]) => path)
  ).toEqual([
    "/employee-imports/preview-one/discard",
    "/employee-imports/preview-one/discard"
  ]);
});
it("reconciles an already committed preview instead of switching files and duplicating an import", async () => {
  session.authorized.mockImplementation(async path => {
    if (path.endsWith("/preview")) return preview();
    if (path.endsWith("/discard"))
      throw { status: 409, errorCode: "INVALID_STATE" };
    if (path.endsWith("/result"))
      return { previewId: "preview-one", importedCount: 1, employeeIds: [17] };
    return { ...preview(), status: "COMMITTED", rows: [] };
  });
  boot();
  await model.pick(new File(["first"], "first.xlsx"));
  await model.inspect();
  await model.pick(new File(["second"], "second.xlsx"));
  expect(model.result.value.importedCount).toBe(1);
  expect(model.preview.value.status).toBe("COMMITTED");
  expect(model.selectedFile.value).toBeNull();
  expect(model.replacementPending.value).toBe(false);
  expect(model.error.value).toContain("未切换到新文件");
  expect(
    session.authorized.mock.calls.filter(([path]) => path.endsWith("/preview"))
  ).toHaveLength(1);
});
it("does not accept another file while the initial upload is in flight", async () => {
  let finish;
  session.authorized.mockImplementation(
    () =>
      new Promise(resolve => {
        finish = resolve;
      })
  );
  boot();
  await model.pick(new File(["first"], "first.xlsx"));
  const reading = model.inspect();
  await model.pick(new File(["second"], "second.xlsx"));
  expect(model.selectedFile.value.name).toBe("first.xlsx");
  finish(preview());
  await reading;
  expect(model.preview.value.id).toBe("preview-one");
});

it("still discards a locally invalidated preview after a confirmation conflict", async () => {
  session.authorized.mockImplementation(async path => {
    if (path.endsWith("/preview")) return preview();
    if (path.endsWith("/confirm")) throw { status: 409 };
    if (path.endsWith("/discard"))
      return { ...preview(), status: "EXPIRED", rows: [], canConfirm: false };
    throw new Error(path);
  });
  boot();
  await model.pick(new File(["first"], "first.xlsx"));
  await model.inspect();
  await model.confirm();
  expect(model.invalidated.value).toBe(true);
  expect(model.preview.value.status).toBe("PREVIEW");
  await model.pick(new File(["second"], "second.xlsx"));
  expect(
    session.authorized.mock.calls.some(([path]) => path.endsWith("/discard"))
  ).toBe(true);
  expect(model.preview.value).toBeNull();
  expect(model.selectedFile.value.name).toBe("second.xlsx");
});
