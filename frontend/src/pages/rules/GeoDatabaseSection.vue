<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Database } from "lucide-vue-next";
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
async function lookup() {
  try {
    error.value = "";
    result.value = await api.lookupGeo(target.value.trim(), dns.value);
  } catch (e: any) {
    error.value = e.message;
  }
}
async function lookupTag(offset = 0) {
  try {
    tagError.value = "";
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
        class="min-w-0 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3.5"
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
      </article>
    </div>
    <div class="mt-4 grid gap-3 lg:grid-cols-2">
      <div
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
      >
        <h4 class="font-medium">Geo 数据库深度查询</h4>
        <div class="mt-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_160px_auto]">
          <input
            v-model="target"
            placeholder="域名或 IP"
            @keydown.enter="lookup"
          />
          <select v-model="dns">
            <option
              v-for="x in [
                'cloudflare-doh',
                'google-doh',
                'aliyun-doh',
                'tencent-doh',
                'system',
              ]"
              :key="x"
            >
              {{ x }}
            </option>
          </select>
          <button class="aw-action-button aw-action-neutral" @click="lookup">
            查询
          </button>
        </div>
        <p v-if="error" class="mt-2 text-xs text-red-400">{{ error }}</p>
        <pre
          v-if="result"
          class="mt-3 max-h-48 overflow-auto rounded-[var(--radius-md)] bg-[var(--bg-surface)] p-3 text-xs"
          >{{ JSON.stringify(result, null, 2) }}</pre>
      </div>
      <div
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-4"
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
            @click="lookupTag()"
          >
            反查条目
          </button>
        </div>
        <p class="mt-2 text-xs text-[var(--text-tertiary)]">
          查询标签包含的域名、CIDR 或关联条目。
        </p>
        <p v-if="tagError" class="mt-2 text-xs text-red-400">{{ tagError }}</p>
      </div>
    </div>
    <div v-if="tags" class="aw-modal-backdrop">
      <div class="aw-modal-panel w-full max-w-3xl p-5">
        <header class="mb-4 flex items-center justify-between gap-3">
          <h3 class="font-semibold">{{ tags.tag }} · 共 {{ tags.total }} 条</h3>
          <button class="aw-modal-close" @click="tags = null">关闭</button>
        </header>
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
        <div class="mt-4 flex justify-end gap-2">
          <button
            class="aw-action-button aw-action-neutral"
            :disabled="!tags.offset"
            @click="lookupTag(Math.max(0, tags!.offset - tags!.limit))"
          >
            上一页
          </button>
          <button
            class="aw-action-button aw-action-neutral"
            :disabled="tags.offset + tags.limit >= tags.total"
            @click="lookupTag(tags!.offset + tags!.limit)"
          >
            下一页
          </button>
        </div>
      </div>
    </div>
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
