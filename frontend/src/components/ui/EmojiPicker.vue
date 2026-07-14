<script lang="ts">
export const emojiGroups = [
  {
    key: "common",
    label: "常用",
    emojis: [
      "🌐",
      "🤖",
      "🎬",
      "📺",
      "🎮",
      "💬",
      "🛡️",
      "🚫",
      "🇨🇳",
      "✈️",
      "⚡",
      "🚀",
      "✨",
      "🔎",
      "🧠",
      "☁️",
    ],
  },
  {
    key: "network",
    label: "网络",
    emojis: [
      "🌐",
      "🛜",
      "📡",
      "🛰️",
      "☁️",
      "🔗",
      "🧭",
      "🧬",
      "🧠",
      "🤖",
      "💬",
      "📨",
      "📧",
      "🔎",
      "💻",
      "📱",
      "🖥️",
      "⌨️",
      "🖱️",
      "🧰",
      "🧪",
      "📦",
      "📁",
      "🗂️",
    ],
  },
  {
    key: "media",
    label: "娱乐",
    emojis: [
      "🎬",
      "📺",
      "🎵",
      "🎧",
      "🎤",
      "🎮",
      "🕹️",
      "📹",
      "📷",
      "🎞️",
      "🎥",
      "🍿",
      "🎭",
      "🎨",
      "📚",
      "📰",
      "🏀",
      "⚽",
      "🏈",
      "🎾",
      "🎲",
      "🧩",
    ],
  },
  {
    key: "security",
    label: "安全",
    emojis: [
      "🛡️",
      "🚫",
      "🔒",
      "🔓",
      "🔐",
      "🔑",
      "🧱",
      "⚠️",
      "✅",
      "❌",
      "⛔",
      "🧯",
      "🚨",
      "👁️",
      "🕵️",
      "🧹",
      "🗑️",
      "📛",
      "🔞",
      "☢️",
      "☣️",
    ],
  },
  {
    key: "region",
    label: "地区",
    emojis: [
      "🇨🇳",
      "🇭🇰",
      "🇲🇴",
      "🇹🇼",
      "🇯🇵",
      "🇰🇷",
      "🇸🇬",
      "🇺🇸",
      "🇬🇧",
      "🇩🇪",
      "🇫🇷",
      "🇳🇱",
      "🇨🇦",
      "🇦🇺",
      "🇮🇳",
      "🇷🇺",
      "🇧🇷",
      "🇪🇺",
      "🇹🇭",
      "🇻🇳",
      "🇲🇾",
      "🇵🇭",
      "🇮🇩",
      "🇹🇷",
    ],
  },
  {
    key: "symbol",
    label: "符号",
    emojis: [
      "⭐",
      "🌟",
      "✨",
      "🔥",
      "⚡",
      "💎",
      "🎯",
      "📌",
      "📍",
      "🔴",
      "🟠",
      "🟡",
      "🟢",
      "🔵",
      "🟣",
      "⚫",
      "⚪",
      "🟤",
      "🔺",
      "🔻",
      "🔸",
      "🔹",
      "🔶",
      "🔷",
    ],
  },
  {
    key: "more",
    label: "更多",
    emojis: [
      "😀",
      "😄",
      "😁",
      "😎",
      "🤔",
      "😺",
      "🐶",
      "🐱",
      "🦊",
      "🐼",
      "🐳",
      "🦄",
      "🌈",
      "☀️",
      "🌙",
      "⭐",
      "🌍",
      "🏠",
      "🏢",
      "🚗",
      "🚄",
      "✈️",
      "🚢",
      "⏱️",
      "📅",
      "💰",
      "💡",
      "🔧",
      "🧲",
      "🪄",
    ],
  },
] as const;

export const defaultEmojis = emojiGroups
  .flatMap((group) => [...group.emojis])
  .filter((emoji, index, items) => items.indexOf(emoji) === index);
</script>

<script setup lang="ts">
import { computed, ref } from "vue";
import { getFlagImageURL } from "@/utils/nodeFlags";

const props = withDefaults(
  defineProps<{ value: string; emojis?: string[]; disabled?: boolean }>(),
  { emojis: () => defaultEmojis },
);

