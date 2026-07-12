<script setup lang="ts">
import { computed } from "vue";
const p = withDefaults(
  defineProps<{
    total: number;
    page: number;
    pageSize: number;
    totalPages: number;
    pageSizeOptions?: number[];
  }>(),
  { pageSizeOptions: () => [10, 25, 50, 100] },
);
const emit = defineEmits<{ pageChange: [number]; pageSizeChange: [number] }>();
const start = computed(() => (p.total ? (p.page - 1) * p.pageSize + 1 : 0));
</script>
<template>
  <div
    class="mt-4 flex items-center justify-between rounded-md border border-[var(--border-default)] px-3 py-3 text-sm"
  >
    <span
      >显示 {{ start }}-{{ Math.min(total, page * pageSize) }} / 共
      {{ total }} 条</span
    >
    <div class="flex items-center gap-2">
      <select
        :value="pageSize"
        @change="
          emit('pageChange', 1);
          emit(
            'pageSizeChange',
            Number(($event.target as HTMLSelectElement).value),
          );
        "
      >
        <option v-for="n in pageSizeOptions" :key="n" :value="n">
          {{ n }}
        </option></select
      ><button :disabled="page <= 1" @click="emit('pageChange', page - 1)">
        上一页</button
      ><span>第 {{ page }} / {{ totalPages }} 页</span
      ><button
        :disabled="page >= totalPages"
        @click="emit('pageChange', page + 1)"
      >
        下一页
      </button>
    </div>
  </div>
</template>
