<script setup lang="ts">
import { computed } from "vue";
import { FileJson2, Plus, Route, Save, Trash2 } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import EmojiPicker, { defaultEmojis } from "@/components/ui/EmojiPicker.vue";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";

const props = defineProps<{
  editing: RouteRule | null;
  name: string;
  enabled: boolean;
  ruleType: string;
  valuesText: string;
  outbound: string;
  invert: boolean;
  subscriptions: RouteRuleSubscription[];
}>();

const emit = defineEmits<{
  close: [];
  save: [];
  "update:name": [string];
  "update:enabled": [boolean];
  "update:ruleType": [string];
  "update:valuesText": [string];
  "update:outbound": [string];
  "update:invert": [boolean];
}>();

const types = [
  ["domain", "完整域名"],
  ["domain_suffix", "域名后缀"],
  ["domain_keyword", "域名关键词"],
  ["ip_cidr", "IP CIDR"],
  ["process_name", "进程名称"],
  ["geoip", "GeoIP"],
  ["geosite", "GeoSite"],
  ["rule_set", "规则集"],
  ["mixed", "混合规则"],
] as const;

type MixedBlock = { ruleType: string; value: string };

const mixedRuleTypes = types.filter((item) => item[0] !== "mixed");

function parseMixedBlocks(valuesText: string): MixedBlock[] {
  const blocks = valuesText
    .split("\n")
    .map((line) => {
      const trimmed = line.trim();
      const separator = trimmed.search(/[:=]/);
      if (separator <= 0)
        return { ruleType: "domain_suffix", value: trimmed };
      return {
        ruleType: trimmed.slice(0, separator).trim(),
        value: trimmed.slice(separator + 1).trim(),
      };
    })
    .filter((item) => item.ruleType || item.value);
  return blocks.length ? blocks : [{ ruleType: "geosite", value: "" }];
}

function serializeMixedBlocks(blocks: MixedBlock[]) {
  return blocks.map((item) => `${item.ruleType}:${item.value}`).join("\n");
}

const mixedBlocks = computed(() => parseMixedBlocks(props.valuesText));

function updateMixedBlock(index: number, patch: Partial<MixedBlock>) {
  const next = mixedBlocks.value.map((item, itemIndex) =>
    itemIndex === index ? { ...item, ...patch } : item,
  );
  emit("update:valuesText", serializeMixedBlocks(next));
}

function addMixedBlock() {
  emit(
    "update:valuesText",
    serializeMixedBlocks([
      ...mixedBlocks.value,
      { ruleType: "domain_suffix", value: "" },
    ]),
  );
}

function removeMixedBlock(index: number) {
  const next = mixedBlocks.value.filter((_, itemIndex) => itemIndex !== index);
  emit(
    "update:valuesText",
    serializeMixedBlocks(
      next.length ? next : [{ ruleType: "geosite", value: "" }],
    ),
  );
}

function generatedGeoRuleSetTag(ruleType: string, value: string) {
  const normalized = value.trim().toLowerCase();
  if (!normalized) return "";
  return normalized.startsWith(`${ruleType}-`)
    ? normalized
    : `${ruleType}-${normalized}`;
}

function previewAction() {
  return props.outbound === "block"
    ? { action: "reject" }
    : { action: "route", outbound: props.outbound };
}

function previewRules() {
  const values = props.valuesText
    .split("\n")
    .map((value) => value.trim())
    .filter(Boolean);
  if (props.ruleType !== "mixed") {
    const key = ["geoip", "geosite"].includes(props.ruleType)
      ? "rule_set"
      : props.ruleType;
    const previewValues = ["geoip", "geosite"].includes(props.ruleType)
      ? values
          .map((value) => generatedGeoRuleSetTag(props.ruleType, value))
          .filter(Boolean)
      : values;
    return [
      {
        [key]: previewValues,
        ...previewAction(),
        ...(props.invert ? { invert: true } : {}),
      },
    ];
  }

  const rules: Array<Record<string, unknown>> = [];
  const groupIndex = new Map<string, number>();
  const appendGrouped = (key: string, value: string) => {
    const index = groupIndex.get(key);
    if (index !== undefined) {
      (rules[index][key] as string[]).push(value);
      return;
    }
    groupIndex.set(key, rules.length);
    rules.push({
      [key]: [value],
      ...previewAction(),
      ...(props.invert ? { invert: true } : {}),
    });
  };
  for (const block of mixedBlocks.value) {
    const value = block.value.trim();
    if (!value) continue;
    if (["geoip", "geosite"].includes(block.ruleType)) {
      appendGrouped(
        "rule_set",
        generatedGeoRuleSetTag(block.ruleType, value),
      );
    } else if (block.ruleType === "rule_set") {
      appendGrouped("rule_set", value);
    } else {
      appendGrouped(block.ruleType, value);
    }
  }
  return rules;
}

