<script setup lang="ts">
import { getFlagImageURL } from "@/utils/nodeFlags";

const props = withDefaults(
  defineProps<{
    name?: string;
    flag?: string;
    fallback?: string;
  }>(),
  { name: "", flag: "", fallback: "(未命名节点)" },
);
</script>

<template>
  <span class="inline-flex min-w-0 items-center gap-1.5" :title="name">
    <img
      v-if="flag"
      :src="getFlagImageURL(flag)"
      :alt="flag"
      class="h-4 w-4 shrink-0"
    />
    <span class="truncate"
      ><slot>{{ name || fallback }}</slot></span
    >
  </span>
</template>
