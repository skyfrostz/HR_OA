import { afterEach, expect, it } from "vitest";
import { effectScope, type EffectScope } from "vue";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { ElDialog, ElMessageBox } from "element-plus";
import { useContractScrollLock } from "./use-contract-scroll-lock";

const scopes: EffectScope[] = [];
let nested: VueWrapper | undefined;
function owner() {
  const scope = effectScope();
  scopes.push(scope);
  return { scope, lock: scope.run(useContractScrollLock)! };
}
afterEach(() => {
  nested?.unmount();
  nested = undefined;
  for (const scope of scopes.splice(0)) scope.stop();
  document.body.className = "";
  document.body.removeAttribute("style");
});

it("can release and reacquire in one turn without a delayed cleanup unlocking the next drawer", async () => {
  const { lock } = owner();
  lock.acquire();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  lock.release();
  lock.acquire();
  await new Promise(resolve => setTimeout(resolve, 220));
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  lock.release();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    false
  );
});

it("releases only its own lease and preserves existing classes, inline width and overflow", () => {
  document.body.classList.add("el-popup-parent--hidden");
  document.body.style.width = "calc(100% - 15px)";
  document.body.style.overflow = "hidden";
  const before = document.body.getAttribute("style");
  const first = owner();
  const second = owner();
  first.lock.acquire();
  second.lock.acquire();
  first.scope.stop();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  second.scope.stop();
  expect(document.body.className).toBe("el-popup-parent--hidden");
  expect(document.body.getAttribute("style")).toBe(before);
});

it("does not remove a preexisting lock it did not introduce", () => {
  document.body.classList.add("oa-contract-scroll-locked");
  const { lock } = owner();
  lock.acquire();
  lock.release();
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
});

it("keeps its own lock while a real nested attachment-style Element dialog closes", async () => {
  const { lock } = owner();
  lock.acquire();
  nested = mount(ElDialog, {
    attachTo: document.body,
    props: { modelValue: false, title: "附件预览" },
    slots: { default: "合成附件" },
    global: { stubs: { transition: false } }
  });
  await nested.setProps({ modelValue: true });
  await flushPromises();
  expect(document.body.classList.contains("el-popup-parent--hidden")).toBe(
    true
  );
  await nested.setProps({ modelValue: false });
  await new Promise(resolve => setTimeout(resolve, 240));
  expect(document.body.classList.contains("el-popup-parent--hidden")).toBe(
    false
  );
  expect(document.body.classList.contains("oa-contract-scroll-locked")).toBe(
    true
  );
  lock.release();
  expect(document.body.className).toBe("");
  expect(document.body.style.width).toBe("");
});

it("does not release the active confirmation dialog's Element lock", async () => {
  const { lock } = owner();
  lock.acquire();
  const confirmation = ElMessageBox.confirm("合成未保存输入", "放弃修改？", {
    confirmButtonText: "放弃修改",
    cancelButtonText: "继续编辑"
  }).catch(() => false);
  await flushPromises();
  expect(
    document.body.classList.contains("el-message-box-parent--hidden")
  ).toBe(true);
  lock.release();
  expect(
    document.body.classList.contains("el-message-box-parent--hidden")
  ).toBe(true);
  const cancel = [
    ...document.querySelectorAll<HTMLButtonElement>(".el-message-box button")
  ].find(button => button.textContent?.trim() === "继续编辑");
  expect(cancel).toBeDefined();
  cancel!.click();
  await confirmation;
  await new Promise(resolve => setTimeout(resolve, 240));
  expect(
    document.body.classList.contains("el-message-box-parent--hidden")
  ).toBe(false);
  expect(document.body.style.width).toBe("");
});
