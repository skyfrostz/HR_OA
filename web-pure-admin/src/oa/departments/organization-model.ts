import dagre from "dagre";
import type { DepartmentTreeNode } from "../business";

export type Organization = ReturnType<typeof buildOrganization>;
export type OrganizationItem = {
  department: DepartmentTreeNode;
  parentId: number | null;
  depth: number;
  childCount: number;
};

export function buildOrganization(rows: DepartmentTreeNode[]) {
  const byId = new Map(rows.map(row => [row.id, row]));
  const parentIds = new Map<number, number | null>();
  const children = new Map<number, number[]>();
  let invalidHierarchy = false;
  for (const row of byId.values()) {
    const seen = new Set([row.id]);
    let parent = row.parentId;
    while (parent !== null) {
      if (seen.has(parent) || !byId.has(parent)) {
        invalidHierarchy = true;
        break;
      }
      seen.add(parent);
      parent = byId.get(parent)!.parentId;
    }
    parentIds.set(row.id, parent === null ? row.parentId : null);
    children.set(row.id, []);
  }
  const roots: number[] = [];
  for (const row of byId.values()) {
    const parent = parentIds.get(row.id);
    if (parent === null) roots.push(row.id);
    else children.get(parent!)?.push(row.id);
  }
  return { byId, parentIds, children, roots, invalidHierarchy };
}

export function defaultExpanded(model: Organization) {
  return new Set(model.roots);
}

export function visibleDepartments(model: Organization, expanded: Set<number>) {
  const result: OrganizationItem[] = [];
  const pending = model.roots.map(id => ({ id, depth: 1 })).reverse();
  while (pending.length) {
    const { id, depth } = pending.pop()!;
    const children = model.children.get(id) || [];
    result.push({
      department: model.byId.get(id)!,
      parentId: model.parentIds.get(id) ?? null,
      depth,
      childCount: children.length
    });
    if (expanded.has(id)) {
      for (let index = children.length - 1; index >= 0; index--)
        pending.push({ id: children[index], depth: depth + 1 });
    }
  }
  return result;
}

export function ancestorIds(model: Organization, id: number) {
  const result: number[] = [];
  let parent = model.parentIds.get(id);
  while (parent !== null && parent !== undefined) {
    result.push(parent);
    parent = model.parentIds.get(parent);
  }
  return result.reverse();
}

export function descendantIds(model: Organization, id: number) {
  const result = new Set<number>();
  const pending = [...(model.children.get(id) || [])];
  while (pending.length) {
    const child = pending.pop()!;
    if (result.has(child)) continue;
    result.add(child);
    pending.push(...(model.children.get(child) || []));
  }
  return result;
}

export function eligibleParents(model: Organization, id?: number) {
  const excluded =
    id === undefined ? new Set<number>() : descendantIds(model, id);
  if (id !== undefined) excluded.add(id);
  return [...model.byId.values()].filter(row => !excluded.has(row.id));
}

export function searchDepartments(model: Organization, query: string) {
  const text = query.trim().toLocaleLowerCase();
  if (!text) return [];
  return [...model.byId.values()].filter(row =>
    row.name.toLocaleLowerCase().includes(text)
  );
}

export function departmentNodeSize(name: string) {
  // Reserve a full line for every 15 CJK characters (30 Latin units), never ellipsize names.
  const units = [...name].reduce(
    (sum, char) => sum + (/[^\x00-\x7f]/.test(char) ? 2 : 1),
    0
  );
  return { width: 260, height: Math.max(112, Math.ceil(units / 30) * 22 + 80) };
}

export function layoutOrganization(items: OrganizationItem[]) {
  if (!items.length) return [];
  const graph = new dagre.graphlib.Graph();
  graph.setGraph({
    rankdir: "TB",
    nodesep: 32,
    ranksep: 56,
    marginx: 24,
    marginy: 24
  });
  graph.setDefaultEdgeLabel(() => ({}));
  const visible = new Set(items.map(item => item.department.id));
  for (const item of items)
    graph.setNode(
      String(item.department.id),
      departmentNodeSize(item.department.name)
    );
  for (const item of items) {
    if (item.parentId !== null && visible.has(item.parentId))
      graph.setEdge(String(item.parentId), String(item.department.id));
  }
  dagre.layout(graph);
  return items.map(item => {
    const { x, y, width, height } = graph.node(String(item.department.id));
    return {
      ...item,
      position: { x: x - width / 2, y: y - height / 2 },
      width,
      height
    };
  });
}
