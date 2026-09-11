<script setup lang="ts">
import { toRefs } from "vue";
import type { SimpleDashboard } from "./useSimpleDashboard";
import { Sparkles } from "lucide-vue-next";
import Button from "@/components/ui/Button.vue";
import SimpleNodesPanel from "./SimpleNodesPanel.vue";
const props = defineProps<{ state: SimpleDashboard }>();
const {
  activeTab,
  loading,
  running,
  busy,
  setup,
  unmanaged,
  canManage,
  runtime,
  action,
  syncing,
  connected,
  options,
  proxyError,
  subscriptionURL,
  startSetup,
  canSetup,
  optionsError,
} = toRefs(props.state);
</script>

<template>
  <section
    v-if="activeTab === 'overview' || activeTab === 'nodes'"
    :id="`simple-panel-${activeTab}`"
    role="tabpanel"
    :aria-labelledby="`simple-tab-${activeTab}`"
    class="simple-card"
    :aria-busy="loading || running || busy"
  >
    <p v-if="loading" role="status" class="text-[var(--text-secondary)]">
      正在读取路由器状态…
    </p>
    <template v-else-if="setup">
      <div class="mb-5 flex items-start gap-3">
        <Sparkles
          :size="23"
          class="mt-1 shrink-0 text-[var(--color-primary-hover)]"
        />
        <div>
          <h2 id="setup-title" class="text-xl font-semibold">
            {{
              activeTab === "nodes"
                ? "订阅与节点"
                : canManage
                  ? "你的家庭网络"
                  : "一步完成配置"
            }}
          </h2>
          <p class="mt-2 text-sm text-[var(--text-secondary)]">
            {{
              canManage
                ? "日常启停、节点切换和分流调整都可以在这里完成。"
                : "使用推荐设置直接开始，也可以先到分流与网络 Tab 调整。"
            }}
          </p>
        </div>
      </div>
      <p v-if="!setup.supported" class="simple-alert">
        当前系统不支持 OpenWrt 一键配置，请使用专业模式。
      </p>
      <p v-else-if="unmanaged" class="simple-alert">
        正在使用已有专业配置。这里可以启停服务、更新订阅和切换节点，分流与 DNS
        请在专业模式管理。
      </p>
      <div
        v-if="running"
        role="status"
        aria-live="polite"
        class="rounded-xl bg-[var(--color-primary-bg)] p-5"
      >
        <h3 class="font-semibold">正在配置，请稍候</h3>
        <p class="mt-2 text-sm">{{ setup.stage || "正在准备配置" }}</p>
        <p class="mt-3 text-xs text-[var(--text-secondary)]">
          可以离开或刷新页面，返回后会继续显示进度。
        </p>
      </div>
      <div
        v-else-if="setup.status === 'failed'"
        role="alert"
        class="simple-alert mb-5"
      >
        <p class="font-medium">配置未完成</p>
        <p class="mt-2 break-words">
          {{ setup.error || "请重试，或前往专业模式查看日志。" }}
        </p>
      </div>
      <div v-if="canManage && !running" class="mt-5 space-y-5">
        <div role="status">
          <p class="text-lg font-medium">
            {{
              runtime?.status === "running"
                ? "代理服务运行中"
                : runtime?.status === "stopped"
                  ? "代理服务已停止"
                  : "代理服务尚未就绪"
            }}
          </p>
          <p
            v-if="setup.status === 'succeeded'"
            class="mt-2 text-sm text-[var(--text-secondary)]"
          >
            一键配置已完成。服务状态不代表所有网站均可访问。
          </p>
        </div>
        <div class="flex flex-wrap gap-3">
          <Button
            v-if="activeTab === 'overview'"
            :disabled="busy || syncing || !runtime"
            @click="action('toggle')"
            >{{
              runtime?.status === "running" ? "关闭代理" : "开启代理"
            }}</Button
          >
          <Button
            v-if="activeTab === 'nodes'"
            variant="secondary"
            :disabled="busy || syncing"
            @click="action('update')"
            >{{ syncing ? "更新中…" : "更新订阅" }}</Button
          >
        </div>
        <p
          v-if="syncing && !connected"
          role="status"
          class="text-sm text-[var(--text-secondary)]"
        >
          进度连接正在恢复，正在定期查询订阅状态。
        </p>
        <div
          v-if="activeTab === 'overview' && options"
          class="grid gap-3 sm:grid-cols-3"
        >
          <div class="simple-stat">
            <span>广告拦截</span
            ><strong>{{ options.ad_block ? "已启用" : "已关闭" }}</strong>
          </div>
          <div class="simple-stat">
            <span>国内流量</span
            ><strong>{{
              options.cn_outbound === "bypass" ? "内核绕过" : "直连"
            }}</strong>
          </div>
          <div class="simple-stat">
            <span>默认去向</span
            ><strong>{{
              options.default_outbound === "proxy" ? "代理" : "直连"
            }}</strong>
          </div>
        </div>
        <SimpleNodesPanel v-if="activeTab === 'nodes'" :state="state" />
        <p
          v-if="proxyError"
          role="alert"
          class="text-sm text-[var(--color-warning)]"
        >
          {{ proxyError }}
        </p>
      </div>
      <form
        v-if="!setup.configured && setup.supported && !unmanaged && !running"
        class="mt-6"
        @submit.prevent="startSetup"
      >
        <label for="subscription-url" class="mb-2 block text-sm font-medium"
          >订阅链接</label
        >
        <input
          id="subscription-url"
          v-model="subscriptionURL"
          type="url"
          :required="!setup.can_retry"
          autocomplete="off"
          spellcheck="false"
          :placeholder="setup.can_retry ? '留空使用已保存的订阅' : 'https://…'"
          class="simple-input w-full"
          :disabled="busy || setup.options_locked"
        />
        <p class="mb-5 mt-2 text-xs text-[var(--text-secondary)]">
          {{
            setup.can_retry
              ? "留空使用已保存的订阅，填写新地址可替换未完成的订阅；已应用的配置仅重试启动。"
              : "使用服务商提供的订阅链接，请勿分享给他人。"
          }}
        </p>
        <Button
          type="submit"
          :disabled="
            !canSetup ||
            !options ||
            !!optionsError ||
            (!setup.can_retry && !subscriptionURL.trim())
          "
          >{{
            busy
              ? "正在提交…"
              : setup.status === "failed"
                ? "重新配置并启用"
                : "一键配置并启用"
          }}</Button
        >
      </form>
      <div
        v-if="!setup.configured && !running && options"
        class="mt-5 flex flex-wrap gap-x-5 gap-y-2 text-xs text-[var(--text-secondary)]"
      >
        <span>广告拦截：{{ options.ad_block ? "开启" : "关闭" }}</span>
        <span
          >国内：{{
            options.cn_outbound === "bypass" ? "内核绕过" : "直连"
          }}</span
        >
        <span
          >其他流量：{{
            options.default_outbound === "proxy" ? "代理" : "直连"
          }}</span
        >
      </div>
    </template>
  </section>
</template>
