<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { Construction, Layers3, ShieldCheck } from "lucide-vue-next";
import PageHeader from "@/components/layout/PageHeader.vue";
import Card from "@/components/ui/Card.vue";

const route = useRoute();
const title = computed(() => String(route.meta.title || "高级功能"));
const description = computed(() => String(route.meta.description || ""));
</script>

<template>
  <div class="space-y-5">
    <PageHeader :title="title" />
    <Card padding="lg">
      <div
        class="relative overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-light)] bg-[var(--bg-surface)] p-6 sm:p-8"
      >
        <div
          class="absolute -top-16 -right-12 h-44 w-44 rounded-full bg-[var(--button-primary-bg)] blur-3xl"
        />
        <div class="relative max-w-2xl">
          <span
            class="inline-flex items-center gap-2 rounded-full border border-[var(--button-primary-border)] bg-[var(--button-primary-bg)] px-3 py-1 text-xs text-[var(--button-primary-text)]"
          >
            <Construction :size="14" />规划中
          </span>
          <h2 class="mt-5 text-xl font-semibold text-[var(--text-primary)]">
            {{ title }}正在进入控制面设计
          </h2>
          <p class="mt-2 text-sm leading-6 text-[var(--text-secondary)]">
            {{ description }}
            当前页面仅标记功能边界，不会写入无效配置或提供无法闭环的操作。
          </p>
          <div class="mt-6 grid gap-3 sm:grid-cols-2">
            <div class="rounded-lg border border-[var(--border-default)] p-4">
              <Layers3 :size="18" class="text-[var(--color-primary)]" />
              <p class="mt-3 text-sm font-medium">分阶段交付</p>
              <p class="mt-1 text-xs leading-5 text-[var(--text-tertiary)]">
                数据模型、API、配置生成和可观测状态将一起完成。
              </p>
            </div>
            <div class="rounded-lg border border-[var(--border-default)] p-4">
              <ShieldCheck :size="18" class="text-[var(--color-success)]" />
              <p class="mt-3 text-sm font-medium">安全默认值</p>
              <p class="mt-1 text-xs leading-5 text-[var(--text-tertiary)]">
                每项能力上线前都会覆盖权限、失败回滚和最小验证。
              </p>
            </div>
          </div>
        </div>
      </div>
    </Card>
  </div>
</template>