function ruleValuePlaceholder(ruleType: string) {
  if (ruleType === "rule_set") return "geosite-cn\ngeoip-cn";
  if (ruleType === "geosite") return "youtube\ngoogle\nnetflix";
  if (ruleType === "geoip") return "cn\nprivate\nus";
  if (ruleType === "process_name") return "chrome.exe\nsing-box";
  if (ruleType === "ip_cidr") return "192.0.2.0/24\n2001:db8::/32";
  return "google.com\ntelegram.org\ngithub.com";
}

function ruleValueHelp(ruleType: string) {
  if (ruleType === "geosite")
    return "填写 geosite 分类名，生成时自动转换为对应 rule_set。";
  if (ruleType === "geoip")
    return "填写 GeoIP 区域代码，例如 cn、private、us。";
  if (ruleType === "rule_set") return "填写已有规则订阅或生成规则集的 tag。";
  if (ruleType === "mixed")
    return "可组合域名、GeoIP、GeoSite、CIDR 等条件；任一条件命中即执行同一出站。";
  if (ruleType === "process_name")
    return "Windows 通常包含 .exe，Linux/macOS 填写可执行文件名。";
  return "每行一个值，保存时自动清理空行和重复项。";
}

const ruleEmojis = defaultEmojis;

function looksLikeEmojiPrefix(value: string) {
  return /^([\p{Extended_Pictographic}\p{Regional_Indicator}\uFE0F\u200D]+)$/u.test(
    value,
  );
}

function stripRuleEmoji(value: string) {
  const trimmed = value.trimStart();
  for (const emoji of ruleEmojis) {
    if (trimmed === emoji) return "";
    if (trimmed.startsWith(`${emoji} `))
      return trimmed.slice(`${emoji} `.length);
    if (trimmed.startsWith(emoji))
      return trimmed.slice(emoji.length).trimStart();
  }
  const [first, ...rest] = trimmed.split(/\s+/);
  return first && looksLikeEmojiPrefix(first) ? rest.join(" ") : value;
}

const selectedEmoji = computed(() => {
  const trimmed = props.name.trimStart();
  const known = ruleEmojis.find(
    (emoji) =>
      trimmed === emoji ||
      trimmed.startsWith(`${emoji} `) ||
      trimmed.startsWith(emoji),
  );
  if (known) return known;
  const first = trimmed.split(/\s+/)[0] || "";
  return looksLikeEmojiPrefix(first) ? first : "";
});

function updateEmoji(emoji: string) {
  const plainName = stripRuleEmoji(props.name);
  emit("update:name", emoji ? `${emoji} ${plainName}`.trim() : plainName);
}

function updatePlainName(event: Event) {
  const plainName = (event.target as HTMLInputElement).value;
  emit(
    "update:name",
    selectedEmoji.value
      ? `${selectedEmoji.value} ${plainName}`.trim()
      : plainName,
  );
}

const title = computed(() =>
  props.editing?.is_system
    ? "查看路由规则"
    : props.editing
      ? "编辑路由规则"
      : "添加路由规则",
);

const preview = computed(() => JSON.stringify(previewRules(), null, 2));

function update(
  event: Event,
  field: "valuesText" | "outbound",
) {
  const value = (
    event.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  ).value;
  if (field === "valuesText") emit("update:valuesText", value);
  else emit("update:outbound", value);
}

function updateRuleType(event: Event) {
  const next = (event.target as HTMLSelectElement).value;
  if (next === "mixed" && props.ruleType !== "mixed") {
    const values = props.valuesText
      .split("\n")
      .map((value) => value.trim())
      .filter(Boolean);
    if (values.length) {
      emit(
        "update:valuesText",
        serializeMixedBlocks(
          values.map((value) => ({ ruleType: props.ruleType, value })),
        ),
      );
    }
  } else if (props.ruleType === "mixed" && next !== "mixed") {
    emit(
      "update:valuesText",
      mixedBlocks.value
        .map((item) => item.value.trim())
        .filter(Boolean)
        .join("\n"),
    );
  }
  emit("update:ruleType", next);
}
</script>

