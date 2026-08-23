<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import {
  sendRealtime,
  useRealtimeSocket,
} from "@/composables/useRealtimeSocket";
import type { WSEvent } from "@/services/types";
import type { SSHSessionCreateResponse } from "@/services/sshTypes";

const props = defineProps<{ session: SSHSessionCreateResponse }>();
const emit = defineEmits<{
  status: [
    state: "connecting" | "online" | "offline" | "error",
    message: string,
  ];
  closed: [];
}>();

const container = ref<HTMLElement | null>(null);
const status = ref<"connecting" | "online" | "offline" | "error">("connecting");
const statusMessage = ref("正在连接实时通道...");
let terminal: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let resizeObserver: ResizeObserver | null = null;
let resizeFrame = 0;
let attached = false;
let closing = false;
const encoder = new TextEncoder();

const { connected } = useRealtimeSocket(handleEvent);

function setStatus(
  state: "connecting" | "online" | "offline" | "error",
  message: string,
) {
  status.value = state;
  statusMessage.value = message;
  emit("status", state, message);
}

function bytesToBase64(content: Uint8Array) {
  let binary = "";
  for (let offset = 0; offset < content.length; offset += 0x8000) {
    binary += String.fromCharCode(...content.subarray(offset, offset + 0x8000));
  }
  return btoa(binary);
}

function base64ToBytes(content: string) {
  const binary = atob(content);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes;
}

function sendInput(content: Uint8Array) {
  if (!attached || closing || content.length === 0) return;
  sendRealtime("ssh.session.input", {
    session_id: props.session.session_id,
    content: bytesToBase64(content),
  });
}

function sendSize() {
  if (!terminal || !attached || closing) return;
  sendRealtime("ssh.session.resize", {
    session_id: props.session.session_id,
    columns: terminal.cols,
    rows: terminal.rows,
  });
}

function fit() {
  if (!fitAddon || !terminal) return;
  try {
    fitAddon.fit();
    sendSize();
  } catch {
    // The workspace can disappear between ResizeObserver and this frame.
  }
}

function scheduleFit() {
  if (resizeFrame) cancelAnimationFrame(resizeFrame);
  resizeFrame = requestAnimationFrame(fit);
}

function attach() {
  if (!connected.value || !terminal || attached || closing) return;
  setStatus("connecting", "正在创建远端 PTY...");
  const sent = sendRealtime("ssh.session.attach", {
    session_id: props.session.session_id,
    attach_token: props.session.attach_token,
    columns: terminal.cols,
    rows: terminal.rows,
  });
  if (!sent) setStatus("connecting", "实时通道尚未就绪，正在重试...");
}

function handleEvent(event: WSEvent) {
  const data = event.data as Record<string, unknown> | undefined;
  if (!data || data.session_id !== props.session.session_id) return;
  if (event.type === "ssh.session.status" && data.status === "attached") {
    attached = true;
    setStatus("online", "终端已连接");
    terminal?.focus();
    return;
  }
  if (event.type === "ssh.session.output" && typeof data.content === "string") {
    terminal?.write(base64ToBytes(data.content));
    return;
  }
  if (event.type === "ssh.session.error") {
    const message =
      typeof data.message === "string" ? data.message : "SSH 终端发生错误";
    setStatus("error", message);
    terminal?.writeln(`\r\n[${message}]`);
    return;
  }
  if (event.type === "ssh.session.closed") {
    attached = false;
    const message =
      typeof data.error_code === "string" && data.error_code
        ? `会话已关闭：${data.error_code}`
        : "会话已关闭";
    setStatus(data.result === "success" ? "offline" : "error", message);
    terminal?.writeln(`\r\n[${message}]`);
    emit("closed");
  }
}

watch(connected, (value) => {
  if (value) attach();
  else if (!closing) {
    attached = false;
    setStatus("connecting", "实时通道已断开");
  }
});

onMounted(async () => {
  emit("status", status.value, statusMessage.value);
  await nextTick();
  if (!container.value) return;
  const theme = getComputedStyle(document.documentElement);
  terminal = new Terminal({
    cursorBlink: true,
    scrollback: 5000,
    convertEol: false,
    fontFamily: '"Cascadia Mono", "JetBrains Mono", Consolas, monospace',
    fontSize: 13,
    theme: {
      background: theme.getPropertyValue("--ssh-terminal-bg").trim(),
      foreground: theme.getPropertyValue("--ssh-terminal-text").trim(),
      cursor: theme.getPropertyValue("--ssh-terminal-cursor").trim(),
      selectionBackground: theme
        .getPropertyValue("--ssh-terminal-selection")
        .trim(),
    },
    windowOptions: {},
    linkHandler: {
      activate: (_event, text) => {
        try {
          const target = new URL(text);
          if (target.protocol === "https:" || target.protocol === "http:") {
            window.open(target.href, "_blank", "noopener,noreferrer");
          }
        } catch {
          // Ignore malformed terminal links.
        }
      },
    },
  });
  fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  terminal.open(container.value);
  terminal.onData((content) => sendInput(encoder.encode(content)));
  terminal.onBinary((content) => {
    const bytes = Uint8Array.from(content, (character) =>
      character.charCodeAt(0),
    );
    sendInput(bytes);
  });
  resizeObserver = new ResizeObserver(scheduleFit);
  resizeObserver.observe(container.value);
  fit();
  attach();
});

onBeforeUnmount(() => {
  closing = true;
  sendRealtime("ssh.session.close", { session_id: props.session.session_id });
  if (resizeFrame) cancelAnimationFrame(resizeFrame);
  resizeObserver?.disconnect();
  terminal?.dispose();
  terminal = null;
  fitAddon = null;
});
</script>

<template>
  <section
    class="relative h-full min-h-0 overflow-hidden bg-[var(--ssh-terminal-bg)]"
  >
    <div
      ref="container"
      class="h-full w-full overflow-hidden p-3"
      aria-label="SSH 交互终端"
    />
    <div
      v-if="status !== 'online'"
      class="pointer-events-none absolute right-4 top-3 rounded-full border border-white/10 bg-black/55 px-3 py-1 text-[11px] text-white/70 backdrop-blur"
    >
      {{ statusMessage }}
    </div>
  </section>
</template>
