<script setup lang="ts">
import { RefreshCw } from "lucide-vue-next";
import type { Rule } from "@/services/clash";
import { monitorPanelBodyClass, monitorPanelClass } from "./monitorUtils";
defineProps<{
  rules: Rule[];
  search: string;
  loading: boolean;
  unavailableReason: string;
}>();
defineEmits<{ searchChange: [string]; refresh: [] }>();
</script>
<template>
  <div class="space-y-4">
    <div :class="[monitorPanelClass, 'flex justify-between']">
      <div>
        <b>规则列表</b>
        <p class="text-xs">共 {{ rules.length }} 条规则</p>
      </div>
      <div>
        <input
          :value="search"
          placeholder="搜索规则..."
          @input="
            $emit('searchChange', ($event.target as HTMLInputElement).value)
          "
        /><button @click="$emit('refresh')">
          <RefreshCw :size="14" />刷新
        </button>
      </div>
    </div>
    <div :class="['overflow-hidden', monitorPanelBodyClass]">
      <div v-if="loading" class="p-12 text-center">加载中...</div>
      <div v-else-if="unavailableReason" class="p-12 text-center text-red-300">
        Clash API 未连接<br />{{ unavailableReason }}
      </div>
      <div v-else-if="!rules.length" class="p-12 text-center">暂无规则</div>
      <table v-else class="w-full">
        <thead>
          <tr>
            <th>类型</th>
            <th>匹配值</th>
            <th>策略</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in rules" :key="i">
            <td>{{ r.type }}</td>
            <td>{{ r.payload }}</td>
            <td>{{ r.proxy }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
