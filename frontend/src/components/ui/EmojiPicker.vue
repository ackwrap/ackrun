<script lang="ts">
export const defaultEmojis = [
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
  "🇭🇰",
  "🇹🇼",
  "🇯🇵",
  "🇰🇷",
  "🇸🇬",
  "🇺🇸",
  "🇬🇧",
  "🇩🇪",
  "🇫🇷",
  "🇨🇦",
  "🇦🇺",
  "🇮🇳",
  "🇪🇺",
  "⭐",
  "🔥",
  "💎",
];
</script>
<script setup lang="ts">
import { computed, ref } from "vue";
const p = withDefaults(
  defineProps<{ value: string; emojis?: string[]; disabled?: boolean }>(),
  { emojis: () => defaultEmojis },
);
const emit = defineEmits<{ change: [string]; "update:value": [string] }>();
const open = ref(false),
  query = ref(""),
  custom = ref("");
const visible = computed(() =>
  p.emojis.filter((e) => e.includes(query.value.trim())),
);
function select(v: string) {
  emit("change", v);
  emit("update:value", v);
  open.value = false;
}
function apply() {
  if (custom.value.trim()) select(custom.value.trim());
}
</script>
<template>
  <div class="relative">
    <button
      type="button"
      :disabled="disabled"
      class="flex h-10 w-12 items-center justify-center rounded-md border border-[var(--border-default)]"
      @click="open = !open"
    >
      {{ value || "无" }}
    </button>
    <div
      v-if="open"
      class="absolute left-0 top-11 z-20 w-[380px] rounded-xl border border-[var(--border-default)] bg-[var(--bg-elevated)] p-3 shadow-[var(--shadow-card)]"
    >
      <div class="mb-2 flex justify-between">
        <b>选择 emoji</b><button @click="select('')">清除</button>
      </div>
      <input
        v-model="query"
        placeholder="搜索或粘贴 emoji"
        class="mb-2 w-full"
      />
      <div class="grid max-h-56 grid-cols-10 gap-1 overflow-auto">
        <button
          v-for="emoji in visible"
          :key="emoji"
          class="h-8 text-lg hover:bg-white/[0.08]"
          @click="select(emoji)"
        >
          {{ emoji }}
        </button>
      </div>
      <div class="mt-3 flex gap-2">
        <input
          v-model="custom"
          placeholder="自定义 emoji"
          @keydown.enter="apply"
        /><button @click="apply">使用</button>
      </div>
    </div>
  </div>
</template>
