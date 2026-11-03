/** Pointer coordinates stay outside Vue's render cycle. Only the active card paints. */
export function attachCardSpotlight(element: HTMLElement): () => void {
  const view = element.ownerDocument.defaultView;
  if (!view) return () => {};
  const finePointer = view.matchMedia("(hover: hover) and (pointer: fine)");
  const reducedMotion = view.matchMedia("(prefers-reduced-motion: reduce)");
  let frame: number | null = null;
  let bounds: DOMRect | null = null;
  let active = false;
  let clientX = 0;
  let clientY = 0;

  const invalidateBounds = () => {
    bounds = null;
  };
  const observer =
    typeof ResizeObserver === "undefined"
      ? null
      : new ResizeObserver(invalidateBounds);

  function paint() {
    frame = null;
    if (!active) return;
    bounds ??= element.getBoundingClientRect();
    const x = Math.max(0, Math.min(bounds.width, clientX - bounds.left));
    const y = Math.max(0, Math.min(bounds.height, clientY - bounds.top));
    const edge = Math.min(x, y, bounds.width - x, bounds.height - y);
    element.style.setProperty("--spotlight-x", `${x}px`);
    element.style.setProperty("--spotlight-y", `${y}px`);
    element.style.setProperty(
      "--spotlight-edge",
      String(Math.max(0, 1 - edge / 80)),
    );
    element.setAttribute("data-spotlight-active", "");
  }

  function leave() {
    active = false;
    if (frame !== null) view!.cancelAnimationFrame(frame);
    frame = null;
    bounds = null;
    element.removeAttribute("data-spotlight-active");
    view!.removeEventListener("scroll", invalidateBounds, true);
    view!.removeEventListener("resize", invalidateBounds);
    observer?.disconnect();
  }

  function move(event: PointerEvent) {
    if (
      event.pointerType !== "mouse" ||
      !finePointer.matches ||
      reducedMotion.matches
    ) {
      if (active) leave();
      return;
    }
    if (!active) {
      active = true;
      bounds = null;
      view!.addEventListener("scroll", invalidateBounds, {
        capture: true,
        passive: true,
      });
      view!.addEventListener("resize", invalidateBounds, { passive: true });
      observer?.observe(element);
    }
    clientX = event.clientX;
    clientY = event.clientY;
    if (frame === null) frame = view!.requestAnimationFrame(paint);
  }

  element.classList.add("aw-spotlight");
  element.addEventListener("pointerenter", move, { passive: true });
  element.addEventListener("pointermove", move, { passive: true });
  element.addEventListener("pointerleave", leave);
  element.addEventListener("pointercancel", leave);
  finePointer.addEventListener("change", leave);
  reducedMotion.addEventListener("change", leave);
  view.addEventListener("blur", leave);

  return () => {
    leave();
    element.removeEventListener("pointerenter", move);
    element.removeEventListener("pointermove", move);
    element.removeEventListener("pointerleave", leave);
    element.removeEventListener("pointercancel", leave);
    finePointer.removeEventListener("change", leave);
    reducedMotion.removeEventListener("change", leave);
    view.removeEventListener("blur", leave);
    element.classList.remove("aw-spotlight");
    for (const property of [
      "--spotlight-x",
      "--spotlight-y",
      "--spotlight-edge",
    ]) {
      element.style.removeProperty(property);
    }
  };
}
