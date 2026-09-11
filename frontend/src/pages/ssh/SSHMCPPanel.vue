<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Copy, Eye, EyeOff, RefreshCw, ShieldCheck } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Toast from "@/components/ui/Toast.vue";
import { sshMcpApi, type SSHMCPSettings } from "@/services/sshMcpApi";
import { writeClipboardText } from "@/utils/clipboard";
import { errorMessage } from "../advanced/advancedUi";

const settings = ref<SSHMCPSettings | null>(null);
const enabled = ref(false);
const loading = ref(true);
const saving = ref(false);
const newToken = ref("");
const issuedToken = ref("");
const visible = ref(false);
const endpoint = ref("");
const message = ref("");
const messageType = ref<"success" | "error">("success");
function formatClientConfig(token: string) {
  return JSON.stringify(
    {
      mcpServers: {
        "ackwrap-ssh": {
          url: endpoint.value,
          headers: {
            Authorization: `Bearer ${token}`,
          },
        },
      },
    },
    null,
    2,
  );
}
const clientConfig = computed(() =>
  formatClientConfig(issuedToken.value || "<MCP_TOKEN>"),
);
const configPreview = computed(() =>
  formatClientConfig(
    visible.value
      ? issuedToken.value || "<MCP_TOKEN>"
      : issuedToken.value
        ? "<已隐藏，复制时包含完整 Token>"
        : "<MCP_TOKEN>",
  ),
);

function show(text: string, type: "success" | "error" = "success") {
  message.value = text;
  messageType.value = type;
}

