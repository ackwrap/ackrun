<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Edit, MapPin, Plus, Trash2 } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import ConfirmDialog from "@/components/ui/ConfirmDialog.vue";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type {
  GeoIPFieldMapping,
  GeoIPProvider,
  GeoIPProviderTemplate,
} from "@/services/types";

const emit = defineEmits<{
  notify: [message: string, type?: "success" | "error" | "info"];
}>();
const providers = ref<GeoIPProvider[]>([]);
const templates = ref<GeoIPProviderTemplate[]>([]);
const loading = ref(true);
const editing = ref<GeoIPProvider | null>(null);
const deleting = ref<GeoIPProvider | null>(null);
const formName = ref("");
const formTemplate = ref("ip.sb");
const formURL = ref("");
const formIPParameter = ref("");
const formMapping = ref("{}");
const formEnabled = ref(true);
const formDefault = ref(false);
const customForm = computed(
  () => formTemplate.value === "custom" || !!editing.value?.id,
);
const input =
  "aw-input w-full outline-none focus:border-[var(--color-primary)]";

async function load() {
  loading.value = true;
  try {
    const response = await api.getGeoIPProviders();
    providers.value = response.items;
    templates.value = response.templates;
  } catch (error: any) {
    emit("notify", `加载 GeoIP Provider 失败: ${error.message}`, "error");
  } finally {
    loading.value = false;
  }
}

function applyTemplate(key: string) {
  const template = templates.value.find((item) => item.key === key);
  if (!template) return;
  formName.value = template.key === "custom" ? "" : template.name;
  formURL.value = template.url || "";
  formIPParameter.value = template.ip_parameter || "";
  formMapping.value = JSON.stringify(template.mapping || {}, null, 2);
}

function openCreate() {
  editing.value = {} as GeoIPProvider;
  formTemplate.value = templates.value[0]?.key || "custom";
  formEnabled.value = true;
  formDefault.value = false;
  applyTemplate(formTemplate.value);
}

function openEdit(item: GeoIPProvider) {
  if (item.builtin) return;
  editing.value = item;
  formName.value = item.name;
  formTemplate.value = item.template || "custom";
  formURL.value = item.url || "";
  formIPParameter.value = item.ip_parameter || "";
  formMapping.value = JSON.stringify(item.mapping || {}, null, 2);
  formEnabled.value = item.enabled;
  formDefault.value = item.is_default;
}

function requestFor(
  item: GeoIPProvider,
  changes: Partial<{ enabled: boolean; is_default: boolean }> = {},
) {
  return {
    name: item.name,
    template: item.template,
    url: item.url,
    ip_parameter: item.ip_parameter,
    mapping: item.mapping,
    enabled: changes.enabled ?? item.enabled,
    is_default: changes.is_default ?? item.is_default,
  };
}

async function saveProvider() {
  let mapping: GeoIPFieldMapping;
  try {
    mapping = JSON.parse(formMapping.value || "{}");
  } catch {
    emit("notify", "JSON 字段映射格式无效", "error");
    return;
  }
  try {
    const body = {
      name: formName.value,
      template: formTemplate.value,
      url: formURL.value,
      ip_parameter: formIPParameter.value,
      mapping,
      enabled: formEnabled.value,
      is_default: formDefault.value,
    };
    if (editing.value?.id)
      await api.updateGeoIPProvider(editing.value.id, body);
    else await api.createGeoIPProvider(body);
    editing.value = null;
    emit("notify", "GeoIP Provider 已保存");
    await load();
  } catch (error: any) {
    emit("notify", `保存失败: ${error.message}`, "error");
  }
}

async function toggle(item: GeoIPProvider) {
  try {
    await api.updateGeoIPProvider(
      item.id,
      requestFor(item, { enabled: !item.enabled }),
    );
    await load();
  } catch (error: any) {
    emit("notify", `更新失败: ${error.message}`, "error");
  }
}

async function makeDefault(item: GeoIPProvider) {
  try {
    await api.updateGeoIPProvider(
      item.id,
      requestFor(item, { enabled: true, is_default: true }),
    );
    emit("notify", `默认 GeoIP Provider 已切换为 ${item.name}`);
    await load();
  } catch (error: any) {
    emit("notify", `切换失败: ${error.message}`, "error");
  }
}