const emit = defineEmits<{ change: [string]; "update:value": [string] }>();
const open = ref(false);
const query = ref("");
const custom = ref("");
type EmojiGroupKey = (typeof emojiGroups)[number]["key"];
const activeGroup = ref<EmojiGroupKey>(emojiGroups[0].key);

const visible = computed(() => {
  if (query.value.trim())
    return props.emojis.filter((emoji) => emoji.includes(query.value.trim()));
  return (
    emojiGroups.find((group) => group.key === activeGroup.value)?.emojis ??
    props.emojis
  );
});

const isFlag = (emoji: string) => /^\p{Regional_Indicator}{2}$/u.test(emoji);

function select(value: string) {
  emit("change", value);
  emit("update:value", value);
  open.value = false;
}

function selectGroup(key: EmojiGroupKey) {
  activeGroup.value = key;
  query.value = "";
}

function apply() {
  const value = custom.value.trim();
  if (!value) return;
  select(value);
  custom.value = "";
}
</script>

<template>
  <div class="relative">
    <button
      type="button"
      :disabled="disabled"
      class="flex h-[34px] w-12 items-center justify-center rounded-md border border-[var(--border-default)] bg-[var(--button-secondary-bg)] text-sm transition-colors hover:border-[var(--button-primary-border)] hover:bg-[var(--button-secondary-hover)]"
      title="选择 emoji"
      @click="open = !open"
    >
      <img
        v-if="value && isFlag(value)"
        :src="getFlagImageURL(value)"
        :alt="value"
        class="h-5 w-5 object-contain"
      />
      <span v-else>{{ value || "无" }}</span>
    </button>

    <div
      v-if="open"
      class="absolute left-0 top-10 z-20 w-[min(380px,calc(100vw-3rem))] rounded-xl border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 shadow-[var(--shadow-card)]"
    >
      <div class="mb-2 flex items-center justify-between gap-2">
        <span class="text-xs font-medium text-[var(--text-primary)]"
          >选择 emoji</span
        >
        <button
          type="button"
          class="aw-action-button aw-action-neutral !h-7"
          @click="select('')"
        >
          清除
        </button>
      </div>

      <input
        v-model="query"
        placeholder="搜索或粘贴 emoji"
        class="mb-2 h-8 w-full"
      />

      <div class="mb-2 flex gap-1 overflow-x-auto pb-1">
        <button
          v-for="group in emojiGroups"
          :key="group.key"
          type="button"
          class="shrink-0 rounded-md border px-2 py-1 text-xs transition-colors"
          :class="
            activeGroup === group.key && !query
              ? 'border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] text-[var(--button-primary-text)]'
              : 'border-[var(--border-default)] bg-[var(--button-secondary-bg)] text-[var(--text-secondary)] hover:bg-[var(--button-secondary-hover)]'
          "
          @click="selectGroup(group.key)"
        >
          {{ group.label }}
        </button>
      </div>

      <div class="grid max-h-56 grid-cols-10 gap-1 overflow-auto pr-1">
        <button
          v-for="emoji in visible"
          :key="emoji"
          type="button"
          class="flex h-8 items-center justify-center rounded-md bg-[var(--button-secondary-bg)] text-base transition-colors hover:bg-[var(--button-secondary-hover)]"
          :class="value === emoji ? 'ring-1 ring-[var(--color-primary)]' : ''"
          :title="emoji"
          @click="select(emoji)"
        >
          <img
            v-if="isFlag(emoji)"
            :src="getFlagImageURL(emoji)"
            :alt="emoji"
            class="h-4 w-4 object-contain"
            loading="lazy"
          />
          <span v-else>{{ emoji }}</span>
        </button>
        <div
          v-if="!visible.length"
          class="col-span-10 py-5 text-center text-xs text-[var(--text-tertiary)]"
        >
          没有匹配的 emoji，可在下方自定义输入
        </div>
      </div>

      <div class="mt-3 grid grid-cols-[minmax(0,1fr)_auto] gap-2">
        <input
          v-model="custom"
          placeholder="自定义 emoji"
          class="h-8"
          @keydown.enter="apply"
        />
        <button
          type="button"
          class="aw-action-button aw-action-success !h-8"
          @click="apply"
        >
          使用
        </button>
      </div>
    </div>
  </div>
</template>
