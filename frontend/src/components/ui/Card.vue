<script setup lang="ts">
import { vSpotlight } from "@/directives/spotlight";

withDefaults(
  defineProps<{
    padding?: "none" | "sm" | "md" | "lg";
    hoverable?: boolean;
    spotlight?: boolean;
  }>(),
  { padding: "md", spotlight: false },
);
const pads = { none: "", sm: "p-3", md: "p-5", lg: "p-6" };
</script>
<template>
  <div
    v-spotlight="spotlight"
    class="aw-card rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-card)]"
    :class="[pads[padding], hoverable && 'card-interactive']"
  >
    <slot />
  </div>
</template>
