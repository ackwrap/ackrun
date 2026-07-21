<script setup lang="ts">
import { Link2 } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import type { RouteRuleSubscriptionRequest } from "@/services/types";

defineProps<{ editing: boolean }>();
const form = defineModel<RouteRuleSubscriptionRequest>("form", {
  required: true,
});
const generate = defineModel<boolean>("generate", { required: true });
const referenceOutbound = defineModel<string>("referenceOutbound", {
  required: true,
});

defineEmits<{
  close: [];
  save: [];
}>();
</script>

<template>
  <Modal
    :open="true"
    :title="editing ? '编辑规则订阅' : '新增规则订阅'"
    size="lg"
    @close="$emit('close')"
  >
    <div class="grid gap-3 sm:grid-cols-2">
      <label class="text-xs">
        名称
        <input v-model="form.name" placeholder="订阅名称" />
      </label>
      <label class="text-xs">
        规则集 Tag
        <input v-model="form.tag" placeholder="例如 private" />
      </label>
      <label class="text-xs sm:col-span-2">
        下载地址
        <input v-model="form.url" placeholder="https://..." />
      </label>
      <label class="text-xs">
        格式
        <select v-model="form.format">
          <option
            v-for="item in ['auto', 'binary', 'source', 'clash']"
            :key="item"
          >
            {{ item }}
          </option>
        </select>
      </label>
      <label class="text-xs">
        同步周期
        <select v-model="form.sync_mode">
          <option
            v-for="item in ['off', 'daily', 'weekly', 'monthly']"
            :key="item"
          >
            {{ item }}
          </option>
        </select>
      </label>
      <label class="text-xs">
        同步时间
        <input v-model="form.sync_time" type="time" step="1" />
      </label>
      <label class="text-xs">
        星期 / 日期
        <input v-model.number="form.sync_weekday" type="number" />
      </label>
    </div>

    <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs">
      <label class="flex items-center gap-2">
        <input v-model="form.enabled" type="checkbox" />启用
      </label>
      <label class="flex items-center gap-2">
        <input v-model="form.use_proxy" type="checkbox" />下载走代理
      </label>
      <label v-if="!editing" class="flex items-center gap-2">
        <input v-model="generate" type="checkbox" />同时生成引用规则
      </label>
      <label v-if="!editing" class="flex items-center gap-2">
        引用出站
        <select v-model="referenceOutbound" class="!mt-0 !w-28">
          <option v-for="item in ['proxy', 'direct', 'block']" :key="item">
            {{ item }}
          </option>
        </select>
      </label>
    </div>

    <template #footer>
      <button
        class="aw-action-button aw-action-neutral"
        @click="$emit('close')"
      >
        取消
      </button>
      <button
        class="aw-action-button aw-action-success"
        @click="$emit('save')"
      >
        <Link2 :size="13" />{{ editing ? "更新" : "添加" }}订阅
      </button>
    </template>
  </Modal>
</template>