<template>
  <Modal :open="true" :title="title" size="xl" @close="emit('close')">
    <template #title>
      <span class="flex items-center gap-2"
        ><Route :size="18" />{{ title }}</span
      >
    </template>

    <div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_minmax(360px,0.9fr)]">
      <div class="space-y-4">
        <label class="block text-xs font-medium">
          规则名称
          <div class="mt-1.5 grid grid-cols-[48px_minmax(0,1fr)] gap-2">
            <EmojiPicker
              :value="selectedEmoji"
              :disabled="editing?.is_system"
              @change="updateEmoji"
            />
            <input
              :value="stripRuleEmoji(name)"
              class="!mt-0"
              :disabled="editing?.is_system"
              placeholder="例如：🤖 OpenAI 代理"
              @input="updatePlainName"
            />
          </div>
        </label>

        <div class="grid gap-3 sm:grid-cols-2">
          <label class="block text-xs font-medium">
            匹配类型
            <select
              :value="ruleType"
              :disabled="editing?.is_system"
              @change="updateRuleType"
            >
              <option v-for="item in types" :key="item[0]" :value="item[0]">
                {{ item[1] }}
              </option>
            </select>
          </label>
          <label class="block text-xs font-medium">
            命中后出站
            <select
              :value="outbound"
              :disabled="editing?.is_system"
              @change="update($event, 'outbound')"
            >
              <option value="direct">直连 direct</option>
              <option value="proxy">策略 proxy</option>
              <option value="block">阻断 block</option>
            </select>
          </label>
        </div>

        <div
          v-if="ruleType === 'mixed'"
          class="space-y-3 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
        >
          <div class="flex items-center justify-between gap-3">
            <div>
              <div class="text-xs font-medium">混合匹配条件</div>
              <p class="mt-1 text-[11px] text-[var(--text-tertiary)]">
                任一条件命中即可；生成时按类型展开为多条相邻规则。
              </p>
            </div>
            <button
              v-if="!editing?.is_system"
              type="button"
              class="aw-action-button aw-action-neutral"
              @click="addMixedBlock"
            >
              <Plus :size="13" />添加条件
            </button>
          </div>
          <div
            v-for="(block, index) in mixedBlocks"
            :key="index"
            class="rounded-[var(--radius-lg)] border border-[var(--border-light)] bg-[var(--bg-surface)] p-3"
          >
            <div class="mb-2 flex items-center justify-between gap-3">
              <span class="text-xs font-medium">条件 #{{ index + 1 }}</span>
              <button
                v-if="mixedBlocks.length > 1 && !editing?.is_system"
                type="button"
                class="aw-action-button aw-action-danger"
                title="删除条件"
                @click="removeMixedBlock(index)"
              >
                <Trash2 :size="13" />删除
              </button>
            </div>
            <div class="grid gap-2 sm:grid-cols-[180px_minmax(0,1fr)]">
              <label class="block text-xs font-medium">
                匹配类型
                <select
                  :value="block.ruleType"
                  :disabled="editing?.is_system"
                  @change="
                    updateMixedBlock(index, {
                      ruleType: ($event.target as HTMLSelectElement).value,
                    })
                  "
                >
                  <option
                    v-for="item in mixedRuleTypes"
                    :key="item[0]"
                    :value="item[0]"
                  >
                    {{ item[1] }}
                  </option>
                </select>
              </label>
              <label class="block text-xs font-medium">
                匹配值
                <input
                  :value="block.value"
                  class="font-mono"
                  :disabled="editing?.is_system"
                  :placeholder="ruleValuePlaceholder(block.ruleType).split('\n')[0]"
                  @input="
                    updateMixedBlock(index, {
                      value: ($event.target as HTMLInputElement).value,
                    })
                  "
                />
              </label>
            </div>
          </div>
        </div>

        <label v-else class="block text-xs font-medium">
          匹配值
          <textarea
            :value="valuesText"
            rows="9"
            class="min-h-[210px] w-full font-mono"
            :disabled="editing?.is_system"
            :placeholder="ruleValuePlaceholder(ruleType)"
            @input="update($event, 'valuesText')"
          />
          <span
            class="mt-1.5 block text-[11px] font-normal text-[var(--text-tertiary)]"
          >
            {{ ruleValueHelp(ruleType) }}
          </span>
        </label>

        <div v-if="ruleType === 'rule_set' && subscriptions.length">
          <div class="mb-2 text-xs font-medium">可用规则集</div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="subscription in subscriptions"
              :key="subscription.id"
              class="aw-filter-chip"
              :disabled="editing?.is_system"
              @click="
                emit(
                  'update:valuesText',
                  [valuesText, subscription.tag].filter(Boolean).join('\n'),
                )
              "
            >
              {{ subscription.tag }}
            </button>
          </div>
        </div>

        <div
          class="flex flex-wrap gap-5 rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2.5 text-xs"
        >
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              :checked="enabled"
              :disabled="editing?.is_system"
              @change="
                emit(
                  'update:enabled',
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />启用规则
          </label>
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              :checked="invert"
              :disabled="editing?.is_system"
              @change="
                emit(
                  'update:invert',
                  ($event.target as HTMLInputElement).checked,
                )
              "
            />反向匹配
          </label>
        </div>
      </div>

      <aside
        class="min-w-0 overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)]"
      >
        <header
          class="flex items-center gap-2 border-b border-[var(--border-light)] px-4 py-3 font-medium"
        >
          <FileJson2 :size="16" /> sing-box 规则预览
        </header>
        <pre
          class="max-h-[480px] overflow-auto p-4 text-xs leading-6 text-[var(--text-secondary)]"
          >{{ preview }}</pre>
      </aside>
    </div>

    <template #footer>
      <button class="aw-action-button aw-action-neutral" @click="emit('close')">
        {{ editing?.is_system ? "关闭" : "取消" }}
      </button>
      <button
        v-if="!editing?.is_system"
        class="aw-action-button aw-action-success"
        @click="emit('save')"
      >
        <Save :size="14" />保存规则
      </button>
    </template>
  </Modal>
</template>
