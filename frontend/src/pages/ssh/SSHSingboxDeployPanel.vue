<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import {
  BookOpen,
  CheckCircle2,
  Clipboard,
  ExternalLink,
  Info,
  LoaderCircle,
  RefreshCw,
  Rocket,
  ServerCog,
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

type DeployProtocol = "vless-reality" | "shadowsocks-2022";

const props = defineProps<{
  host: SSHHost;
  details: SSHDeviceDetails | null;
  detailsError: string;
  detailsLoading: boolean;
}>();
const emit = defineEmits<{
  busy: [value: boolean];
  refresh: [];
}>();

const deploying = ref(false);
const deployError = ref("");
const notice = ref("");
const noticeError = ref(false);
const result = ref<SSHSingboxDeployResponse | null>(null);
const regenerateConfirmed = ref(false);
const protocol = ref<DeployProtocol>("vless-reality");
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
const activePort = computed({
  get: () =>
    protocol.value === "vless-reality"
      ? form.vless_reality_port
      : form.shadowsocks_port,
  set: (value: number) => {
    if (protocol.value === "vless-reality") form.vless_reality_port = value;
    else form.shadowsocks_port = value;
  },
});
const environmentReady = computed(
  () => !props.detailsLoading && Boolean(props.details) && !props.detailsError,
);
const addressValid = computed(() => {
  const value = form.server_address.trim();
  if (/^\d+(?:\.\d+){3}$/.test(value)) return validIPv4(value);
  return validDomain(value) || validIPv6(value);
});
const portValid = computed(
  () =>
    Number.isInteger(activePort.value) &&
    activePort.value > 0 &&
    activePort.value <= 65535 &&
    activePort.value !== props.host.port,
);
const protocolValid = computed(() =>
  protocol.value === "vless-reality"
    ? validDomain(form.reality_server_name.trim())
    : true,
);
const deploymentReady = computed(
  () =>
    environmentReady.value &&
    addressValid.value &&
    portValid.value &&
    protocolValid.value,
);

watch(
  () => props.host.id,
  () => {
    protocol.value = "vless-reality";
    Object.assign(form, emptyForm(props.host));
    deployError.value = "";
    notice.value = "";
    result.value = null;
    regenerateConfirmed.value = false;
  },
);

watch(protocol, (value) => {
  form.vless_reality_enabled = value === "vless-reality";
  form.shadowsocks_enabled = value === "shadowsocks-2022";
  deployError.value = "";
});

function emptyForm(host: SSHHost): SSHSingboxDeployRequest {
  return {
    server_address: host.host,
    vless_reality_enabled: true,
    vless_reality_port: 443,
    reality_server_name: "www.microsoft.com",
    shadowsocks_enabled: false,
    shadowsocks_port: 8388,
    replace_existing_config: false,
  };
}

function validDomain(value: string) {
  if (!value || value.length > 253 || value.startsWith(".") || value.endsWith(".")) {
    return false;
  }
  return value.split(".").every(
    (label) =>
      Boolean(label) &&
      label.length <= 63 &&
      !label.startsWith("-") &&
      !label.endsWith("-") &&
      /^[A-Za-z0-9-]+$/.test(label),
  );
}

function validIPv4(value: string) {
  const parts = value.split(".");
  return (
    parts.length === 4 &&
    parts.every(
      (part) =>
        /^\d{1,3}$/.test(part) &&
        Number(part) >= 0 &&
        Number(part) <= 255,
    )
  );
}

function validIPv6(value: string) {
  if (!value.includes(":")) return false;
  try {
    const parsed = new URL(`http://[${value}]/`);
    return parsed.hostname.startsWith("[") && parsed.hostname.endsWith("]");
  } catch {
    return false;
  }
}

function validate() {
  if (!environmentReady.value) return "请先完成远端环境检测";
  if (!addressValid.value) return "请输入有效的客户端连接 IP 或域名";
  if (!portValid.value) return "监听端口无效或与 SSH 端口冲突";
  if (!protocolValid.value) return "请填写有效的 Reality 伪装域名";
  return "";
}

async function deploy() {
  if (deploying.value || !environmentReady.value) return;
  deployError.value = validate();
  if (deployError.value) return;
  deploying.value = true;
  emit("busy", true);
  result.value = null;
  notice.value = "";
  const requestHostID = props.host.id;
  try {
    const response = await sshApi.deploySingbox(requestHostID, {
      ...form,
      server_address: form.server_address.trim(),
      reality_server_name: form.reality_server_name.trim(),
    });
    if (props.host.id !== requestHostID) return;
    result.value = response;
    regenerateConfirmed.value = false;
    noticeError.value = false;
    notice.value = "部署完成。连接信息仅在本次结果中展示，请立即保存。";
  } catch (cause) {
    if (props.host.id === requestHostID) deployError.value = errorMessage(cause);
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
    <div class="mb-4 flex items-center gap-2">
      <ServerCog :size="19" class="text-[var(--color-primary)]" />
      <h3 class="text-sm font-semibold">sing-box 服务端部署</h3>
    </div>

    <div class="deploy-steps">
      <section class="deploy-step">
        <div class="step-marker"><span>1</span></div>
        <div class="step-card">
          <header class="step-header">
            <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
              <h4>检测环境</h4>
              <p>检测 sing-box 安装状态与远端部署条件</p>
            </div>
            <Button
              variant="link"
              size="sm"
              :disabled="deploying"
              @click="emit('refresh')"
            >
              <template #icon><RefreshCw :size="14" /></template>重新检测
            </Button>
          </header>
          <div class="environment-row">
            <div class="environment-primary">
              <CheckCircle2
                :size="22"
                :class="detailsError ? 'text-[var(--color-error)]' : environmentReady ? 'text-[var(--color-success)]' : 'text-[var(--color-warning)]'"
              />
              <div>
                <strong>{{ detailsLoading ? '正在检测' : detailsError ? '检测失败' : installedVersion ? '已安装' : environmentReady ? '未安装，将自动安装' : '等待检测' }}</strong>
                <small v-if="detailsLoading">正在读取远端软件状态</small>
                <small v-else-if="installedVersion">{{ installedVersion }}</small>
                <small v-else>官方 APT 源 · Debian / Ubuntu</small>
              </div>
            </div>
            <div class="environment-item">
              <span>服务状态</span>
              <strong>{{ result ? 'systemd · active (running)' : detailsLoading ? '正在检测' : installedVersion ? '部署时检查并重启' : '安装后自动启用' }}</strong>
            </div>
            <div class="environment-item">
              <span>配置保护</span>
              <strong>指纹校验 · 失败回滚</strong>
            </div>
            <div class="environment-item">
              <span>配置文件</span>
              <strong class="font-mono">/etc/sing-box/config.json</strong>
            </div>
          </div>
        </div>
      </section>

      <section class="deploy-step">
        <div class="step-marker"><span>2</span></div>
        <div class="step-card">
          <header class="step-header">
            <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
              <h4>配置协议</h4>
              <p>选择部署协议并填写客户端连接所需参数</p>
            </div>
          </header>
          <div class="protocol-fields">
            <label>
              客户端连接地址
              <input
                v-model="form.server_address"
                class="aw-input mt-1 w-full font-mono"
                maxlength="253"
                placeholder="proxy.example.com"
                :disabled="deploying"
              />
            </label>
            <label>
              部署协议
              <select v-model="protocol" class="aw-input mt-1 w-full" :disabled="deploying">
                <option value="vless-reality">VLESS + REALITY</option>
                <option value="shadowsocks-2022">Shadowsocks 2022</option>
              </select>
            </label>
            <label>
              监听端口
              <input
                v-model.number="activePort"
                class="aw-input mt-1 w-full font-mono"
                type="number"
                min="1"
                max="65535"
                :disabled="deploying"
              />
            </label>
            <label>
              {{ protocol === 'vless-reality' ? 'Reality 伪装域名' : '加密方法' }}
              <input
                v-if="protocol === 'vless-reality'"
                v-model="form.reality_server_name"
                class="aw-input mt-1 w-full font-mono"
                maxlength="253"
                :disabled="deploying"
              />
              <input
                v-else
                class="aw-input mt-1 w-full font-mono"
                value="2022-blake3-aes-128-gcm"
                disabled
              />
            </label>
            <label>
              {{ protocol === 'vless-reality' ? 'UUID' : '访问密码' }}
              <div class="generated-field mt-1">自动安全生成</div>
            </label>
          </div>

          <div class="protocol-reference">
            <div class="reference-title">
              <BookOpen :size="16" />
              <span>配置参考</span>
            </div>
            <ul v-if="protocol === 'vless-reality'">
              <li>传输协议：TCP</li>
              <li>请在云防火墙与系统防火墙放行 TCP {{ activePort }}</li>
              <li>自动生成 UUID、Reality 密钥和 Short ID</li>
              <li>客户端默认启用 uTLS Chrome 指纹</li>
            </ul>
            <ul v-else>
              <li>传输协议：TCP + UDP</li>
              <li>请在云防火墙与系统防火墙放行 {{ activePort }}</li>
              <li>自动生成符合 Shadowsocks 2022 长度要求的密码</li>
              <li>客户端配置和 SIP002 分享链接同时生成</li>
            </ul>
          </div>
        </div>
      </section>

      <section class="deploy-step deploy-step-last">
        <div class="step-marker"><span>3</span></div>
        <div class="step-card">
          <header class="step-header">
            <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1">
              <h4>校验并部署</h4>
              <p>本地参数通过后，远端还会执行权限、配置与服务检查</p>
            </div>
          </header>
          <div class="check-grid">
            <div class="check-item" :class="{ 'check-passed': environmentReady }">
              <CheckCircle2 :size="20" />
              <div><strong>设备信息读取</strong><small>{{ environmentReady ? '通过' : '待检测' }}</small></div>
            </div>
            <div class="check-item" :class="{ 'check-passed': addressValid }">
              <CheckCircle2 :size="20" />
              <div><strong>连接地址格式</strong><small>{{ addressValid ? '通过' : '待完善' }}</small></div>
            </div>
            <div class="check-item" :class="{ 'check-passed': portValid }">
              <CheckCircle2 :size="20" />
              <div><strong>监听端口参数</strong><small>{{ portValid ? '通过' : '待完善' }}</small></div>
            </div>
            <div class="check-item" :class="{ 'check-passed': protocolValid }">
              <CheckCircle2 :size="20" />
              <div><strong>协议参数</strong><small>{{ protocolValid ? '通过' : '待完善' }}</small></div>
            </div>
          </div>

          <div class="deploy-actions">
            <div class="min-w-0 flex-1 space-y-3">
              <label class="switch-row">
                <input v-model="form.replace_existing_config" type="checkbox" :disabled="deploying" />
                <span class="switch-track"><span /></span>
                <span>
                  <strong>允许备份并覆盖未知配置</strong>
                  <small>Ackwrap 管理的配置始终自动备份；未知或被修改的配置默认拒绝覆盖</small>
                </span>
              </label>
              <label v-if="installedVersion" class="confirm-row">
                <input v-model="regenerateConfirmed" type="checkbox" :disabled="deploying" />
                <span>我了解重新部署会生成新凭据，并使上次连接信息失效</span>
              </label>
            </div>
            <Button
              class="deploy-button"
              variant="primary"
              size="lg"
              :disabled="deploying || !deploymentReady || (Boolean(installedVersion) && !regenerateConfirmed)"
              @click="deploy"
            >
              <template #icon>
                <LoaderCircle v-if="deploying" :size="16" class="animate-spin" />
                <Rocket v-else :size="16" />
              </template>
              {{ deploying ? '正在安装、校验并启动...' : installedVersion ? '校验并重新部署' : '校验并部署到生产' }}
            </Button>
          </div>
        </div>
      </section>
    </div>

    <p class="mt-3 flex items-start gap-2 text-[11px] leading-5 text-[var(--text-tertiary)]">
      <Info :size="14" class="mt-0.5 shrink-0" />
      实际耗时取决于远端 APT 下载速度；部署期间请勿关闭弹窗。系统不会自动修改 DNS、云防火墙或系统防火墙。
    </p>

    <p v-if="deployError" role="alert" class="mt-4 rounded-[var(--radius-md)] bg-[var(--color-error-bg)] p-3 text-xs text-[var(--color-error)]">
      {{ deployError }}
    </p>
    <p
      v-if="notice"
      aria-live="polite"
      class="mt-4 rounded-[var(--radius-md)] p-3 text-xs"
      :class="noticeError ? 'bg-[var(--color-error-bg)] text-[var(--color-error)]' : 'bg-[var(--color-success-bg)] text-[var(--color-success)]'"
    >{{ notice }}</p>

    <div v-if="result" class="deployment-result">
      <div class="flex items-center gap-2">
        <CheckCircle2 :size="18" class="text-[var(--color-success)]" />
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
          <p class="mt-1 text-[10px] text-[var(--text-secondary)]">连接信息已生成并通过 sing-box 校验</p>
        </div>
        <Button size="sm" variant="ghost" @click="copy(node.share_uri, `${node.name} 分享链接`)">
          <template #icon><Clipboard :size="13" /></template>复制链接
        </Button>
        <Button size="sm" variant="ghost" @click="copy(JSON.stringify(node.client_outbound, null, 2), `${node.name} 出站配置`)">
          <template #icon><ExternalLink :size="13" /></template>复制出站
        </Button>
      </article>
      <details class="result-config">
        <summary>查看完整 sing-box 客户端配置</summary>
        <div>
          <pre>{{ clientConfigText }}</pre>
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
.deploy-steps {
  display: grid;
  gap: 0.75rem;
}
.deploy-step {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 0.75rem;
}
.step-marker {
  position: relative;
  display: flex;
  justify-content: center;
  padding-top: 0.7rem;
}
.step-marker::after {
  position: absolute;
  top: 2.7rem;
  bottom: -1.15rem;
  left: 50%;
  width: 1px;
  content: "";
  background: color-mix(in srgb, var(--color-primary) 72%, var(--border-default));
}
.deploy-step-last .step-marker::after {
  display: none;
}
.step-marker span {
  position: relative;
  z-index: 1;
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  border-radius: 999px;
  background: var(--color-primary);
  color: var(--button-primary-text);
  font-size: 0.8rem;
  font-weight: 700;
  box-shadow: 0 0 0 4px var(--bg-elevated);
}
.step-card,
.deployment-result {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--bg-surface) 92%, transparent);
}
.step-card {
  padding: 0.9rem 1rem;
}
.step-header {
  display: flex;
  min-height: 2rem;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}
.step-header h4 {
  font-size: 0.85rem;
  font-weight: 650;
}
.step-header p {
  font-size: 0.68rem;
  color: var(--text-tertiary);
}
.environment-row {
  display: grid;
  gap: 0.75rem;
  margin-top: 0.65rem;
  padding: 0.8rem 1rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--bg-base) 44%, transparent);
}
.environment-primary,
.environment-item {
  min-width: 0;
}
.environment-primary {
  display: flex;
  align-items: center;
  gap: 0.7rem;
}
.environment-primary strong,
.environment-primary small,
.environment-item span,
.environment-item strong {
  display: block;
}
.environment-primary strong,
.environment-item strong {
  overflow: hidden;
  font-size: 0.72rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.environment-primary small,
.environment-item span {
  margin-bottom: 0.15rem;
  font-size: 0.62rem;
  color: var(--text-tertiary);
}
.protocol-fields {
  display: grid;
  gap: 0.75rem;
  margin-top: 0.7rem;
}
.protocol-fields label {
  min-width: 0;
  font-size: 0.65rem;
  color: var(--text-secondary);
}
.generated-field {
  display: flex;
  min-height: 2rem;
  align-items: center;
  justify-content: space-between;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0 0.7rem;
  background: var(--button-secondary-bg);
  color: var(--text-tertiary);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.7rem;
}
.protocol-reference {
  display: grid;
  gap: 0.75rem;
  margin-top: 0.75rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.7rem 0.85rem;
  background: color-mix(in srgb, var(--color-primary) 4%, var(--bg-base));
}
.reference-title {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  color: var(--color-primary);
  font-size: 0.7rem;
  font-weight: 600;
}
.protocol-reference ul {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.25rem 1.5rem;
  font-size: 0.64rem;
  color: var(--text-tertiary);
}
.protocol-reference li::before {
  margin-right: 0.4rem;
  content: "•";
}
.check-grid {
  display: grid;
  gap: 0.5rem;
  margin-top: 0.7rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.75rem 1rem;
  background: color-mix(in srgb, var(--bg-base) 44%, transparent);
}
.check-item {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  color: var(--text-disabled);
}
.check-item strong,
.check-item small {
  display: block;
}
.check-item strong {
  font-size: 0.68rem;
  color: var(--text-secondary);
}
.check-item small {
  margin-top: 0.1rem;
  font-size: 0.62rem;
}
.check-passed {
  color: var(--color-success);
}
.deploy-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 1rem;
  margin-top: 0.7rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.7rem 0.8rem;
  background: color-mix(in srgb, var(--bg-base) 44%, transparent);
}
.switch-row,
.confirm-row {
  display: flex;
  align-items: flex-start;
  gap: 0.55rem;
  font-size: 0.67rem;
  color: var(--text-secondary);
}
.switch-row > input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}
.switch-track {
  position: relative;
  width: 2.25rem;
  height: 1.2rem;
  flex: 0 0 auto;
  border-radius: 999px;
  background: var(--button-secondary-bg);
  transition: background 160ms ease;
}
.switch-track span {
  position: absolute;
  top: 0.15rem;
  left: 0.15rem;
  width: 0.9rem;
  height: 0.9rem;
  border-radius: 999px;
  background: var(--text-secondary);
  transition: transform 160ms ease, background 160ms ease;
}
.switch-row > input:checked + .switch-track {
  background: var(--color-primary);
}
.switch-row > input:checked + .switch-track span {
  background: var(--button-primary-text);
  transform: translateX(1.05rem);
}
.switch-row:focus-within .switch-track {
  outline: 2px solid color-mix(in srgb, var(--color-primary) 55%, transparent);
  outline-offset: 2px;
}
.switch-row strong,
.switch-row small {
  display: block;
}
.switch-row strong {
  font-weight: 600;
}
.switch-row small {
  margin-top: 0.15rem;
  color: var(--text-tertiary);
}
.confirm-row {
  color: var(--color-warning);
}
.deploy-button {
  min-width: 15rem;
}
.deployment-result {
  display: grid;
  gap: 0.7rem;
  margin: 1rem 0 0 2.85rem;
  padding: 1rem;
}
.result-card {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  padding: 0.75rem 1rem;
  background: var(--bg-surface);
}
.result-config {
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  background: var(--bg-surface);
}
.result-config summary {
  cursor: pointer;
  padding: 0.75rem 1rem;
  font-size: 0.7rem;
  font-weight: 600;
}
.result-config > div {
  border-top: 1px solid var(--border-default);
  padding: 0.75rem;
}
.result-config pre {
  max-height: 16rem;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.62rem;
  line-height: 1.5;
  color: var(--text-secondary);
}
@media (min-width: 720px) {
  .environment-row {
    grid-template-columns: 1.2fr repeat(3, minmax(0, 1fr));
  }
  .protocol-fields {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }
  .check-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
@media (max-width: 719px) {
  .deploy-step {
    grid-template-columns: 28px minmax(0, 1fr);
    gap: 0.5rem;
  }
  .step-marker span {
    width: 1.7rem;
    height: 1.7rem;
  }
  .step-marker::after {
    top: 2.35rem;
  }
  .step-card {
    padding: 0.8rem;
  }
  .protocol-reference ul {
    grid-template-columns: 1fr;
  }
  .deploy-button {
    width: 100%;
    min-width: 0;
  }
  .deployment-result {
    margin-left: 2.2rem;
  }
}
</style>
