<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { ArrowRight, ShieldCheck, Sparkles, Sun, Moon } from "lucide-vue-next";
import { api } from "@/services/api";
import { setupApi, type SetupStatus } from "@/services/setup";
import type { RuntimeResponse } from "@/services/types";
import Button from "@/components/ui/Button.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import ClashAPIClient, {
  type ProxyNode,
  type ProxyGroup,
} from "@/services/clash";

const setup = ref<SetupStatus | null>(null);
const runtime = ref<RuntimeResponse | null>(null);
const subscriptionURL = ref("");
const loading = ref(true);
const busy = ref(false);
const error = ref("");
const readError = ref("");
const proxyError = ref("");
const proxy = ref<ProxyNode | ProxyGroup | null>(null);
const selectedProxy = ref("");
const clash = new ClashAPIClient();
const notice = ref("");
const syncing = ref(false);
let syncStartedAt = 0;
let syncDeadline = 0;
let syncRevision = 0;
const syncBaseline = new Map<number, number>();
const theme = ref(
  localStorage.getItem("ackwrap.theme") === "light" ? "light" : "dark",
);
let timer: ReturnType<typeof setTimeout> | undefined;
let disposed = false;
let mutation = 0;
const running = computed(() => setup.value?.status === "running");
const unmanaged = computed(
  () => setup.value?.has_existing_config && !setup.value.configured,
);
const canSetup = computed(
  () =>
    setup.value?.supported &&
    !setup.value.configured &&
    !unmanaged.value &&
    !running.value &&
    !busy.value,
);
const rules = [
  { name: "广告拦截", detail: "常见广告与追踪域名", policy: "默认拦截" },
  { name: "国内 / CN", detail: "国内 IP 流量", policy: "内核 bypass" },
  { name: "AI", detail: "ChatGPT · Claude · Gemini", policy: "代理" },
  { name: "视频", detail: "YouTube · Netflix · Disney+", policy: "代理" },
  { name: "Google", detail: "搜索 · Gmail · Drive", policy: "代理" },
  { name: "社交", detail: "Telegram · X · Facebook", policy: "代理" },
  { name: "开发", detail: "GitHub · Docker", policy: "代理" },
  {
    name: "局域网 / 其他",
    detail: "局域网直连，其他流量走代理",
    policy: "自动分流",
  },
];

watch(
  theme,
  (value) => {
    document.documentElement.dataset.theme = value;
    localStorage.setItem("ackwrap.theme", value);
  },
  { immediate: true },
);

function professional() {
  localStorage.setItem("ackwrap.ui-mode", "professional");
}

const { connected } = useRealtimeSocket((event) => {
  if (event.type === "config.reconcile") {
    const data = event.data as { status?: string; error?: string };
    if (data.status === "failed") {
      notice.value = "";
      error.value = "配置自动应用失败，请在专业模式查看日志并重新应用配置。";
    }
    return;
  }
  if (!syncing.value || event.type !== "subscription.sync_all") return;
  const data = event.data as { status?: string; failed?: number };
  if (data.status !== "completed") return;
  syncRevision++;
  syncing.value = false;
  syncStartedAt = 0;
  if (data.failed) {
    notice.value = "";
    error.value = `${data.failed} 个订阅更新失败，请重试或在专业模式查看订阅错误。`;
  } else notice.value = "订阅更新完成";
});

