<script lang="ts">
export type CardTextureVariant = "dots" | "topographic" | "mesh" | "circuit";
</script>

<script setup lang="ts">
defineProps<{ variant: CardTextureVariant }>();
</script>

<template>
  <span class="aw-card-texture" :data-texture="variant" aria-hidden="true" />
</template>
