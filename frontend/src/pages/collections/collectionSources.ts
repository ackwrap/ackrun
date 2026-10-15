import type { DetailedProxyCollection } from "./collectionTypes";

export const directNodeGroup = {
  name: "全球直连",
  outbound: "direct",
} as const;

export function collectionGroupNames(collection: DetailedProxyCollection) {
  const names =
    collection.source_type === "manual"
      ? []
      : (collection.referenced_groups || []).map((group) => group.name);
  if ((collection.node_uids || []).includes(directNodeGroup.outbound)) {
    names.push(directNodeGroup.name);
  }
  return names;
}
