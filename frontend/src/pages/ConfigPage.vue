<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  CheckCircle2,
  ChevronDown,
  FileJson2,
  Play,
  RefreshCw,
  Save,
  XCircle,
} from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import JsonPreview from "@/components/JsonPreview.vue";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import { api } from "@/services/api";
import type {
  ConfigBackup,
  ConfigFileItem,
  ConfigGenerateRequest,
} from "@/services/types";
const request = ref<ConfigGenerateRequest | null>(null);
const generated = ref<any>(null),
  configFiles = ref<ConfigFileItem[]>([]),
  configBackups = ref<ConfigBackup[]>([]),
  loadingFiles = ref(true),
  generating = ref(false),
  applying = ref(false),
  applyDialog = ref(false),
  applyFileName = ref("config.json"),
  showFullPreview = ref(false),
  expandedModules = ref<Record<string, boolean>>({}),
  message = ref(""),
  messageType = ref<"success" | "error">("error");
const config = computed(() => generated.value?.config);
const activeConfig = computed(() =>
  configFiles.value.find((item) => item.active),
);
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
const formatSize = (value: number) =>
  value < 1024 ? `${value} B` : `${(value / 1024).toFixed(1)} KiB`;
const formatUpdatedAt = (value: number) =>
  value ? new Date(value).toLocaleString() : "-";
function backupSummary(configName: string) {
  const backups = configBackups.value.filter(
    (item) => item.config_name === configName,
  );
  return {
    count: backups.length,
    latest: backups[0]?.backup_date || "",
  };
}

async function loadConfigFiles() {
  loadingFiles.value = true;
  try {
    const [files, backups] = await Promise.all([
      api.getConfigFiles(),
      api.getConfigBackups(),
    ]);
    configFiles.value = files;
    configBackups.value = backups;
  } catch (e: any) {
    showMessage(`加载配置文件失败: ${e.message}`);
  } finally {
    loadingFiles.value = false;
  }
}

