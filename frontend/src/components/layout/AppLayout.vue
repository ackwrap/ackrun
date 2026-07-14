<script setup lang="ts">
import { ref, watch } from "vue";
import { Menu, Moon, Sun } from "lucide-vue-next";
import Sidebar from "./Sidebar.vue";
import PageSkeleton from "./PageSkeleton.vue";
import ErrorBoundary from "./ErrorBoundary.vue";
import Toast from "@/components/ui/Toast.vue";
import { useRealtimeSocket } from "@/composables/useRealtimeSocket";

type Theme = "dark" | "light";
const collapsed = ref(false);
const mobileOpen = ref(false);
const theme = ref<Theme>(
  (localStorage.getItem("ackwrap.theme") as Theme) || "dark",
);
const reconcileError = ref("");
watch(
  theme,
  (value) => {
    document.documentElement.dataset.theme = value;
    localStorage.setItem("ackwrap.theme", value);
  },
  { immediate: true },
);
watch(reconcileError, (value) => {
  if (value)
    setTimeout(() => {
      reconcileError.value = "";
    }, 6000);
});
const { connected } = useRealtimeSocket((event) => {
  if (event.type !== "config.reconcile") return;
  const data = event.data as { status?: string; error?: string };
  if (data.status === "failed")
    reconcileError.value = data.error || "配置自动应用失败";
});
</script>
<template>
  <div class="flex h-screen bg-[var(--bg-base)] text-[var(--text-primary)]">
    <Toast :message="reconcileError" type="error" />
    <div
      v-if="mobileOpen"
      class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm lg:hidden"
      @click="mobileOpen = false"
    />
    <Sidebar
      :collapsed="collapsed"
      :mobile-open="mobileOpen"
      @toggle="collapsed = !collapsed"
      @close="mobileOpen = false"
    />
    <main
      id="main-content"
      class="flex min-w-0 flex-1 flex-col overflow-hidden"
    >
      <header
        class="flex h-[62px] shrink-0 items-center justify-between border-b border-[var(--border-light)] bg-[var(--header-bg)] px-5 backdrop-blur-xl"
      >
        <button
          class="flex h-9 w-9 items-center justify-center rounded-full border border-[var(--border-default)] bg-[var(--button-secondary-bg)] lg:hidden"
          title="打开导航"
          @click="mobileOpen = true"
        >
          <Menu :size="18" />
        </button>
        <div class="hidden w-5 lg:block" />
        <div class="ml-auto flex items-center gap-3 text-sm">
          <button
            class="inline-flex h-9 items-center gap-2 rounded-full border border-[var(--border-default)] bg-white/[0.08] px-3 text-xs"
            @click="theme = theme === 'dark' ? 'light' : 'dark'"
          >
            <Sun v-if="theme === 'dark'" :size="15" /><Moon
              v-else
              :size="15"
            />{{ theme === "dark" ? "白天" : "夜间" }}
          </button>
          <span class="inline-flex items-center gap-2"
            ><span
              class="h-2 w-2 rounded-full"
              :class="
                connected
                  ? 'bg-emerald-400 animate-status-pulse'
                  : 'bg-gray-500'
              "
            />{{ connected ? "已连接" : "连接中" }}</span
          >
        </div>
      </header>
      <div class="flex-1 overflow-auto">
        <div class="h-full px-4 py-5 sm:px-6 lg:px-7">
          <RouterView v-slot="{ Component }"
            ><ErrorBoundary
              ><Suspense
                ><component :is="Component" /><template #fallback
                  ><PageSkeleton /></template></Suspense></ErrorBoundary
          ></RouterView>
        </div>
      </div>
    </main>
  </div>
</template>
