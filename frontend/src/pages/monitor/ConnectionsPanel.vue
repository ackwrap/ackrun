<script setup lang="ts">
import { RefreshCw, X } from "lucide-vue-next";
import type { Connection } from "@/services/clash";
import {
  formatBytes,
  monitorPanelBodyClass,
  monitorPanelClass,
} from "./monitorUtils";
defineProps<{ connections: Connection[]; search: string; loading: boolean }>();
defineEmits<{
  searchChange: [string];
  refresh: [];
  closeConnection: [string];
  closeAll: [];
}>();
</script>
<template>
  <div class="space-y-4">
    <div
      :class="[
        monitorPanelClass,
        'flex flex-wrap items-center justify-between gap-3',
      ]"
    >
      <div>
        <h3 class="font-semibold">活动连接</h3>
        <p class="text-xs text-[var(--text-tertiary)]">
          共 {{ connections.length }} 个连接
        </p>
      </div>
      <div class="flex gap-2">
        <input
          :value="search"
          placeholder="搜索域名、IP..."
          @input="
            $emit('searchChange', ($event.target as HTMLInputElement).value)
          "
        /><button :disabled="loading" @click="$emit('refresh')">
          <RefreshCw
            :size="14"
            :class="loading ? 'animate-spin' : ''"
          />刷新</button
        ><button class="text-red-300" @click="$emit('closeAll')">
          <X :size="14" />关闭所有
        </button>
      </div>
    </div>
    <div :class="['overflow-hidden', monitorPanelBodyClass]">
      <div v-if="!connections.length" class="p-12 text-center">暂无连接</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full min-w-[800px]">
          <thead>
            <tr>
              <th
                v-for="x in [
                  '目标',
                  '来源',
                  '策略链',
                  '规则',
                  '上传/下载',
                  '操作',
                ]"
              >
                {{ x }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="c in connections" :key="c.id">
              <td>
                {{ c.metadata.host || c.metadata.destinationIP
                }}<small>{{ c.metadata.destinationPort }}</small>
              </td>
              <td>{{ c.metadata.sourceIP }}:{{ c.metadata.sourcePort }}</td>
              <td>{{ c.chains?.join(" → ") || "-" }}</td>
              <td>{{ c.rule || "-" }} {{ c.rulePayload }}</td>
              <td>
                <span class="text-blue-400">↑ {{ formatBytes(c.upload) }}</span
                ><br /><span class="text-emerald-400"
                  >↓ {{ formatBytes(c.download) }}</span
                >
              </td>
              <td>
                <button
                  class="text-red-300"
                  @click="$emit('closeConnection', c.id)"
                >
                  关闭
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
