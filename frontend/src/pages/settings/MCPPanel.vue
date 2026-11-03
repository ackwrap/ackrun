<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Copy, Eye, EyeOff, Plug, RefreshCw } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import { mcpApi, type MCPSettings } from "@/services/mcpApi";
import { writeClipboardText } from "@/utils/clipboard";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const settings = ref<MCPSettings | null>(null);
const enabled = ref(false);
const loading = ref(true);
const saving = ref(false);
const issuedToken = ref("");
const visible = ref(false);
const endpoint = ref("");
function formatConfig(token: string) {
  return JSON.stringify(
    {
      mcpServers: {
        ackrun: {
          url: endpoint.value,
          headers: { Authorization: `Bearer ${token}` },
        },
      },
    },
    null,
    2,
  );
}
const configPreview = computed(() => formatConfig("<MCP_TOKEN>"));

async function load() {
  loading.value = true;
  try {
    settings.value = await mcpApi.getSettings();
    enabled.value = settings.value.enabled;
    endpoint.value = new URL(
      settings.value.endpoint_path,
      window.location.origin,
    ).href;
  } catch (error) {
    emit("notify", `读取 MCP 配置失败：${message(error)}`, "error");
  } finally {
    loading.value = false;
  }
}

function message(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

async function save(rotate = false) {
  if (saving.value) return;
  saving.value = true;
  try {
    const result = await mcpApi.updateSettings({
      enabled: enabled.value,
      generate_token:
        rotate || (enabled.value && !settings.value?.token_configured),
    });
    if (result.token) {
      issuedToken.value = result.token;
      visible.value = false;
    }
    settings.value = { ...result, token: undefined };
    emit("notify", "MCP 配置已保存", "success");
  } catch (error) {
    emit("notify", `保存失败：${message(error)}`, "error");
  } finally {
    saving.value = false;
  }
}

async function copy(value: string) {
  try {
    await writeClipboardText(value);
    emit("notify", "已复制", "success");
  } catch {
    emit("notify", "复制失败，请手动复制", "error");
  }
}

onMounted(load);
</script>

<template>
  <Card>
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="flex items-center gap-2 font-semibold">
        <Plug :size="18" />Ackrun MCP
      </h2>
      <span v-if="settings" class="text-xs text-[var(--text-secondary)]">
        {{ settings.enabled ? "已启用" : "已关闭" }}
      </span>
    </div>
    <p v-if="loading" class="mt-4 text-sm text-[var(--text-secondary)]">
      正在读取配置…
    </p>
    <Button v-else-if="!settings" class="mt-4" @click="load">重新加载</Button>
    <div v-else class="mt-4 space-y-5">
      <p class="text-sm text-[var(--text-secondary)]">
        通过 MCP 管理订阅、节点、分组、规则、DNS、配置和核心运行。
      </p>
      <div class="flex flex-wrap items-center gap-3">
        <label class="flex items-center gap-2 text-sm">
          <input v-model="enabled" type="checkbox" :disabled="saving" />
          启用 MCP
        </label>
        <Button variant="primary" size="sm" :loading="saving" @click="save()">
          保存
        </Button>
        <Button size="sm" :disabled="saving" @click="save(true)">
          <template #icon><RefreshCw :size="14" /></template>
          {{ settings.token_configured ? "更换 Token" : "生成 Token" }}
        </Button>
      </div>
      <p class="text-xs text-[var(--text-secondary)]">
        仅接受本机或局域网 IP 直连。独立 Token
        可管理上述功能，更换后旧值立即失效。
      </p>
      <div v-if="issuedToken" class="space-y-2">
        <label class="block text-xs" for="ackrun-mcp-token"
          >本次 Token（仅显示一次）</label
        >
        <div class="flex flex-wrap gap-2">
          <input
            id="ackrun-mcp-token"
            :value="issuedToken"
            :type="visible ? 'text' : 'password'"
            readonly
            class="aw-input min-w-0 flex-1 basis-full font-mono sm:basis-auto"
          />
          <Button
            size="sm"
            :aria-label="visible ? '隐藏 Token' : '显示 Token'"
            @click="visible = !visible"
            ><component :is="visible ? EyeOff : Eye" :size="14"
          /></Button>
          <Button size="sm" @click="copy(issuedToken)">
            <Copy :size="14" />复制 Token
          </Button>
        </div>
      </div>
      <div class="space-y-2">
        <label class="block text-xs" for="ackrun-mcp-endpoint"
          >Streamable HTTP 地址</label
        >
        <div class="flex flex-wrap gap-2">
          <input
            id="ackrun-mcp-endpoint"
            v-model="endpoint"
            class="aw-input min-w-0 flex-1 basis-full font-mono sm:basis-auto"
            spellcheck="false"
          />
          <Button size="sm" @click="copy(endpoint)"
            ><Copy :size="14" />复制地址</Button
          >
          <Button
            size="sm"
            :disabled="!settings.enabled"
            @click="copy(formatConfig(issuedToken || '<MCP_TOKEN>'))"
            ><Copy :size="14" />{{
              issuedToken ? "复制完整配置" : "复制配置模板"
            }}</Button
          >
        </div>
      </div>
      <details class="text-xs text-[var(--text-secondary)]">
        <summary class="cursor-pointer">客户端配置</summary>
        <pre
          class="mt-3 overflow-x-auto rounded-[var(--radius-md)] bg-[var(--bg-elevated)] p-3"
          >{{ configPreview }}</pre>
      </details>
    </div>
  </Card>
</template>
