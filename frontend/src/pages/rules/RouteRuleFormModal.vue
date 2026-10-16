<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { FileJson2, Plus, Route, Save, Trash2 } from "lucide-vue-next";
import Modal from "@/components/ui/Modal.vue";
import EmojiPicker, { defaultEmojis } from "@/components/ui/EmojiPicker.vue";
import { api } from "@/services/api";
import type { RouteRule, RouteRuleSubscription } from "@/services/types";

const props = defineProps<{
  editing: RouteRule | null;
  name: string;
  enabled: boolean;
  ruleType: string;
  valuesText: string;
  directDomainsText: string;
  outbound: string;
  invert: boolean;
  subscriptions: RouteRuleSubscription[];
  saving?: boolean;
}>();

const emit = defineEmits<{
  close: [];
  save: [];
  "update:name": [string];
  "update:enabled": [boolean];
  "update:ruleType": [string];
  "update:valuesText": [string];
  "update:directDomainsText": [string];
  "update:outbound": [string];
  "update:invert": [boolean];
}>();

const types = [
  ["domain", "完整域名"],
  ["domain_suffix", "域名后缀"],
  ["domain_keyword", "域名关键词"],
  ["ip_cidr", "IP CIDR"],
  ["source_ip_cidr", "设备 IP / 网段（DNS 跟随）"],
  ["process_name", "进程名称"],
  ["geoip", "GeoIP"],
  ["geosite", "GeoSite"],
  ["rule_set", "规则集"],
  ["mixed", "混合规则"],
] as const;

type MixedBlock = { ruleType: string; value: string };
type GeoRuleType = "geoip" | "geosite";

const mixedRuleTypes = types.filter(
  (item) => item[0] !== "mixed" && item[0] !== "source_ip_cidr",
);
const isDevice = computed(() => props.ruleType === "source_ip_cidr");
const geoTagDraft = ref("");
const geoTagOptions = ref<Record<GeoRuleType, string[]>>({
  geoip: [],
  geosite: [],
});
const geoTagMessage = ref<Record<GeoRuleType, string>>({
  geoip: "",
  geosite: "",
});
const geoTagError = ref<Record<GeoRuleType, string>>({
  geoip: "",
  geosite: "",
});
const geoTagRequest = { geoip: 0, geosite: 0 };
const geoTagSearchTimer: Record<
  GeoRuleType,
  ReturnType<typeof setTimeout> | undefined
> = { geoip: undefined, geosite: undefined };

function isGeoRuleType(ruleType: string): ruleType is GeoRuleType {
  return ruleType === "geoip" || ruleType === "geosite";
}

async function searchGeoTags(ruleType: string, query: string) {
  if (!isGeoRuleType(ruleType)) return;
  const requestID = ++geoTagRequest[ruleType];
  try {
    geoTagError.value[ruleType] = "";
    const response = await api.getGeoTags(ruleType, query.trim());
    if (requestID !== geoTagRequest[ruleType]) return;
    geoTagOptions.value[ruleType] = response.tags;
    const message = response.message === "查询完成" ? "" : response.message;
    geoTagMessage.value[ruleType] =
      response.ready && !response.tags.length
        ? ["未找到匹配分类", message].filter(Boolean).join("；")
        : message;
  } catch (error: any) {
    if (requestID !== geoTagRequest[ruleType]) return;
    geoTagOptions.value[ruleType] = [];
    geoTagMessage.value[ruleType] = "";
    geoTagError.value[ruleType] = error.message;
  }
}

function scheduleGeoTagSearch(ruleType: string, query: string) {
  if (!isGeoRuleType(ruleType)) return;
  clearTimeout(geoTagSearchTimer[ruleType]);
  geoTagSearchTimer[ruleType] = setTimeout(
    () => searchGeoTags(ruleType, query),
    200,
  );
}

function searchGeoTagsFromEvent(ruleType: string, event: Event) {
  scheduleGeoTagSearch(ruleType, (event.target as HTMLInputElement).value);
}

