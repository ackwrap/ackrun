<script setup lang="ts">
import { onMounted, ref } from "vue";
import { BookOpen, Pencil, Plus, Trash2 } from "lucide-vue-next";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { authenticatedFetch } from "@/services/apiAuth";

interface DNSHost {
  id: number;
  domain: string;
  addresses: string[];
  enabled: boolean;
  comment: string;
}

interface DNSHostForm {
  id?: number;
  domain: string;
  addresses: string;
  enabled: boolean;
  comment: string;
}

const emit = defineEmits<{
  notify: [message: string, type: "success" | "error"];
}>();

const items = ref<DNSHost[]>([]);
const loading = ref(true);
const saving = ref(false);
const pendingID = ref(0);
const form = ref<DNSHostForm | null>(null);
const deleting = ref<DNSHost | null>(null);

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await authenticatedFetch(url, init);
  const body = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(body?.error?.message || response.statusText);
  }
  return body as T;
}

async function load() {
  loading.value = true;
  try {
    const result = await request<DNSHost[]>("/api/v1/dns/hosts");
    items.value = Array.isArray(result) ? result : [];
  } catch (error) {
    notifyError("加载 Hosts 映射失败", error);
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.value = {
    domain: "",
    addresses: "",
    enabled: true,
    comment: "",
  };
}

function openEdit(item: DNSHost) {
  form.value = {
    id: item.id,
    domain: item.domain,
    addresses: item.addresses.join("\n"),
    enabled: item.enabled,
    comment: item.comment,
  };
}

function addresses(text: string) {
  return [...new Set(text.split(/[\n,，]+/).map((item) => item.trim()).filter(Boolean))];
}

async function save() {
  if (!form.value || saving.value) return;
  const current = form.value;
  const values = addresses(current.addresses);
  if (!current.domain.trim() || !values.length) {
    emit("notify", "请填写精确域名和至少一个 IP 地址", "error");
    return;
  }
  saving.value = true;
  try {
    await request<DNSHost>(
      current.id
        ? `/api/v1/dns/hosts/${current.id}`
        : "/api/v1/dns/hosts",
      {
        method: current.id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          domain: current.domain.trim(),
          addresses: values,
          enabled: current.enabled,
          comment: current.comment.trim(),
        }),
      },
    );
    emit("notify", current.id ? "Hosts 映射已更新" : "Hosts 映射已添加", "success");
    form.value = null;
    await load();
  } catch (error) {
    notifyError("保存 Hosts 映射失败", error);
  } finally {
    saving.value = false;
  }
}

