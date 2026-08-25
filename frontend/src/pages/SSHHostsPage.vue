<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
} from "vue";
import {
  Fingerprint,
  KeyRound,
  Laptop,
  Pencil,
  Plus,
  Share2,
  ShieldAlert,
  Trash2,
  Upload,
} from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import { ApiRequestError } from "@/services/api";
import { advancedApi } from "@/services/advancedApi";
import type { NodeExposure } from "@/services/advancedTypes";
import { sshApi } from "@/services/sshApi";
import type {
  SSHCredential,
  SSHCredentialAuthType,
  SSHCredentialRequest,
  SSHHost,
  SSHHostImportResponse,
  SSHHostKey,
  SSHHostKeyChallenge,
  SSHHostRequest,
  SSHSessionCreateResponse,
} from "@/services/sshTypes";
import { errorMessage, formatTime } from "./advanced/advancedUi";
import SSHHostFormModal from "./ssh/SSHHostFormModal.vue";
import SSHHostDetailsModal from "./ssh/SSHHostDetailsModal.vue";
import SSHSingboxDeployModal from "./ssh/SSHSingboxDeployModal.vue";
import SSHCredentialFormModal from "./ssh/SSHCredentialFormModal.vue";
import SSHHostShareModal from "./ssh/SSHHostShareModal.vue";
import SSHHostTable from "./ssh/SSHHostTable.vue";
import { useSSHHostDetails } from "./ssh/useSSHHostDetails";

const SSHTerminalModal = defineAsyncComponent(
  () => import("./ssh/SSHTerminalModal.vue"),
);

type PageTab = "hosts" | "credentials";
interface CredentialForm {
  name: string;
  auth_type: SSHCredentialAuthType;
  secret: string;
  passphrase: string;
}
interface HostKeyPrompt {
  host: SSHHost;
  challenge: SSHHostKeyChallenge;
  rotate: boolean;
}

const tab = ref<PageTab>("hosts");
const hosts = ref<SSHHost[]>([]);
const credentials = ref<SSHCredential[]>([]);
const exposures = ref<NodeExposure[]>([]);
const loading = ref(true);
const saving = ref(false);
const testingID = ref(0);
const openingShellID = ref(0);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const editingHost = ref<SSHHost | null>(null);
const hostFormOpen = ref(false);
const deletingHost = ref<SSHHost | null>(null);
const editingCredential = ref<SSHCredential | null>(null);
const credentialFormOpen = ref(false);
const credentialRequestedByHost = ref(false);
const preferredCredentialID = ref(0);
const deletingCredential = ref<SSHCredential | null>(null);
const hostKeyPrompt = ref<HostKeyPrompt | null>(null);
const trustedKey = ref<{ host: SSHHost; key: SSHHostKey } | null>(null);
const shareOpen = ref(false);
const sharingHosts = ref<SSHHost[]>([]);
const selectedHostIDs = ref<number[]>([]);
const quickShell = ref<{
  host: SSHHost;
  session: SSHSessionCreateResponse;
} | null>(null);
const detailsMode = ref<"details" | "singbox" | null>(null);
const credentialForm = reactive<CredentialForm>(emptyCredentialForm());
const selectedHosts = computed(() => {
  const selected = new Set(selectedHostIDs.value);
  return hosts.value.filter((host) => selected.has(host.id));
});
const maxSharedHosts = 100;
let shellRequestGeneration = 0;
let pageUnmounting = false;

const {
  detailsHost,
  deviceDetails,
  detailsError,
  loadingDetailsID,
  openDetails: openDeviceDetails,
  retryDetails,
  retryDetailsAfterTrust,
  closeDetails,
  disposeDetails,
} = useSSHHostDetails({
  applyHostKeyError,
  notifyError: (text) => show(text, "error"),
});

