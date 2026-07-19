<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Database } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import { api } from "@/services/api";
import type {
  GeoAsset,
  GeoAssetRequest,
  GeoLookupResponse,
  GeoDomainsResponse,
} from "@/services/types";
const p = defineProps<{ geoAssets: GeoAsset[]; syncing: boolean }>(),
  emit = defineEmits<{
    syncAll: [];
    syncOne: [GeoAsset];
    update: [GeoAsset, GeoAssetRequest];
  }>(),
  target = ref(""),
  dns = ref("cloudflare-doh"),
  result = ref<GeoLookupResponse | null>(null),
  error = ref(""),
  tag = ref(""),
  tags = ref<GeoDomainsResponse | null>(null),
  tagError = ref(""),
  drafts = ref<Record<number, GeoAssetRequest>>({}),
  editing = ref<number | null>(null);
watch(
  () => p.geoAssets,
  (a) =>
    a.forEach(
      (x) =>
        (drafts.value[x.id] ??= {
          url: x.url,
          use_proxy: x.use_proxy,
          sync_mode: x.sync_mode || "off",
          sync_time: x.sync_time || "03:30:00",
          sync_weekday: x.sync_weekday || 0,
        }),
    ),
  { immediate: true },
);
const asset = computed(() => p.geoAssets.find((x) => x.id === editing.value));
const tagResultTitle = computed(() =>
  tags.value ? `${tags.value.tag} · 共 ${tags.value.total} 条` : "GeoSite 反查结果",
);
const geoLookupResultTitle = computed(() =>
  result.value?.target
    ? `Geo 查询结果 · ${result.value.target}`
    : "GeoIP 查询结果",
);
function formatUpdatedAt(value: number) {
  if (!value) return "尚未更新";
  const timestamp = value < 1_000_000_000_000 ? value * 1000 : value;
  return new Date(timestamp).toLocaleString();
}
async function lookup() {
  try {
    error.value = "";
    if (!p.geoAssets.some((x) => x.type === "geoip" && x.available)) {
      error.value = "GeoIP 数据库文件不存在，请先更新 GeoIP 数据库";
      return;
    }
    result.value = await api.lookupGeo(target.value.trim(), dns.value);
  } catch (e: any) {
    error.value = e.message;
  }
}
async function lookupTag(offset = 0) {
  try {
    tagError.value = "";
    if (!p.geoAssets.some((x) => x.type === "geosite" && x.available)) {
      tagError.value = "GeoSite 数据库文件不存在，请先更新 GeoSite 数据库";
      return;
    }
    tags.value = await api.lookupGeositeDomains(tag.value.trim(), 100, offset);
  } catch (e: any) {
    tagError.value = e.message;
  }
}
</script>
<template>
  <section
    class="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-5"
  >
    <header class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="flex items-center gap-2 font-semibold">
          <Database :size="17" /> Geo 数据库
        </h3>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          管理 GeoIP 与 GeoSite 数据，并提供域名和标签查询。
        </p>
      </div>
      <button
        class="aw-action-button aw-action-neutral"
        :disabled="syncing"
        @click="$emit('syncAll')"
      >
        {{ syncing ? "更新中..." : "更新全部 Geo" }}
      </button>
    </header>
    <div class="grid gap-3 lg:grid-cols-2">
      <article
        v-for="x in geoAssets"
        :key="x.id"
        class="min-w-0 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
      >
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <b>{{ x.name }}</b>
              <span
                class="rounded-full bg-[var(--button-secondary-bg)] px-2 py-0.5 text-[11px] text-[var(--text-secondary)]"
              >
                {{ x.type }}.db
              </span>
              <span
                class="text-[11px]"
                :class="
                  x.sync_status === 'failed'
                    ? 'text-red-400'
                    : x.sync_status === 'syncing'
                      ? 'text-blue-400'
                      : 'text-[var(--text-tertiary)]'
                "
              >
                {{ x.sync_status }}
              </span>
            </div>
            <small
              class="mt-1 block truncate text-[var(--text-tertiary)]"
              :title="x.url"
            >
              {{ x.url }}
            </small>
          </div>
          <div class="flex shrink-0 gap-2">
            <button
              class="aw-action-button aw-action-neutral"
              :disabled="syncing"
              @click="$emit('syncOne', x)"
            >
              更新
            </button>
            <button
              class="aw-action-button aw-action-neutral"
              @click="editing = x.id"
            >
              自动更新
            </button>
          </div>
        </div>
        <p v-if="x.sync_error" class="mt-2 text-xs text-red-400">
          {{ x.sync_error }}
        </p>
        <div
          v-if="x.type === 'geoip'"
          class="mt-4 border-t border-[var(--border-light)] pt-4"
        >
          <h4 class="font-medium">GeoIP 数据库深度查询</h4>
          <div class="mt-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_160px_auto]">
            <input
              v-model="target"
              placeholder="域名或 IP"
              @keydown.enter="lookup"
            />
            <select v-model="dns">
              <option
                v-for="server in [
                  'cloudflare-doh',
                  'google-doh',
                  'aliyun-doh',
                  'tencent-doh',
                  'system',
                ]"
                :key="server"
              >
                {{ server }}
              </option>
            </select>
            <button
              class="aw-action-button aw-action-neutral"
              :disabled="!x.available"
              :title="x.available ? '查询 GeoIP' : '数据库文件不存在，请先更新'"
              @click="lookup"
            >
              查询
            </button>
          </div>
          <p v-if="!x.available" class="mt-2 text-xs text-[var(--text-tertiary)]">
            数据库文件不存在，请先点击“更新”后查询。
          </p>
          <p v-if="error" class="mt-2 text-xs text-red-400">{{ error }}</p>
        </div>
        <div
          v-else-if="x.type === 'geosite'"
          class="mt-4 border-t border-[var(--border-light)] pt-4"
        >
          <h4 class="font-medium">GeoSite tag 条目反查</h4>
          <div class="mt-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
            <input
              v-model="tag"
              placeholder="输入 GeoSite tag"
              @keydown.enter="lookupTag()"
            />
            <button
              class="aw-action-button aw-action-neutral"
              :disabled="!x.available"
              :title="x.available ? '反查 GeoSite 条目' : '数据库文件不存在，请先更新'"
              @click="lookupTag()"
            >
              反查条目
            </button>
          </div>
          <p v-if="!x.available" class="mt-2 text-xs text-[var(--text-tertiary)]">
            数据库文件不存在，请先点击“更新”后反查。
          </p>
          <p class="mt-2 text-xs text-[var(--text-tertiary)]">
            查询标签包含的域名、CIDR 或关联条目。
          </p>
          <p v-if="tagError" class="mt-2 text-xs text-red-400">
            {{ tagError }}
          </p>
        </div>
      </article>
    </div>
    <Modal
      :open="!!result"
      :title="geoLookupResultTitle"
      size="lg"
      @close="result = null"
    >
      <div v-if="result" class="space-y-4">
        <div class="grid gap-3 sm:grid-cols-3">
          <div
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
          >
            <small class="text-[var(--text-tertiary)]">查询目标</small>
            <strong class="mt-1 block break-all text-sm">{{ result.target }}</strong>
          </div>
          <div
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
          >
            <small class="text-[var(--text-tertiary)]">目标类型</small>
            <strong class="mt-1 block text-sm">
              {{ result.target_type === "ip" ? "IP 地址" : "域名" }}
            </strong>
          </div>
          <div
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
          >
            <small class="text-[var(--text-tertiary)]">DNS 解析</small>
            <strong class="mt-1 block truncate text-sm">
              {{ result.target_type === "domain" ? result.dns_server : "无需解析" }}
            </strong>
          </div>
        </div>

        <section v-if="result.target_type === 'domain'">
          <h4 class="mb-2 font-medium">解析地址</h4>
          <div v-if="result.resolved_ips.length" class="flex flex-wrap gap-2">
            <span
              v-for="ip in result.resolved_ips"
              :key="ip"
              class="rounded-full border border-[var(--border-default)] bg-[var(--bg-base)] px-2.5 py-1 font-mono text-xs"
            >
              {{ ip }}
            </span>
          </div>
          <p v-else class="text-xs text-[var(--text-tertiary)]">未解析到 IP 地址</p>
        </section>

        <div class="grid gap-3" :class="result.target_type === 'domain' ? 'sm:grid-cols-2' : ''">
          <section
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
          >
            <div class="mb-3 flex items-center justify-between gap-3">
              <h4 class="font-medium">GeoIP 命中</h4>
              <span class="text-xs text-[var(--text-tertiary)]">
                {{ result.geoip_matches.length }} 条
              </span>
            </div>
            <div v-if="result.geoip_matches.length" class="space-y-2">
              <div
                v-for="match in result.geoip_matches"
                :key="match"
                class="break-all rounded-[var(--radius-md)] bg-[var(--bg-surface)] px-3 py-2 font-mono text-xs"
              >
                {{ match }}
              </div>
            </div>
            <p v-else class="text-xs text-[var(--text-tertiary)]">暂无匹配结果</p>
          </section>

          <section
            v-if="result.target_type === 'domain'"
            class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
          >
            <div class="mb-3 flex items-center justify-between gap-3">
              <h4 class="font-medium">GeoSite 命中</h4>
              <span class="text-xs text-[var(--text-tertiary)]">
                {{ result.geosite_matches.length }} 条
              </span>
            </div>
            <div v-if="result.geosite_matches.length" class="flex flex-wrap gap-2">
              <span
                v-for="match in result.geosite_matches"
                :key="match"
                class="rounded-full border border-[var(--border-default)] bg-[var(--bg-surface)] px-2.5 py-1 text-xs"
              >
                {{ match }}
              </span>
            </div>
            <p v-else class="text-xs text-[var(--text-tertiary)]">暂无匹配结果</p>
          </section>
        </div>

        <section>
          <h4 class="mb-2 font-medium">数据库状态</h4>
          <div class="grid gap-2 sm:grid-cols-2">
            <div
              v-for="item in result.geo_assets"
              :key="item.type"
              class="flex items-start justify-between gap-3 rounded-[var(--radius-lg)] border border-[var(--border-default)] px-3 py-2.5"
            >
              <div class="min-w-0">
                <b>{{ item.name }}</b>
                <small class="mt-0.5 block text-[var(--text-tertiary)]">
                  {{ formatUpdatedAt(item.updated_at) }}
                </small>
                <small
                  v-if="item.error"
                  class="mt-1 block text-[var(--color-error)]"
                >
                  {{ item.error }}
                </small>
              </div>
              <span
                class="shrink-0 rounded-full px-2 py-0.5 text-[11px]"
                :class="
                  item.ready
                    ? 'bg-[var(--color-success-bg)] text-[var(--color-success)]'
                    : 'bg-[var(--color-error-bg)] text-[var(--color-error)]'
                "
              >
                {{ item.ready ? "可用" : "未就绪" }}
              </span>
            </div>
          </div>
        </section>

        <p
          v-if="result.message && result.message !== '查询完成'"
          class="rounded-[var(--radius-lg)] border border-[var(--color-warning)] bg-[var(--color-warning-bg)] px-3 py-2 text-xs text-[var(--color-warning)]"
        >
          {{ result.message }}
        </p>
      </div>
    </Modal>
    <Modal
      :open="!!tags"
      :title="tagResultTitle"
      size="lg"
      @close="tags = null"
    >
      <template v-if="tags">
        <div v-if="tags.suggestions.length" class="mb-3 flex flex-wrap gap-2">
          <button
            v-for="s in tags.suggestions"
            :key="s"
            class="aw-filter-chip"
            @click="
              tag = s;
              lookupTag();
            "
          >
            {{ s }}
          </button>
        </div>
        <div
          class="max-h-[55vh] overflow-auto rounded-[var(--radius-lg)] border border-[var(--border-default)]"
        >
          <div
            v-for="x in tags.items"
            :key="`${x.type}:${x.value}`"
            class="border-b border-[var(--border-light)] px-3 py-2 last:border-b-0"
          >
            <span class="mr-2 text-[var(--text-tertiary)]">{{ x.type }}</span
            >{{ x.value }}
          </div>
        </div>
      </template>
      <template #footer>
          <button
            class="aw-action-button aw-action-neutral"
            :disabled="!tags?.offset"
            @click="lookupTag(Math.max(0, tags!.offset - tags!.limit))"
          >
            上一页
          </button>
          <button
            class="aw-action-button aw-action-neutral"
            :disabled="!tags || tags.offset + tags.limit >= tags.total"
            @click="lookupTag(tags!.offset + tags!.limit)"
          >
            下一页
          </button>
      </template>
    </Modal>
    <div v-if="asset" class="aw-modal-backdrop">
      <div class="aw-modal-panel w-full max-w-xl p-5">
        <h3 class="mb-4 font-semibold">{{ asset.name }} 自动更新</h3>
        <div class="grid gap-3 sm:grid-cols-3">
          <label class="text-xs"
            >周期
            <select v-model="drafts[asset.id].sync_mode">
              <option
                v-for="x in ['off', 'daily', 'weekly', 'monthly']"
                :key="x"
              >
                {{ x }}
              </option>
            </select>
          </label>
          <label class="text-xs"
            >时间
            <input v-model="drafts[asset.id].sync_time" type="time" step="1" />
          </label>
          <label class="text-xs"
            >星期/日期
            <input
              v-model.number="drafts[asset.id].sync_weekday"
              type="number"
            />
          </label>
        </div>
        <label class="mt-4 flex items-center gap-2 text-xs">
          <input
            v-model="drafts[asset.id].use_proxy"
            type="checkbox"
          />下载走代理
        </label>
        <div class="mt-5 flex justify-end gap-2">
          <button
            class="aw-action-button aw-action-neutral"
            @click="editing = null"
          >
            取消
          </button>
          <button
            class="aw-action-button aw-action-success"
            @click="
              $emit('update', asset!, drafts[asset!.id]);
              editing = null;
            "
          >
            保存
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
