<script setup lang="ts">
import { computed } from "vue";
import type { ProxyGroup } from "@/services/clash";
import { getFlagImageURL } from "@/utils/nodeFlags";
import { proxyGroupIcon } from "./monitorUtils";
const p = withDefaults(defineProps<{ group: ProxyGroup; class?: string }>(), {
    class: "h-5 w-5",
  }),
  icon = computed(() => proxyGroupIcon(p.group)),
  flag = computed(() => /^\p{Regional_Indicator}{2}$/u.test(icon.value));
</script>
<template>
  <img
    v-if="flag"
    :src="getFlagImageURL(icon)"
    :alt="icon"
    :class="[p.class, 'object-contain']"
  /><span
    v-else
    :class="['flex items-center justify-center leading-none', p.class]"
    >{{ icon }}</span
  >
</template>