function emptyCredentialForm(): CredentialForm {
  return { name: "", auth_type: "password", secret: "", passphrase: "" };
}

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  try {
    const [hostItems, credentialItems, exposureItems] = await Promise.all([
      sshApi.getHosts(),
      sshApi.getCredentials(),
      advancedApi.getNodeExposures(),
    ]);
    hosts.value = hostItems;
    const availableHostIDs = new Set(hostItems.map((host) => host.id));
    selectedHostIDs.value = selectedHostIDs.value.filter((id) =>
      availableHostIDs.has(id),
    );
    credentials.value = credentialItems;
    exposures.value = exposureItems;
  } catch (error) {
    show(`加载 SSH 主机管理数据失败：${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function openCreateHost() {
  editingHost.value = null;
  preferredCredentialID.value = 0;
  hostFormOpen.value = true;
}

function openEditHost(host: SSHHost) {
  editingHost.value = host;
  preferredCredentialID.value = 0;
  hostFormOpen.value = true;
}

async function saveHost(payload: SSHHostRequest) {
  if (saving.value) return;
  saving.value = true;
  try {
    if (editingHost.value) {
      await sshApi.updateHost(editingHost.value.id, payload);
    } else {
      await sshApi.createHost(payload);
    }
    show(editingHost.value ? "SSH 主机已更新" : "SSH 主机已创建");
    hostFormOpen.value = false;
    await load();
  } catch (error) {
    show(`保存 SSH 主机失败：${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function removeHost() {
  const host = deletingHost.value;
  deletingHost.value = null;
  if (!host) return;
  try {
    await sshApi.deleteHost(host.id);
    show("SSH 主机已删除");
    await load();
  } catch (error) {
    show(`删除 SSH 主机失败：${errorMessage(error)}`, "error");
  }
}

function applyHostKeyError(host: SSHHost, error: unknown) {
  if (!(error instanceof ApiRequestError)) return false;
  if (!["SSH_HOST_KEY_UNKNOWN", "SSH_HOST_KEY_CHANGED"].includes(error.code)) {
    return false;
  }
  const challenge = error.details as SSHHostKeyChallenge | undefined;
  if (!challenge?.challenge_id || !challenge.fingerprint_sha256) return false;
  hostKeyPrompt.value = {
    host,
    challenge,
    rotate: error.code === "SSH_HOST_KEY_CHANGED",
  };
  return true;
}

async function testHost(host: SSHHost) {
  if (testingID.value || loadingDetailsID.value) return;
  testingID.value = host.id;
  try {
    const result = await sshApi.testHost(host.id);
    show(`连接成功，耗时 ${result.latency_ms} ms`);
    await load();
  } catch (error) {
    if (!applyHostKeyError(host, error)) {
      show(`SSH 连接测试失败：${errorMessage(error)}`, "error");
      await load();
    }
  } finally {
    testingID.value = 0;
  }
}

async function trustHostKey() {
  const prompt = hostKeyPrompt.value;
  if (!prompt || saving.value) return;
  saving.value = true;
  try {
    const payload = {
      challenge_id: prompt.challenge.challenge_id,
      fingerprint_sha256: prompt.challenge.fingerprint_sha256,
    };
    if (prompt.rotate) await sshApi.rotateHostKey(prompt.host.id, payload);
    else await sshApi.trustHostKey(prompt.host.id, payload);
    show(prompt.rotate ? "Host Key 已显式轮换" : "Host Key 已信任");
    hostKeyPrompt.value = null;
    await load();
    const refreshed = hosts.value.find((host) => host.id === prompt.host.id);
    retryDetailsAfterTrust(prompt.host.id, refreshed);
  } catch (error) {
    show(`更新 Host Key 信任失败：${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function showHostKey(host: SSHHost) {
  try {
    trustedKey.value = { host, key: await sshApi.getHostKey(host.id) };
  } catch (error) {
    show(`读取 Host Key 失败：${errorMessage(error)}`, "error");
  }
}

async function deleteHostKey() {
  const item = trustedKey.value;
  if (!item) return;
  try {
    await sshApi.deleteHostKey(item.host.id);
    trustedKey.value = null;
    show("Host Key 信任已删除");
    await load();
  } catch (error) {
    show(`删除 Host Key 信任失败：${errorMessage(error)}`, "error");
  }
}

async function openShell(host: SSHHost) {
  if (
    testingID.value ||
    openingShellID.value ||
    loadingDetailsID.value ||
    quickShell.value
  )
    return;
  const generation = ++shellRequestGeneration;
  openingShellID.value = host.id;
  try {
    const session = await sshApi.createSession(host.id, 100, 30);
    if (pageUnmounting || generation !== shellRequestGeneration) {
      try {
        await sshApi.closeSession(session.session_id);
      } catch {
        // The session may already have expired or closed with its transport.
      }
      return;
    }
    quickShell.value = { host, session };
  } catch (error) {
    if (pageUnmounting || generation !== shellRequestGeneration) return;
    if (!applyHostKeyError(host, error)) {
      show(`创建临时 Shell 失败：${errorMessage(error)}`, "error");
    }
  } finally {
    if (!pageUnmounting && generation === shellRequestGeneration) {
      openingShellID.value = 0;
    }
  }
}

function openDetails(host: SSHHost) {
  if (testingID.value || loadingDetailsID.value || openingShellID.value) return;
  detailsMode.value = "details";
  openDeviceDetails(host);
}

function openSingboxDeploy(host: SSHHost) {
  if (testingID.value || loadingDetailsID.value || openingShellID.value) return;
  detailsMode.value = "singbox";
  openDeviceDetails(host);
}

function closeDeviceAction() {
  detailsMode.value = null;
  closeDetails();
}

async function closeShell() {
  const current = quickShell.value;
  quickShell.value = null;
  if (!current) return;
  try {
    await sshApi.closeSession(current.session.session_id);
  } catch (error) {
    if (
      !(error instanceof ApiRequestError) ||
      error.code !== "SSH_SESSION_NOT_FOUND"
    ) {
      show(`关闭临时 Shell 失败：${errorMessage(error)}`, "error");
    }
  }
}

function openTerminal(host: SSHHost) {
  const opened = window.open(
    `/ssh-terminal/${encodeURIComponent(host.id)}`,
    "_blank",
  );
  if (opened) opened.opener = null;
  else show("浏览器阻止了新标签页，请允许本站打开弹出窗口", "error");
}

function openCreateCredential(requestedByHost = false) {
  editingCredential.value = null;
  credentialRequestedByHost.value = requestedByHost;
  Object.assign(credentialForm, emptyCredentialForm());
  credentialFormOpen.value = true;
}

function closeCredentialForm() {
  if (saving.value) return;
  credentialFormOpen.value = false;
  credentialRequestedByHost.value = false;
}

function openEditCredential(item: SSHCredential) {
  editingCredential.value = item;
  credentialRequestedByHost.value = false;
  Object.assign(credentialForm, {
    name: item.name,
    auth_type: item.auth_type,
    secret: "",
    passphrase: "",
  });
  credentialFormOpen.value = true;
}

function credentialPayload(): SSHCredentialRequest {
  return {
    name: credentialForm.name.trim(),
    auth_type: credentialForm.auth_type,
    secret: credentialForm.secret,
    passphrase:
      credentialForm.auth_type === "private_key"
        ? credentialForm.passphrase
        : "",
  };
}

async function saveCredential() {
  if (saving.value) return;
  if (!credentialForm.name.trim()) return show("请输入凭据名称", "error");
  if (!editingCredential.value && !credentialForm.secret) {
    return show("请输入凭据秘密", "error");
  }
  saving.value = true;
  try {
    const requestedByHost = credentialRequestedByHost.value;
    let createdCredential: SSHCredential | null = null;
    if (editingCredential.value) {
      await sshApi.updateCredential(
        editingCredential.value.id,
        credentialPayload(),
      );
    } else {
      createdCredential = await sshApi.createCredential(credentialPayload());
    }
    show(editingCredential.value ? "SSH 凭据已更新" : "SSH 凭据已创建");
    await load();
    if (requestedByHost && createdCredential) {
      if (!credentials.value.some((item) => item.id === createdCredential.id)) {
        credentials.value = [...credentials.value, createdCredential];
      }
      preferredCredentialID.value = createdCredential.id;
    }
    credentialRequestedByHost.value = false;
    credentialFormOpen.value = false;
  } catch (error) {
    show(`保存 SSH 凭据失败：${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function removeCredential() {
  const item = deletingCredential.value;
  deletingCredential.value = null;
  if (!item) return;
  try {
    await sshApi.deleteCredential(item.id);
    show("SSH 凭据已删除");
    await load();
  } catch (error) {
    show(`删除 SSH 凭据失败：${errorMessage(error)}`, "error");
  }
}

function importedHost(result: SSHHostImportResponse) {
  shareOpen.value = false;
  selectedHostIDs.value = [];
  show(
    result.converted_to_direct
      ? `已导入 ${result.host_count} 台 SSH 主机，其中 ${result.converted_to_direct_count} 台已从节点入口改为直连`
      : `已导入 ${result.host_count} 台 SSH 主机，请测试连接并核验 Host Key`,
  );
  void load();
}

function openBatchShare() {
  if (selectedHosts.value.length > maxSharedHosts) {
    show(`单次最多分享 ${maxSharedHosts} 台 SSH 主机`, "error");
    return;
  }
  sharingHosts.value = selectedHosts.value;
  shareOpen.value = true;
}

onMounted(load);
onBeforeUnmount(() => {
  pageUnmounting = true;
  shellRequestGeneration++;
  disposeDetails();
  const current = quickShell.value;
  quickShell.value = null;
  if (current) void sshApi.closeSession(current.session.session_id).catch(() => {});
});
</script>

<template>
  <div class="space-y-5">
    <PageHeader
      title="SSH 主机"
    >
      <template #actions>
        <Button
          v-if="tab === 'hosts'"
          @click="sharingHosts = []; shareOpen = true"
        >
          <template #icon><Upload :size="14" /></template>导入主机
        </Button>
        <Button
          v-if="tab === 'hosts' && selectedHosts.length"
          @click="openBatchShare"
        >
          <template #icon><Share2 :size="14" /></template>
          分享选中（{{ selectedHosts.length }}）
        </Button>
        <Button
          v-if="tab === 'hosts'"
          variant="primary"
          @click="openCreateHost"
        >
          <template #icon><Plus :size="14" /></template>新增主机
        </Button>
        <Button v-else variant="primary" @click="openCreateCredential()">
          <template #icon><Plus :size="14" /></template>新增凭据
        </Button>
      </template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <div
      class="flex gap-1 border-b border-[var(--border-default)]"
      role="tablist"
    >
      <button
        v-for="item in [
          { id: 'hosts', label: '主机', icon: Laptop },
          { id: 'credentials', label: '凭据', icon: KeyRound },
        ] as const"
        :key="item.id"
        class="relative inline-flex h-11 items-center gap-2 px-4 text-sm font-medium"
        :class="
          tab === item.id
            ? 'text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
        "
        role="tab"
        :aria-selected="tab === item.id"
        @click="tab = item.id"
      >
        <component :is="item.icon" :size="15" />{{ item.label }}
        <span
          v-if="tab === item.id"
          class="absolute inset-x-2 bottom-0 h-0.5 rounded-full bg-[var(--color-primary)]"
        />
      </button>
    </div>

    <SSHHostTable
      v-if="tab === 'hosts'"
      v-model:selected-ids="selectedHostIDs"
      :hosts="hosts"
      :loading="loading"
      :testing-id="testingID"
      :opening-shell-id="openingShellID"
      :loading-details-id="loadingDetailsID"
      :details-mode="detailsMode"
      :shell-open="!!quickShell"
      @test="testHost"
      @shell="openShell"
      @terminal="openTerminal"
      @details="openDetails"
      @deploy-singbox="openSingboxDeploy"
      @edit="openEditHost"
      @share="sharingHosts = [$event]; shareOpen = true"
      @delete="deletingHost = $event"
      @show-host-key="showHostKey"
    />

    <Card v-else padding="none">
      <div
        v-if="loading"
        class="p-10 text-center text-sm text-[var(--text-secondary)]"
      >
        加载中...
      </div>
      <div
        v-else-if="!credentials.length"
        class="p-12 text-center text-sm text-[var(--text-secondary)]"
      >
        <KeyRound :size="34" class="mx-auto mb-3 text-[var(--text-tertiary)]" />
        暂无 SSH 凭据。秘密将由后端加密保存且不会回传。
      </div>
      <div v-else class="aw-data-table-wrap rounded-none border-0">
        <table class="aw-data-table min-w-[720px]">
          <thead>
            <tr>
              <th>名称</th>
              <th>认证类型</th>
              <th>公钥指纹</th>
              <th>更新时间</th>
              <th class="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in credentials" :key="item.id">
              <td class="font-medium">{{ item.name }}</td>
              <td>{{ item.auth_type === "password" ? "密码" : "私钥" }}</td>
              <td class="max-w-80 truncate font-mono text-xs">
                {{ item.key_fingerprint || "--" }}
              </td>
              <td class="text-xs text-[var(--text-secondary)]">
                {{ formatTime(item.updated_at) }}
              </td>
              <td>
                <div class="flex justify-end gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    @click="openEditCredential(item)"
                  >
                    <template #icon><Pencil :size="13" /></template>编辑
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    @click="deletingCredential = item"
                  >
                    <template #icon><Trash2 :size="13" /></template>
                  </Button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </Card>

    <SSHHostFormModal
      :open="hostFormOpen"
      :editing="editingHost"
      :credentials="credentials"
      :preferred-credential-id="preferredCredentialID"
      :exposures="exposures"
      :saving="saving"
      @close="hostFormOpen = false"
      @create-credential="openCreateCredential(true)"
      @invalid="show($event, 'error')"
      @save="saveHost"
    />

    <SSHHostDetailsModal
      :host="detailsMode === 'details' ? detailsHost : null"
      :details="deviceDetails"
      :loading="loadingDetailsID > 0"
      :error="detailsError"
      @close="closeDeviceAction"
      @retry="retryDetails"
    />

    <SSHSingboxDeployModal
      :host="detailsMode === 'singbox' ? detailsHost : null"
      :details="deviceDetails"
      :loading="loadingDetailsID > 0"
      :error="detailsError"
      @close="closeDeviceAction"
      @retry="retryDetails"
    />

    <SSHCredentialFormModal
      v-model:name="credentialForm.name"
      v-model:auth-type="credentialForm.auth_type"
      v-model:secret="credentialForm.secret"
      v-model:passphrase="credentialForm.passphrase"
      :open="credentialFormOpen"
      :editing="editingCredential"
      :saving="saving"
      @close="closeCredentialForm"
      @save="saveCredential"
    />

    <SSHHostShareModal
      :open="shareOpen"
      :hosts="sharingHosts"
      @close="shareOpen = false"
      @imported="importedHost"
    />

    <SSHTerminalModal
      v-if="quickShell"
      open
      :host-name="quickShell.host.name"
      :session="quickShell.session"
      @close="closeShell"
    />

    <Modal
      :open="!!hostKeyPrompt"
      :title="hostKeyPrompt?.rotate ? 'Host Key 已变化' : '首次信任 Host Key'"
      size="md"
      :closable="!saving"
      @close="!saving && (hostKeyPrompt = null)"
    >
      <div class="rounded-[var(--radius-lg)] bg-[var(--color-warning-bg)] p-4">
        <div class="flex items-center gap-2 font-medium">
          <ShieldAlert :size="18" class="text-[var(--color-warning)]" />
          {{ hostKeyPrompt?.host.name }}
        </div>
        <p class="mt-3 text-xs text-[var(--text-secondary)]">
          请通过可信渠道核对 SHA-256 指纹。确认前后端不会发送用户认证信息。
        </p>
      </div>
      <dl class="mt-4 space-y-3 text-sm">
        <div>
          <dt class="text-xs text-[var(--text-tertiary)]">算法</dt>
          <dd class="mt-1 font-mono">
            {{ hostKeyPrompt?.challenge.key_type }}
          </dd>
        </div>
        <div>
          <dt class="text-xs text-[var(--text-tertiary)]">当前指纹</dt>
          <dd class="mt-1 break-all font-mono text-xs">
            {{ hostKeyPrompt?.challenge.fingerprint_sha256 }}
          </dd>
        </div>
        <div v-if="hostKeyPrompt?.challenge.trusted_fingerprint">
          <dt class="text-xs text-[var(--color-error)]">原信任指纹</dt>
          <dd class="mt-1 break-all font-mono text-xs">
            {{ hostKeyPrompt.challenge.trusted_fingerprint }}
          </dd>
        </div>
      </dl>
      <template #footer>
        <Button :disabled="saving" @click="hostKeyPrompt = null">取消</Button>
        <Button
          :variant="hostKeyPrompt?.rotate ? 'danger' : 'primary'"
          :loading="saving"
          @click="trustHostKey"
        >
          {{ hostKeyPrompt?.rotate ? "确认轮换" : "确认信任" }}
        </Button>
      </template>
    </Modal>

    <Modal
      :open="!!trustedKey"
      title="已信任 Host Key"
      size="md"
      @close="trustedKey = null"
    >
      <div class="flex items-center gap-3">
        <Fingerprint :size="24" class="text-[var(--color-success)]" />
        <div>
          <p class="font-medium">{{ trustedKey?.host.name }}</p>
          <p class="text-xs text-[var(--text-tertiary)]">
            {{ trustedKey?.key.key_type }}
          </p>
        </div>
      </div>
      <p
        class="mt-4 break-all rounded-[var(--radius-lg)] bg-[var(--bg-sidebar-hover)] p-3 font-mono text-xs"
      >
        {{ trustedKey?.key.fingerprint_sha256 }}
      </p>
      <p class="mt-3 text-xs text-[var(--text-tertiary)]">
        最近验证：{{ formatTime(trustedKey?.key.last_seen_at) }}
      </p>
      <template #footer>
        <Button variant="danger" @click="deleteHostKey">删除信任</Button>
        <Button @click="trustedKey = null">关闭</Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="!!deletingHost"
      title="删除 SSH 主机"
      :message="`确认删除 ${deletingHost?.name || ''}？已保存的 Host Key 将一并删除。`"
      confirm-text="删除"
      danger
      @confirm="removeHost"
      @cancel="deletingHost = null"
    />
    <ConfirmDialog
      :open="!!deletingCredential"
      title="删除 SSH 凭据"
      :message="`确认删除 ${deletingCredential?.name || ''}？仍被主机引用时后端会拒绝。`"
      confirm-text="删除"
      danger
      @confirm="removeCredential"
      @cancel="deletingCredential = null"
    />
  </div>
</template>
