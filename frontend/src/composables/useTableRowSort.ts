import { computed, onBeforeUnmount, ref, shallowRef, watch } from "vue";

export function useTableRowSort<T extends { id: number }>(
  getItems: () => T[],
  isSaving: () => boolean,
  reorder: (ids: number[]) => void,
  isFixed: (item: T) => boolean,
) {
  const tableBody = ref<HTMLTableSectionElement | null>(null);
  const preview = shallowRef<T[] | null>(null);
  const draggingId = ref<number | null>(null);
  const displayItems = computed(() => preview.value ?? getItems());
  let origin: T[] = [];
  let pointerId: number | null = null;
  let source = 0;
  let destination = 0;
  let first = 0;
  let last = 0;
  let startY = 0;
  let pointerX = 0;
  let pointerY = 0;
  let grabOffsetX = 0;
  let grabOffsetY = 0;
  let centerOffset = 0;
  let frame = 0;
  let lastFrame = 0;
  let scrollContainers: HTMLElement[] = [];
  let sourceRow: HTMLTableRowElement | null = null;
  let ghost: HTMLDivElement | null = null;

  function positionGhost() {
    if (!ghost) return;
    ghost.style.transform = `translate3d(${pointerX - grabOffsetX}px, ${pointerY - grabOffsetY}px, 0)`;
  }

  function createGhost() {
    if (!sourceRow?.isConnected) return false;
    const rect = sourceRow.getBoundingClientRect();
    const clone = sourceRow.cloneNode(true) as HTMLTableRowElement;
    clone.removeAttribute("data-sort-id");
    clone
      .querySelectorAll("[id]")
      .forEach((element) => element.removeAttribute("id"));
    const table = sourceRow
      .closest("table")!
      .cloneNode(false) as HTMLTableElement;
    table.removeAttribute("id");
    table.style.width = "100%";
    table.style.minWidth = "0";
    table.style.tableLayout = "fixed";
    const columns = document.createElement("colgroup");
    Array.from(sourceRow.cells).forEach((cell) => {
      const column = document.createElement("col");
      column.style.width = `${cell.getBoundingClientRect().width}px`;
      columns.append(column);
    });
    const body = tableBody.value!.cloneNode(false) as HTMLTableSectionElement;
    body.removeAttribute("id");
    body.append(clone);
    table.append(columns, body);
    ghost = document.createElement("div");
    ghost.className = "aw-sort-ghost";
    ghost.style.width = `${rect.width}px`;
    ghost.setAttribute("aria-hidden", "true");
    ghost.setAttribute("inert", "");
    ghost.append(table);
    positionGhost();
    document.body.append(ghost);
    document.body.classList.add("aw-sort-active");
    return true;
  }

  function moveItem(from: number, to: number, items = getItems()) {
    const next = [...items];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    return next;
  }

  function updatePreview() {
    if (draggingId.value === null || !tableBody.value) return;
    let target = first;
    const center = pointerY + centerOffset;
    tableBody.value
      .querySelectorAll<HTMLTableRowElement>("tr[data-sort-id]")
      .forEach((row, index) => {
        if (
          index < first ||
          index > last ||
          Number(row.dataset.sortId) === draggingId.value
        )
          return;
        const rect = row.getBoundingClientRect();
        if (center > rect.top + rect.height / 2) target++;
      });
    if (target === destination) return;
    destination = target;
    preview.value = moveItem(source, target, origin);
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
    ghost?.remove();
    ghost = null;
    sourceRow = null;
    document.body.classList.remove("aw-sort-active");
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
    pointerX = event.clientX;
    pointerY = event.clientY;
    if (draggingId.value === null && Math.abs(pointerY - startY) >= 4) {
      if (!createGhost()) {
        cancel();
        return;
      }
      draggingId.value = origin[source].id;
      preview.value = [...origin];
    }
    positionGhost();
    updatePreview();
  }

  function end(event: PointerEvent) {
    if (event.pointerId !== pointerId) return;
    const ids =
      draggingId.value !== null && source !== destination
        ? moveItem(source, destination, origin).map((item) => item.id)
        : null;
    cancel();
    if (ids) reorder(ids);
  }

  function escape(event: KeyboardEvent) {
    if (event.key !== "Escape") return;
    event.preventDefault();
    cancel();
  }

  function start(event: PointerEvent, item: T) {
    if (
      !event.isPrimary ||
      event.button !== 0 ||
      pointerId !== null ||
      isSaving() ||
      isFixed(item)
    )
      return;
    const handle = event.currentTarget as HTMLButtonElement;
    const row = handle.closest("tr");
    if (!row) return;
    event.preventDefault();
    handle.focus({ preventScroll: true });
    origin = [...getItems()];
    source = origin.findIndex((entry) => entry.id === item.id);
    if (source < 0) return;
    destination = first = last = source;
    while (first > 0 && !isFixed(origin[first - 1])) first--;
    while (last < origin.length - 1 && !isFixed(origin[last + 1])) last++;
    pointerId = event.pointerId;
    pointerX = event.clientX;
    startY = pointerY = event.clientY;
    const rect = row.getBoundingClientRect();
    sourceRow = row;
    grabOffsetX = pointerX - rect.left;
    grabOffsetY = pointerY - rect.top;
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

  function keydown(event: KeyboardEvent, item: T) {
    if (!["ArrowUp", "ArrowDown"].includes(event.key)) return;
    event.preventDefault();
    if (isSaving() || pointerId !== null || isFixed(item)) return;
    const items = getItems();
    const index = items.findIndex((entry) => entry.id === item.id);
    const target = index + (event.key === "ArrowUp" ? -1 : 1);
    if (index < 0 || !items[target] || isFixed(items[target])) return;
    reorder(moveItem(index, target).map((entry) => entry.id));
  }

  watch(getItems, cancel);
  watch(isSaving, (saving) => {
    if (saving) cancel();
  });
  onBeforeUnmount(cancel);
  return { tableBody, displayItems, draggingId, start, keydown };
}
