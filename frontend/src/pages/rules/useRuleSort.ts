import { computed, onBeforeUnmount, ref, watch } from "vue";
import type { RouteRule } from "@/services/types";

export function isFixedRouteRule(rule: RouteRule) {
  return (
    rule.system_key === "ad_block" ||
    rule.system_key === "global_direct" ||
    ["fallback", "final"].includes(rule.rule_type) ||
    (rule.is_system && ["广告拦截", "全球直连", "最终策略"].includes(rule.name))
  );
}

export function useRuleSort(
  getRules: () => RouteRule[],
  isSaving: () => boolean,
  reorder: (ids: number[]) => void,
) {
  const tableBody = ref<HTMLTableSectionElement | null>(null);
  const preview = ref<RouteRule[] | null>(null);
  const draggingId = ref<number | null>(null);
  const displayRules = computed(() => preview.value ?? getRules());
  let origin: RouteRule[] = [];
  let pointerId: number | null = null;
  let source = 0;
  let destination = 0;
  let first = 0;
  let last = 0;
  let startY = 0;
  let pointerY = 0;
  let centerOffset = 0;
  let frame = 0;
  let lastFrame = 0;
  let scrollContainers: HTMLElement[] = [];

  function moveRule(from: number, to: number, rules = getRules()) {
    const next = [...rules];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    return next;
  }

  function updatePreview() {
    if (draggingId.value === null || !tableBody.value) return;
    let target = first;
    const center = pointerY + centerOffset;
    tableBody.value
      .querySelectorAll<HTMLTableRowElement>("tr[data-rule-id]")
      .forEach((row, index) => {
        if (
          index < first ||
          index > last ||
          Number(row.dataset.ruleId) === draggingId.value
        )
          return;
        const rect = row.getBoundingClientRect();
        if (center > rect.top + rect.height / 2) target++;
      });
    if (target === destination) return;
    destination = target;
    preview.value = moveRule(source, target, origin);
  }

  function tick(time: number) {
    const elapsed = Math.min(32, time - (lastFrame || time));
    lastFrame = time;
    if (draggingId.value !== null) {
      for (const container of scrollContainers) {
        const rect = container.getBoundingClientRect();
        const top = Math.max(0, rect.top);
        const bottom = Math.min(window.innerHeight, rect.bottom);
        const edge = Math.min(48, (bottom - top) / 4);
        if (edge <= 0) continue;
        const direction =
          pointerY < top + edge ? -1 : pointerY > bottom - edge ? 1 : 0;
        if (!direction) continue;
        const previous = container.scrollTop;
        container.scrollTop += direction * elapsed * 0.6;
        if (container.scrollTop !== previous) break;
      }
      updatePreview();
    }
    frame = requestAnimationFrame(tick);
  }

  function cancel() {
    cancelAnimationFrame(frame);
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", end);
    window.removeEventListener("pointercancel", cancel);
    window.removeEventListener("keydown", escape);
    window.removeEventListener("blur", cancel);
    if (
      pointerId !== null &&
      document.documentElement.hasPointerCapture(pointerId)
    ) {
      document.documentElement.releasePointerCapture(pointerId);
    }
    pointerId = null;
    draggingId.value = null;
    preview.value = null;
    origin = [];
    scrollContainers = [];
  }

  function move(event: PointerEvent) {
    if (event.pointerId !== pointerId) return;
    event.preventDefault();
    pointerY = event.clientY;
    if (draggingId.value === null && Math.abs(pointerY - startY) >= 4) {
      draggingId.value = origin[source].id;
      preview.value = [...origin];
    }
    updatePreview();
  }

  function end(event: PointerEvent) {
    if (event.pointerId !== pointerId) return;
    const ids =
      draggingId.value !== null && source !== destination
        ? moveRule(source, destination, origin).map((rule) => rule.id)
        : null;
    cancel();
    if (ids) reorder(ids);
  }

  function escape(event: KeyboardEvent) {
    if (event.key !== "Escape") return;
    event.preventDefault();
    cancel();
  }

  function start(event: PointerEvent, rule: RouteRule) {
    if (
      !event.isPrimary ||
      event.button !== 0 ||
      pointerId !== null ||
      isSaving() ||
      isFixedRouteRule(rule)
    )
      return;
    const handle = event.currentTarget as HTMLButtonElement;
    const row = handle.closest("tr");
    if (!row) return;
    event.preventDefault();
    handle.focus({ preventScroll: true });
    origin = [...getRules()];
    source = origin.findIndex((item) => item.id === rule.id);
    if (source < 0) return;
    destination = first = last = source;
    while (first > 0 && !isFixedRouteRule(origin[first - 1])) first--;
    while (last < origin.length - 1 && !isFixedRouteRule(origin[last + 1]))
      last++;
    pointerId = event.pointerId;
    startY = pointerY = event.clientY;
    const rect = row.getBoundingClientRect();
    centerOffset = rect.top + rect.height / 2 - pointerY;
    for (
      let parent = tableBody.value?.parentElement;
      parent;
      parent = parent.parentElement
    ) {
      if (
        /(auto|scroll)/.test(getComputedStyle(parent).overflowY) &&
        parent.scrollHeight > parent.clientHeight
      ) {
        scrollContainers.push(parent);
      }
    }
    document.documentElement.setPointerCapture(pointerId);
    window.addEventListener("pointermove", move, { passive: false });
    window.addEventListener("pointerup", end);
    window.addEventListener("pointercancel", cancel);
    window.addEventListener("keydown", escape);
    window.addEventListener("blur", cancel);
    lastFrame = 0;
    frame = requestAnimationFrame(tick);
  }

  function keydown(event: KeyboardEvent, rule: RouteRule) {
    if (!["ArrowUp", "ArrowDown"].includes(event.key)) return;
    event.preventDefault();
    if (isSaving() || pointerId !== null || isFixedRouteRule(rule)) return;
    const rules = getRules();
    const index = rules.findIndex((item) => item.id === rule.id);
    const target = index + (event.key === "ArrowUp" ? -1 : 1);
    if (index < 0 || !rules[target] || isFixedRouteRule(rules[target])) return;
    reorder(moveRule(index, target).map((item) => item.id));
  }

  watch(getRules, cancel);
  watch(isSaving, (saving) => {
    if (saving) cancel();
  });
  onBeforeUnmount(cancel);
  return { tableBody, displayRules, draggingId, start, keydown };
}
