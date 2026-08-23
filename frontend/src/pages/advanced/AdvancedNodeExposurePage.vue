<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { KeyRound, Pencil, Plus, Radio, Trash2 } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import Toast from "@/components/ui/Toast.vue";
import { api } from "@/services/api";
import { advancedApi } from "@/services/advancedApi";
import type {
  NodeExposure,
  NodeExposureInboundType,
  NodeExposureRequest,
} from "@/services/advancedTypes";
import type { NodeItem } from "@/services/types";

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });

interface ExposureForm {
  name: string;
  nodeKey: string;
  inbound_type: NodeExposureInboundType;
  listen: string;
  listen_port: number;
  username: string;
  password: string;
  clear_password: boolean;
  enabled: boolean;
}

const items = ref<NodeExposure[]>([]);
const nodes = ref<NodeItem[]>([]);
const loading = ref(true);
const saving = ref(false);
const editing = ref<NodeExposure | null>(null);
const formOpen = ref(false);
const deleting = ref<NodeExposure | null>(null);
const message = ref("");
const messageType = ref<"success" | "error" | "info">("success");
const form = reactive<ExposureForm>(emptyForm());

const selectedNode = computed(() =>
  nodes.value.find(
    (node) => nodeKey(node.subscription_id, node.uid) === form.nodeKey,
  ),
);
const remoteListen = computed(() => !isLoopback(form.listen));

function emptyForm(): ExposureForm {
  return {
    name: "",
    nodeKey: "",
    inbound_type: "mixed",
    listen: "127.0.0.1",
    listen_port: 18080,
    username: "",
    password: "",
    clear_password: false,
    enabled: true,
  };
}

function nodeKey(subscriptionID: number, uid: string) {
  return `${subscriptionID}:${uid}`;
}

function show(text: string, type: "success" | "error" | "info" = "success") {
  message.value = text;
  messageType.value = type;
}

function isLoopback(listen: string) {
  const value = listen.trim().toLowerCase();
  return value === "127.0.0.1" || value === "::1";
}

function displayAddress(item: NodeExposure) {
  const listen = item.listen.includes(":") ? `[${item.listen}]` : item.listen;
  return `${item.inbound_type.toUpperCase()} · ${listen}:${item.listen_port}`;
}

async function loadNodes() {
  const result: NodeItem[] = [];
  let offset = 0;
  let total = 0;
  do {
    const page = await api.getNodes({ enabled: true, limit: 200, offset });
    result.push(...page.items);
    total = page.total;
    offset += page.items.length;
  } while (offset < total && offset > 0);
  nodes.value = result;
}