function appendGeoTag(tag = geoTagDraft.value) {
  const normalized = tag.trim();
  if (!normalized || !isGeoRuleType(props.ruleType)) return;
  const current = props.valuesText
    .split("\n")
    .map((value) => value.trim())
    .filter(Boolean);
  if (
    !current.some((value) => value.toLowerCase() === normalized.toLowerCase())
  ) {
    emit("update:valuesText", [...current, normalized].join("\n"));
  }
  geoTagDraft.value = "";
}

function parseMixedBlocks(valuesText: string): MixedBlock[] {
  const blocks = valuesText
    .split("\n")
    .map((line) => {
      const trimmed = line.trim();
      const separator = trimmed.search(/[:=]/);
      if (separator <= 0) return { ruleType: "domain_suffix", value: trimmed };
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

function updateMixedValue(index: number, ruleType: string, event: Event) {
  const value = (event.target as HTMLInputElement).value;
  updateMixedBlock(index, { value });
  scheduleGeoTagSearch(ruleType, value);
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
  if (props.outbound === "block") return { action: "reject" };
  if (props.outbound === "bypass") return { action: "bypass" };
  return { action: "route", outbound: props.outbound };
}

function previewRules() {
  const values = props.valuesText
    .split("\n")
    .map((value) => value.trim())
    .filter(Boolean);
  if (isDevice.value) {
    const domains = props.directDomainsText
      .split("\n")
      .map((value) => value.trim().toLowerCase().replace(/\.$/, ""))
      .filter(Boolean);
    return [
      ...(domains.length
        ? [
            {
              source_ip_cidr: values,
              domain_suffix: domains,
              action: "route",
              outbound: "direct",
            },
          ]
        : []),
      {
        source_ip_cidr: values,
        action: "route",
        outbound: props.name || "设备固定节点",
      },
    ];
  }
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
      appendGrouped("rule_set", generatedGeoRuleSetTag(block.ruleType, value));
    } else if (block.ruleType === "rule_set") {
      appendGrouped("rule_set", value);
    } else {
      appendGrouped(block.ruleType, value);
    }
  }
  return rules;
}

function ruleValuePlaceholder(ruleType: string) {
  if (ruleType === "source_ip_cidr") return "192.168.1.100\nfd12:3456::100/128";
  if (ruleType === "rule_set") return "geosite-cn\ngeoip-cn";
  if (ruleType === "geosite") return "geolocation-!cn\nyoutube\ngoogle";
  if (ruleType === "geoip") return "cn\nprivate\nus";
  if (ruleType === "process_name") return "chrome.exe\nsing-box";
  if (ruleType === "ip_cidr") return "192.0.2.0/24\n2001:db8::/32";
  return "google.com\ntelegram.org\ngithub.com";
}

function ruleValueHelp(ruleType: string) {
  if (ruleType === "geosite")
    return "填写或搜索 GeoSite 分类名；geolocation-!cn 中的 ! 是官方分类名的一部分，其他排除请使用“反向匹配”。";
  if (ruleType === "geoip")
    return "填写 GeoIP 区域代码，例如 cn、private、us；不要添加 !，排除请使用“反向匹配”。";
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

const isFinalStrategy = computed(
  () =>
    props.editing?.system_key === "global_direct" ||
    ["fallback", "final"].includes(props.editing?.rule_type || ""),
);

const title = computed(() =>
  isFinalStrategy.value
    ? "编辑最终策略"
    : props.editing?.is_system
      ? "查看路由规则"
      : props.editing
        ? "编辑路由规则"
        : "添加路由规则",
);

const preview = computed(() =>
  JSON.stringify(
    isFinalStrategy.value
      ? {
          route: {
            final: props.outbound === "proxy" ? "proxy" : "全球直连",
          },
        }
      : previewRules(),
    null,
    2,
  ),
);

function update(event: Event, field: "valuesText" | "outbound") {
  const value = (
    event.target as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  ).value;
  if (field === "valuesText") emit("update:valuesText", value);
  else emit("update:outbound", value);
}

function updateRuleType(event: Event) {
  const next = (event.target as HTMLSelectElement).value;
  if (next === "source_ip_cidr") {
    emit("update:outbound", "proxy");
    emit("update:invert", false);
  }
  geoTagDraft.value = "";
  if (next === "mixed" && props.ruleType === "source_ip_cidr") {
    emit("update:valuesText", "");
  } else if (next === "mixed" && props.ruleType !== "mixed") {
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

onBeforeUnmount(() => {
  clearTimeout(geoTagSearchTimer.geoip);
  clearTimeout(geoTagSearchTimer.geosite);
});
</script>

<template>
  <Modal
    :open="true"
    :title="title"
    size="xl"
    :closable="!saving"
    @close="emit('close')"
  >
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

        <p
          v-if="isFinalStrategy"
          class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] px-3 py-2.5 text-xs text-[var(--text-secondary)]"
        >
          控制所有未命中前置规则的流量。选择“策略
          proxy”可避免未知域名回落直连；该系统规则固定启用且不可删除或排序。
        </p>

        <div class="grid gap-3 sm:grid-cols-2">
          <label v-if="!isFinalStrategy" class="block text-xs font-medium">
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
            命中后动作
            <select
              :value="outbound"
              :disabled="isDevice || (editing?.is_system && !isFinalStrategy)"
              @change="update($event, 'outbound')"
            >
              <option value="direct">直连 direct</option>
              <option value="proxy">策略 proxy</option>
              <option v-if="!isFinalStrategy" value="bypass">
                内核绕过 bypass
              </option>
              <option v-if="!isFinalStrategy" value="block">阻断 block</option>
            </select>
            <span
              v-if="!isFinalStrategy && outbound === 'bypass'"
              class="mt-1.5 block text-[11px] font-normal text-[var(--text-tertiary)]"
            >
              Linux TUN auto_redirect 下仅在目标仍为 IP
              且条件已可判定时优先绕过代理；GeoSite/域名依赖已有 DNS
              映射，其他模式不提供同等保证。
            </span>
          </label>
        </div>

        <p
          v-if="isDevice"
          class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3 text-xs leading-6 text-[var(--text-secondary)]"
        >
          每行填写一个设备 IP 或网段，例如 192.168.1.100 或 fd12:3456::100/128。
          建议固定 DHCP 地址；同时填写设备实际使用的 IPv4 和 IPv6 地址。
          保存后选择一个固定节点，非排除接口的外网流量默认走该节点，DNS
          默认使用同一出口并优先于普通 DNS、FakeIP
          和业务分流规则。本地网络自动直连：IPv4 私网、链路本地地址及 IPv6
          ULA、链路本地地址，不强制经过节点。FakeIP
          地址池会从本地直连范围中扣除， 还原为真实目标后再决定出口。需要
          Linux/OpenWrt、规则模式、TUN 接管及 DNS 管理；设备网关须指向运行
          Ackwrap 的路由器。明确排除接口的入站流量优先直连，不受设备绑定、设备
          DNS 接管和防泄漏阻断影响。本地目标 IP/CIDR
          和进程排除可保留；需移除外网目标
          IP、域名排除及与设备重叠的来源排除，并关闭软/硬件流量卸载。
          高级平台路由不能覆盖该设备；独立代理入口仍按各自策略转发。
          未命中直连白名单的外网流量在节点失效时连接失败，不自动直连；本地网络仍可访问。
          节点服务连接的防回环排除仍生效；UDP 等协议需节点支持。
        </p>

        <div
          v-if="!isFinalStrategy && ruleType === 'mixed'"
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
                  :placeholder="
                    ruleValuePlaceholder(block.ruleType).split('\n')[0]
                  "
                  @focus="searchGeoTags(block.ruleType, block.value)"
                  @input="updateMixedValue(index, block.ruleType, $event)"
                />
                <span
                  v-if="['geoip', 'geosite'].includes(block.ruleType)"
                  class="mt-1.5 block text-[11px] font-normal text-[var(--text-tertiary)]"
                >
                  {{
                    block.ruleType === "geosite"
                      ? "可搜索数据库分类；geolocation-!cn 中的 ! 属于分类名。"
                      : "仅填写区域代码（如 cn），排除请使用“反向匹配”。"
                  }}
                </span>
                <span
                  v-if="
                    isGeoRuleType(block.ruleType) && geoTagError[block.ruleType]
                  "
                  class="mt-1.5 block text-[11px] font-normal text-[var(--color-error)]"
                >
                  标签加载失败：{{ geoTagError[block.ruleType] }}
                </span>
                <span
                  v-else-if="
                    isGeoRuleType(block.ruleType) &&
                    geoTagMessage[block.ruleType]
                  "
                  class="mt-1.5 block text-[11px] font-normal text-[var(--text-tertiary)]"
                >
                  {{ geoTagMessage[block.ruleType] }}
                </span>
              </label>
            </div>
          </div>
        </div>

        <label v-else-if="!isFinalStrategy" class="block text-xs font-medium">
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

        <label v-if="isDevice" class="block text-xs font-medium">
          直连域名白名单（可选）
          <textarea
            :value="directDomainsText"
            rows="4"
            class="w-full font-mono"
            placeholder="baidu.com&#10;example.com"
            @input="
              emit(
                'update:directDomainsText',
                ($event.target as HTMLTextAreaElement).value,
              )
            "
          />
          <span
            class="mt-1.5 block text-[11px] font-normal leading-5 text-[var(--text-tertiary)]"
          >
            每行一个域名，包含该域名及其子域名；最多 256
            个，不填协议、路径或通配符。 仅对上方设备生效：白名单流量和 DNS
            直连，其余默认走绑定节点。 DNS 使用全局默认的远程上游（默认项非远程
            DNS 时使用代理 DNS Final 上游），经直连查询。域名通过受管 DNS
            或连接嗅探识别；无法识别域名时仍走绑定节点。
            开启流量防泄漏后，核心停止时白名单外网流量也会阻断。
          </span>
        </label>

        <div
          v-if="isGeoRuleType(ruleType)"
          class="rounded-[var(--radius-lg)] border border-[var(--border-default)] bg-[var(--bg-base)] p-3"
        >
          <div class="mb-2 text-xs font-medium">
            搜索并添加
            {{ ruleType === "geosite" ? "GeoSite 分类" : "GeoIP 区域" }}
          </div>
          <div class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
            <input
              v-model="geoTagDraft"
              :disabled="editing?.is_system"
              :placeholder="
                ruleType === 'geosite' ? '例如 geolocation-!cn' : '例如 cn'
              "
              @focus="searchGeoTags(ruleType, geoTagDraft)"
              @input="searchGeoTagsFromEvent(ruleType, $event)"
              @keydown.enter.prevent="appendGeoTag()"
            />
            <button
              type="button"
              class="aw-action-button aw-action-neutral"
              :disabled="editing?.is_system || !geoTagDraft.trim()"
              @click="appendGeoTag()"
            >
              <Plus :size="13" />添加
            </button>
          </div>
          <div
            v-if="geoTagOptions[ruleType].length"
            class="mt-2 flex max-h-24 flex-wrap gap-2 overflow-auto"
          >
            <button
              v-for="tag in geoTagOptions[ruleType]"
              :key="tag"
              type="button"
              class="aw-filter-chip"
              :disabled="editing?.is_system"
              @click="appendGeoTag(tag)"
            >
              {{ tag }}
            </button>
          </div>
          <p
            v-if="geoTagMessage[ruleType]"
            class="mt-2 text-xs text-[var(--text-tertiary)]"
          >
            {{ geoTagMessage[ruleType] }}
          </p>
          <p
            v-if="geoTagError[ruleType]"
            class="mt-2 text-xs text-[var(--color-error)]"
          >
            标签加载失败：{{ geoTagError[ruleType] }}
          </p>
        </div>

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
          v-if="!isFinalStrategy"
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
              :disabled="editing?.is_system || isDevice"
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
      <button
        class="aw-action-button aw-action-neutral"
        :disabled="saving"
        @click="emit('close')"
      >
        {{ editing?.is_system && !isFinalStrategy ? "关闭" : "取消" }}
      </button>
      <button
        v-if="!editing?.is_system || isFinalStrategy"
        class="aw-action-button aw-action-success"
        :disabled="saving"
        @click="emit('save')"
      >
        <Save :size="14" />{{ isFinalStrategy ? "保存最终策略" : "保存规则" }}
      </button>
    </template>
  </Modal>
</template>
