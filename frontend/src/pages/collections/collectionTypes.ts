import type {
  ProxyCollectionRequest,
  ProxyCollectionWithNodes,
} from "@/services/types";

export interface NodeGroup {
  id: number;
  name: string;
  type: string;
  filter_protocols: string;
  filter_subscriptions: string;
  filter_include: string;
  filter_exclude: string;
  node_uids: string;
  test_url: string;
  test_interval: number;
  tolerance: number;
  enabled: boolean;
  priority: number;
  matched_node_count: number;
}

export interface FacetItem {
  value: string;
  label: string;
  count: number;
}

export type CollectionSourceType =
  | "node_groups"
  | "node_groups_and_nodes"
  | "manual";

export interface DetailedProxyCollection extends ProxyCollectionWithNodes {
  source_type: CollectionSourceType;
  referenced_group_ids: string;
  referenced_groups: NodeGroup[];
}

export interface StrategyCollectionRequest extends ProxyCollectionRequest {
  source_type: CollectionSourceType;
  referenced_group_ids: number[];
}

export interface NodeGroupRequest {
  name: string;
  type: string;
  filter_protocols: string;
  filter_subscriptions: string;
  filter_include: string;
  filter_exclude: string;
  node_uids: string[];
  enabled: boolean;
  priority: number;
  tolerance: number;
}
