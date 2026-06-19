interface FacetItem {
  value: string;
  label: string;
  count: number;
}

interface NodeGroup {
  id: number;
  name: string;
  filter_protocols: string;
  filter_subscriptions: string;
  filter_include: string;
  filter_exclude: string;
}

interface NodeGroupMatchedNode {
  uid: string;
  name: string;
  type: string;
  subscription_id: number;
  subscription_name: string;
  latency_ms: number;
  status: string;
}

interface NodeGroupDetailModalProps {
  group: NodeGroup;
  nodes: NodeGroupMatchedNode[];
  loading: boolean;
  subscriptions: FacetItem[];
  onClose: () => void;
}

export function NodeGroupDetailModal({ group, nodes, loading, subscriptions, onClose }: NodeGroupDetailModalProps) {
  const subscriptionLabel = group.filter_subscriptions
    ? group.filter_subscriptions.split(',').map(id => subscriptions.find(item => item.value === id)?.label || id).join('、')
    : '全部';

  return (
    <div className="aw-modal-backdrop" onClick={onClose}>
      <div className="aw-modal-panel w-full max-w-5xl p-6" onClick={e => e.stopPropagation()}>
        <div className="mb-4 flex items-start justify-between gap-4">
          <div>
            <h4 className="text-lg font-semibold text-white">节点组详情：{group.name}</h4>
            <p className="aw-modal-muted mt-1 text-sm">当前匹配 {nodes.length} 个节点。排除关键词优先，随后包含关键词任意命中即加入。</p>
          </div>
          <button onClick={onClose} className="aw-modal-close">✕</button>
        </div>

        <div className="aw-modal-summary mb-4 grid gap-3 md:grid-cols-4">
          <div><span className="aw-modal-label">协议：</span><span className="text-white uppercase">{group.filter_protocols || '全部'}</span></div>
          <div><span className="aw-modal-label">订阅：</span><span className="text-white">{subscriptionLabel}</span></div>
          <div className="md:col-span-2"><span className="aw-modal-label">包含：</span><span className="font-mono text-white">{group.filter_include || '无'}</span></div>
          <div className="md:col-span-4"><span className="aw-modal-label">排除：</span><span className="font-mono text-white">{group.filter_exclude || '无'}</span></div>
        </div>

        <div className="aw-data-table-wrap max-h-[60vh]">
          <table className="aw-data-table min-w-[820px]">
            <thead>
              <tr>{['节点名称', '协议', '订阅来源', '延迟', '状态'].map(col => <th key={col}>{col}</th>)}</tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={5} className="py-10 text-center text-slate-400">加载中...</td></tr>
              ) : nodes.length === 0 ? (
                <tr><td colSpan={5} className="py-10 text-center text-slate-400">没有匹配到节点</td></tr>
              ) : nodes.map(node => (
                <tr key={node.uid}>
                  <td className="max-w-[420px] truncate font-medium text-white" title={node.name}>{node.name || '(未命名节点)'}</td>
                  <td className="uppercase text-blue-200">{node.type}</td>
                  <td className="text-slate-200">{node.subscription_name || `订阅 ${node.subscription_id}`}</td>
                  <td>{node.latency_ms > 0 ? `${node.latency_ms} ms` : '-'}</td>
                  <td>{node.status || 'unknown'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