async function checkSubscriptions() {
  const revision = mutation;
  const syncVersion = syncRevision;
  const subscriptions = (await api.getSubscriptions()).filter(
    (item) => item.url !== "manual://local",
  );
  if (disposed || revision !== mutation || syncVersion !== syncRevision) return;
  if (subscriptions.some((item) => item.sync_status === "syncing")) {
    syncing.value = true;
    return;
  }
  if (!syncing.value) return;
  if (
    syncStartedAt &&
    subscriptions.length &&
    subscriptions.every(
      (item) =>
        item.last_sync_at > (syncBaseline.get(item.id) ?? item.last_sync_at),
    )
  ) {
    syncRevision++;
    syncing.value = false;
    syncStartedAt = 0;
    const failed = subscriptions.filter(
      (item) => item.sync_status === "failed",
    ).length;
    if (failed) {
      notice.value = "";
      error.value = `${failed} 个订阅更新失败，请在专业模式查看详情。`;
    } else notice.value = "订阅更新完成";
  } else if (!syncStartedAt || Date.now() > syncDeadline) {
    syncRevision++;
    syncing.value = false;
    syncStartedAt = 0;
    notice.value = "";
    error.value =
      "订阅当前未在同步，但未收到本次更新的完整结果，请在专业模式确认或重试。";
  }
}

async function refresh() {
  const revision = mutation;
  try {
    const status = await setupApi.status();
    if (disposed || revision !== mutation) return;
    setup.value = status;
    const currentRuntime = await api.getRuntime();
    if (disposed || revision !== mutation) return;
    runtime.value = currentRuntime;
    readError.value = "";
    if (status.configured && !running.value) await checkSubscriptions();
    if (disposed || revision !== mutation) return;
    if (
      status.configured &&
      runtime.value.status === "running" &&
      !busy.value
    ) {
      try {
        const current = await clash.getProxy("默认代理");
        if (disposed || revision !== mutation) return;
        if (
          !selectedProxy.value ||
          selectedProxy.value === proxy.value?.now ||
          !current.all?.includes(selectedProxy.value)
        )
          selectedProxy.value = current.now || "";
        proxy.value = current;
        proxyError.value = "";
      } catch {
        proxy.value = null;
        proxyError.value = "节点状态读取失败，请重试或在专业模式检查代理服务。";
      }
    } else if (runtime.value.status !== "running") {
      proxy.value = null;
    }
  } catch (cause) {
    if (!disposed)
      readError.value =
        cause instanceof Error ? cause.message : "状态读取失败，请重试";
  } finally {
    loading.value = false;
  }
}

async function changeProxy() {
  if (!selectedProxy.value || busy.value || running.value) return;
  busy.value = true;
  mutation++;
  error.value = "";
  notice.value = "";
  const changed: { name: string; previous: string }[] = [];
  try {
    const root = await clash.getProxy("proxy");
    if (root.now !== "默认代理")
      throw new Error("主代理已在专业模式修改，请在专业模式调整节点。");
    const groups = await Promise.all(
      ["默认代理", "AI", "视频", "Google", "社交", "开发"].map((name) =>
        clash.getProxy(name),
      ),
    );
    if (
      groups.some(
        (group) => !group.now || !group.all?.includes(selectedProxy.value),
      )
    )
      throw new Error("部分应用分流已在专业模式修改，请在专业模式调整节点。");
    for (const group of groups) {
      await clash.selectProxy(group.name, selectedProxy.value);
      changed.push({ name: group.name, previous: group.now! });
    }
    notice.value = "默认代理及 AI、视频、Google、社交、开发节点已切换";
  } catch (cause) {
    error.value =
      cause instanceof Error ? cause.message : "节点切换失败，请重试";
    const rollback = await Promise.allSettled(
      changed.map((group) => clash.selectProxy(group.name, group.previous)),
    );
    if (rollback.some((result) => result.status === "rejected"))
      error.value += " 部分节点选择未能恢复，请在专业模式检查各应用分流。";
    else if (changed.length) error.value += " 已恢复本次操作前的节点选择。";
  } finally {
    busy.value = false;
    await refresh();
  }
}

async function poll() {
  await refresh();
  if (!disposed) timer = setTimeout(poll, running.value ? 1500 : 5000);
}

async function startSetup() {
  if (!canSetup.value) return;
  busy.value = true;
  mutation++;
  error.value = "";
  notice.value = "";
  try {
    setup.value = await setupApi.start(subscriptionURL.value.trim());
    subscriptionURL.value = "";
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "配置失败，请重试";
  } finally {
    busy.value = false;
  }
}

