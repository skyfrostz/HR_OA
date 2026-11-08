import { describe, expect, it } from "vitest";
import type { DepartmentTreeNode } from "../business";
import {
  ancestorIds,
  buildOrganization,
  defaultExpanded,
  departmentNodeSize,
  descendantIds,
  eligibleParents,
  layoutOrganization,
  searchDepartments,
  visibleDepartments
} from "./organization-model";

const row = (
  id: number,
  parentId: number | null = null,
  name = `部门${id}`
): DepartmentTreeNode => ({
  id,
  parentId,
  name,
  version: 1,
  employeeCount: id % 7
});
describe("complete organization forest", () => {
  it("handles empty data without a fabricated root", () => {
    const model = buildOrganization([]);
    expect(model.roots).toEqual([]);
    expect(visibleDepartments(model, defaultExpanded(model))).toEqual([]);
    expect(layoutOrganization([])).toEqual([]);
  });
  it("shows every real root and its direct children, leaving level three collapsed", () => {
    const model = buildOrganization([
      row(1),
      row(2, 1),
      row(3, 2),
      row(4),
      row(5, 4)
    ]);
    expect(model.roots).toEqual([1, 4]);
    expect([...defaultExpanded(model)]).toEqual([1, 4]);
    const visible = visibleDepartments(model, defaultExpanded(model));
    expect(visible.map(item => item.department.id)).toEqual([1, 2, 4, 5]);
    expect(visible.map(item => item.depth)).toEqual([1, 2, 1, 2]);
    expect(visible[1].childCount).toBe(1);
  });
  it("locates a twentieth-level name by opening every ancestor and excludes all descendants from parents", () => {
    const model = buildOrganization(
      Array.from({ length: 20 }, (_, index) =>
        row(
          index + 1,
          index || null,
          index === 19 ? "战略研究中心" : `部门${index + 1}`
        )
      )
    );
    expect(searchDepartments(model, " 研究 ").map(item => item.id)).toEqual([
      20
    ]);
    expect(ancestorIds(model, 20)).toEqual(
      Array.from({ length: 19 }, (_, index) => index + 1)
    );
    const visible = visibleDepartments(model, new Set(ancestorIds(model, 20)));
    expect(visible).toHaveLength(20);
    expect(visible.at(-1)?.depth).toBe(20);
    expect([...descendantIds(model, 17)].sort()).toEqual([18, 19, 20]);
    expect(eligibleParents(model, 2).map(item => item.id)).toEqual([1]);
  });
  it("keeps unrelated roots eligible and guards invalid server relationships without recursion loops", () => {
    const model = buildOrganization([row(1), row(2, 1), row(3), row(4, 2)]);
    expect(eligibleParents(model, 2).map(item => item.id)).toEqual([1, 3]);
    const invalid = buildOrganization([row(1, 2), row(2, 1), row(3, 999)]);
    expect(invalid.invalidHierarchy).toBe(true);
    expect(visibleDepartments(invalid, new Set([1, 2, 3]))).toHaveLength(3);
  });
  it("reserves additional height for complete long CJK names", () => {
    const name = "组织管理".repeat(32);
    const model = buildOrganization([row(1, null, name), row(2, 1)]);
    const layout = layoutOrganization(
      visibleDepartments(model, defaultExpanded(model))
    );
    expect(layout[0].department.name).toBe(name);
    expect(layout[0].width).toBe(260);
    expect(layout[0].height).toBeGreaterThan(departmentNodeSize("组织").height);
    expect(layout[1].position.y).toBeGreaterThan(
      layout[0].position.y + layout[0].height
    );
  });
  it.each([100, 500])(
    "lays out %i departments at readable node size with unique, finite positions",
    count => {
      const rows = Array.from({ length: count }, (_, index) =>
        row(index + 1, index ? Math.floor((index - 1) / 8) + 1 : null)
      );
      const model = buildOrganization(rows);
      const layout = layoutOrganization(
        visibleDepartments(model, new Set(model.byId.keys()))
      );
      expect(layout).toHaveLength(count);
      expect(
        new Set(layout.map(item => `${item.position.x}:${item.position.y}`))
          .size
      ).toBe(count);
      for (const item of layout) {
        expect(item.width).toBe(260);
        expect(Number.isFinite(item.position.x + item.position.y)).toBe(true);
        if (item.parentId) {
          const parent = layout.find(
            node => node.department.id === item.parentId
          )!;
          expect(item.position.y).toBeGreaterThan(
            parent.position.y + parent.height
          );
        }
      }
    }
  );
});
