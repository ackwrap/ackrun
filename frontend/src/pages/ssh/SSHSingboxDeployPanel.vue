<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import {
  Check,
  Clipboard,
  ExternalLink,
  KeyRound,
  LoaderCircle,
  Network,
  ServerCog,
  ShieldCheck,
} from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import StatusBadge from "@/components/ui/StatusBadge.vue";
import { sshApi } from "@/services/sshApi";
import type {
  SSHDeviceDetails,
  SSHHost,
  SSHSingboxDeployRequest,
  SSHSingboxDeployResponse,
} from "@/services/sshTypes";
import { writeClipboardText } from "@/utils/clipboard";
import { errorMessage } from "../advanced/advancedUi";

const props = defineProps<{
  host: SSHHost;
  details: SSHDeviceDetails | null;
  detailsError: string;
}>();
const emit = defineEmits<{ busy: [value: boolean] }>();

const deploying = ref(false);
const deployError = ref("");
const notice = ref("");
const noticeError = ref(false);
const result = ref<SSHSingboxDeployResponse | null>(null);
const regenerateConfirmed = ref(false);
const form = reactive<SSHSingboxDeployRequest>(emptyForm(props.host));

const detectedStatus = computed(() =>
  props.details?.software?.find((item) => item.key === "sing-box"),
);
const installedVersion = computed(
  () => result.value?.version || detectedStatus.value?.version || "",
);
const clientConfigText = computed(() =>
  result.value ? JSON.stringify(result.value.client_config, null, 2) : "",
);

watch(
  () => props.host.id,
  () => {
    Object.assign(form, emptyForm(props.host));
    deployError.value = "";
    notice.value = "";
    result.value = null;
    regenerateConfirmed.value = false;
  },
);

function emptyForm(host: SSHHost): SSHSingboxDeployRequest {
  return {
    server_address: host.host,
    vless_reality_enabled: true,
    vless_reality_port: 443,
    reality_server_name: "www.microsoft.com",
    shadowsocks_enabled: true,
    shadowsocks_port: 8388,
    replace_existing_config: false,
  };
}

function validate() {
  if (!form.server_address.trim()) return "请输入客户端可访问的公网 IP 或域名";
  if (!form.vless_reality_enabled && !form.shadowsocks_enabled) {
    return "至少启用一种协议";
  }
  if (
    form.vless_reality_enabled &&
    (!Number.isInteger(form.vless_reality_port) ||
      form.vless_reality_port < 1 ||
      form.vless_reality_port > 65535)
  ) {
    return "VLESS + REALITY 端口必须在 1-65535 之间";
  }
  if (form.vless_reality_enabled && !form.reality_server_name.trim()) {
    return "请输入 Reality 伪装域名";
  }
  if (
    form.shadowsocks_enabled &&
    (!Number.isInteger(form.shadowsocks_port) ||
      form.shadowsocks_port < 1 ||
      form.shadowsocks_port > 65535)
  ) {
    return "Shadowsocks 端口必须在 1-65535 之间";
  }
  if (
    form.vless_reality_enabled &&
    form.shadowsocks_enabled &&
    form.vless_reality_port === form.shadowsocks_port
  ) {
    return "两个协议不能使用相同端口";
  }
  return "";
}

async function deploy() {
  if (deploying.value || !props.details) return;
  deployError.value = validate();
  if (deployError.value) return;
  deploying.value = true;
  emit("busy", true);
  result.value = null;
  notice.value = "";
  try {
    result.value = await sshApi.deploySingbox(props.host.id, {
      ...form,
      server_address: form.server_address.trim(),
      reality_server_name: form.reality_server_name.trim(),
    });
    regenerateConfirmed.value = false;
    noticeError.value = false;
    notice.value = "部署完成。连接信息仅在本次结果中展示，请立即保存。";
  } catch (cause) {
    deployError.value = errorMessage(cause);
  } finally {
    deploying.value = false;
    emit("busy", false);
  }
}

async function copy(value: string, label: string) {
  try {
    await writeClipboardText(value);
    noticeError.value = false;
    notice.value = `${label}已复制`;
  } catch (cause) {
    noticeError.value = true;
    notice.value = errorMessage(cause);
  }
}
</script>

