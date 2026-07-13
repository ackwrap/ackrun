<script setup lang="ts">
import {
  Cloud,
  Edit3,
  FileJson2,
  Power,
  RefreshCw,
  Route,
  Trash2,
} from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
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
  <Modal :open="true" :title="item.name" size="lg" @close="$emit('close')">
    <template #title>
      <span class="flex items-center gap-2"
        ><Cloud :size="18" />{{ item.name }}</span
      >
    </template>

    <div
      class="mb-4 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
    >
      <div class="flex flex-wrap items-center gap-2">
        <b>{{ item.tag }}</b>
        <span
          class="rounded-full bg-[var(--button-secondary-bg)] px-2 py-0.5 text-[11px]"
          >{{ item.format }}</span
        >
        <span class="text-[11px] text-[var(--text-tertiary)]">{{
          item.sync_status
        }}</span>
      </div>
      <p
        class="mt-1 truncate text-xs text-[var(--text-secondary)]"
        :title="item.url"
      >
        {{ item.url }}
      </p>
      <p v-if="item.sync_error" class="mt-2 text-xs text-[var(--color-error)]">
        {{ item.sync_error }}
      </p>
    </div>

    <div class="grid gap-3 sm:grid-cols-3">
      <section
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
      >
        <h4 class="mb-3 flex items-center gap-2 font-medium">
          <Route :size="15" />生成引用规则
        </h4>
        <div class="grid gap-2">
          <button
            v-for="outbound in ['proxy', 'direct', 'block']"
            :key="outbound"
            class="aw-action-button aw-action-neutral w-full"
            @click="$emit('createRule', item, outbound)"
          >
            {{ outbound }}
          </button>
        </div>
      </section>

      <section
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
      >
        <h4 class="mb-3 flex items-center gap-2 font-medium">
          <FileJson2 :size="15" />内容与更新
        </h4>
        <div class="grid gap-2">
          <button
            class="aw-action-button aw-action-neutral w-full"
            @click="$emit('preview', item)"
          >
            <FileJson2 :size="13" />预览 JSON
          </button>
          <button
            class="aw-action-button aw-action-neutral w-full"
            @click="$emit('sync', item)"
          >
            <RefreshCw :size="13" />立即同步
          </button>
        </div>
      </section>

      <section
        class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
      >
        <h4 class="mb-3 flex items-center gap-2 font-medium">
          <Edit3 :size="15" />订阅管理
        </h4>
        <div class="grid gap-2">
          <button
            class="aw-action-button aw-action-neutral w-full"
            @click="$emit('toggle', item)"
          >
            <Power :size="13" />{{ item.enabled ? "停用" : "启用" }}
          </button>
          <button
            class="aw-action-button aw-action-neutral w-full"
            @click="$emit('edit', item)"
          >
            <Edit3 :size="13" />编辑订阅
          </button>
          <button
            class="aw-action-button aw-action-danger w-full"
            @click="$emit('remove', item)"
          >
            <Trash2 :size="13" />删除订阅
          </button>
        </div>
      </section>
    </div>
  </Modal>
</template>