async function action(kind: "toggle" | "update") {
  if (busy.value || running.value || syncing.value || !setup.value?.configured)
    return;
  busy.value = true;
  mutation++;
  error.value = "";
  notice.value = "";
  try {
    if (kind === "update") {
      const subscriptions = (await api.getSubscriptions()).filter(
        (item) => item.url !== "manual://local",
      );
      if (!subscriptions.length) throw new Error("没有可更新的订阅");
      if (subscriptions.some((item) => item.sync_status === "syncing")) {
        syncing.value = true;
        notice.value = "订阅正在更新，请稍候";
        return;
      }
      syncBaseline.clear();
      subscriptions.forEach((item) =>
        syncBaseline.set(item.id, item.last_sync_at),
      );
      syncRevision++;
      syncing.value = true;
      syncStartedAt = Date.now();
      syncDeadline =
        syncStartedAt +
        Math.max(
          120000,
          subscriptions.reduce(
            (sum, item) => sum + Math.max(60, item.sync_timeout_seconds) * 1000,
            30000,
          ),
        );
      const result = await api.syncAllSubscriptions();
      if (!result.success) throw new Error(result.message || "订阅更新失败");
      if (syncing.value) notice.value = "订阅更新已开始，请稍候";
    } else if (runtime.value?.status === "running") {
      const result = await api.stopCore();
      if (!result.success) throw new Error(result.message || "停止失败");
      notice.value = "已停止代理服务";
    } else {
      const result = await api.startCore();
      if (!result.success) throw new Error(result.message || "启动失败");
      notice.value = "已提交启动请求";
    }
    await refresh();
  } catch (cause) {
    if (kind === "update") {
      syncRevision++;
      syncing.value = false;
      syncStartedAt = 0;
    }
    error.value = cause instanceof Error ? cause.message : "操作失败，请重试";
  } finally {
    busy.value = false;
  }
}

onMounted(() => {
  localStorage.setItem("ackwrap.ui-mode", "simple");
  void poll();
});
onBeforeUnmount(() => {
  disposed = true;
  clearTimeout(timer);
});
</script>

