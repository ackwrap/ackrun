import { onBeforeUnmount, onMounted, ref, type Ref } from "vue";

export function useAdaptiveActions(
  headerRow: Ref<HTMLElement | null>,
  statusGroup: Ref<HTMLElement | null>,
  actionsRow: Ref<HTMLElement | null>,
  actionsId: string,
) {
  const actionsCollapsed = ref(true);
  const actionsOpen = ref(false);
  let resizeObserver: ResizeObserver | undefined;
  let mutationObserver: MutationObserver | undefined;
  let frame = 0;
  let mounted = false;
  let actionsWidth = 0;

  function contentWidth(element: HTMLElement) {
    const children = Array.from(element.children).filter(
      (child) => getComputedStyle(child).display !== "none",
    );
    const gap = parseFloat(getComputedStyle(element).columnGap) || 0;
    return (
      children.reduce(
        (width, child) => width + child.getBoundingClientRect().width,
        0,
      ) +
      Math.max(0, children.length - 1) * gap
    );
  }

  function measure() {
    if (!headerRow.value || !statusGroup.value || !actionsRow.value) return;
    // Dropdown buttons stretch to one width; measure their natural widths only
    // while they are laid out horizontally, including the hidden measuring row.
    if (getComputedStyle(actionsRow.value).flexDirection !== "column") {
      actionsWidth = contentWidth(actionsRow.value);
    }
    const gap = parseFloat(getComputedStyle(headerRow.value).columnGap) || 0;
    const collapsed =
      contentWidth(statusGroup.value) + actionsWidth + gap >
      headerRow.value.clientWidth;
    if (collapsed !== actionsCollapsed.value) {
      actionsCollapsed.value = collapsed;
      actionsOpen.value = false;
    }
  }

  function scheduleMeasure() {
    if (!mounted) return;
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(measure);
  }

  function observeChildren() {
    for (const row of [statusGroup.value, actionsRow.value]) {
      if (!row) continue;
      for (const child of row.children) resizeObserver?.observe(child);
    }
    scheduleMeasure();
  }

  function closeActions(focusTrigger = false) {
    actionsOpen.value = false;
    if (focusTrigger) {
      headerRow.value
        ?.querySelector<HTMLButtonElement>(`[aria-controls='${actionsId}']`)
        ?.focus();
    }
  }

  function outside(event: Event) {
    if (
      actionsOpen.value &&
      event.target instanceof Node &&
      !headerRow.value?.contains(event.target)
    ) {
      closeActions();
    }
  }

  function escape(event: KeyboardEvent) {
    if (event.key === "Escape" && actionsOpen.value) {
      event.preventDefault();
      closeActions(true);
    }
  }

  function chooseAction(event: MouseEvent) {
    if (
      actionsCollapsed.value &&
      event.target instanceof Element &&
      event.target.closest("button")
    ) {
      closeActions(true);
    }
  }

  onMounted(() => {
    mounted = true;
    resizeObserver = new ResizeObserver(scheduleMeasure);
    for (const element of [
      headerRow.value,
      statusGroup.value,
      actionsRow.value,
    ]) {
      if (element) resizeObserver.observe(element);
    }
    mutationObserver = new MutationObserver(observeChildren);
    if (headerRow.value) {
      mutationObserver.observe(headerRow.value, {
        childList: true,
        characterData: true,
        subtree: true,
      });
    }
    observeChildren();
    document.addEventListener("pointerdown", outside);
    document.addEventListener("focusin", outside);
    document.addEventListener("keydown", escape);
    void document.fonts.ready.then(scheduleMeasure);
  });

  onBeforeUnmount(() => {
    mounted = false;
    resizeObserver?.disconnect();
    mutationObserver?.disconnect();
    cancelAnimationFrame(frame);
    document.removeEventListener("pointerdown", outside);
    document.removeEventListener("focusin", outside);
    document.removeEventListener("keydown", escape);
  });

  return { actionsCollapsed, actionsOpen, chooseAction };
}
