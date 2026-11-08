import { computed, onScopeDispose, shallowRef, watch } from "vue";
import { saveAttachment } from "./api";
import type { ImportPreview, ImportResult } from "./business";
import { useSession } from "./session";

export function useEmployeeImport() {
  const session = useSession();
  const selectedFile = shallowRef<File | null>(null);
  const preview = shallowRef<ImportPreview | null>(null);
  const result = shallowRef<ImportResult | null>(null);
  const pending = shallowRef("");
  const uncertain = shallowRef(false);
  const invalidated = shallowRef(false);
  const error = shallowRef("");
  const revision = shallowRef(0);
  const replacementFile = shallowRef<File | null>(null);
  const replacementPending = computed(() => Boolean(replacementFile.value));
  const controller = new AbortController();
  let expiryTimer: ReturnType<typeof setTimeout>;
  const allowed = computed(
    () => session.account?.permissions.canImportEmployees === true
  );
  const locked = computed(
    () => Boolean(pending.value) || uncertain.value || replacementPending.value
  );
  const canConfirm = computed(() =>
    Boolean(
      preview.value?.canConfirm &&
      preview.value.status === "PREVIEW" &&
      !invalidated.value &&
      !result.value &&
      !replacementPending.value
    )
  );
  const read = <T = any>(path: string, options = {}) =>
    session.authorized<T>(path, { ...options, signal: controller.signal });
  const path = (id = preview.value?.id) =>
    "/employee-imports/" + encodeURIComponent(id || "");

  function clearRows(status = preview.value?.status) {
    selectedFile.value = null;
    if (preview.value)
      preview.value = { ...preview.value, rows: [], canConfirm: false, status };
  }
  function expire() {
    if (preview.value?.status !== "PREVIEW") return;
    const remaining = Date.parse(preview.value.expiresAt) - Date.now();
    if (!(remaining > 0)) {
      invalidated.value = true;
      clearRows("EXPIRED");
    }
  }
  watch(preview, record => {
    clearTimeout(expiryTimer);
    if (record?.status === "PREVIEW") {
      const remaining = Date.parse(record.expiresAt) - Date.now();
      expiryTimer = setTimeout(
        expire,
        Math.max(0, Math.min(remaining || 0, 2_147_483_647))
      );
    }
  });
  document.addEventListener("visibilitychange", expire);
  onScopeDispose(() => {
    controller.abort();
    clearTimeout(expiryTimer);
    document.removeEventListener("visibilitychange", expire);
    selectedFile.value = null;
    preview.value = null;
    result.value = null;
    replacementFile.value = null;
  });

  function applyFile(file: File) {
    selectedFile.value = file;
    preview.value = null;
    result.value = null;
    error.value = "";
    invalidated.value = false;
  }
  async function pick(file?: File) {
    if (locked.value || !file) return;
    if (
      !/\.xlsx$/i.test(file.name) ||
      !file.size ||
      file.size > 2 * 1024 * 1024
    ) {
      error.value = "请选择不超过 2 MiB 的非空 .xlsx 文件，当前预览保持不变。";
      return;
    }
    if (preview.value?.status === "PREVIEW") {
      replacementFile.value = file;
      await retryReplacement();
    } else applyFile(file);
  }
  async function retryReplacement() {
    if (
      pending.value ||
      uncertain.value ||
      !replacementFile.value ||
      !preview.value
    )
      return;
    const file = replacementFile.value;
    const previousId = preview.value.id;
    await action("discard", async () => {
      try {
        await read<ImportPreview>(path(previousId) + "/discard", {
          method: "POST",
          data: {}
        });
        if (controller.signal.aborted) return;
        replacementFile.value = null;
        applyFile(file);
        revision.value++;
      } catch (cause) {
        if (controller.signal.aborted) return;
        if (cause.status === 409) {
          preview.value = await read<ImportPreview>(path(previousId));
          if (preview.value.status === "COMMITTED") {
            await readResult();
            replacementFile.value = null;
            revision.value++;
            error.value =
              "原预览已导入，未切换到新文件。请先核对导入结果，避免重复导入。";
            return;
          }
        }
        error.value =
          "暂时无法确认旧预览已作废。当前内容已保留，请重试替换文件；确认前不能导入。";
      }
    });
  }
  function clear() {
    if (locked.value) return;
    selectedFile.value = null;
    preview.value = null;
    result.value = null;
    error.value = "";
    invalidated.value = false;
  }
  async function action(kind: string, task: () => Promise<void>) {
    if (pending.value || !allowed.value) return;
    pending.value = kind;
    error.value = "";
    try {
      await task();
    } catch (cause) {
      if (controller.signal.aborted) return;
      if (cause.status === 410) {
        invalidated.value = true;
        uncertain.value = false;
        clearRows("EXPIRED");
      }
      error.value =
        cause.status === 410
          ? "预览已过期，敏感内容已清理，请重新选择文件。"
          : cause.message;
    } finally {
      pending.value = "";
    }
  }
  async function downloadTemplate() {
    if (locked.value) return;
    await action("template", async () =>
      saveAttachment(
        await read<Blob>("/employees/import-template.xlsx", { binary: true }),
        "员工导入模板.xlsx"
      )
    );
  }
  async function inspect() {
    if (locked.value || !selectedFile.value) return;
    const file = selectedFile.value;
    let received = false;
    await action("preview", async () => {
      const response = await read<ImportPreview>("/employee-imports/preview", {
        method: "POST",
        file
      });
      if (controller.signal.aborted) return;
      preview.value = response;
      received = true;
      result.value = null;
      invalidated.value = false;
      revision.value++;
      expire();
    });
    if (received) selectedFile.value = null;
  }
  async function readResult() {
    result.value = await read<ImportResult>(path() + "/result");
    uncertain.value = false;
    clearRows("COMMITTED");
  }
  async function open(id: string) {
    if (locked.value) return;
    await action("status", async () => {
      preview.value = null;
      result.value = null;
      selectedFile.value = null;
      preview.value = await read<ImportPreview>(path(id));
      invalidated.value = preview.value.status === "EXPIRED";
      if (preview.value.status === "COMMITTED") {
        clearRows("COMMITTED");
        await readResult();
      } else expire();
    });
  }
  async function checkStatus() {
    if (pending.value || !preview.value) return;
    await action("status", async () => {
      preview.value = await read<ImportPreview>(path());
      if (preview.value.status === "COMMITTED") {
        clearRows("COMMITTED");
        await readResult();
      } else if (preview.value.status === "EXPIRED") {
        uncertain.value = false;
        invalidated.value = true;
        clearRows("EXPIRED");
      } else {
        expire();
        error.value = "预览尚未完成；如上次结果未知，请重试同一预览。";
      }
    });
    revision.value++;
  }
  async function confirm() {
    expire();
    if (
      pending.value ||
      !preview.value ||
      !allowed.value ||
      (!uncertain.value && !canConfirm.value)
    )
      return;
    await action("confirm", async () => {
      try {
        result.value = await read<ImportResult>(path() + "/confirm", {
          method: "POST",
          data: {}
        });
        uncertain.value = false;
        clearRows("COMMITTED");
        revision.value++;
      } catch (cause) {
        if (controller.signal.aborted) return;
        uncertain.value =
          !cause.status || cause.status >= 500 || cause.status === 429;
        if (uncertain.value)
          error.value = "暂时无法确认导入结果。请查询状态或重试同一预览。";
        else {
          invalidated.value = true;
          clearRows(cause.status === 410 ? "EXPIRED" : preview.value.status);
          error.value =
            cause.status === 410
              ? "预览已过期，请重新选择文件。"
              : cause.status === 409
                ? "预览已失效或记录有变动，请重新预览。"
                : cause.message;
        }
      }
    });
  }
  async function downloadErrors() {
    expire();
    if (
      locked.value ||
      !preview.value ||
      preview.value.status !== "PREVIEW" ||
      invalidated.value
    )
      return;
    await action("errors", async () => {
      const blob = await read<Blob>(path() + "/errors.xlsx", { binary: true });
      expire();
      if (preview.value?.status === "PREVIEW")
        saveAttachment(blob, "员工导入错误行.xlsx");
    });
  }
  return {
    selectedFile,
    preview,
    result,
    pending,
    uncertain,
    invalidated,
    error,
    revision,
    locked,
    canConfirm,
    replacementPending,
    retryReplacement,
    pick,
    clear,
    inspect,
    confirm,
    checkStatus,
    open,
    downloadTemplate,
    downloadErrors
  };
}
