export async function writeClipboardText(value: string): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return;
    }
  } catch {
    // Fall back for browsers that expose the API but deny it in this context.
  }

  let textArea: HTMLTextAreaElement | undefined;
  try {
    textArea = document.createElement("textarea");
    textArea.value = value;
    textArea.setAttribute("readonly", "");
    textArea.style.position = "fixed";
    textArea.style.opacity = "0";
    textArea.style.pointerEvents = "none";
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();
    textArea.setSelectionRange(0, value.length);

    if (typeof document.execCommand !== "function" || !document.execCommand("copy"))
      throw new Error();
  } catch {
    throw new Error("浏览器无法自动复制，请检查页面权限或手动复制");
  } finally {
    textArea?.remove();
  }
}