<template>
  <section class="border-t border-[var(--border-default)] pt-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex items-center gap-2">
          <ServerCog :size="18" class="text-[var(--color-primary)]" />
          <h3 class="text-sm font-semibold">sing-box 服务端部署</h3>
          <StatusBadge
            :status="detailsError ? 'error' : !details ? 'pending' : installedVersion ? 'online' : 'offline'"
            :label="detailsError ? '检测失败' : !details ? '待检测' : installedVersion ? '已安装' : '未安装'"
            size="sm"
          />
        </div>
        <p class="mt-1 max-w-3xl text-xs leading-5 text-[var(--text-tertiary)]">
          面向 Debian / Ubuntu，通过官方 APT 源安装。配置会在远端校验后原子写入，并启用 systemd 服务。
        </p>
      </div>
      <span
        v-if="installedVersion"
        class="max-w-72 truncate font-mono text-[10px] text-[var(--text-secondary)]"
        :title="installedVersion"
      >{{ installedVersion }}</span>
    </div>

    <div class="deployment-grid">
      <div class="space-y-4">
        <label class="block text-xs font-medium text-[var(--text-secondary)]">
          公网 IP 或域名
          <input
            v-model="form.server_address"
            class="aw-input mt-1 w-full font-mono"
            maxlength="253"
            placeholder="例如 203.0.113.10 或 proxy.example.com"
            :disabled="deploying"
          />
          <small class="mt-1 block font-normal text-[var(--text-tertiary)]">
            仅用于生成客户端连接信息，不会自动修改 DNS 或云防火墙。
          </small>
        </label>

        <article class="protocol-card" :class="{ 'protocol-card-active': form.vless_reality_enabled }">
          <label class="flex cursor-pointer items-start gap-3">
            <input v-model="form.vless_reality_enabled" type="checkbox" class="mt-1" :disabled="deploying" />
            <ShieldCheck :size="19" class="mt-0.5 text-[var(--color-primary)]" />
            <span class="min-w-0 flex-1">
              <strong class="block text-sm">VLESS + REALITY</strong>
              <small class="text-[var(--text-tertiary)]">无需域名证书，生成 UUID、Reality 密钥与 Short ID</small>
            </span>
          </label>
          <div v-if="form.vless_reality_enabled" class="mt-3 grid gap-3 sm:grid-cols-2">
            <label class="text-xs text-[var(--text-secondary)]">监听端口
              <input v-model.number="form.vless_reality_port" class="aw-input mt-1 w-full" type="number" min="1" max="65535" :disabled="deploying" />
            </label>
            <label class="text-xs text-[var(--text-secondary)]">Reality 伪装域名
              <input v-model="form.reality_server_name" class="aw-input mt-1 w-full font-mono" maxlength="253" :disabled="deploying" />
            </label>
          </div>
        </article>

        <article class="protocol-card" :class="{ 'protocol-card-active': form.shadowsocks_enabled }">
          <label class="flex cursor-pointer items-start gap-3">
            <input v-model="form.shadowsocks_enabled" type="checkbox" class="mt-1" :disabled="deploying" />
            <KeyRound :size="19" class="mt-0.5 text-[var(--color-success)]" />
            <span class="min-w-0 flex-1">
              <strong class="block text-sm">Shadowsocks 2022</strong>
              <small class="text-[var(--text-tertiary)]">AES-128-GCM，自动生成符合密钥长度要求的随机密码</small>
            </span>
          </label>
          <label v-if="form.shadowsocks_enabled" class="mt-3 block text-xs text-[var(--text-secondary)]">
            监听端口
            <input v-model.number="form.shadowsocks_port" class="aw-input mt-1 w-full" type="number" min="1" max="65535" :disabled="deploying" />
          </label>
        </article>
      </div>

      <aside class="deploy-summary">
        <div class="flex items-start gap-3">
          <Network :size="18" class="mt-0.5 text-[var(--color-info)]" />
          <div>
            <h4 class="text-sm font-medium">部署检查</h4>
            <ul class="mt-2 space-y-2 text-xs leading-5 text-[var(--text-secondary)]">
              <li>需要 root 或免密 sudo 权限</li>
              <li>服务端与客户端配置均执行 sing-box check</li>
              <li>启动失败会恢复部署前配置</li>
              <li>请自行放行所选 TCP / UDP 端口</li>
            </ul>
          </div>
        </div>
        <label class="mt-4 flex items-start gap-2 border-t border-[var(--border-default)] pt-4 text-xs text-[var(--text-secondary)]">
          <input v-model="form.replace_existing_config" type="checkbox" class="mt-0.5" :disabled="deploying" />
          <span>允许备份并覆盖已有的非 Ackwrap sing-box 配置</span>
        </label>
        <p class="mt-2 text-[10px] leading-4 text-[var(--color-warning)]">
          检测到未知或已被修改的配置时默认拒绝覆盖。勾选后，原文件仍会保留为带时间戳的备份。
        </p>
        <label
          v-if="installedVersion"
          class="mt-3 flex items-start gap-2 rounded-[var(--radius-md)] bg-[var(--color-warning-bg)] p-3 text-xs text-[var(--text-secondary)]"
        >
          <input v-model="regenerateConfirmed" type="checkbox" class="mt-0.5" :disabled="deploying" />
          <span>我了解重新部署会生成新凭据，并使上次生成的连接信息失效</span>
        </label>
        <Button
          class="mt-4 w-full"
          variant="primary"
          :disabled="deploying || !details || Boolean(detailsError) || (Boolean(installedVersion) && !regenerateConfirmed)"
          @click="deploy"
        >
          <template #icon>
            <LoaderCircle v-if="deploying" :size="14" class="animate-spin" />
            <ServerCog v-else :size="14" />
          </template>
          {{ deploying ? '正在远端安装并校验...' : installedVersion ? '重新生成并部署' : '一键安装并生成节点' }}
        </Button>
      </aside>
    </div>

    <p v-if="deployError" class="mt-4 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] p-3 text-xs text-[var(--color-error)]">
      {{ deployError }}
    </p>
    <p
      v-if="notice"
      aria-live="polite"
      class="mt-4 rounded-[var(--radius-md)] p-3 text-xs"
      :class="noticeError ? 'bg-[var(--color-error-bg)] text-[var(--color-error)]' : 'bg-[var(--color-success-bg)] text-[var(--color-success)]'"
    >{{ notice }}</p>

    <div v-if="result" class="mt-5 space-y-3 border-t border-[var(--border-default)] pt-5">
      <div class="flex items-center gap-2">
        <Check :size="17" class="text-[var(--color-success)]" />
        <h4 class="text-sm font-semibold">节点与客户端配置</h4>
        <StatusBadge status="online" label="部署成功" size="sm" />
      </div>
      <p class="text-xs leading-5 text-[var(--text-tertiary)]">
        连接秘密不会保存到 Ackwrap 数据库。关闭弹窗前请复制分享链接或完整客户端配置。
      </p>
      <article v-for="node in result.nodes" :key="node.type" class="result-card">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <strong class="text-sm">{{ node.name }}</strong>
            <span class="font-mono text-[10px] text-[var(--text-tertiary)]">{{ node.network }} · :{{ node.listen_port }}</span>
          </div>
          <p class="mt-1 truncate font-mono text-[10px] text-[var(--text-secondary)]">连接信息已生成并通过配置校验</p>
        </div>
        <Button size="sm" variant="ghost" @click="copy(node.share_uri, `${node.name} 分享链接`)">
          <template #icon><Clipboard :size="13" /></template>复制链接
        </Button>
        <Button size="sm" variant="ghost" @click="copy(JSON.stringify(node.client_outbound, null, 2), `${node.name} 出站配置`)">
          <template #icon><ExternalLink :size="13" /></template>复制出站
        </Button>
      </article>
      <details class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-surface)]">
        <summary class="cursor-pointer px-4 py-3 text-xs font-medium">查看完整 sing-box 客户端配置</summary>
        <div class="border-t border-[var(--border-default)] p-3">
          <pre class="max-h-64 overflow-auto whitespace-pre-wrap break-all font-mono text-[10px] leading-5 text-[var(--text-secondary)]">{{ clientConfigText }}</pre>
          <Button class="mt-3" size="sm" @click="copy(clientConfigText, '完整客户端配置')">
            <template #icon><Clipboard :size="13" /></template>复制完整配置
          </Button>
        </div>
      </details>
      <p class="text-[10px] text-[var(--text-tertiary)]">
        远端配置：{{ result.config_path }}<template v-if="result.backup_path"> · 原配置备份：{{ result.backup_path }}</template>
      </p>
    </div>
  </section>
</template>

<style scoped>
.deployment-grid {
  display: grid;
  gap: 1rem;
}
@media (min-width: 900px) {
  .deployment-grid {
    grid-template-columns: minmax(0, 1.5fr) minmax(250px, 0.75fr);
  }
}
.protocol-card,
.deploy-summary,
.result-card {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: var(--bg-surface);
}
.protocol-card,
.deploy-summary {
  padding: 1rem;
}
.protocol-card-active {
  border-color: color-mix(in srgb, var(--color-primary) 48%, var(--border-default));
  background: color-mix(in srgb, var(--color-primary) 5%, var(--bg-surface));
}
.result-card {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1rem;
}
</style>