async function toggle(item: DNSHost) {
  if (pendingID.value) return;
  pendingID.value = item.id;
  try {
    await request<DNSHost>(`/api/v1/dns/hosts/${item.id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        domain: item.domain,
        addresses: item.addresses,
        enabled: !item.enabled,
        comment: item.comment,
      }),
    });
    emit("notify", `Hosts 映射已${item.enabled ? "停用" : "启用"}`, "success");
    await load();
  } catch (error) {
    notifyError("更新 Hosts 映射状态失败", error);
  } finally {
    pendingID.value = 0;
  }
}

async function remove() {
  if (!deleting.value || pendingID.value) return;
  const item = deleting.value;
  deleting.value = null;
  pendingID.value = item.id;
  try {
    await request(`/api/v1/dns/hosts/${item.id}`, { method: "DELETE" });
    emit("notify", "Hosts 映射已删除", "success");
    await load();
  } catch (error) {
    notifyError("删除 Hosts 映射失败", error);
  } finally {
    pendingID.value = 0;
  }
}

function notifyError(prefix: string, error: unknown) {
  emit(
    "notify",
    `${prefix}: ${error instanceof Error ? error.message : "请求失败"}`,
    "error",
  );
}

onMounted(load);
</script>

<template>
  <section
    class="rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="font-semibold"><BookOpen class="inline" :size="18" /> Hosts 静态映射</h3>
        <p class="mt-1 text-xs text-[var(--text-tertiary)]">
          为精确域名直接返回固定 IPv4/IPv6，不请求上游 DNS；优先于普通 DNS 规则和 FakeIP。
        </p>
      </div>
      <button class="aw-action-button aw-action-neutral" @click="openCreate">
        <Plus :size="13" />新增映射
      </button>
    </header>

    <div class="aw-data-table-wrap mt-4">
      <table class="aw-data-table min-w-[760px]">
        <thead>
          <tr>
            <th>精确域名</th>
            <th>返回地址</th>
            <th>备注</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="5" class="py-10 text-center">加载中...</td>
          </tr>
          <tr v-else-if="!items.length">
            <td colspan="5" class="py-10 text-center">暂无 Hosts 静态映射</td>
          </tr>
          <tr v-for="item in items" v-else :key="item.id">
            <td class="font-medium text-[var(--text-primary)]">{{ item.domain }}</td>
            <td>
              <div class="flex max-w-[460px] flex-wrap gap-1.5">
                <code
                  v-for="address in item.addresses"
                  :key="address"
                  class="rounded bg-[var(--bg-sidebar-hover)] px-1.5 py-0.5 text-xs"
                >{{ address }}</code>
              </div>
            </td>
            <td class="max-w-[260px] truncate" :title="item.comment">
              {{ item.comment || "-" }}
            </td>
            <td>
              <button
                class="aw-action-button"
                :class="item.enabled ? 'aw-action-success' : 'aw-action-neutral'"
                :disabled="pendingID === item.id"
                @click="toggle(item)"
              >
                {{ item.enabled ? "启用" : "停用" }}
              </button>
            </td>
            <td>
              <div class="flex gap-2">
                <button
                  class="aw-action-button aw-action-neutral"
                  :disabled="pendingID === item.id"
                  @click="openEdit(item)"
                >
                  <Pencil :size="13" />编辑
                </button>
                <button
                  class="aw-action-button aw-action-danger"
                  :disabled="pendingID === item.id"
                  @click="deleting = item"
                >
                  <Trash2 :size="13" />删除
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Modal
      :open="!!form"
      :title="form?.id ? '编辑 Hosts 映射' : '新增 Hosts 映射'"
      size="lg"
      :closable="!saving"
      @close="form = null"
    >
      <form v-if="form" class="grid gap-4" @submit.prevent="save">
        <label class="text-sm text-[var(--text-secondary)]">
          精确域名
          <input
            v-model="form.domain"
            class="aw-input mt-1 w-full"
            maxlength="253"
            placeholder="例如 api.example.com"
          />
          <span class="mt-1 block text-xs text-[var(--text-tertiary)]">
            不支持通配符；国际化域名请填写 punycode。
          </span>
        </label>
        <label class="text-sm text-[var(--text-secondary)]">
          返回地址
          <textarea
            v-model="form.addresses"
            class="aw-input mt-1 w-full resize-y font-mono"
            rows="5"
            placeholder="192.168.1.10\nfd00::10"
          />
          <span class="mt-1 block text-xs text-[var(--text-tertiary)]">
            每行一个 IPv4 或 IPv6，最多 16 个。
          </span>
        </label>
        <label class="text-sm text-[var(--text-secondary)]">
          备注
          <input
            v-model="form.comment"
            class="aw-input mt-1 w-full"
            maxlength="200"
            placeholder="可选"
          />
        </label>
        <label class="inline-flex items-center gap-2 text-sm">
          <input v-model="form.enabled" type="checkbox" />保存后启用此映射
        </label>
      </form>
      <template #footer>
        <button
          class="aw-action-button aw-action-neutral"
          :disabled="saving"
          @click="form = null"
        >
          取消
        </button>
        <button
          class="aw-action-button aw-action-success"
          :disabled="saving"
          @click="save"
        >
          {{ saving ? "保存中..." : "保存" }}
        </button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="!!deleting"
      title="删除 Hosts 映射"
      :message="`确定删除「${deleting?.domain || ''}」吗？`"
      confirm-text="删除"
      danger
      @confirm="remove"
      @cancel="deleting = null"
    />
  </section>
</template>
