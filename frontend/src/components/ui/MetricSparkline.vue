<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { createSparklineTransition } from "@/utils/sparklineMotion";
const props = defineProps<{ samples: readonly number[] }>();
const path = ref<SVGPathElement | null>(null);
const preference = window.matchMedia("(prefers-reduced-motion: reduce)");
const transition = createSparklineTransition(
  (value) => {
    path.value?.setAttribute("d", value);
  },
  {
    now: () => performance.now(),
    request: (callback) => requestAnimationFrame(callback),
    cancel: (id) => cancelAnimationFrame(id),
  },
);
function update(animate = true) {
  transition.set(
    props.samples,
    animate && !preference.matches && !document.hidden,
  );
}
function preferenceChanged() {
  update(false);
}
watch(
  () => props.samples,
  () => update(),
);
onMounted(() => {
  update(false);
  preference.addEventListener("change", preferenceChanged);
});
onBeforeUnmount(() => {
  transition.dispose();
  preference.removeEventListener("change", preferenceChanged);
});
</script>

<template>
  <svg
    class="aw-metric-sparkline"
    viewBox="0 0 100 12"
    preserveAspectRatio="none"
    aria-hidden="true"
    focusable="false"
  >
    <path
      ref="path"
      fill="none"
      stroke="currentColor"
      stroke-width="1.25"
      vector-effect="non-scaling-stroke"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
  </svg>
</template>
