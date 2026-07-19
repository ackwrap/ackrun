<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Edit, Gauge, Plus, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type { ConnectivityTarget } from "@/services/types";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const targets = ref<ConnectivityTarget[]>([]);
const selectedURL = ref("");
const interval = ref(300);
const loading = ref(true);
const editing = ref<ConnectivityTarget | null>(null);
const formName = ref("");
const formURL = ref("");
const formEnabled = ref(true);
const deleting = ref<ConnectivityTarget | null>(null);
const enabledTargets = computed(() =>
  targets.value.filter((item) => item.enabled),
);
const input =
  "aw-input w-full outline-none focus:border-[var(--color-primary)]";

async function load() {
  loading.value = true;
  try {
    const [settings, items] = await Promise.all([
      api.getConnectivitySettings(),
      api.getConnectivityTargets(),
    ]);
    targets.value = items;
    selectedURL.value = settings.test_url;
    interval.value = settings.interval_seconds;
  } catch (error: any) {
    emit("notify", `加载连通性地址失败: ${error.message}`, "error");
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  editing.value = {} as ConnectivityTarget;
  formName.value = "";
  formURL.value = "";
  formEnabled.value = true;
}

function openEdit(item: ConnectivityTarget) {
  editing.value = item;
  formName.value = item.name;
  formURL.value = item.url;
  formEnabled.value = item.enabled;
}

async function saveTarget() {
  try {
    const body = {
      name: formName.value,
      url: formURL.value,
      enabled: formEnabled.value,
    };
    if (editing.value?.id)
      await api.updateConnectivityTarget(editing.value.id, body);
    else await api.createConnectivityTarget(body);
    editing.value = null;
    emit("notify", "连通性地址已保存");
    await load();
  } catch (error: any) {
    emit("notify", `保存失败: ${error.message}`, "error");
  }
}

async function toggle(item: ConnectivityTarget) {
  try {
    await api.updateConnectivityTarget(item.id, {
      name: item.name,
      url: item.url,
      enabled: !item.enabled,
    });
    await load();
  } catch (error: any) {
    emit("notify", `更新失败: ${error.message}`, "error");
  }
}

async function saveSettings() {
  try {
    await api.setConnectivitySettings({
      test_url: selectedURL.value,
      interval_seconds: interval.value,
    });
    emit("notify", "连通性测速设置已保存，配置将自动生成并应用");
  } catch (error: any) {
    emit("notify", `保存失败: ${error.message}`, "error");
  }
}

async function removeTarget() {
  if (!deleting.value) return;
  try {
    await api.deleteConnectivityTarget(deleting.value.id);
    deleting.value = null;
    emit("notify", "连通性地址已删除");
    await load();
  } catch (error: any) {
    emit("notify", `删除失败: ${error.message}`, "error");
  }
}

onMounted(load);
</script>

<template>
  <section
    class="flex h-full flex-col rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5 shadow-[var(--shadow-card)]"
  >
    <div class="mb-4 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <Gauge :size="18" class="text-[var(--color-primary)]" />
        <h2 class="font-semibold">连通性测速</h2>
      </div>
      <Button size="sm" variant="secondary" @click="openCreate"
        ><Plus :size="14" />添加地址</Button
      >
    </div>
    <div class="flex min-h-0 flex-1 flex-col gap-4">
      <label class="text-sm"
        >当前地址
        <select v-model="selectedURL" :class="input" :disabled="loading">
          <option
            v-for="target in enabledTargets"
            :key="target.id"
            :value="target.url"
          >
            {{ target.name }} · {{ target.url }}
          </option>
        </select>
      </label>
      <label class="text-sm"
        >连通间隔（秒）
        <input
          v-model.number="interval"
          type="number"
          min="60"
          max="3600"
          :class="input"
        />
      </label>
      <p
        class="rounded-[var(--radius-md)] bg-[var(--color-primary-bg)] p-2 text-xs text-[var(--text-secondary)]"
      >
        统一用于节点组、策略组自动 URLTest
        与后台健康检查；测速页面只使用此处已启用的地址。
      </p>
      <div class="aw-data-table-wrap h-[170px] shrink-0">
        <table class="aw-data-table h-full min-w-[620px]">
          <thead>
            <tr>
              <th>名称</th>
              <th>URL</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="4" class="text-center">加载中...</td>
            </tr>
            <tr v-for="target in targets" v-else :key="target.id">
              <td>
                {{ target.name
                }}<span
                  v-if="target.builtin"
                  class="ml-1 text-xs text-[var(--text-tertiary)]"
                  >内置</span
                >
              </td>
              <td
                class="max-w-72 truncate font-mono text-xs"
                :title="target.url"
              >
                {{ target.url }}
              </td>
              <td>
                <button
                  class="aw-action-button aw-action-neutral"
                  @click="toggle(target)"
                >
                  {{ target.enabled ? "停用" : "启用" }}
                </button>
              </td>
              <td>
                <div class="flex gap-1">
                  <button
                    class="aw-action-button aw-action-neutral"
                    title="编辑"
                    @click="openEdit(target)"
                  >
                    <Edit :size="13" />
                  </button>
                  <button
                    class="aw-action-button aw-action-danger"
                    title="删除"
                    :disabled="target.builtin"
                    @click="deleting = target"
                  >
                    <Trash2 :size="13" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Button
        class="self-start"
        size="sm"
        :disabled="!selectedURL"
        @click="saveSettings"
        >保存测速设置</Button
      >
    </div>
  </section>

  <Modal
    :open="!!editing"
    :title="editing?.id ? '编辑连通性地址' : '添加连通性地址'"
    @close="editing = null"
  >
    <div class="grid gap-4">
      <label class="text-sm"
        >名称<input v-model.trim="formName" :class="input"
      /></label>
      <label class="text-sm"
        >HTTP/HTTPS URL<input
          v-model.trim="formURL"
          :class="input"
          :disabled="editing?.builtin"
          placeholder="https://example.com/generate_204"
      /></label>
      <label class="flex items-center gap-2 text-sm"
        ><input v-model="formEnabled" type="checkbox" />启用</label
      >
    </div>
    <template #footer
      ><Button variant="secondary" @click="editing = null">取消</Button
      ><Button @click="saveTarget">保存</Button></template
    >
  </Modal>
  <ConfirmDialog
    :open="!!deleting"
    title="删除连通性地址"
    :message="`确定删除“${deleting?.name || ''}”吗？`"
    @confirm="removeTarget"
    @cancel="deleting = null"
  />
</template>
