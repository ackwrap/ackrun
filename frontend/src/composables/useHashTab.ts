import { onBeforeUnmount, onMounted, ref, type Ref } from "vue";

export function useHashTab<T extends string>(
  tabs: readonly T[],
  fallback: T,
) {
  const tabFromHash = () => {
    const value = window.location.hash.slice(1) as T;
    return tabs.includes(value) ? value : fallback;
  };
  const activeTab = ref<T>(tabFromHash()) as Ref<T>;

  function selectTab(tab: T) {
    activeTab.value = tab;
    if (window.location.hash !== `#${tab}`) window.location.hash = tab;
  }

  function syncTabFromHash() {
    activeTab.value = tabFromHash();
  }

  onMounted(() => window.addEventListener("hashchange", syncTabFromHash));
  onBeforeUnmount(() =>
    window.removeEventListener("hashchange", syncTabFromHash),
  );

  return { activeTab, selectTab };
}