async function generate() {
  const currentRequest = request.value;
  if (!currentRequest) return showMessage("配置生成参数尚未加载");
  try {
    generating.value = true;
    const result = await api.generateConfig(currentRequest);
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
function openApplyDialog() {
  applyFileName.value = activeConfig.value?.name || "config.json";
  applyDialog.value = true;
}
async function apply() {
  if (applying.value) return;
  if (!generated.value?.valid) return showMessage("请先生成并校验通过配置");
  if (!applyFileName.value.trim()) return showMessage("请输入配置文件名");
  try {
    applying.value = true;
    await api.applyConfig({
      file_name: applyFileName.value.trim(),
      restart_core: true,
    });
    applyDialog.value = false;
    await loadConfigFiles();
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
  const [requestResult] = await Promise.allSettled([
    api.getConfigGenerateRequest(),
    loadConfigFiles(),
  ]);
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
        <Button
          variant="primary"
          size="lg"
          :loading="generating"
          :disabled="!request"
          @click="generate"
        >
          <Play :size="15" />生成完整配置
        </Button>
        <Button
          variant="primary"
          size="lg"
          :disabled="!generated?.valid || applying"
          @click="openApplyDialog"
        >
          <Save :size="15" />应用当前生成结果
        </Button>
      </div>
    </div>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <div class="grid gap-4 lg:grid-cols-2">
      <section
        class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <div class="flex items-start justify-between gap-3">
          <div>
            <h3 class="flex items-center gap-2 text-sm font-semibold">
              <FileJson2
                :size="16"
                class="text-[var(--color-primary)]"
              />配置文件
            </h3>
            <p class="mt-1 text-xs text-[var(--text-tertiary)]">
              当前使用：{{ activeConfig?.name || "未选择" }}
            </p>
          </div>
          <Button size="sm" :loading="loadingFiles" @click="loadConfigFiles">
            <RefreshCw :size="13" />刷新
          </Button>
        </div>
        <div class="aw-data-table-wrap mt-4 max-h-[262px]">
          <table class="aw-data-table min-w-[560px]">
            <thead>
              <tr>
                <th>文件名</th>
                <th>状态</th>
                <th>备份</th>
                <th>大小</th>
                <th>更新时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loadingFiles">
                <td colspan="5" class="text-center">加载中...</td>
              </tr>
              <tr v-else-if="!configFiles.length">
                <td colspan="5" class="text-center">暂无配置文件</td>
              </tr>
              <tr v-for="item in configFiles" v-else :key="item.path">
                <td class="font-mono">
                  {{ item.name }}
                  <span
                    v-if="item.active"
                    class="ml-1 rounded-full bg-[var(--color-primary-bg)] px-2 py-0.5 text-[10px] text-[var(--color-primary)]"
                  >
                    当前使用
                  </span>
                </td>
                <td
                  :class="
                    item.valid
                      ? 'text-[var(--color-success)]'
                      : 'text-[var(--color-error)]'
                  "
                  :title="item.error"
                >
                  {{ item.valid ? "校验通过" : "校验失败" }}
                </td>
                <td
                  class="whitespace-nowrap"
                  :title="
                    backupSummary(item.name).latest
                      ? `最近备份：${backupSummary(item.name).latest}`
                      : '暂无备份'
                  "
                >
                  {{
                    backupSummary(item.name).count
                      ? `${backupSummary(item.name).count} 天`
                      : "无"
                  }}
                </td>
                <td class="tabular-nums">{{ formatSize(item.size_bytes) }}</td>
                <td class="whitespace-nowrap">
                  {{ formatUpdatedAt(item.updated_at) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section
        class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
      >
        <h3 class="text-sm font-semibold">生成参数</h3>
        <p class="mt-1 text-xs text-[var(--text-tertiary)]">
          生成结果先写入临时文件并通过 sing-box check，应用时再命名保存。
        </p>
        <div v-if="request" class="mt-4 grid gap-3 sm:grid-cols-2">
          <label
            ><span class="text-xs">默认出站</span
            ><select
              v-model="request.default_outbound"
              class="aw-input mt-1 w-full"
            >
              <option value="proxy">proxy（策略）</option>
              <option value="direct">direct（直连）</option>
            </select></label
          ><label
            ><span class="text-xs">Mixed 监听地址</span
            ><input
              v-model="request.inbound_listen"
              class="aw-input mt-1 w-full" /></label
          ><label
            ><span class="text-xs">Mixed 监听端口</span
            ><input
              v-model.number="request.inbound_port"
              type="number"
              class="aw-input mt-1 w-full" /></label
          ><label
            ><span class="text-xs">TUN IPv4 地址</span
            ><input
              v-model="request.tun_ipv4_address"
              class="aw-input mt-1 w-full"
              placeholder="172.19.0.1/30"
            /><small class="mt-1 block text-[var(--text-tertiary)]"
              >使用 CIDR，需避开 LAN、Docker 和 VPN 已占用网段。</small
            ></label
          ><label
            ><span class="text-xs">TUN IPv6 地址</span
            ><input
              v-model="request.tun_ipv6_address"
              class="aw-input mt-1 w-full"
              placeholder="fdfe:dcba:9876::1/126"
            /><small class="mt-1 block text-[var(--text-tertiary)]"
              >使用 IPv6 CIDR；修改后需重新生成并应用配置。</small
            ></label
          ><label
            ><span class="text-xs">日志级别</span
            ><select v-model="request.log_level" class="aw-input mt-1 w-full">
              <option
                v-for="l in ['trace', 'debug', 'info', 'warn', 'error']"
                :key="l"
              >
                {{ l }}
              </option>
            </select></label
          >
        </div>
        <p v-else class="mt-4 text-xs text-[var(--text-tertiary)]">
          正在从后端加载配置生成参数...
        </p>
      </section>
    </div>
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
              class="flex w-full items-start justify-between gap-3 text-left"
              @click="toggleRow(index)"
            >
              <span
                ><b>{{ item.name }}</b
                ><small class="mt-1 block text-[var(--text-tertiary)]">{{
                  item.detail
                }}</small></span
              ><span class="flex shrink-0 items-center gap-2"
                ><button
                  v-if="expandedModules[item.key]"
                  @click.stop="
                    copy({ [item.key]: item.data ?? null }, item.key)
                  "
                >
                  复制</button
                ><span
                  class="inline-flex h-6 min-w-6 items-center justify-center rounded-full px-1.5 text-xs leading-none tabular-nums"
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
    <Modal
      :open="applyDialog"
      title="保存并应用配置"
      size="sm"
      :closable="!applying"
      @close="applyDialog = false"
    >
      <label class="text-sm">
        配置文件名
        <input
          v-model.trim="applyFileName"
          class="aw-input mt-1 w-full"
          maxlength="128"
          placeholder="例如：home.json"
          autofocus
          @keyup.enter="apply"
        />
      </label>
      <p class="mt-3 text-xs leading-5 text-[var(--text-tertiary)]">
        未填写扩展名时自动补充
        .json。文件将保存到本地配置目录并设为当前使用配置，然后重载核心。
      </p>
      <template #footer>
        <Button :disabled="applying" @click="applyDialog = false">取消</Button>
        <Button variant="primary" :loading="applying" @click="apply"
          >保存并应用</Button
        >
      </template>
    </Modal>
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
