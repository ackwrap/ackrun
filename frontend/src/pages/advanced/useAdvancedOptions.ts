import { ref } from "vue";
import { api } from "@/services/api";
import { advancedApi } from "@/services/advancedApi";
import type { NodeExposure } from "@/services/advancedTypes";
import type { ProxyCollectionWithNodes } from "@/services/types";

export interface SafeNodeOption {
  value: string;
  name: string;
  type: string;
}

export function useAdvancedOptions() {
  const nodes = ref<SafeNodeOption[]>([]);
  const collections = ref<ProxyCollectionWithNodes[]>([]);
  const exposures = ref<NodeExposure[]>([]);

  async function loadNodes() {
    const safe: SafeNodeOption[] = [];
    let offset = 0;
    let total = 0;
    do {
      const page = await api.getNodes({ enabled: true, limit: 200, offset });
      safe.push(
        ...page.items.map((node) => ({
          value: `node:${node.subscription_id}:${node.uid}`,
          name: node.name,
          type: node.type,
        })),
      );
      total = page.total;
      if (!page.items.length) break;
      offset += page.items.length;
    } while (offset < total && offset > 0);
    nodes.value = safe;
  }

  async function loadOptions() {
    const [, collectionItems, exposureItems] = await Promise.all([
      loadNodes(),
      api.getProxyCollections(),
      advancedApi.getNodeExposures(),
    ]);
    collections.value = collectionItems;
    exposures.value = exposureItems;
  }

  function targetName(value: string) {
    if (value === "direct") return "直连";
    if (value.startsWith("collection:")) {
      const id = Number(value.split(":")[1]);
      return collections.value.find((item) => item.id === id)?.name || "策略组失效";
    }
    return nodes.value.find((item) => item.value === value)?.name || "节点失效";
  }

  return { nodes, collections, exposures, loadOptions, targetName };
}
