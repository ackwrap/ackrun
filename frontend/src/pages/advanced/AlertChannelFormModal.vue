<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import Button from "@/components/ui/Button.vue";
import Modal from "@/components/ui/Modal.vue";
import type {
  AlertChannel,
  AlertChannelRequest,
  AlertChannelType,
} from "@/services/advancedTypes";
import { lines } from "./advancedUi";

const props = defineProps<{
  open: boolean;
  editing: AlertChannel | null;
  saving: boolean;
}>();
const emit = defineEmits<{
  close: [];
  save: [AlertChannelRequest];
}>();

interface ChannelForm {
  name: string;
  type: AlertChannelType;
  enabled: boolean;
  secret: string;
  webhookAllowPrivate: boolean;
  telegramChatID: string;
  smtpHost: string;
  smtpPort: number;
  smtpUsername: string;
  smtpFrom: string;
  smtpRecipients: string;
  smtpTLSMode: "starttls" | "tls" | "none";
}

const form = reactive<ChannelForm>(emptyForm());
const error = ref("");

function emptyForm(): ChannelForm {
  return {
    name: "",
    type: "webhook",
    enabled: true,
    secret: "",
    webhookAllowPrivate: false,
    telegramChatID: "",
    smtpHost: "",
    smtpPort: 587,
    smtpUsername: "",
    smtpFrom: "",
    smtpRecipients: "",
    smtpTLSMode: "starttls",
  };
}

function reset() {
  const item = props.editing;
  Object.assign(
    form,
    item
      ? {
          name: item.name,
          type: item.type,
          enabled: item.enabled,
          secret: "",
          webhookAllowPrivate: item.config.webhook_allow_private || false,
          telegramChatID: item.config.telegram_chat_id || "",
          smtpHost: item.config.smtp_host || "",
          smtpPort: item.config.smtp_port || 587,
          smtpUsername: item.config.smtp_username || "",
          smtpFrom: item.config.smtp_from || "",
          smtpRecipients: (item.config.smtp_recipients || []).join("\n"),
          smtpTLSMode: item.config.smtp_tls_mode || "starttls",
        }
      : emptyForm(),
  );
  error.value = "";
}

function submit() {
  const name = form.name.trim();
  if (!name) {
    error.value = "请输入渠道名称";
    return;
  }
  if (!props.editing && !form.secret.trim() && form.type !== "email") {
    error.value = form.type === "webhook" ? "请输入 Webhook URL" : "请输入 Bot Token";
    return;
  }
  const request: AlertChannelRequest = {
    name,
    type: form.type,
    enabled: form.enabled,
    config: {},
    secret: form.type === "email" ? form.secret : form.secret.trim(),
  };
  if (form.type === "webhook") {
    request.config.webhook_allow_private = form.webhookAllowPrivate;
  }
  if (form.type === "telegram") {
    if (!form.telegramChatID.trim()) {
      error.value = "请输入 Telegram Chat ID";
      return;
    }
    request.config.telegram_chat_id = form.telegramChatID.trim();
  }
  if (form.type === "email") {
    const recipients = lines(form.smtpRecipients);
    if (!form.smtpHost.trim() || !form.smtpFrom.trim() || !recipients.length) {
      error.value = "请填写 SMTP 主机、发件人和至少一个收件人";
      return;
    }
    if (form.smtpUsername.trim() && !props.editing && !form.secret) {
      error.value = "填写 SMTP 用户名后必须提供密码";
      return;
    }
    if (props.editing?.type === "email" && form.smtpUsername.trim() !== (props.editing.config.smtp_username || "") && form.smtpUsername.trim() && !form.secret) {
      error.value = "更改 SMTP 用户名时必须提供新的密码";
      return;
    }
	if (form.smtpTLSMode === "none" && form.smtpUsername.trim()) {
	  error.value = "不加密模式仅支持匿名 SMTP，请清空用户名和密码";
	  return;
	}
    request.config = {
      smtp_host: form.smtpHost.trim(),
      smtp_port: Number(form.smtpPort),
      smtp_username: form.smtpUsername.trim(),
      smtp_from: form.smtpFrom.trim(),
      smtp_recipients: recipients,
      smtp_tls_mode: form.smtpTLSMode,
    };
  }
  error.value = "";
  emit("save", request);
}

function changeType() {
  form.secret = "";
  error.value = "";
}

watch(
  () => props.open,
  (open) => {
    if (open) reset();
  },
);
</script>

