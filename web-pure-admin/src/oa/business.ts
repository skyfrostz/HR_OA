import type { OaError } from "./api";

export type Page<T> = {
  list: T[];
  total: number;
  page: number;
  pageSize: number;
};

export type Department = {
  id: number;
  name: string;
  parentId: number | null;
  version: number;
};

export type Employee = {
  id: number;
  employeeNo: string;
  name: string;
  department: string;
  departmentId?: number;
  birthDate: string;
  hireDate: string;
  age: number;
  tenureYears: number;
  salaryCents?: number;
  version?: number;
  employmentStatus?: "ACTIVE" | "OFFBOARDED";
};

export type AnnualLeave = {
  id: number;
  employeeId: number;
  year: number;
  entitledHalfDays: number;
  usedHalfDays: number;
  approvalUsedHalfDays?: number;
  reservedHalfDays?: number;
  remainingHalfDays: number;
  note: string;
  version: number;
  updatedAt: string;
};

export type ImportRow = {
  row: number;
  values: {
    employeeNo: string;
    name: string;
    department: string;
    birthDate: string;
    hireDate: string;
    salary: string;
  };
  errors: { field: string; message: string; code?: string }[];
};

export type ImportPreview = {
  id: string;
  createdAt: string;
  expiresAt: string;
  status: "PREVIEW" | "EXPIRED" | "COMMITTED";
  totalRows: number;
  validRows: number;
  errorRows: number;
  canConfirm: boolean;
  rows: ImportRow[];
};

export type ImportResult = {
  previewId: string;
  importedCount: number;
  employeeIds: number[];
};

export type ContractReminder = {
  contractId: number;
  contractNo: string;
  employeeId: number;
  employeeNo: string;
  employeeName: string;
  endDate: string;
  daysRemaining: number;
  status: "SIGNED" | "EXPIRED";
};

export function writeNeedsRefresh(error: OaError) {
  return (
    !error.status || error.status >= 500 || error.errorCode === "STALE_RECORD"
  );
}

export function formatYuan(cents: number) {
  if (!Number.isSafeInteger(cents)) return "";
  const remainder = cents % 100;
  return (
    String((cents - remainder) / 100) + "." + String(remainder).padStart(2, "0")
  );
}

export async function findAcrossPages<T>(
  read: (page: number) => Promise<Page<T>>,
  match: (value: T) => boolean
) {
  let page = 1;
  for (;;) {
    const result = await read(page);
    const row = result.list.find(match);
    if (
      row ||
      result.page * result.pageSize >= result.total ||
      !result.list.length
    )
      return row;
    page++;
  }
}

export async function listDepartments(
  read: (
    path: string,
    options?: { signal?: AbortSignal }
  ) => Promise<Page<Department>>,
  signal?: AbortSignal
) {
  const rows: Department[] = [];
  await findAcrossPages(
    async page => {
      const result = await read("/departments?page=" + page + "&pageSize=100", {
        signal
      });
      rows.push(...result.list);
      return result;
    },
    () => false
  );
  return rows;
}

export function halfDays(value: string) {
  if (!value.trim()) return null;
  const units = Number(value) * 2;
  return Number.isInteger(units) && units >= 0 && units <= 732 ? units : null;
}
