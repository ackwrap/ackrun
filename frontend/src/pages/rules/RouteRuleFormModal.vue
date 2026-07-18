<script setup lang="ts">
import { computed } from "vue";
import { FileJson2, Route, Save } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import EmojiPicker, { defaultEmojis } from "@/components/ui/EmojiPicker.vue";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";

const props = defineProps<{
  editing: RouteRule | null;
  name: string;
  enabled: boolean;
  ruleType: string;
  valuesText: string;
  outbound: string;
  invert: boolean;
  subscriptions: RouteRuleSubscription[];
}>();

const emit = defineEmits<{
  close: [];
  save: [];
  "update:name": [string];
  "update:enabled": [boolean];
  "update:ruleType": [string];
  "update:valuesText": [string];
  "update:outbound": [string];
  "update:invert": [boolean];
}>();

const types = [
  ["domain", "完整域名"],
  ["domain_suffix", "域名后缀"],
  ["domain_keyword", "域名关键词"],
  ["ip_cidr", "IP CIDR"],
  ["process_name", "进程名称"],
  ["geoip", "GeoIP"],
  ["geosite", "GeoSite"],
  ["rule_set", "规则集"],
  ["mixed", "混合规则"],
] as const;

const ruleEmojis = defaultEmojis;

function looksLikeEmojiPrefix(value: string) {
  return /^([\p{Extended_Pictographic}\p{Regional_Indicator}\uFE0F\u200D]+)$/u.test(
    value,
  );
}

function stripRuleEmoji(value: string) {
  const trimmed = value.trimStart();
  for (const emoji of ruleEmojis) {
    if (trimmed === emoji) return "";
    if (trimmed.startsWith(`${emoji} `))
      return trimmed.slice(`${emoji} `.length);
    if (trimmed.startsWith(emoji))
      return trimmed.slice(emoji.length).trimStart();
  }
  const [first, ...rest] = trimmed.split(/\s+/);
  return first && looksLikeEmojiPrefix(first) ? rest.join(" ") : value;
}

const selectedEmoji = computed(() => {
  const trimmed = props.name.trimStart();
  const known = ruleEmojis.find(
    (emoji) =>
      trimmed === emoji ||
      trimmed.startsWith(`${emoji} `) ||
      trimmed.startsWith(emoji),
  );
  if (known) return known;
  const first = trimmed.split(/\s+/)[0] || "";
  return looksLikeEmojiPrefix(first) ? first : "";
});

function updateEmoji(emoji: string) {
  const plainName = stripRuleEmoji(props.name);
  emit("update:name", emoji ? `${emoji} ${plainName}`.trim() : plainName);
}

const title = computed(() =>
  props.editing?.is_system
    ? "查看路由规则"
    : props.editing
      ? "编辑路由规则"
      : "添加路由规则",
);

const preview = computed(() =>
  JSON.stringify(
    [
      {
        [props.ruleType]: props.valuesText
          .split("\n")
          .map((value) => value.trim())
          .filter(Boolean),
        outbound: props.outbound,
        ...(props.invert ? { invert: true } : {}),
      },
    ],
    null,
    2,
  ),
);

function update(
  event: Event,
  field: "name" | "ruleType" | "valuesText" | "outbound",
) {
  const value = (
    event.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  ).value;
  if (field === "name") emit("update:name", value);
  else if (field === "ruleType") emit("update:ruleType", value);
  else if (field === "valuesText") emit("update:valuesText", value);
  else emit("update:outbound", value);
}
</script>

<template>
  <Modal :open="true" :title="title" size="xl" @close="emit('close')">
    <template #title>
      <span class="flex items-center gap-2"
        ><Route :size="18" />{{ title }}</span
      >
    </template>

    <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(360px,0.9fr)]">
      <div class="space-y-4">
        <label class="block text-xs font-medium">
          规则名称
          <div class="mt-1.5 grid grid-cols-[48px_minmax(0,1fr)] gap-2">
            <EmojiPicker
              :value="selectedEmoji"
              :disabled="editing?.is_system"
              @change="updateEmoji"
            />
            <input
              :value="name"
              class="!mt-0"
              :disabled="editing?.is_system"
              placeholder="例如：🤖 OpenAI 代理"
              @input="update($event, 'name')"
            />
          </div>
        </label>

        <div class="grid gap-3 sm:grid-cols-2">
          <label class="block text-xs font-medium">
            匹配类型
            <select
              :value="ruleType"
              :disabled="editing?.is_system"
              @change="update($event, 'ruleType')"
            >
              <option v-for="item in types" :key="item[0]" :value="item[0]">
                {{ item[1] }}
              </option>
            </select>
          </label>
          <label class="block text-xs font-medium">
            命中后出站
            <select
              :value="outbound"
              :disabled="editing?.is_system"
              @change="update($event, 'outbound')"
            >
              <option value="direct">直连 direct</option>
              <option value="proxy">策略 proxy</option>
              <option value="block">阻断 block</option>
            </select>
          </label>
        </div>

        <label class="block text-xs font-medium">
          匹配值
          <textarea
            :value="valuesText"
            rows="9"
            class="min-h-[210px] w-full font-mono"
            :disabled="editing?.is_system"
            :placeholder="
              ruleType === 'process_name'
                ? '每行一个进程名，例如 chrome.exe'
                : '每行一个匹配值'
            "
            @input="update($event, 'valuesText')"
          />
          <span
            class="mt-1.5 block text-[11px] font-normal text-[var(--text-tertiary)]"
          >
            <template v-if="ruleType === 'process_name'">
              每行一个进程名；Windows 通常包含 .exe，Linux/macOS 填写可执行文件名。
            </template>
            <template v-else>
              每行一个值，保存时自动清理空行和重复项。
            </template>
          </span>
        </label>

        <div v-if="ruleType === 'rule_set' && subscriptions.length">
          <div class="mb-2 text-xs font-medium">可用规则集</div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="subscription in subscriptions"
              :key="subscription.id"
              class="aw-filter-chip"
              :disabled="editing?.is_system"
              @click="
                emit(
                  'update:valuesText',
                  [valuesText, subscription.tag].filter(Boolean).join('\n'),
                )
              "
            >
              {{ subscription.tag }}
            </button>
          </div>
        </div>

        <div
          class="flex flex-wrap gap-5 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2.5 text-xs"
        >
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              :checked="enabled"
              :disabled="editing?.is_system"
              @change="
                emit(
                  'update:enabled',
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />启用规则
          </label>
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              :checked="invert"
              :disabled="editing?.is_system"
              @change="
                emit(
                  'update:invert',
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />反向匹配
          </label>
        </div>
      </div>

      <aside
        class="min-w-0 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)]"
      >
        <header
          class="flex items-center gap-2 border-b border-[var(--border-light)] px-4 py-3 font-medium"
        >
          <FileJson2 :size="16" /> sing-box 规则预览
        </header>
        <pre
          class="max-h-[480px] overflow-auto p-4 text-xs leading-6 text-[var(--text-secondary)]"
          >{{ preview }}</pre>
      </aside>
    </div>

    <template #footer>
      <button class="aw-action-button aw-action-neutral" @click="emit('close')">
        {{ editing?.is_system ? "关闭" : "取消" }}
      </button>
      <button
        v-if="!editing?.is_system"
        class="aw-action-button aw-action-success"
        @click="emit('save')"
      >
        <Save :size="14" />保存规则
      </button>
    </template>
  </Modal>
</template>
