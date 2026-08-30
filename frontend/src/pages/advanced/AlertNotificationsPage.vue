<script setup lang="ts">
import { ref } from "vue";
import { BellRing, ListChecks, RadioTower } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import AlertChannelsTab from "./AlertChannelsTab.vue";
import AlertDeliveriesTab from "./AlertDeliveriesTab.vue";
import AlertRulesTab from "./AlertRulesTab.vue";

withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false });

type AlertTab = "channels" | "rules" | "deliveries";
const activeTab = ref<AlertTab>("channels");
const tabs = [
  { id: "channels", label: "渠道", icon: RadioTower },
  { id: "rules", label: "规则", icon: ListChecks },
  { id: "deliveries", label: "投递记录", icon: BellRing },
] as const;
</script>

<template>
  <div class="space-y-5">
    <PageHeader
      title="告警通知"
      description="将熔断、恢复和订阅同步失败事件投递到 Webhook、Telegram 或邮件。"
      :embedded="embedded"
    />

    <div
      class="inline-flex max-w-full gap-1 overflow-x-auto rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-1"
      role="tablist"
      aria-label="告警通知模块"
    >
      <button
        v-for="tab in tabs"
        :key="tab.id"
        type="button"
        class="inline-flex h-9 shrink-0 items-center gap-2 rounded-lg px-4 text-sm transition-colors"
        :class="
          activeTab === tab.id
            ? 'bg-[var(--bg-sidebar-active)] text-[var(--color-primary)]'
            : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
        "
        role="tab"
        :aria-selected="activeTab === tab.id"
        @click="activeTab = tab.id"
      >
        <component :is="tab.icon" :size="14" />{{ tab.label }}
      </button>
    </div>

    <AlertChannelsTab v-if="activeTab === 'channels'" />
    <AlertRulesTab v-else-if="activeTab === 'rules'" />
    <AlertDeliveriesTab v-else />
  </div>
</template>
