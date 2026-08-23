<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import {
  ExternalLink,
  PanelLeft,
  RefreshCw,
  ShieldAlert,
  SquareTerminal,
  X,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import { ApiRequestError } from "@/services/api";
import { authenticatedFetch } from "@/services/apiAuth";
import { sshApi } from "@/services/sshApi";
import type {
  SSHHost,
  SSHHostKeyChallenge,
  SSHSessionCreateResponse,
} from "@/services/sshTypes";
import SFTPFileManager from "./ssh/SFTPFileManager.vue";
import SSHTerminalPane from "./ssh/SSHTerminalPane.vue";

type TerminalStatus = "connecting" | "online" | "offline" | "error";

const route = useRoute();
const hostID = Number(route.params.hostId);
const host = ref<SSHHost | null>(null);
const session = ref<SSHSessionCreateResponse | null>(null);
const loading = ref(true);
const error = ref("");
const challenge = ref<SSHHostKeyChallenge | null>(null);
const rotateHostKey = ref(false);
const trusting = ref(false);
const terminalStatus = ref<TerminalStatus>("connecting");
const terminalStatusMessage = ref("正在建立 SSH 会话...");
const sftpWidth = ref(
  Math.max(
    300,
    Math.min(560, Number(localStorage.getItem("ackwrap.sftp.width")) || 380),
  ),
);
let resizing = false;

const statusClass = computed(() => ({
  "bg-emerald-400": terminalStatus.value === "online",
  "bg-amber-400": terminalStatus.value === "connecting",
  "bg-red-400": terminalStatus.value === "error",
  "bg-gray-400": terminalStatus.value === "offline",
}));

function applyPageTheme() {
  const theme = localStorage.getItem("ackwrap.theme");
  document.documentElement.dataset.theme = theme === "light" ? "light" : "dark";
}

async function closeSession(current = session.value) {
  if (!current) return;
  session.value = null;
  try {
    await sshApi.closeSession(current.session_id);
  } catch (cause) {
    if (
      !(cause instanceof ApiRequestError) ||
      cause.code !== "SSH_SESSION_NOT_FOUND"
    ) {
      console.error("[SSH] close session failed", cause);
    }
  }
}

async function connect() {
  if (!Number.isInteger(hostID) || hostID <= 0) {
    error.value = "SSH 主机 ID 无效";
    loading.value = false;
    return;
  }
  loading.value = true;
  error.value = "";
  challenge.value = null;
  terminalStatus.value = "connecting";
  terminalStatusMessage.value = "正在建立 SSH 会话...";
  await closeSession();
  try {
    host.value = await sshApi.getHost(hostID);
    document.title = `SSH · ${host.value.name}`;
    session.value = await sshApi.createSession(hostID, 120, 36);
  } catch (cause) {
    if (
      cause instanceof ApiRequestError &&
      ["SSH_HOST_KEY_UNKNOWN", "SSH_HOST_KEY_CHANGED"].includes(cause.code)
    ) {
      const details = cause.details as SSHHostKeyChallenge | undefined;
      if (details?.challenge_id && details.fingerprint_sha256) {
        challenge.value = details;
        rotateHostKey.value = cause.code === "SSH_HOST_KEY_CHANGED";
      } else {
        error.value = cause.message;
      }
    } else {
      error.value = cause instanceof Error ? cause.message : "SSH 会话创建失败";
    }
    terminalStatus.value = "error";
    terminalStatusMessage.value = error.value || "等待确认 Host Key";
  } finally {
    loading.value = false;
  }
}

async function trustAndConnect() {
  if (!challenge.value || trusting.value) return;
  trusting.value = true;
  try {
    const payload = {
      challenge_id: challenge.value.challenge_id,
      fingerprint_sha256: challenge.value.fingerprint_sha256,
    };
    if (rotateHostKey.value) await sshApi.rotateHostKey(hostID, payload);
    else await sshApi.trustHostKey(hostID, payload);
    await connect();
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "Host Key 更新失败";
  } finally {
    trusting.value = false;
  }
}

function updateTerminalStatus(state: TerminalStatus, message: string) {
  terminalStatus.value = state;
  terminalStatusMessage.value = message;
}

function startResize(event: PointerEvent) {
  if (window.innerWidth < 900) return;
  resizing = true;
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  window.addEventListener("pointermove", resize);
  window.addEventListener("pointerup", stopResize, { once: true });
}

function resize(event: PointerEvent) {
  if (!resizing) return;
  sftpWidth.value = Math.max(
    280,
    Math.min(window.innerWidth * 0.58, event.clientX),
  );
}

function stopResize() {
  if (!resizing) return;
  resizing = false;
  localStorage.setItem(
    "ackwrap.sftp.width",
    String(Math.round(sftpWidth.value)),
  );
  window.removeEventListener("pointermove", resize);
}

function closeTab() {
  window.close();
  window.setTimeout(() => {
    if (!window.closed) window.location.href = "/advanced/ssh";
  }, 100);
}

function closeOnPageHide() {
  const current = session.value;
  if (!current) return;
  void authenticatedFetch(
    `/api/v1/advanced/ssh/sessions/${encodeURIComponent(current.session_id)}`,
    { method: "DELETE", credentials: "same-origin", keepalive: true },
  );
}

onMounted(() => {
  applyPageTheme();
  window.addEventListener("pagehide", closeOnPageHide);
  void connect();
});

onBeforeUnmount(() => {
  window.removeEventListener("pagehide", closeOnPageHide);
  window.removeEventListener("pointermove", resize);
  window.removeEventListener("pointerup", stopResize);
  void closeSession();
});
</script>

<template>
  <main
    class="flex h-screen min-h-[520px] flex-col overflow-hidden bg-[var(--bg-base)] text-[var(--text-primary)]"
  >
    <header
      class="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-[var(--border-default)] bg-[var(--header-bg)] px-3 sm:px-4"
    >
      <div class="flex min-w-0 items-center gap-3">
        <div
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-lg)] bg-[var(--color-primary-bg)] text-[var(--color-primary)]"
        >
          <SquareTerminal :size="19" />
        </div>
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <h1 class="truncate text-sm font-semibold">
              {{ host?.name || "SSH 工作台" }}
            </h1>
            <span
              class="hidden rounded-full bg-[var(--bg-sidebar-hover)] px-2 py-0.5 text-[10px] text-[var(--text-secondary)] sm:inline"
            >
              独立会话
            </span>
          </div>
          <p
            v-if="host"
            class="truncate font-mono text-[11px] text-[var(--text-tertiary)]"
          >
            {{ host.username }}@{{ host.host }}:{{ host.port }} ·
            {{
              host.connection_mode === "direct"
                ? "直连"
                : host.node_exposure_name
            }}
          </p>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <div
          class="hidden max-w-56 items-center gap-2 rounded-full border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 py-1.5 text-xs sm:flex"
          :title="terminalStatusMessage"
        >
          <span class="h-2 w-2 shrink-0 rounded-full" :class="statusClass" />
          <span class="truncate">{{ terminalStatusMessage }}</span>
        </div>
        <Button size="sm" :disabled="loading" title="重新连接" @click="connect">
          <template #icon><RefreshCw :size="13" /></template>
          <span class="hidden sm:inline">重连</span>
        </Button>
        <a
          href="/advanced/ssh"
          target="_blank"
          class="btn-press focus-ring inline-flex h-7 items-center justify-center gap-1.5 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--button-secondary-bg)] px-3 text-xs font-medium"
        >
          <ExternalLink :size="13" /><span class="hidden sm:inline"
            >主机管理</span
          >
        </a>
        <button
          class="aw-modal-close inline-flex h-8 w-8 items-center justify-center"
          title="关闭标签页"
          @click="closeTab"
        >
          <X :size="16" />
        </button>
      </div>
    </header>

    <section
      v-if="session"
      class="ssh-workspace min-h-0 flex-1"
      :style="{ '--sftp-width': `${sftpWidth}px` }"
    >
      <SFTPFileManager :session="session" />
      <div
        class="ssh-workspace-divider"
        title="拖动调整 SFTP 面板宽度"
        @pointerdown="startResize"
      >
        <PanelLeft :size="12" />
      </div>
      <SSHTerminalPane
        :session="session"
        @status="updateTerminalStatus"
        @closed="terminalStatus = 'offline'"
      />
    </section>

    <section v-else class="flex min-h-0 flex-1 items-center justify-center p-5">
      <div
        v-if="loading"
        class="text-center text-sm text-[var(--text-secondary)]"
      >
        <RefreshCw
          :size="24"
          class="mx-auto mb-3 animate-spin text-[var(--color-primary)]"
        />
        正在建立安全 SSH 连接...
      </div>

      <div
        v-else-if="challenge"
        class="w-full max-w-xl rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-6 shadow-[var(--shadow-card)]"
      >
        <div class="flex items-start gap-3">
          <ShieldAlert
            :size="24"
            class="mt-0.5 shrink-0 text-[var(--color-warning)]"
          />
          <div>
            <h2 class="font-semibold">
              {{
                rotateHostKey
                  ? "远端 Host Key 已变化"
                  : "首次连接需要确认 Host Key"
              }}
            </h2>
            <p class="mt-1 text-sm text-[var(--text-secondary)]">
              请通过可信渠道核对指纹。确认之前不会发送 SSH 用户凭据。
            </p>
          </div>
        </div>
        <dl class="mt-5 space-y-3 text-sm">
          <div>
            <dt class="text-xs text-[var(--text-tertiary)]">算法</dt>
            <dd class="mt-1 font-mono">{{ challenge.key_type }}</dd>
          </div>
          <div>
            <dt class="text-xs text-[var(--text-tertiary)]">当前指纹</dt>
            <dd
              class="mt-1 break-all rounded-[var(--radius-lg)] bg-[var(--bg-base)] p-3 font-mono text-xs"
            >
              {{ challenge.fingerprint_sha256 }}
            </dd>
          </div>
          <div v-if="challenge.trusted_fingerprint">
            <dt class="text-xs text-[var(--color-error)]">原信任指纹</dt>
            <dd class="mt-1 break-all font-mono text-xs">
              {{ challenge.trusted_fingerprint }}
            </dd>
          </div>
        </dl>
        <div class="mt-6 flex justify-end gap-2">
          <Button @click="closeTab">取消</Button>
          <Button
            :variant="rotateHostKey ? 'danger' : 'primary'"
            :loading="trusting"
            @click="trustAndConnect"
          >
            {{ rotateHostKey ? "确认轮换并连接" : "信任并连接" }}
          </Button>
        </div>
      </div>

      <div v-else class="w-full max-w-md text-center">
        <SquareTerminal :size="36" class="mx-auto text-[var(--color-error)]" />
        <h2 class="mt-4 font-semibold">SSH 工作台连接失败</h2>
        <p class="mt-2 text-sm text-[var(--text-secondary)]">
          {{ error || "会话已经关闭" }}
        </p>
        <div class="mt-5 flex justify-center gap-2">
          <Button @click="closeTab">关闭</Button>
          <Button variant="primary" @click="connect">重新连接</Button>
        </div>
      </div>
    </section>
  </main>
</template>

<style scoped>
.ssh-workspace {
  display: grid;
  grid-template-rows: minmax(250px, 42vh) minmax(0, 1fr);
}

.ssh-workspace-divider {
  display: none;
}

@media (min-width: 900px) {
  .ssh-workspace {
    grid-template-columns: var(--sftp-width) 7px minmax(0, 1fr);
    grid-template-rows: minmax(0, 1fr);
  }

  .ssh-workspace-divider {
    display: flex;
    cursor: col-resize;
    touch-action: none;
    align-items: center;
    justify-content: center;
    border-left: 1px solid var(--border-default);
    border-right: 1px solid var(--border-default);
    background: var(--bg-elevated);
    color: var(--text-tertiary);
  }

  .ssh-workspace-divider:hover {
    background: var(--color-primary-bg);
    color: var(--color-primary);
  }
}
</style>