async function removeProvider() {
  if (!deleting.value) return;
  try {
    await api.deleteGeoIPProvider(deleting.value.id);
    deleting.value = null;
    emit("notify", "GeoIP Provider 已删除");
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
        <MapPin :size="18" class="text-[var(--color-primary)]" />
        <h2 class="font-semibold">GeoIP Provider</h2>
      </div>
      <Button size="sm" variant="secondary" @click="openCreate"
        ><Plus :size="14" />添加 Provider</Button
      >
    </div>
    <p class="mb-3 text-xs text-[var(--text-secondary)]">
      出口 IP
      与路由追踪页面从这里读取已启用接口；自定义接口由后端直连请求，不经过当前代理节点。
    </p>
    <div class="aw-data-table-wrap min-h-[170px] flex-1">
      <table class="aw-data-table h-full min-w-[700px]">
        <thead>
          <tr>
            <th>名称</th>
            <th>类型</th>
            <th>状态</th>
            <th>默认</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="5" class="text-center">加载中...</td>
          </tr>
          <tr v-for="provider in providers" v-else :key="provider.id">
            <td>{{ provider.name }}</td>
            <td>{{ provider.builtin ? "内置" : provider.template }}</td>
            <td>
              <button
                class="aw-action-button aw-action-neutral"
                @click="toggle(provider)"
              >
                {{ provider.enabled ? "启用" : "停用" }}
              </button>
            </td>
            <td>
              <button
                class="aw-action-button aw-action-neutral"
                :disabled="provider.is_default"
                @click="makeDefault(provider)"
              >
                {{ provider.is_default ? "当前默认" : "设为默认" }}
              </button>
            </td>
            <td>
              <div class="flex gap-1">
                <button
                  class="aw-action-button aw-action-neutral"
                  title="编辑"
                  :disabled="provider.builtin"
                  @click="openEdit(provider)"
                >
                  <Edit :size="13" />
                </button>
                <button
                  class="aw-action-button aw-action-danger"
                  title="删除"
                  :disabled="provider.builtin"
                  @click="deleting = provider"
                >
                  <Trash2 :size="13" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>

  <Modal
    :open="!!editing"
    :title="editing?.id ? '编辑 GeoIP Provider' : '添加 GeoIP Provider'"
    size="lg"
    @close="editing = null"
  >
    <div class="grid gap-4">
      <label class="text-sm"
        >预置模板<select
          v-model="formTemplate"
          :class="input"
          :disabled="!!editing?.id"
          @change="applyTemplate(formTemplate)"
        >
          <option
            v-for="template in templates"
            :key="template.key"
            :value="template.key"
          >
            {{ template.name }}
          </option>
        </select></label
      >
      <label class="text-sm"
        >名称<input v-model.trim="formName" :class="input"
      /></label>
      <template v-if="customForm">
        <label class="text-sm"
          >HTTPS URL<input
            v-model.trim="formURL"
            :class="input"
            placeholder="https://api.example.com/geo/{ip}"
        /></label>
        <label class="text-sm"
          >IP 查询参数名<input
            v-model.trim="formIPParameter"
            :class="input"
            placeholder="URL 含 {ip} 时可留空"
        /></label>
        <label class="text-sm"
          >JSON 字段映射<textarea
            v-model="formMapping"
            :class="input"
            class="min-h-52 font-mono text-xs"
            spellcheck="false"
          />
        </label>
      </template>
      <div class="flex flex-wrap gap-5 text-sm">
        <label class="flex items-center gap-2"
          ><input v-model="formEnabled" type="checkbox" />启用</label
        ><label class="flex items-center gap-2"
          ><input v-model="formDefault" type="checkbox" />设为默认</label
        >
      </div>
      <p class="text-xs text-[var(--text-tertiary)]">
        字段路径使用点号，例如 data.country；数组下标使用数字，例如
        data.0.location。至少配置 country 或 country_code。
      </p>
    </div>
    <template #footer
      ><Button variant="secondary" @click="editing = null">取消</Button
      ><Button @click="saveProvider">保存</Button></template
    >
  </Modal>
  <ConfirmDialog
    :open="!!deleting"
    title="删除 GeoIP Provider"
    :message="`确定删除“${deleting?.name || ''}”吗？`"
    @confirm="removeProvider"
    @cancel="deleting = null"
  />
</template>