async function load() {
  loading.value = true;
  try {
    const [exposures] = await Promise.all([
      advancedApi.getNodeExposures(),
      loadNodes(),
    ]);
    items.value = exposures;
  } catch (error) {
    show(
      `加载失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = null;
  Object.assign(form, emptyForm());
  if (nodes.value.length) {
    form.nodeKey = nodeKey(nodes.value[0].subscription_id, nodes.value[0].uid);
  }
  formOpen.value = true;
}

function openEdit(item: NodeExposure) {
  editing.value = item;
  Object.assign(form, {
    name: item.name,
    nodeKey: nodeKey(item.subscription_id, item.node_uid),
    inbound_type: item.inbound_type,
    listen: item.listen,
    listen_port: item.listen_port,
    username: item.username,
    password: "",
    clear_password: false,
    enabled: item.enabled,
  });
  formOpen.value = true;
}

function buildRequest(): NodeExposureRequest {
  const separator = form.nodeKey.indexOf(":");
  const subscriptionID = Number(form.nodeKey.slice(0, separator));
  const uid = form.nodeKey.slice(separator + 1);
  return {
    name: form.name.trim(),
    subscription_id: subscriptionID,
    node_uid: uid,
    inbound_type: form.inbound_type,
    listen: form.listen.trim(),
    listen_port: Number(form.listen_port),
    username: form.clear_password ? "" : form.username.trim(),
    password: form.password,
    clear_password: form.clear_password,
    enabled: form.enabled,
  };
}

async function save() {
  if (saving.value) return;
  if (!form.name.trim() || !form.nodeKey) {
    show("请填写名称并选择目标节点", "error");
    return;
  }
  saving.value = true;
  try {
    const payload = buildRequest();
    if (editing.value) {
      await advancedApi.updateNodeExposure(editing.value.id, payload);
    } else {
      await advancedApi.createNodeExposure(payload);
    }
    show(editing.value ? "节点暴露已更新" : "节点暴露已创建");
    formOpen.value = false;
    await load();
  } catch (error) {
    show(
      `保存失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  } finally {
    saving.value = false;
  }
}

async function remove() {
  if (!deleting.value) return;
  const item = deleting.value;
  deleting.value = null;
  try {
    await advancedApi.deleteNodeExposure(item.id);
    show("节点暴露已删除");
    await load();
  } catch (error) {
    show(
      `删除失败: ${error instanceof Error ? error.message : "请求失败"}`,
      "error",
    );
  }
}

onMounted(load);
</script>

<template>
  <div class="space-y-5">
    <PageHeader title="节点入口" :embedded="embedded">
      <template #actions>
        <Button
          variant="primary"
          :disabled="loading || !nodes.length"
          @click="openCreate"
        >
          <template #icon><Plus :size="15" /></template>新增暴露
        </Button>
      </template>
    </PageHeader>
    <Toast :message="message" :type="messageType" @dismiss="message = ''" />

    <div class="grid gap-4">
      <Card padding="none">
        <div
          class="flex items-center justify-between border-b border-[var(--border-light)] px-5 py-4"
        >
          <div>
            <h2 class="text-sm font-semibold">独立代理入口</h2>
            <p class="mt-1 text-xs text-[var(--text-tertiary)]">
              每个入口固定路由到一个节点，不跟随默认策略组切换。
            </p>
          </div>
          <span class="font-mono text-xs text-[var(--text-tertiary)]"
            >{{ items.length }} entries</span
          >
        </div>

        <div
          v-if="loading"
          class="p-10 text-center text-sm text-[var(--text-secondary)]"
        >
          加载中...
        </div>
        <div v-else-if="!items.length" class="p-10 text-center">
          <Radio :size="30" class="mx-auto text-[var(--text-tertiary)]" />
          <p class="mt-3 text-sm font-medium">尚未创建节点入口</p>
          <p class="mt-1 text-xs text-[var(--text-tertiary)]">
            选择一个已启用节点，为它分配专用代理端口。
          </p>
        </div>
        <div v-else class="aw-data-table-wrap rounded-none border-0">
          <table class="aw-data-table min-w-[820px]">
            <thead>
              <tr>
                <th>名称 / 状态</th>
                <th>目标节点</th>
                <th>代理入口</th>
                <th>认证</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in items" :key="item.id">
                <td>
                  <p class="font-medium text-[var(--text-primary)]">
                    {{ item.name }}
                  </p>
                  <span
                    class="mt-1 inline-flex items-center gap-1.5 text-[11px]"
                    :class="
                      !item.node_exists || !item.node_enabled
                        ? 'text-[var(--color-error)]'
                        : item.enabled
                          ? 'text-[var(--color-success)]'
                          : 'text-[var(--text-tertiary)]'
                    "
                  >
                    <span class="h-1.5 w-1.5 rounded-full bg-current" />
                    {{
                      !item.node_exists || !item.node_enabled
                        ? "目标失效"
                        : item.enabled
                          ? "已启用"
                          : "已停用"
                    }}
                  </span>
                </td>
                <td>
                  <p
                    class="max-w-60 truncate text-[var(--text-primary)]"
                    :title="item.node_name"
                  >
                    {{ item.node_name || "节点不存在" }}
                  </p>
                  <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">
                    {{ item.node_type || "unknown" }} ·
                    {{
                      item.subscription_name || `订阅 #${item.subscription_id}`
                    }}
                  </p>
                </td>
                <td class="font-mono text-[11px] text-[var(--text-primary)]">
                  {{ displayAddress(item) }}
                </td>
                <td>
                  <span
                    v-if="item.username && item.has_password"
                    class="inline-flex items-center gap-1.5"
                  >
                    <KeyRound :size="13" class="text-[var(--color-success)]" />
                    {{ item.username }}
                  </span>
                  <span v-else class="text-[var(--text-tertiary)]">无认证</span>
                </td>
                <td>
                  <div class="flex justify-end gap-2">
                    <button
                      class="aw-control-action"
                      title="编辑"
                      @click="openEdit(item)"
                    >
                      <Pencil :size="13" />编辑
                    </button>
                    <button
                      class="aw-control-action aw-action-danger"
                      title="删除"
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
      </Card>
    </div>

    <Modal
      :open="formOpen"
      :title="editing ? '编辑节点暴露' : '新增节点暴露'"
      size="lg"
      @close="formOpen = false"
    >
      <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
        <label class="sm:col-span-2">
          <span class="aw-modal-label text-xs">名称</span>
          <input
            v-model="form.name"
            class="aw-input mt-1 w-full"
            maxlength="100"
            placeholder="例如：香港节点专线"
          />
        </label>
        <label class="sm:col-span-2">
          <span class="aw-modal-label text-xs">目标节点</span>
          <select v-model="form.nodeKey" class="aw-input mt-1 w-full">
            <option
              v-if="editing && (!editing.node_exists || !editing.node_enabled)"
              :value="nodeKey(editing.subscription_id, editing.node_uid)"
            >
              {{ editing.node_exists ? "已停用" : "已失效" }} ·
              {{ editing.node_name || editing.node_uid }}
            </option>
            <option
              v-for="node in nodes"
              :key="`${node.subscription_id}:${node.uid}`"
              :value="nodeKey(node.subscription_id, node.uid)"
            >
              {{ node.name }} · {{ node.type }} · {{ node.subscription_name }}
            </option>
          </select>
          <p
            v-if="selectedNode"
            class="mt-1 text-[11px] text-[var(--text-tertiary)]"
          >
            绑定 {{ selectedNode.subscription_name }} 中的
            {{ selectedNode.type }} 节点
          </p>
        </label>
        <label>
          <span class="aw-modal-label text-xs">入口协议</span>
          <select v-model="form.inbound_type" class="aw-input mt-1 w-full">
            <option value="mixed">Mixed</option>
            <option value="http">HTTP</option>
            <option value="socks">SOCKS</option>
          </select>
        </label>
        <label>
          <span class="aw-modal-label text-xs">监听端口</span>
          <input
            v-model.number="form.listen_port"
            class="aw-input mt-1 w-full"
            type="number"
            min="1"
            max="65535"
          />
        </label>
        <label class="sm:col-span-2">
          <span class="aw-modal-label text-xs">监听地址</span>
          <input
            v-model="form.listen"
            class="aw-input mt-1 w-full font-mono"
            placeholder="127.0.0.1"
          />
          <p
            v-if="remoteListen"
            class="mt-1 text-[11px] text-[var(--color-warning)]"
          >
            非回环监听必须配置完整认证信息。
          </p>
        </label>
        <label>
          <span class="aw-modal-label text-xs">用户名</span>
          <input
            v-model="form.username"
            class="aw-input mt-1 w-full"
            autocomplete="off"
            :disabled="form.clear_password"
          />
        </label>
        <label>
          <span class="aw-modal-label text-xs">
            {{ editing?.has_password ? "新密码（留空不变）" : "密码" }}
          </span>
          <input
            v-model="form.password"
            class="aw-input mt-1 w-full"
            type="password"
            autocomplete="new-password"
            :disabled="form.clear_password"
          />
        </label>
        <label
          v-if="editing?.has_password"
          class="sm:col-span-2 flex items-center gap-2 text-xs text-[var(--text-secondary)]"
        >
          <input v-model="form.clear_password" type="checkbox" />清除现有认证
        </label>
        <label
          class="sm:col-span-2 flex items-center gap-2 rounded-lg border border-[var(--border-default)] p-3 text-sm"
        >
          <input v-model="form.enabled" type="checkbox" />保存后启用该代理入口
        </label>
      </form>
      <template #footer>
        <Button @click="formOpen = false">取消</Button>
        <Button variant="primary" :loading="saving" @click="save">保存</Button>
      </template>
    </Modal>

    <ConfirmDialog
      :open="!!deleting"
      title="删除节点暴露"
      :message="`确定删除「${deleting?.name || ''}」吗？对应代理端口将停止监听。`"
      confirm-text="删除"
      danger
      @confirm="remove"
      @cancel="deleting = null"
    />
  </div>
</template>
