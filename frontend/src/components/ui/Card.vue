<script setup lang="ts">
import { vSpotlight } from "@/directives/spotlight";
import CardTexture, { type CardTextureVariant } from "./CardTexture.vue";

withDefaults(
  defineProps<{
    padding?: "none" | "sm" | "md" | "lg";
    hoverable?: boolean;
    spotlight?: boolean;
    texture?: CardTextureVariant;
  }>(),
  { padding: "md", spotlight: false },
);
const pads = { none: "", sm: "p-3", md: "p-5", lg: "p-6" };
</script>
<template>
  <div
    v-spotlight="spotlight"
    class="aw-card rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-card)]"
    :class="[
      pads[padding],
      hoverable && 'card-interactive',
      texture && 'aw-textured',
    ]"
  >
    <CardTexture v-if="texture" :variant="texture" />
    <slot />
  </div>
</template>
