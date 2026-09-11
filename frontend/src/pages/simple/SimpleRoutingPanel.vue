<script setup lang="ts">
import { toRefs } from "vue";
import type { SimpleDashboard } from "./useSimpleDashboard";
import { ShieldCheck } from "lucide-vue-next";
const props = defineProps<{ state: SimpleDashboard }>();
const { options, optionsDisabled, rules, unmanaged, professional } = toRefs(
  props.state,
);
</script>

<template>
  <section
    id="simple-panel-rules"
    role="tabpanel"
    aria-labelledby="simple-tab-rules"
    class="simple-card"
  >
    <div class="mb-4 flex items-center gap-2">
      <ShieldCheck :size="19" class="text-[var(--color-primary-hover)]" />
      <h2 id="rules-title" class="text-xl font-semibold">分流规则</h2>
    </div>
    <p class="mb-6 text-sm leading-6 text-[var(--text-secondary)]">
      选择哪些流量使用代理。默认拦截广告，国内流量绕过，常用海外应用走代理。
    </p>
    <fieldset
      v-if="options"
      :disabled="optionsDisabled"
      class="min-w-0 space-y-5"
    >
      <label
        class="flex items-center justify-between gap-4 rounded-xl border border-[var(--border-light)] p-4"
      >
        <span
          ><span class="block font-medium">广告拦截</span
          ><span class="mt-1 block text-xs text-[var(--text-secondary)]"
            >拦截常见广告与追踪域名</span
          ></span
        >
        <input
          v-model="options.ad_block"
          type="checkbox"
          class="simple-checkbox"
        />
      </label>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="space-y-2 text-sm"
          ><span class="block font-medium">国内 / CN</span
          ><select v-model="options.cn_outbound" class="simple-input w-full">
            <option value="bypass">内核绕过（推荐）</option>
            <option value="direct">直连</option>
          </select></label
        >
        <label class="space-y-2 text-sm"
          ><span class="block font-medium">未匹配流量</span
          ><select
            v-model="options.default_outbound"
            class="simple-input w-full"
          >
            <option value="proxy">使用代理（默认）</option>
            <option value="direct">直接连接</option>
          </select></label
        >
      </div>
      <div class="grid gap-3 sm:grid-cols-2">
        <div
          v-for="rule in rules"
          :key="rule.name"
          class="flex items-center justify-between gap-3 rounded-xl border border-[var(--border-light)] bg-[var(--bg-surface)] p-4"
        >
          <div class="min-w-0">
            <p class="text-sm font-medium">{{ rule.name }}</p>
            <p class="mt-1 text-xs leading-5 text-[var(--text-secondary)]">
              {{ rule.detail }}
            </p>
          </div>
          <select
            v-model="options.app_routing[rule.name]"
            :aria-label="`${rule.name}分流方式`"
            class="simple-input w-24 shrink-0 text-sm"
          >
            <option value="proxy">代理</option>
            <option value="direct">直连</option>
          </select>
        </div>
      </div>
    </fieldset>
    <p v-if="unmanaged" class="simple-alert">
      当前沿用专业模式的分流规则。
      <RouterLink to="/rules" class="underline" @click="professional"
        >管理现有分流规则</RouterLink
      >
    </p>
    <p class="mt-4 text-xs leading-6 text-[var(--text-secondary)]">
      分类使用的代理节点可在「订阅与节点」中分别选择。局域网保持直连。
    </p>
  </section>
</template>
