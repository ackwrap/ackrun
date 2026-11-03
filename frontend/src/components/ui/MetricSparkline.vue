<script setup lang="ts">
import { computed } from "vue";
import { trafficSparkline } from "@/utils/metricMotion";
const props = defineProps<{ samples: readonly number[] }>();
const points = computed(() => trafficSparkline(props.samples));
</script>

<template>
  <svg
    v-if="points"
    class="aw-metric-sparkline"
    viewBox="0 0 100 12"
    preserveAspectRatio="none"
    aria-hidden="true"
    focusable="false"
  >
    <polyline
      :points="points"
      fill="none"
      stroke="currentColor"
      stroke-width="1.25"
      vector-effect="non-scaling-stroke"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
  </svg>
</template>