async function load() {
  loading.value = true;
  try {
    settings.value = await sshMcpApi.getSettings();
    enabled.value = settings.value.enabled;
    endpoint.value = new URL(
      settings.value.endpoint_path,
      window.location.origin,
    ).href;
  } catch (error) {
    show(`读取 MCP 配置失败：${errorMessage(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

async function save(generate = false) {
  if (saving.value) return;
  if (
    !generate &&
    newToken.value &&
    !/^[\x21-\x7e]{32,256}$/.test(newToken.value)
  ) {
    show("Token 必须为 32 到 256 位非空白 ASCII 字符", "error");
    return;
  }
  saving.value = true;
  try {
    const result = await sshMcpApi.updateSettings({
      enabled: enabled.value,
      token: generate ? undefined : newToken.value || undefined,
      generate_token:
        generate ||
        (enabled.value && !settings.value?.token_configured && !newToken.value),
    });
    if (result.token) {
      issuedToken.value = result.token;
      visible.value = false;
    }
    settings.value = { ...result, token: undefined };
    newToken.value = "";
    show(
      result.token
        ? "配置已保存，新 Token 已生效；旧 Token 已失效"
        : "MCP 配置已保存并生效",
    );
  } catch (error) {
    show(`保存 MCP 配置失败：${errorMessage(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function copy(value: string) {
  try {
    await writeClipboardText(value);
    show("已复制");
  } catch {
    show("复制失败，请选择文本手动复制", "error");
  }
}

onMounted(load);
</script>

<template>
  <div class="space-y-4">
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />
    <Card v-if="loading">
      <p class="text-sm text-[var(--text-secondary)]">正在读取 MCP 配置...</p>
    </Card>
    <Card v-else-if="!settings">
      <p class="mb-3 text-sm text-[var(--text-secondary)]">
        无法读取配置，请重试。
      </p>
      <Button @click="load">重新加载</Button>
    </Card>
    <template v-else>
      <Card>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h3 class="font-semibold">SSH MCP 服务</h3>
            <p class="mt-2 text-sm text-[var(--text-secondary)]">
              让 LAN 内的 AI 客户端通过 HTTP 操作这里保存的 SSH 主机。
            </p>
          </div>
          <span
            class="rounded-full bg-[var(--bg-elevated)] px-3 py-1 text-xs text-[var(--text-secondary)]"
          >
            {{ settings.enabled ? "已启用" : "已关闭" }}
          </span>
        </div>
        <form class="mt-5 space-y-4" @submit.prevent="save()">
          <label class="inline-flex cursor-pointer items-center gap-3 text-sm">
            <input
              v-model="enabled"
              :disabled="saving"
              type="checkbox"
              class="h-4 w-4 accent-[var(--color-primary)]"
            />
            启用 HTTP MCP
          </label>
          <div
            class="flex items-start gap-2 rounded-lg bg-[var(--bg-elevated)] p-3 text-xs leading-6 text-[var(--text-secondary)]"
          >
            <ShieldCheck
              :size="16"
              class="mt-1 shrink-0 text-[var(--color-primary)]"
            />
            <p>
              仅接受本机和 LAN 私有 IP 的直接访问，每次请求均需 MCP
              Token。首次启用时会自动生成 Token。
            </p>
          </div>
          <label class="block max-w-2xl">
            <span class="aw-modal-label text-xs">自定义 Token（可选）</span>
            <input
              v-model="newToken"
              :disabled="saving"
              type="password"
              autocomplete="new-password"
              maxlength="256"
              class="aw-input mt-1 w-full font-mono"
              :placeholder="
                settings.token_configured
                  ? '已配置，留空保留当前 Token'
                  : '留空自动生成，或输入至少 32 位 Token'
              "
            />
          </label>
          <div class="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" :loading="saving"
              >保存配置</Button
            >
            <Button :disabled="saving" @click="save(true)">
              <template #icon><RefreshCw :size="14" /></template>
              {{
                settings.token_configured
                  ? "更换 Token 并保存"
                  : "生成 Token 并保存"
              }}
            </Button>
          </div>
          <p class="text-xs text-[var(--text-tertiary)]">
            更换 Token 后旧值立即失效，已接入的客户端需要更新。
          </p>
        </form>
      </Card>

      <Card v-if="issuedToken">
        <h3 class="font-semibold">本次生成的 Token</h3>
        <p class="mt-2 text-sm text-[var(--text-secondary)]">
          只在本次页面显示，请复制到 MCP 客户端。刷新或离开后无法再次查看。
        </p>
        <div class="mt-4 flex flex-wrap gap-2">
          <input
            :value="issuedToken"
            :type="visible ? 'text' : 'password'"
            readonly
            aria-label="本次 MCP Token"
            class="aw-input min-w-0 flex-1 font-mono"
          />
          <Button
            :aria-label="visible ? '隐藏 Token' : '显示 Token'"
            @click="visible = !visible"
            ><component :is="visible ? EyeOff : Eye" :size="14"
          /></Button>
          <Button @click="copy(issuedToken)"
            ><Copy :size="14" />复制 Token</Button
          >
        </div>
      </Card>

      <Card>
        <h3 class="font-semibold">客户端接入</h3>
        <p class="mt-2 text-sm text-[var(--text-secondary)]">
          选择 Streamable HTTP，填写地址和 Authorization 请求头即可接入。
        </p>
        <label class="mt-4 block">
          <span class="aw-modal-label text-xs">MCP HTTP 地址</span>
          <input
            v-model="endpoint"
            class="aw-input mt-1 w-full font-mono"
            spellcheck="false"
          />
        </label>
        <p class="mt-2 text-xs leading-6 text-[var(--text-tertiary)]">
          其他设备接入时，将地址中的 localhost 或 127.0.0.1 改为运行 Ackwrap 的
          LAN IP。使用 Ackwrap 当前端口和 /mcp/ssh 路径，无需另启进程。
        </p>
        <div class="my-4 flex flex-wrap gap-2">
          <Button @click="copy(endpoint)"><Copy :size="14" />复制地址</Button>
          <Button :disabled="!settings.enabled" @click="copy(clientConfig)"
            ><Copy :size="14" />{{
              issuedToken ? "复制完整配置" : "复制配置模板"
            }}</Button
          >
        </div>
        <pre
          class="overflow-x-auto rounded-lg bg-[var(--bg-elevated)] p-4 text-xs leading-6 text-[var(--text-secondary)]"
          >{{ configPreview }}</pre>
        <p v-if="!issuedToken" class="mt-2 text-xs text-[var(--text-tertiary)]">
          模板中的 &lt;MCP_TOKEN&gt; 请替换为已保存的 MCP
          Token；不要使用后台登录 Token。
        </p>
      </Card>

      <Card>
        <h3 class="font-semibold">可用功能</h3>
        <div class="mt-4 grid gap-4 text-sm sm:grid-cols-3">
          <div>
            <p class="font-medium">主机管理</p>
            <p class="mt-1 text-xs leading-6 text-[var(--text-secondary)]">
              主机列表、凭据元数据、增删改主机、连接测试。
            </p>
          </div>
          <div>
            <p class="font-medium">远程命令</p>
            <p class="mt-1 text-xs leading-6 text-[var(--text-secondary)]">
              单台或批量执行，返回输出与退出码，支持超时设置。
            </p>
          </div>
          <div>
            <p class="font-medium">文件操作</p>
            <p class="mt-1 text-xs leading-6 text-[var(--text-secondary)]">
              文本读写、Base64 上传下载、目录与文件信息，单文件上限 1 MiB。
            </p>
          </div>
        </div>
        <p class="mt-4 text-xs leading-6 text-[var(--text-tertiary)]">
          复用已保存的凭据和连接路径。首次连接或 Host Key
          变化时，请回到“主机”页核验；MCP 不会自动信任新密钥。
        </p>
      </Card>
    </template>
  </div>
</template>
