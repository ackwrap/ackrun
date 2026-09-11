<script setup lang="ts">
import { reactive, toRefs } from "vue";
import { ArrowRight, Sun, Moon } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import SimpleStatusPanel from "./simple/SimpleStatusPanel.vue";
import SimpleRoutingPanel from "./simple/SimpleRoutingPanel.vue";
import SimpleNetworkPanel from "./simple/SimpleNetworkPanel.vue";
import { useSimpleDashboard } from "./simple/useSimpleDashboard";
import "./simple/simple.css";

const state = reactive(useSimpleDashboard());
const {
  theme,
  professional,
  tabs,
  activeTab,
  activateTab,
  refresh,
  error,
  readError,
  notice,
  optionsError,
  optionsDirty,
  optionsDisabled,
  busy,
  saveSettings,
  setup,
  options,
} = toRefs(state);
</script>

<template>
  <div class="simple-shell min-h-screen text-[var(--text-primary)]">
    <header
      class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 px-5 py-5 sm:px-8"
    >
      <RouterLink
        to="/simple"
        class="flex items-center gap-3 text-lg font-semibold"
      >
        <img src="/favicon.png" alt="" class="h-9 w-9" /> AckWrap
        <span class="text-xs font-normal text-[var(--text-secondary)]"
          >简易模式</span
        >
      </RouterLink>
      <div class="flex items-center gap-3">
        <button
          class="simple-theme"
          :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'"
          @click="theme = theme === 'dark' ? 'light' : 'dark'"
        >
          <Sun v-if="theme === 'dark'" :size="18" /><Moon v-else :size="18" />
        </button>
        <RouterLink
          to="/control"
          class="flex items-center gap-1 text-sm text-[var(--text-secondary)]"
          @click="professional"
          >专业模式 <ArrowRight :size="15"
        /></RouterLink>
      </div>
    </header>

    <nav class="mx-auto max-w-5xl px-5 sm:px-8" aria-label="简易模式功能">
      <div role="tablist" aria-label="功能菜单" class="simple-tabs">
        <button
          v-for="(tab, index) in tabs"
          :id="`simple-tab-${tab.id}`"
          :key="tab.id"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.id"
          :aria-controls="`simple-panel-${tab.id}`"
          :tabindex="activeTab === tab.id ? 0 : -1"
          :class="['simple-tab', { 'simple-tab-active': activeTab === tab.id }]"
          @click="
            activeTab = tab.id;
            refresh();
          "
          @keydown.right.prevent="activateTab(index + 1)"
          @keydown.left.prevent="activateTab(index - 1)"
          @keydown.home.prevent="activateTab(0)"
          @keydown.end.prevent="activateTab(tabs.length - 1)"
        >
          {{ tab.label }}
        </button>
      </div>
    </nav>

    <main
      id="main-content"
      class="mx-auto max-w-5xl px-5 pb-12 pt-7 sm:px-8 sm:pt-12"
    >
      <div v-if="activeTab === 'overview'" class="mb-8">
        <p class="mb-3 text-sm font-medium text-[var(--color-primary-hover)]">
          OPENWRT · 家庭网络
        </p>
        <h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">
          让上网配置简单一点
        </h1>
        <p class="mt-4 max-w-xl text-sm leading-7 text-[var(--text-secondary)]">
          添加订阅，按你的选择完成节点、分流、DNS 和网络接管。适用于 OpenWrt
          主路由；旁路由需要终端将网关与 DNS 指向本机。
        </p>
      </div>

      <div v-if="error || readError" role="alert" class="simple-alert mb-5">
        <p v-if="error" class="break-words">{{ error }}</p>
        <p v-if="readError" class="break-words">{{ readError }}</p>
        <button class="mt-2 underline" :disabled="busy" @click="refresh">
          重新读取状态
        </button>
      </div>
      <p
        v-if="notice"
        role="status"
        class="mb-5 text-sm text-[var(--color-success)]"
      >
        {{ notice }}
      </p>

      <p v-if="optionsError" role="alert" class="simple-alert mb-5">
        {{ optionsError }}
      </p>
      <div
        v-if="optionsDirty"
        class="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-[var(--border-default)] bg-[var(--bg-surface)] p-4"
        role="status"
      >
        <p class="text-sm">有尚未保存的设置，切换 Tab 会保留当前修改。</p>
        <Button
          variant="primary"
          :disabled="optionsDisabled"
          :loading="busy"
          @click="saveSettings"
          >{{ setup?.configured ? "保存并应用" : "保存配置选项" }}</Button
        >
      </div>

      <SimpleStatusPanel
        v-if="activeTab === 'overview' || activeTab === 'nodes'"
        :state="state"
      />
      <SimpleRoutingPanel v-if="activeTab === 'rules'" :state="state" />
      <SimpleNetworkPanel v-if="activeTab === 'network'" :state="state" />

      <div
        v-if="options && (activeTab === 'rules' || activeTab === 'network')"
        class="mt-5 flex flex-wrap items-center gap-4"
      >
        <Button
          variant="primary"
          :disabled="optionsDisabled || !optionsDirty"
          :loading="busy"
          @click="saveSettings"
          >{{ setup?.configured ? "保存并应用" : "保存配置选项" }}</Button
        >
        <p class="text-xs leading-6 text-[var(--text-secondary)]">
          {{
            setup?.options_locked
              ? "配置已应用，请先返回概览重试启动。"
              : setup?.configured
                ? "保存前会校验配置，应用时代理连接可能短暂重连。"
                : "保存后返回概览，填写订阅并一键启用。"
          }}
        </p>
      </div>
    </main>
  </div>
</template>
