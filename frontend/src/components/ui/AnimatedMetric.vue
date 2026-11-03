<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { createNumberTransition } from "@/utils/metricMotion";

const props = withDefaults(
  defineProps<{
    value: number | null;
    format?: (value: number) => string;
    animate?: boolean;
    feedback?: boolean;
  }>(),
  { animate: true, feedback: false },
);
const element = ref<HTMLElement | null>(null);
const displayed = ref<number | null>(null);
const preference = window.matchMedia("(prefers-reduced-motion: reduce)");
const reduced = ref(preference.matches);
let feedbackAnimation: Animation | undefined;
const transition = createNumberTransition(
  (value) => {
    displayed.value = value;
  },
  {
    now: () => performance.now(),
    request: (callback) => requestAnimationFrame(callback),
    cancel: (id) => cancelAnimationFrame(id),
  },
);
function format(value: number | null) {
  return value === null
    ? "--"
    : (props.format?.(value) ?? String(Math.round(value)));
}
const label = computed(() => format(props.value));
function preferenceChanged() {
  reduced.value = preference.matches;
  feedbackAnimation?.cancel();
}
preference.addEventListener("change", preferenceChanged);
watch(
  [() => props.value, () => props.animate, reduced],
  ([value, animate], previous) => {
    transition.set(value, animate && !reduced.value && !document.hidden);
    feedbackAnimation?.cancel();
    if (
      props.feedback &&
      !reduced.value &&
      !document.hidden &&
      value !== null &&
      previous?.[0] != null &&
      previous[0] !== value
    ) {
      feedbackAnimation = element.value?.animate?.(
        [
          { opacity: 0.65, transform: "translateY(1px)" },
          { opacity: 1, transform: "translateY(0)" },
        ],
        { duration: 200, easing: "ease-out" },
      );
    }
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  transition.dispose();
  feedbackAnimation?.cancel();
  preference.removeEventListener("change", preferenceChanged);
});
</script>

<template>
  <span ref="element" class="aw-animated-metric" :title="label"
    ><span class="sr-only">{{ label }}</span
    ><span aria-hidden="true">{{ format(displayed) }}</span></span
  >
</template>
