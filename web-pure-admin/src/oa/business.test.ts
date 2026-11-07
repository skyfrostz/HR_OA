import { expect, it, vi } from "vitest";
import { OaError } from "./api";
import { findAcrossPages, formatYuan, halfDays, writeNeedsRefresh } from "./business";

it("preserves exact integer cents and half-day units", () => {
  expect(formatYuan(9007199254740991)).toBe("90071992547409.91");
  expect(formatYuan(1201)).toBe("12.01");
  expect(halfDays("10.5")).toBe(21);
  expect(halfDays("366")).toBe(732);
  expect(halfDays("10.25")).toBeNull();
  expect(halfDays("366.5")).toBeNull();
  expect(halfDays("")).toBeNull();
});

it("reads later pages to reconcile an unknown employee create", async () => {
  const read = vi.fn(async (page: number) => ({ list: [page], total: 3, page, pageSize: 1 }));
  expect(await findAcrossPages(read, row => row === 2)).toBe(2);
  expect(read.mock.calls).toEqual([[1], [2]]);
  expect(writeNeedsRefresh(new OaError(0))).toBe(true);
  expect(writeNeedsRefresh(new OaError(409, undefined, "STALE_RECORD"))).toBe(true);
  expect(writeNeedsRefresh(new OaError(409, undefined, "DUPLICATE_EMPLOYEE_NO"))).toBe(false);
});
