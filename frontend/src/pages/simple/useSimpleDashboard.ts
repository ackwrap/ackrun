import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { api } from "@/services/api";
import {
  setupApi,
  type SetupStatus,
  type SetupOptions,
} from "@/services/setup";
import type { RuntimeResponse, Subscription } from "@/services/types";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";
import ClashAPIClient, {
  type ProxyNode,
  type ProxyGroup,
} from "@/services/clash";

import type { UnwrapNestedRefs } from "vue";

export function useSimpleDashboard() {
  const setup = ref<SetupStatus | null>(null);
  const runtime = ref<RuntimeResponse | null>(null);
  const subscriptionURL = ref("");
  const tabs = [
    { id: "overview", label: "概览" },
    { id: "nodes", label: "订阅与节点" },
    { id: "rules", label: "分流规则" },
    { id: "network", label: "网络设置" },
  ] as const;
  const activeTab = ref<(typeof tabs)[number]["id"]>("overview");
  const options = ref<SetupOptions | null>(null);
  const savedOptions = ref("");
  const devicesText = ref("");
  const optionsError = ref("");
  const subscriptions = ref<Subscription[]>([]);
  const nodeCount = ref(0);
  const applicationGroups = ref<Record<string, ProxyNode | ProxyGroup>>({});
  const groupSelections = ref<Record<string, string>>({});
  const groupNames = ["默认代理", "AI", "视频", "Google", "社交", "开发"];
  const delayResult = ref<Record<string, string>>({});
  const loading = ref(true);
  const busy = ref(false);
  const error = ref("");
  const readError = ref("");
  const proxyError = ref("");
  const proxy = ref<ProxyNode | ProxyGroup | null>(null);
  const selectedProxy = ref("");
  const primaryGroupName = ref("默认代理");
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
  const canManage = computed(
    () => !!(setup.value?.configured || unmanaged.value),
  );
  const nodeGroups = computed(() =>
    Object.keys(applicationGroups.value)
      .filter((name) => name !== primaryGroupName.value)
      .map((name) => ({ name })),
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
    { name: "AI", detail: "ChatGPT · Claude · Gemini" },
    { name: "视频", detail: "YouTube · Netflix · Disney+" },
    { name: "Google", detail: "搜索 · Gmail · Drive" },
    { name: "社交", detail: "Telegram · X · Facebook" },
    { name: "开发", detail: "GitHub · Docker" },
  ];
  const optionsPayload = computed(() =>
    options.value
      ? {
          ...options.value,
          direct_devices: devicesText.value
            .split(/\r?\n/)
            .map((value) => value.trim())
            .filter(Boolean),
        }
      : null,
  );
  const optionsDirty = computed(
    () =>
      optionsPayload.value !== null &&
      JSON.stringify(optionsPayload.value) !== savedOptions.value,
  );
  const optionsDisabled = computed(
    () =>
      busy.value ||
      running.value ||
      syncing.value ||
      !setup.value?.supported ||
      unmanaged.value ||
      setup.value?.options_locked ||
      !!optionsError.value,
  );

  function activateTab(index: number) {
    const tab = tabs[(index + tabs.length) % tabs.length]!;
    activeTab.value = tab.id;
    document.getElementById(`simple-tab-${tab.id}`)?.focus();
  }

  function acceptOptions(value: SetupOptions) {
    options.value = value;
    devicesText.value = value.direct_devices.join("\n");
    savedOptions.value = JSON.stringify(value);
    optionsError.value = "";
  }

  async function persistOptions() {
    if (!optionsPayload.value) throw new Error("配置选项尚未读取，请稍后重试");
    acceptOptions(await setupApi.saveOptions(optionsPayload.value));
  }

  async function saveSettings() {
    if (optionsDisabled.value || !optionsDirty.value) return;
    busy.value = true;
    mutation++;
    error.value = "";
    notice.value = "";
    try {
      await persistOptions();
      notice.value = setup.value?.configured
        ? "设置已校验并保存，运行中的代理服务已重新加载。"
        : "选项已保存，填写订阅后一键配置即可使用。";
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : "保存设置失败";
    } finally {
      busy.value = false;
      await refresh();
    }
  }

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
    const currentSubscriptions = (await api.getSubscriptions()).filter(
      (item) => item.url !== "manual://local",
    );
    if (disposed || revision !== mutation || syncVersion !== syncRevision)
      return;
    subscriptions.value = currentSubscriptions;
    if (currentSubscriptions.some((item) => item.sync_status === "syncing")) {
      syncing.value = true;
      return;
    }
    if (!syncing.value) return;
    if (
      syncStartedAt &&
      currentSubscriptions.length &&
      currentSubscriptions.every(
        (item) =>
          item.last_sync_at > (syncBaseline.get(item.id) ?? item.last_sync_at),
      )
    ) {
      syncRevision++;
      syncing.value = false;
      syncStartedAt = 0;
      const failed = currentSubscriptions.filter(
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
      if (unmanaged.value) {
        options.value = null;
        savedOptions.value = "";
        optionsError.value = "";
      } else if (!busy.value && !running.value && !optionsDirty.value) {
        try {
          const nextOptions = await setupApi.options();
          if (!disposed && revision === mutation && !optionsDirty.value)
            acceptOptions(nextOptions);
        } catch (cause) {
          optionsError.value =
            cause instanceof Error ? cause.message : "配置选项读取失败";
        }
        if (disposed || revision !== mutation) return;
      }
      const currentRuntime = await api.getRuntime();
      if (disposed || revision !== mutation) return;
      runtime.value = currentRuntime;
      readError.value = "";
      if (canManage.value && !running.value) await checkSubscriptions();
      if (canManage.value && activeTab.value === "nodes") {
        const facets = await api.getNodeFacets();
        if (disposed || revision !== mutation) return;
        nodeCount.value = facets.total;
      }
      if (disposed || revision !== mutation) return;
      if (
        canManage.value &&
        runtime.value.status === "running" &&
        !busy.value
      ) {
        try {
          const { proxies } = await clash.getProxies();
          if (disposed || revision !== mutation) return;
          const names = unmanaged.value
            ? Object.keys(proxies).filter(
                (name) => proxies[name]?.type === "Selector",
              )
            : groupNames;
          const primary = names.includes("默认代理")
            ? "默认代理"
            : names.includes("proxy")
              ? "proxy"
              : names[0];
          const current = primary ? proxies[primary] : null;
          if (!primary || !current?.all?.length)
            throw new Error("可手动选择的节点组不可用");
          if (
            primaryGroupName.value !== primary ||
            !selectedProxy.value ||
            selectedProxy.value === proxy.value?.now ||
            !current.all?.includes(selectedProxy.value)
          )
            selectedProxy.value = current.now || "";
          primaryGroupName.value = primary;
          proxy.value = current;
          const nextGroups: Record<string, ProxyNode | ProxyGroup> = {};
          for (const name of names) {
            const group = proxies[name];
            if (!group?.all?.length) continue;
            nextGroups[name] = group;
            if (
              !groupSelections.value[name] ||
              groupSelections.value[name] ===
                applicationGroups.value[name]?.now ||
              !group.all.includes(groupSelections.value[name]!)
            )
              groupSelections.value[name] = group.now || "";
          }
          applicationGroups.value = nextGroups;
          proxyError.value = "";
        } catch {
          proxy.value = null;
          applicationGroups.value = {};
          proxyError.value =
            "节点状态读取失败，请重试或在专业模式检查代理服务。";
        }
      } else if (runtime.value.status !== "running") {
        proxy.value = null;
        applicationGroups.value = {};
      }
    } catch (cause) {
      if (!disposed)
        readError.value =
          cause instanceof Error ? cause.message : "状态读取失败，请重试";
    } finally {
      loading.value = false;
    }
  }

  async function changeProxy(name = primaryGroupName.value) {
    const selected =
      name === primaryGroupName.value
        ? selectedProxy.value
        : groupSelections.value[name];
    if (!selected || busy.value || running.value) return;
    busy.value = true;
    mutation++;
    error.value = "";
    notice.value = "";
    try {
      if (!unmanaged.value) {
        const root = await clash.getProxy("proxy");
        if (root.now !== "默认代理")
          throw new Error("主代理已在专业模式修改，请在专业模式调整节点。");
      }
      const group = await clash.getProxy(name);
      if (group.type !== "Selector" || !group.all?.includes(selected))
        throw new Error("节点组已改变，请重新读取状态");
      await clash.selectProxy(name, selected);
      notice.value = `${name}节点已切换`;
    } catch (cause) {
      error.value =
        cause instanceof Error ? cause.message : "节点切换失败，请重试";
    } finally {
      busy.value = false;
      await refresh();
    }
  }

  async function testProxy(name: string) {
    if (busy.value || running.value) return;
    busy.value = true;
    mutation++;
    delayResult.value[name] = "测试中…";
    try {
      const result = await clash.delayTest(name);
      delayResult.value[name] = `${result.delay} ms`;
    } catch {
      delayResult.value[name] = "测试失败，请更换节点或重试";
    } finally {
      busy.value = false;
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
      if (!setup.value?.options_locked) await persistOptions();
      setup.value = await setupApi.start(subscriptionURL.value.trim());
      subscriptionURL.value = "";
    } catch (cause) {
      error.value = cause instanceof Error ? cause.message : "配置失败，请重试";
    } finally {
      busy.value = false;
    }
  }

  async function action(kind: "toggle" | "update") {
    if (busy.value || running.value || syncing.value || !canManage.value)
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
              (sum, item) =>
                sum + Math.max(60, item.sync_timeout_seconds) * 1000,
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

  return {
    setup,
    runtime,
    subscriptionURL,
    tabs,
    activeTab,
    options,
    devicesText,
    optionsError,
    subscriptions,
    nodeCount,
    applicationGroups,
    groupSelections,
    delayResult,
    loading,
    busy,
    error,
    readError,
    proxyError,
    proxy,
    selectedProxy,
    primaryGroupName,
    nodeGroups,
    canManage,
    notice,
    syncing,
    theme,
    running,
    unmanaged,
    canSetup,
    rules,
    optionsDirty,
    optionsDisabled,
    activateTab,
    saveSettings,
    professional,
    connected,
    refresh,
    changeProxy,
    testProxy,
    startSetup,
    action,
  };
}

export type SimpleDashboard = UnwrapNestedRefs<
  ReturnType<typeof useSimpleDashboard>
>;
