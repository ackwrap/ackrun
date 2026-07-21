<script setup lang="ts">
import { AlertTriangle, Check, Info } from "lucide-vue-next";
import { onBeforeUnmount, ref, watch } from "vue";

type ToastType = "success" | "error" | "info";
interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
}

const props = withDefaults(
  defineProps<{
    message: string;
    type?: ToastType;
    duration?: number;
  }>(),
  { type: "info" },
);
const emit = defineEmits<{ dismiss: [] }>();
const items = ref<ToastItem[]>([]);
const timers = new Map<number, number>();
let nextID = 0;
let currentID: number | undefined;

function icon(type: ToastType) {
  return type === "success" ? Check : type === "error" ? AlertTriangle : Info;
}

function label(type: ToastType) {
  return type === "success" ? "成功" : type === "error" ? "失败" : "提示";
}

function remove(id: number) {
  const timer = timers.get(id);
  if (timer !== undefined) window.clearTimeout(timer);
  timers.delete(id);
  items.value = items.value.filter((item) => item.id !== id);
  if (currentID === id) {
    currentID = undefined;
    emit("dismiss");
  }
}

watch(
  () => [props.message, props.type, props.duration] as const,
  ([message, type, duration]) => {
    if (!message) {
      currentID = undefined;
      return;
    }
    const id = ++nextID;
    currentID = id;
    items.value.push({ id, message, type });
    const dismissAfter = duration ?? (type === "error" ? 5000 : 3000);
    if (dismissAfter > 0) {
      timers.set(id, window.setTimeout(() => remove(id), dismissAfter));
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  timers.forEach((timer) => window.clearTimeout(timer));
  timers.clear();
});
</script>

<template>
  <Teleport to="body">
    <TransitionGroup
      name="aw-toast-stack"
      tag="div"
      class="pointer-events-none fixed right-0 bottom-[13vh] z-[var(--z-toast)] flex w-full max-w-xl flex-col gap-3 px-4 sm:right-2"
    >
      <div
        v-for="item in items"
        :key="item.id"
        class="aw-toast"
        :class="`aw-toast-${item.type}`"
        :role="item.type === 'error' ? 'alert' : 'status'"
      >
        <span class="aw-toast-icon"
          ><component :is="icon(item.type)" :size="19" /></span
        ><span
          ><span class="aw-toast-label">{{ label(item.type) }}</span
          ><span class="aw-toast-message">{{ item.message }}</span></span
        >
      </div>
    </TransitionGroup>
  </Teleport>
</template>

<style scoped>
.aw-toast-stack-enter-active,
.aw-toast-stack-leave-active,
.aw-toast-stack-move {
  transition:
    opacity 180ms ease,
    transform 180ms ease;
}

.aw-toast-stack-enter-from,
.aw-toast-stack-leave-to {
  opacity: 0;
  transform: translateY(12px);
}

.aw-toast-stack-leave-active {
  position: absolute;
  width: calc(100% - 2rem);
}
</style>