<template>
  <Modal
    :open="open"
    :title="editing ? '编辑告警渠道' : '新建告警渠道'"
    size="lg"
    :closable="!saving"
    @close="emit('close')"
  >
    <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="submit">
      <fieldset class="contents" :disabled="saving">
        <label class="sm:col-span-2">
          <span class="aw-modal-label text-xs">渠道名称</span>
          <input v-model="form.name" class="aw-input mt-1 w-full" maxlength="100" placeholder="例如：运维群通知" />
        </label>
        <label>
          <span class="aw-modal-label text-xs">渠道类型</span>
          <select v-model="form.type" class="aw-input mt-1 w-full" @change="changeType">
            <option value="webhook">Webhook</option>
            <option value="telegram">Telegram</option>
            <option value="email">邮件</option>
          </select>
        </label>
        <label class="flex items-end">
          <span class="flex h-10 w-full items-center gap-2 rounded-lg border border-[var(--border-default)] px-3 text-sm">
            <input v-model="form.enabled" type="checkbox" />保存后启用
          </span>
        </label>

        <template v-if="form.type === 'webhook'">
          <label class="sm:col-span-2">
            <span class="aw-modal-label text-xs">Webhook URL</span>
            <input v-model="form.secret" class="aw-input mt-1 w-full font-mono text-xs" type="password" autocomplete="new-password" :placeholder="editing ? '留空保留现有 URL' : 'https://hooks.example.com/...'" />
          </label>
          <label class="sm:col-span-2 flex items-start gap-2 rounded-lg border border-[var(--border-default)] p-3 text-sm">
            <input v-model="form.webhookAllowPrivate" class="mt-0.5" type="checkbox" />
            <span><span class="block">允许内网目标</span><span class="mt-1 block text-xs text-[var(--text-tertiary)]">仅用于受信任的局域网 Webhook；默认阻止私网、环回和链路本地地址。</span></span>
          </label>
        </template>

        <template v-else-if="form.type === 'telegram'">
          <label>
            <span class="aw-modal-label text-xs">Bot Token</span>
            <input v-model="form.secret" class="aw-input mt-1 w-full" type="password" autocomplete="new-password" :placeholder="editing ? '留空保留现有 Token' : '123456:ABC...'" />
          </label>
          <label>
            <span class="aw-modal-label text-xs">Chat ID</span>
            <input v-model="form.telegramChatID" class="aw-input mt-1 w-full font-mono" placeholder="-1001234567890" />
          </label>
        </template>

        <template v-else>
          <label>
            <span class="aw-modal-label text-xs">SMTP 主机</span>
            <input v-model="form.smtpHost" class="aw-input mt-1 w-full" placeholder="smtp.example.com" />
          </label>
          <label>
            <span class="aw-modal-label text-xs">SMTP 端口</span>
            <input v-model.number="form.smtpPort" class="aw-input mt-1 w-full" type="number" min="1" max="65535" />
          </label>
          <label>
            <span class="aw-modal-label text-xs">加密模式</span>
            <select v-model="form.smtpTLSMode" class="aw-input mt-1 w-full">
              <option value="starttls">STARTTLS</option>
              <option value="tls">隐式 TLS</option>
              <option value="none">不加密（仅匿名）</option>
            </select>
          </label>
          <label>
            <span class="aw-modal-label text-xs">SMTP 用户名</span>
            <input v-model="form.smtpUsername" class="aw-input mt-1 w-full" autocomplete="username" placeholder="可留空" />
          </label>
          <label>
            <span class="aw-modal-label text-xs">SMTP 密码</span>
            <input v-model="form.secret" class="aw-input mt-1 w-full" type="password" autocomplete="new-password" :placeholder="editing ? '留空保留现有密码' : '无认证可留空'" />
          </label>
          <label>
            <span class="aw-modal-label text-xs">发件人</span>
            <input v-model="form.smtpFrom" class="aw-input mt-1 w-full" type="email" placeholder="alerts@example.com" />
          </label>
          <label class="sm:col-span-2">
            <span class="aw-modal-label text-xs">收件人（每行一个）</span>
            <textarea v-model="form.smtpRecipients" class="aw-input mt-1 min-h-28 w-full resize-y" placeholder="ops@example.com" />
          </label>
        </template>
      </fieldset>
    </form>
    <p class="mt-4 text-xs leading-5 text-[var(--text-tertiary)]">
      Webhook URL、Bot Token 和 SMTP 密码使用本地密钥加密保存，列表 API 不会回显原文。
      远程管理时请通过 HTTPS 反向代理或受信任 VPN 提交这些秘密。
    </p>
    <p v-if="error" class="mt-3 text-sm text-[var(--color-error)]" role="alert">{{ error }}</p>
    <template #footer>
      <Button :disabled="saving" @click="emit('close')">取消</Button>
      <Button variant="primary" :loading="saving" @click="submit">保存</Button>
    </template>
  </Modal>
</template>
