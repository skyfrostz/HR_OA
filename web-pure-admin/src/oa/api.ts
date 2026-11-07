const API = "/api/oa/v1";

const errorMessages: Record<number, string> = {
  400: "提交的内容无效，请检查后重试。",
  401: "登录已失效，请重新登录。",
  403: "当前账号没有执行此操作的权限。",
  404: "该记录不存在或当前账号无法访问。",
  409: "编号已存在或记录已变化，请检查现有记录。",
  413: "文件超过上传限制。",
  422: "提交内容无效，请检查后重试。",
  429: "请求过于频繁，请稍后重试。"
};

export class OaError extends Error {
  constructor(
    readonly status: number,
    message = errorMessages[status] || "服务暂时不可用，请稍后重试。",
    readonly errorCode = ""
  ) {
    super(message);
  }
}

export type RequestOptions = {
  method?: string;
  data?: object;
  file?: File;
  token?: string;
  signal?: AbortSignal;
  sessionSignal?: AbortSignal;
  binary?: boolean;
  idempotencyKey?: string;
};

export async function request<T>(
  path: string,
  { method = "GET", data, file, token, signal, sessionSignal, binary = false, idempotencyKey }: RequestOptions = {}
): Promise<T> {
  const headers = new Headers();
  let body: BodyInit | undefined;
  if (idempotencyKey) headers.set("Idempotency-Key", idempotencyKey);
  if (token) headers.set("x-token", token);
  if (file) {
    const form = new FormData();
    form.append("file", file);
    body = form;
  } else if (data) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(data);
  }

  const controller = new AbortController();
  const signals = [signal, sessionSignal].filter(Boolean);
  const cancel = () => controller.abort();
  for (const source of signals) {
    if (source.aborted) cancel();
    else source.addEventListener("abort", cancel, { once: true });
  }
  const timeout = setTimeout(cancel, 30_000);

  try {
    const response = await fetch(API + path, {
      method, headers, body, signal: controller.signal,
      credentials: "omit", cache: "no-store", redirect: "error"
    });
    if (!response.ok) {
      const errorCode = response.headers.get("Content-Type")?.includes("application/json")
        ? (await response.json().catch(() => null))?.data?.errorCode
        : "";
      throw new OaError(response.status, undefined, typeof errorCode === "string" ? errorCode : "");
    }
    if (binary) {
      if (!/^attachment(?:;|$)/i.test(response.headers.get("Content-Disposition") || "")) {
        throw new OaError(0, "附件响应异常，请稍后重试。");
      }
      return await response.blob() as T;
    }
    const envelope = await response.json();
    if (envelope?.code !== 0 || !envelope.data) {
      throw new OaError(0, "服务响应异常，请稍后重试。");
    }
    return envelope.data as T;
  } catch (error) {
    if (signals.some(source => source.aborted) || error instanceof OaError) throw error;
    throw new OaError(0, "无法完成请求，请检查网络后重试。");
  } finally {
    clearTimeout(timeout);
    for (const source of signals) source.removeEventListener("abort", cancel);
  }
}

export const listPage = (path: string, page: number) => path + "?page=" + page + "&pageSize=20";

export function saveAttachment(blob: Blob, originalName: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = originalName.replace(/[\\/\u0000-\u001f\u007f]/g, "_") || "合同附件";
  document.body.append(link);
  try {
    link.click();
  } finally {
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
}
