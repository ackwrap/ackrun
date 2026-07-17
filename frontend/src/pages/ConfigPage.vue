<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  CheckCircle2,
  ChevronDown,
  Play,
  Save,
  XCircle,
} from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import JsonPreview from "@/components/JsonPreview.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Toast from "@/components/ui/Toast.vue";
import { api } from "@/services/api";
import type { ConfigGenerateRequest } from "@/services/types";
const request = ref<ConfigGenerateRequest>({
  default_outbound: "proxy",
  inbound_listen: "127.0.0.1",
  inbound_port: 7890,
  log_level: "info",
});
const generated = ref<any>(null),
  hasConfig = ref(false),
  generating = ref(false),
  applying = ref(false),
  confirmApply = ref(false),
  showFullPreview = ref(false),
  expandedModules = ref<Record<string, boolean>>({}),
  message = ref(""),
  messageType = ref<"success" | "error">("error");
const config = computed(() => generated.value?.config);
const moduleItems = computed(() => {
  const c = config.value,
    r = c?.route || {};
  return [
    {
      key: "log",
      name: "日志",
      count: c?.log ? 1 : 0,
      detail: c?.log?.level ? `level=${c.log.level}` : "未生成",
      data: c?.log,
      accent: "border-blue-400/50 bg-blue-500/5",
      badge: "bg-blue-500/10 text-blue-300",
    },
    {
      key: "inbounds",
      name: "入站",
      count: c?.inbounds?.length || 0,
      detail: "来自运行模式与本地监听设置",
      data: c?.inbounds,
      accent: "border-cyan-400/50 bg-cyan-500/5",
      badge: "bg-cyan-500/10 text-cyan-300",
    },
    {
      key: "outbounds",
      name: "出站/策略组",
      count: c?.outbounds?.length || 0,
      detail: "来自节点、节点组、策略组",
      data: c?.outbounds,
      accent: "border-emerald-400/50 bg-emerald-500/5",
      badge: "bg-emerald-500/10 text-emerald-300",
    },
    {
      key: "endpoints",
      name: "端点",
      count: c?.endpoints?.length || 0,
      detail: "WireGuard 等 endpoint 类型节点",
      data: c?.endpoints,
      accent: "border-teal-400/50 bg-teal-500/5",
      badge: "bg-teal-500/10 text-teal-300",
    },
    {
      key: "route.rules",
      name: "路由规则",
      count: r.rules?.length || 0,
      detail: "来自规则管理和策略组绑定",
      data: r.rules,
      accent: "border-amber-400/50 bg-amber-500/5",
      badge: "bg-amber-500/10 text-amber-300",
    },
    {
      key: "route.rule_set",
      name: "规则集",
      count: r.rule_set?.length || 0,
      detail: "来自 Geo/规则订阅自动生成",
      data: r.rule_set,
      accent: "border-orange-400/50 bg-orange-500/5",
      badge: "bg-orange-500/10 text-orange-300",
    },
    {
      key: "dns",
      name: "DNS",
      count: c?.dns?.servers?.length || 0,
      detail: c?.dns ? `${c.dns.rules?.length || 0} 条 DNS 规则` : "未启用",
      data: c?.dns,
      accent: "border-violet-400/50 bg-violet-500/5",
      badge: "bg-violet-500/10 text-violet-300",
    },
    {
      key: "ntp",
      name: "NTP",
      count: c?.ntp ? 1 : 0,
      detail: c?.ntp?.server || "未启用",
      data: c?.ntp,
      accent: "border-pink-400/50 bg-pink-500/5",
      badge: "bg-pink-500/10 text-pink-300",
    },
    {
      key: "experimental",
      name: "实验功能",
      count: c?.experimental ? Object.keys(c.experimental).length : 0,
      detail: "Clash API / Cache File",
      data: c?.experimental,
      accent: "border-slate-400/50 bg-slate-500/5",
      badge: "bg-slate-500/10 text-slate-300",
    },
  ];
});
function showMessage(v: string, t: "success" | "error" = "error") {
  message.value = v;
  messageType.value = t;
}
async function generate() {
  try {
    generating.value = true;
    const result = await api.generateConfig(request.value);
    generated.value = result;
    showMessage(
      result.valid
        ? "完整配置已生成并校验通过"
        : `配置校验失败: ${result.error}`,
      result.valid ? "success" : "error",
    );
  } catch (e: any) {
    showMessage(`生成失败: ${e.message}`);
  } finally {
    generating.value = false;
  }
}
async function apply() {
  if (!hasConfig.value) return showMessage("当前没有可应用的配置文件");
  if (!generated.value?.valid) return showMessage("请先生成并校验通过配置");
  try {
    applying.value = true;
    await api.applyConfig({ restart_core: true });
    confirmApply.value = false;
    showMessage("配置已应用", "success");
  } catch (e: any) {
    showMessage(`应用失败: ${e.message}`);
  } finally {
    applying.value = false;
  }
}
function toggleRow(index: number) {
  const row = moduleItems.value.slice(
      Math.floor(index / 2) * 2,
      Math.floor(index / 2) * 2 + 2,
    ),
    next = !row.every((i) => expandedModules.value[i.key]);
  row.forEach((i) => (expandedModules.value[i.key] = next));
}
async function copy(value: any, label: string) {
  try {
    await navigator.clipboard.writeText(JSON.stringify(value, null, 2));
    showMessage(`${label} 已复制`, "success");
  } catch (e: any) {
    showMessage(`复制失败: ${e.message || "浏览器不支持剪贴板"}`);
  }
}
onMounted(async () => {
  const [statusResult, requestResult] = await Promise.allSettled([
    api.getConfigStatus(),
    api.getConfigGenerateRequest(),
  ]);
  hasConfig.value =
    statusResult.status === "fulfilled" && statusResult.value.has_config;
  if (requestResult.status === "rejected") {
    showMessage(
      `加载配置生成参数失败: ${requestResult.reason?.message || "请求失败"}`,
    );
    return;
  }
  request.value = requestResult.value;
  await generate();
});
</script>
<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <PageHeader
        title="配置生成"
        description="从当前订阅、节点、策略组、路由规则、DNS、Geo 和运行设置生成完整 sing-box 配置"
      />
      <div class="flex gap-2">
        <button
          class="inline-flex h-9 items-center gap-2 rounded-md border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 text-sm"
          :disabled="generating"
          @click="generate"
        >
          <Play :size="15" />{{
            generating ? "生成中..." : "生成完整配置"
          }}</button
        ><button
          class="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-3 text-sm text-white disabled:opacity-50"
          :disabled="!hasConfig || !generated?.valid || applying"
          @click="confirmApply = true"
        >
          <Save :size="15" />{{ applying ? "应用中..." : "应用当前生成结果" }}
        </button>
      </div>
    </div>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
    >
      <h3 class="text-sm font-semibold">生成参数</h3>
      <div class="mt-4 grid gap-3">
        <label
          ><span class="text-xs">默认出站</span
          ><select
            v-model="request.default_outbound"
            class="mt-1 w-full rounded-md border bg-[var(--bg-base)] px-3 py-2"
          >
            <option value="proxy">proxy（策略）</option>
            <option value="direct">direct（直连）</option>
          </select></label
        ><label
          ><span class="text-xs">Mixed 监听地址</span
          ><input
            v-model="request.inbound_listen"
            class="mt-1 w-full rounded-md border bg-[var(--bg-base)] px-3 py-2" /></label
        ><label
          ><span class="text-xs">Mixed 监听端口</span
          ><input
            v-model.number="request.inbound_port"
            type="number"
            class="mt-1 w-full rounded-md border bg-[var(--bg-base)] px-3 py-2" /></label
        ><label
          ><span class="text-xs">日志级别</span
          ><select
            v-model="request.log_level"
            class="mt-1 w-full rounded-md border bg-[var(--bg-base)] px-3 py-2"
          >
            <option
              v-for="l in ['trace', 'debug', 'info', 'warn', 'error']"
              :key="l"
            >
              {{ l }}
            </option>
          </select></label
        >
      </div>
    </section>
    <section
      class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)]"
    >
      <div class="flex items-center justify-between border-b px-5 py-4">
        <div>
          <h3 class="text-sm font-semibold">模块化配置预览</h3>
          <p class="mt-1 text-xs text-[var(--text-tertiary)]">
            每块对应最终 sing-box JSON
            的一个顶层字段；生成结果仍会先写入临时文件并执行 sing-box check。
          </p>
        </div>
        <div v-if="generated" class="flex gap-2">
          <span
            :class="generated.valid ? 'text-emerald-300' : 'text-red-300'"
            class="inline-flex items-center gap-1"
          >
            <CheckCircle2 v-if="generated.valid" :size="14" />
            <XCircle v-else :size="14" />{{
              generated.valid ? "校验通过" : "校验失败"
            }} </span
          ><button
            v-if="generated.valid"
            class="rounded border px-3 text-xs"
            @click="showFullPreview = true"
          >
            预览最终配置文件
          </button>
        </div>
      </div>
      <div class="p-5">
        <div
          v-if="generated?.error"
          class="mb-4 rounded bg-red-500/10 p-3 text-red-300"
        >
          {{ generated.error }}
        </div>
        <div v-if="config" class="grid gap-4 xl:grid-cols-2">
          <div
            v-for="(item, index) in moduleItems"
            :key="item.key"
            class="overflow-hidden rounded-[var(--radius-xl)] border p-4"
            :class="item.accent"
          >
            <button
              class="flex w-full justify-between text-left"
              @click="toggleRow(index)"
            >
              <span
                ><b>{{ item.name }}</b
                ><small class="mt-1 block text-[var(--text-tertiary)]">{{
                  item.detail
                }}</small></span
              ><span class="flex gap-2"
                ><button
                  v-if="expandedModules[item.key]"
                  @click.stop="
                    copy({ [item.key]: item.data ?? null }, item.key)
                  "
                >
                  复制</button
                ><span
                  class="rounded-full px-2 py-1 text-xs"
                  :class="item.badge"
                  >{{ item.count }}</span
                >
                <ChevronDown
                  :size="16"
                  :class="{ 'rotate-180': expandedModules[item.key] }"
                />
              </span>
            </button>
            <JsonPreview
              v-if="expandedModules[item.key]"
              class="mt-3"
              :data="{ [item.key]: item.data ?? null }"
              max-height="320px"
            />
          </div>
        </div>
        <div v-else class="flex h-64 items-center justify-center">
          {{ generating ? "生成中..." : "暂无配置预览" }}
        </div>
      </div>
    </section>
    <ConfirmDialog
      :open="confirmApply"
      title="应用当前生成结果"
      message="将把已校验通过的临时配置覆盖为正式配置。应用前会备份当前配置。"
      confirm-text="应用配置"
      @confirm="apply"
      @cancel="confirmApply = false"
    />
    <div
      v-if="showFullPreview && config"
      class="aw-modal-backdrop"
      @click="showFullPreview = false"
    >
      <div class="aw-modal-panel max-h-[90vh] w-full max-w-6xl p-5" @click.stop>
        <div class="mb-4 flex justify-between">
          <div>
            <h3>最终配置文件</h3>
            <p class="text-xs">这是将写入 sing-box 的完整 JSON 配置。</p>
          </div>
          <div>
            <button class="mr-2" @click="copy(config, '完整配置')">复制</button
            ><button class="aw-modal-close" @click="showFullPreview = false">
              ✕
            </button>
          </div>
        </div>
        <JsonPreview :data="config" max-height="72vh" />
      </div>
    </div>
  </div>
</template>
