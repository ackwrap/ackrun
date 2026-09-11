<script setup lang="ts">
import { toRefs } from "vue";
import type { SimpleDashboard } from "./useSimpleDashboard";
import Button from "@/components/ui/Button.vue";
const props = defineProps<{ state: SimpleDashboard }>();
const {
  subscriptions,
  nodeCount,
  proxy,
  selectedProxy,
  busy,
  changeProxy,
  testProxy,
  delayResult,
  nodeGroups,
  primaryGroupName,
  applicationGroups,
  groupSelections,
} = toRefs(props.state);
</script>

<template>
  <div class="space-y-3">
    <p class="text-sm text-[var(--text-secondary)]">
      {{ subscriptions.length }} 个订阅 · {{ nodeCount }} 个节点
    </p>
    <div
      v-for="item in subscriptions"
      :key="item.id"
      class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-[var(--border-light)] p-4"
    >
      <div>
        <p class="font-medium">{{ item.name }}</p>
        <p class="mt-1 text-xs text-[var(--text-secondary)]">
          {{ item.node_count }} 个节点 ·
          {{
            item.sync_mode === "off" ? "手动更新" : `定时更新 ${item.sync_time}`
          }}
        </p>
      </div>
      <span class="text-sm">{{
        item.sync_status === "syncing"
          ? "更新中"
          : item.sync_status === "updated"
            ? "已更新"
            : item.sync_status === "failed"
              ? "更新失败"
              : "等待更新"
      }}</span>
    </div>
  </div>
  <div v-if="proxy?.all?.length">
    <label for="proxy-node" class="mb-2 block text-sm font-medium"
      >{{ primaryGroupName }}节点</label
    >
    <div class="flex flex-wrap items-center gap-3">
      <select
        id="proxy-node"
        v-model="selectedProxy"
        :disabled="busy"
        class="simple-input min-w-0 flex-1"
      >
        <option v-for="name in proxy.all" :key="name" :value="name">
          {{ name }}
        </option>
      </select>
      <Button
        :disabled="busy || selectedProxy === proxy.now"
        @click="changeProxy()"
        >切换节点</Button
      >
      <Button :disabled="busy" @click="testProxy(primaryGroupName)"
        >测延迟</Button
      >
    </div>
    <p v-if="delayResult[primaryGroupName]" role="status" class="mt-2 text-sm">
      {{ delayResult[primaryGroupName] }}
    </p>
    <p class="mt-2 text-xs text-[var(--text-secondary)]">
      切换仅影响当前节点组，其他组可以在下方单独调整。
    </p>
    <details v-if="nodeGroups.length" class="simple-details mt-5">
      <summary>高级节点选择 · 按应用分别指定</summary>
      <div class="mt-4 space-y-4">
        <template v-for="rule in nodeGroups" :key="rule.name">
          <div
            v-if="applicationGroups[rule.name]?.all?.length"
            class="space-y-2"
          >
            <label
              :for="`node-${rule.name}`"
              class="block text-sm font-medium"
              >{{ rule.name }}</label
            >
            <div class="flex flex-wrap items-center gap-2">
              <select
                :id="`node-${rule.name}`"
                v-model="groupSelections[rule.name]"
                :disabled="busy"
                class="simple-input min-w-0 flex-1"
              >
                <option
                  v-for="name in applicationGroups[rule.name]!.all"
                  :key="name"
                  :value="name"
                >
                  {{ name }}
                </option>
              </select>
              <Button
                :disabled="
                  busy ||
                  groupSelections[rule.name] ===
                    applicationGroups[rule.name]?.now
                "
                @click="changeProxy(rule.name)"
                >切换</Button
              >
              <Button :disabled="busy" @click="testProxy(rule.name)"
                >测延迟</Button
              >
            </div>
            <p v-if="delayResult[rule.name]" role="status" class="text-sm">
              {{ delayResult[rule.name] }}
            </p>
          </div>
        </template>
        <p class="text-xs leading-6 text-[var(--text-secondary)]">
          选择直连的分类不需要代理节点。节点切换立即生效。
        </p>
      </div>
    </details>
  </div>
</template>