<template>
  <div class="simple-shell min-h-screen text-[var(--text-primary)]">
    <header
      class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 px-5 py-5 sm:px-8"
    >
      <RouterLink
        to="/simple"
        class="flex items-center gap-3 text-lg font-semibold"
      >
        <img src="/favicon.png" alt="" class="h-9 w-9" /> AckWrap
        <span class="text-xs font-normal text-[var(--text-secondary)]"
          >简易模式</span
        >
      </RouterLink>
      <div class="flex items-center gap-3">
        <button
          class="simple-theme"
          :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
          @click="theme = theme === 'dark' ? 'light' : 'dark'"
        >
          <Sun v-if="theme === 'dark'" :size="18" /><Moon v-else :size="18" />
        </button>
        <RouterLink
          to="/control"
          class="flex items-center gap-1 text-sm text-[var(--text-secondary)]"
          @click="professional"
          >专业模式 <ArrowRight :size="15"
        /></RouterLink>
      </div>
    </header>

    <main
      id="main-content"
      class="mx-auto max-w-5xl px-5 pb-12 pt-7 sm:px-8 sm:pt-12"
    >
      <div class="mb-8">
        <p class="mb-3 text-sm font-medium text-[var(--color-primary-hover)]">
          OPENWRT · 家庭网络
        </p>
        <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">
          让上网配置简单一点
        </h1>
        <p class="mt-4 max-w-xl text-sm leading-7 text-[var(--text-secondary)]">
          添加订阅，自动完成节点、分流、DNS 和网络接管。适用于 OpenWrt
          主路由；旁路由需要终端将网关与 DNS 指向本机。
        </p>
      </div>

      <div v-if="error || readError" role="alert" class="simple-alert mb-5">
        <p v-if="error" class="break-words">{{ error }}</p>
        <p v-if="readError" class="break-words">{{ readError }}</p>
        <button class="mt-2 underline" :disabled="busy" @click="refresh">
          重新读取状态
        </button>
      </div>
      <p
        v-if="notice"
        role="status"
        class="mb-5 text-sm text-[var(--color-success)]"
      >
        {{ notice }}
      </p>

      <section
        class="simple-card"
        aria-labelledby="setup-title"
        :aria-busy="loading || running || busy"
      >
        <p v-if="loading" role="status" class="text-[var(--text-secondary)]">
          正在读取路由器状态…
        </p>
        <template v-else-if="setup">
          <div class="mb-5 flex items-start gap-3">
            <Sparkles
              :size="23"
              class="mt-1 shrink-0 text-[var(--color-primary-hover)]"
            />
            <div>
              <h2 id="setup-title" class="text-xl font-semibold">
                {{ setup.configured ? "你的家庭网络" : "一步完成配置" }}
              </h2>
              <p class="mt-2 text-sm text-[var(--text-secondary)]">
                {{
                  setup.configured
                    ? "配置已保存，日常只需开启或更新订阅。"
                    : "内置常用规则，广告拦截与 CN 内核 bypass 默认启用。"
                }}
              </p>
            </div>
          </div>
          <p v-if="!setup.supported" class="simple-alert">
            当前系统不支持 OpenWrt 一键配置，请使用专业模式。
          </p>
          <p v-else-if="unmanaged" class="simple-alert">
            检测到已有专业配置。请在专业模式中继续管理，简易向导不会覆盖现有配置。
          </p>
          <div
            v-if="running"
            role="status"
            aria-live="polite"
            class="rounded-xl bg-[var(--color-primary-bg)] p-5"
          >
            <h3 class="font-semibold">正在配置，请稍候</h3>
            <p class="mt-2 text-sm">{{ setup.stage || "正在准备配置" }}</p>
            <p class="mt-3 text-xs text-[var(--text-secondary)]">
              可以离开或刷新页面，返回后会继续显示进度。
            </p>
          </div>
          <div
            v-else-if="setup.status === 'failed'"
            role="alert"
            class="simple-alert mb-5"
          >
            <p class="font-medium">配置未完成</p>
            <p class="mt-2 break-words">
              {{ setup.error || "请重试，或前往专业模式查看日志。" }}
            </p>
          </div>
          <div v-if="setup.configured && !running" class="space-y-5">
            <div role="status">
              <p class="text-lg font-medium">
                {{
                  runtime?.status === "running"
                    ? "代理服务运行中"
                    : runtime?.status === "stopped"
                      ? "代理服务已停止"
                      : "代理服务尚未就绪"
                }}
              </p>
              <p
                v-if="setup.status === 'succeeded'"
                class="mt-2 text-sm text-[var(--text-secondary)]"
              >
                一键配置已完成。服务状态不代表所有网站均可访问。
              </p>
            </div>
            <div class="flex flex-wrap gap-3">
              <Button
                :disabled="busy || syncing || !runtime"
                @click="action('toggle')"
                >{{
                  runtime?.status === "running" ? "关闭代理" : "开启代理"
                }}</Button
              >
              <Button
                variant="secondary"
                :disabled="busy || syncing"
                @click="action('update')"
                >{{ syncing ? "更新中…" : "更新订阅" }}</Button
              >
            </div>
            <p
              v-if="syncing && !connected"
              role="status"
              class="text-sm text-[var(--text-secondary)]"
            >
              进度连接正在恢复，正在定期查询订阅状态。
            </p>
            <div v-if="proxy?.all?.length">
              <label for="proxy-node" class="mb-2 block text-sm font-medium"
                >默认代理节点</label
              >
              <div class="flex flex-wrap items-center gap-3">
                <select
                  id="proxy-node"
                  v-model="selectedProxy"
                  :disabled="busy"
                  class="simple-input min-w-0 flex-1"
                >
                  <option v-for="name in proxy.all" :key="name" :value="name">
                    {{ name }}
                  </option>
                </select>
                <Button
                  :disabled="busy || selectedProxy === proxy.now"
                  @click="changeProxy"
                  >切换节点</Button
                >
              </div>
              <p class="mt-2 text-xs text-[var(--text-secondary)]">
                默认自动选择节点。此操作同时切换
                AI、视频、Google、社交和开发分类的节点。
              </p>
            </div>
            <p
              v-if="proxyError"
              role="alert"
              class="text-sm text-[var(--color-warning)]"
            >
              {{ proxyError }}
            </p>
          </div>
          <form
            v-if="
              !setup.configured && setup.supported && !unmanaged && !running
            "
            class="mt-6"
            @submit.prevent="startSetup"
          >
            <label for="subscription-url" class="mb-2 block text-sm font-medium"
              >订阅链接</label
            >
            <input
              id="subscription-url"
              v-model="subscriptionURL"
              type="url"
              :required="!setup.can_retry"
              autocomplete="off"
              spellcheck="false"
              :placeholder="
                setup.can_retry ? '留空使用已保存的订阅' : 'https://…'
              "
              class="simple-input w-full"
              :disabled="busy"
            />
            <p class="mb-5 mt-2 text-xs text-[var(--text-secondary)]">
              {{
                setup.can_retry
                  ? "留空使用已保存的订阅，填写新地址可替换未完成的订阅；已应用的配置仅重试启动。"
                  : "使用服务商提供的订阅链接，请勿分享给他人。"
              }}
            </p>
            <Button
              type="submit"
              :disabled="
                !canSetup || (!setup.can_retry && !subscriptionURL.trim())
              "
              >{{
                busy
                  ? "正在提交…"
                  : setup.status === "failed"
                    ? "重新配置并启用"
                    : "一键配置并启用"
              }}</Button
            >
          </form>
        </template>
      </section>

      <section class="mt-8" aria-labelledby="rules-title">
        <div class="mb-4 flex items-center gap-2">
          <ShieldCheck :size="19" class="text-[var(--color-primary-hover)]" />
          <h2 id="rules-title" class="font-semibold">一键配置包含这些规则</h2>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <div
            v-for="rule in rules"
            :key="rule.name"
            class="flex items-center justify-between gap-3 rounded-xl border border-[var(--border-light)] bg-[var(--bg-surface)] p-4"
          >
            <div class="min-w-0">
              <p class="text-sm font-medium">{{ rule.name }}</p>
              <p class="mt-1 text-xs leading-5 text-[var(--text-secondary)]">
                {{ rule.detail }}
              </p>
            </div>
            <span
              class="shrink-0 rounded-full bg-[var(--color-primary-bg)] px-3 py-1 text-xs text-[var(--color-primary-hover)]"
              >{{ rule.policy }}</span
            >
          </div>
        </div>
        <p class="mt-4 text-xs leading-6 text-[var(--text-secondary)]">
          应用分类默认跟随代理节点。更细的节点与分流设置可在专业模式调整；切换界面不会修改配置。
        </p>
      </section>
    </main>
  </div>
</template>

<style scoped>
.simple-shell {
  background: var(--app-bg-image);
}
.simple-card {
  padding: 28px;
  border: 1px solid var(--border-default);
  border-radius: 18px;
  background: var(--bg-surface);
  box-shadow: var(--shadow-card);
}
.simple-alert {
  padding: 16px;
  border-radius: 12px;
  background: var(--color-warning-bg);
  color: var(--text-primary);
  font-size: 14px;
  line-height: 1.7;
  overflow-wrap: anywhere;
}
.simple-input {
  min-height: 48px;
  padding: 10px 14px;
  border: 1px solid var(--border-default);
  border-radius: 8px;
  background: var(--bg-base);
  color: var(--text-primary);
}
.simple-input:focus-visible,
.simple-theme:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 3px;
}
.simple-theme {
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border: 1px solid var(--border-default);
  border-radius: 50%;
}
@media (max-width: 480px) {
  .simple-card {
    padding: 20px;
  }
}
</style>
