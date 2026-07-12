<script setup lang="ts">
import type { RouteRuleSubscription } from "@/services/types";
defineProps<{ item: RouteRuleSubscription }>();
defineEmits<{
  close: [];
  createRule: [RouteRuleSubscription, string];
  preview: [RouteRuleSubscription];
  sync: [RouteRuleSubscription];
  toggle: [RouteRuleSubscription];
  edit: [RouteRuleSubscription];
  remove: [RouteRuleSubscription];
}>();
</script>
<template>
  <div class="aw-modal-backdrop">
    <div class="aw-modal-panel max-w-2xl p-5">
      <button class="float-right" @click="$emit('close')">×</button>
      <h3>{{ item.name }} · {{ item.tag }}</h3>
      <p>{{ item.url }}</p>
      <p v-if="item.sync_error" class="text-red-300">{{ item.sync_error }}</p>
      <h4>生成引用规则</h4>
      <button
        v-for="x in ['proxy', 'direct', 'block']"
        @click="$emit('createRule', item, x)"
      >
        {{ x }}
      </button>
      <h4>内容与更新</h4>
      <button @click="$emit('preview', item)">预览 JSON</button
      ><button @click="$emit('sync', item)">立即同步</button>
      <h4>订阅管理</h4>
      <button @click="$emit('toggle', item)">
        {{ item.enabled ? "停用" : "启用" }}</button
      ><button @click="$emit('edit', item)">编辑订阅</button
      ><button class="text-red-300" @click="$emit('remove', item)">
        删除订阅
      </button>
    </div>
  </div>
</template>
