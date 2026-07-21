const panel = document.querySelector(".panel");
const message = document.querySelector("#message");
const endpoint = document.querySelector("#endpoint");
const attempt = document.querySelector("#attempt");
const retry = document.querySelector("#retry");

function updateStatus(status) {
  if (!status) return;
  panel.classList.toggle("failed", status.state === "failed");
  message.textContent = status.message || "正在检查 Ackwrap Service";
  endpoint.textContent = (status.url || "http://127.0.0.1:18080").replace(
    "http://",
    "",
  );
  attempt.textContent =
    status.state === "failed" ? "连接失败" : `第 ${status.attempt || 1} 次检查`;
  retry.hidden = status.state !== "failed";
}

retry.addEventListener("click", () => {
  retry.hidden = true;
  panel.classList.remove("failed");
  message.textContent = "正在重新连接 Ackwrap Service";
  window.go?.main?.App?.ConnectService();
});

if (window.runtime?.EventsOn) {
  window.runtime.EventsOn("service.status", updateStatus);
} else {
  updateStatus({
    state: "failed",
    message: "Wails Runtime 未就绪，请重新启动 Ackwrap GUI",
  });
}
