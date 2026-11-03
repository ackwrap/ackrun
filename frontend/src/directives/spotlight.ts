import type { ObjectDirective } from "vue";
import { attachCardSpotlight } from "@/utils/cardSpotlight";

const cleanups = new WeakMap<HTMLElement, () => void>();

function update(element: HTMLElement, enabled: boolean) {
  if (enabled && !cleanups.has(element)) {
    cleanups.set(element, attachCardSpotlight(element));
  } else if (!enabled) {
    cleanups.get(element)?.();
    cleanups.delete(element);
  }
}

export const vSpotlight: ObjectDirective<HTMLElement, boolean | undefined> = {
  mounted: (element, binding) => update(element, binding.value !== false),
  updated: (element, binding) => update(element, binding.value !== false),
  unmounted: (element) => update(element, false),
};
