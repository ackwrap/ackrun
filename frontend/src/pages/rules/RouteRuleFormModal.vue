<script setup lang="ts">
import { computed } from "vue";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";
const p = defineProps<{
    editing: RouteRule | null;
    name: string;
    enabled: boolean;
    ruleType: string;
    valuesText: string;
    outbound: string;
    invert: boolean;
    subscriptions: RouteRuleSubscription[];
  }>(),
  emit = defineEmits<{
    close: [];
    save: [];
    "update:name": [string];
    "update:enabled": [boolean];
    "update:ruleType": [string];
    "update:valuesText": [string];
    "update:outbound": [string];
    "update:invert": [boolean];
  }>(),
  types = [
    "domain",
    "domain_suffix",
    "domain_keyword",
    "ip_cidr",
    "geoip",
    "geosite",
    "rule_set",
    "mixed",
  ],
  preview = computed(() =>
    JSON.stringify(
      [
        {
          [p.ruleType]: p.valuesText.split("\n").filter(Boolean),
          outbound: p.outbound,
          ...(p.invert ? { invert: true } : {}),
        },
      ],
      null,
      2,
    ),
  );
</script>
<template>
  <div class="aw-modal-backdrop">
    <div class="aw-modal-panel max-w-5xl p-5">
      <header class="flex justify-between">
        <h3>
          {{ editing?.is_system ? "查看" : editing ? "编辑" : "添加" }}路由规则
        </h3>
        <button @click="$emit('close')">×</button>
      </header>
      <div class="grid gap-4 xl:grid-cols-2">
        <div>
          <label
            >名称<input
              :value="name"
              :disabled="editing?.is_system"
              @input="
                $emit('update:name', ($event.target as HTMLInputElement).value)
              " /></label
          ><label
            >匹配类型<select
              :value="ruleType"
              @change="
                $emit(
                  'update:ruleType',
                  ($event.target as HTMLSelectElement).value,
                )
              "
            >
              <option v-for="x in types">{{ x }}</option>
            </select></label
          ><label
            >命中后走<select
              :value="outbound"
              @change="
                $emit(
                  'update:outbound',
                  ($event.target as HTMLSelectElement).value,
                )
              "
            >
              <option value="direct">直连</option>
              <option value="proxy">策略</option>
              <option value="block">阻断</option>
            </select></label
          ><textarea
            :value="valuesText"
            rows="8"
            @input="
              $emit(
                'update:valuesText',
                ($event.target as HTMLTextAreaElement).value,
              )
            "
          />
          <div v-if="ruleType === 'rule_set'">
            <button
              v-for="s in subscriptions"
              @click="
                $emit(
                  'update:valuesText',
                  [valuesText, s.tag].filter(Boolean).join('\n'),
                )
              "
            >
              {{ s.tag }}
            </button>
          </div>
          <label
            ><input
              type="checkbox"
              :checked="enabled"
              @change="
                $emit(
                  'update:enabled',
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />启用</label
          ><label
            ><input
              type="checkbox"
              :checked="invert"
              @change="
                $emit(
                  'update:invert',
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />反向</label
          ><button v-if="!editing?.is_system" @click="$emit('save')">
            保存规则
          </button>
        </div>
        <pre>{{ preview }}</pre>
      </div>
    </div>
  </div>
</template>
