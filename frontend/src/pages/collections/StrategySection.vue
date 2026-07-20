<script setup lang="ts">
import { Edit, Eye, Trash2 } from "lucide-vue-next";
import type {
  CollectionTestResponse,
  ProxyCollectionWithNodes,
  StrategyItem,
} from "@/services/types";
import type { DetailedProxyCollection } from "./collectionTypes";

defineProps<{
  strategies: StrategyItem[];
  tests: Record<number, CollectionTestResponse>;
}>();
const emit = defineEmits<{
  configure: [StrategyItem];
  preview: [StrategyItem];
  remove: [StrategyItem];
}>();

const kindLabels: Record<StrategyItem["kind"], string> = {
  reject: "拒绝",
  direct: "直连",
  proxy: "代理",
  final: "最终规则",
};

function details(collection: ProxyCollectionWithNodes) {
  return collection as DetailedProxyCollection;
}

function selectorLabel(strategy: StrategyItem) {
  if (strategy.kind === "reject") return "Reject（只读）";
  if (strategy.kind === "direct") return "Direct（固定）";
  if (strategy.kind === "final") return "Direct（默认兜底）";
  if (!strategy.collection) return "待配置";
  return strategy.collection.type === "urltest" ? "自动选择" : "手动切换";
}

function sourceLabel(strategy: StrategyItem) {
  if (strategy.kind === "reject") return "固定拒绝";
  if (strategy.kind === "direct") return "固定直连";
  if (strategy.kind === "final") return "默认兜底";
  if (!strategy.collection) return "尚未配置节点来源";
  const collection = details(strategy.collection);
  if (collection.source_type === "manual") {
    return `手动节点（${collection.node_uids.length}）`;
  }
  const names = (collection.referenced_groups || []).map((group) => group.name);
  const suffix =
    collection.source_type === "node_groups_and_nodes" ? "（含组内节点）" : "";
  return names.length ? `${names.join("、")}${suffix}` : "未选择节点组";
}
</script>

<template>
  <section class="space-y-4">
    <p class="text-sm text-[var(--text-secondary)]">
      策略顺序与规则列表一致；代理规则可在此配置节点来源，排序请前往规则管理。
    </p>
    <div class="overflow-x-auto border border-[var(--border-default)]">
      <table class="aw-data-table min-w-[940px]">
        <thead>
          <tr>
            <th>顺序</th>
            <th>业务规则</th>
            <th>用途</th>
            <th>选择方式</th>
            <th>节点来源</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="!strategies.length">
            <td colspan="7" class="py-12 text-center">暂无业务策略</td>
          </tr>
          <tr v-for="(strategy, index) in strategies" :key="strategy.rule_id">
            <td class="text-[var(--text-tertiary)]">#{{ index + 1 }}</td>
            <td>
              <span class="font-medium text-[var(--text-primary)]">{{
                strategy.name
              }}</span>
              <small
                v-if="strategy.read_only"
                class="ml-2 rounded-full bg-[var(--button-secondary-bg)] px-2 py-0.5 text-[10px] text-[var(--text-secondary)]"
              >
                只读
              </small>
            </td>
            <td>{{ kindLabels[strategy.kind] }}</td>
            <td>{{ selectorLabel(strategy) }}</td>
            <td class="max-w-[360px]" :title="sourceLabel(strategy)">
              <span class="line-clamp-2">{{ sourceLabel(strategy) }}</span>
            </td>
            <td>
              {{ strategy.enabled ? "规则启用" : "规则停用" }}
              <template v-if="strategy.kind === 'proxy'">
                ·
                {{
                  strategy.collection
                    ? strategy.collection.enabled
                      ? "配置启用"
                      : "配置停用"
                    : "未配置"
                }}
              </template>
              <template v-if="strategy.collection && tests[strategy.collection.id]">
                ·
                <span>
                  {{ tests[strategy.collection.id].available }} /
                  {{ tests[strategy.collection.id].tested }} 可用
                </span>
              </template>
            </td>
            <td>
              <div v-if="strategy.kind === 'proxy'" class="flex flex-wrap gap-2">
                <button
                  v-if="!strategy.collection"
                  class="aw-action-button aw-action-neutral"
                  @click="emit('configure', strategy)"
                >
                  配置
                </button>
                <template v-else>
                  <button
                    class="aw-action-button aw-action-neutral"
                    @click="emit('configure', strategy)"
                  >
                    <Edit :size="12" />编辑
                  </button>
                  <button
                    class="aw-action-button aw-action-neutral"
                    @click="emit('preview', strategy)"
                  >
                    <Eye :size="12" />预览
                  </button>
                  <button
                    class="aw-action-button aw-action-danger"
                    @click="emit('remove', strategy)"
                  >
                    <Trash2 :size="12" />删除配置
                  </button>
                </template>
              </div>
              <span v-else class="text-xs text-[var(--text-tertiary)]">
                固定策略，无需配置
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
