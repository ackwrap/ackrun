import type { ProxyGroup } from '@/services/clash';

export type MonitorTab = 'overview' | 'proxies' | 'connections' | 'rules';

export const monitorPanelClass = 'rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] p-4 shadow-[var(--shadow-card)]';
export const monitorPanelBodyClass = 'rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[var(--bg-surface)] shadow-[var(--shadow-card)]';

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
}

export function formatSpeed(bytesPerSecond: number): string {
  return formatBytes(bytesPerSecond) + '/s';
}

export function proxyGroupIcon(group: ProxyGroup): string {
  const customEmoji = group.name.match(/^(\S+)\s+(.+)$/u);
  if (customEmoji && /[\p{Extended_Pictographic}\p{Regional_Indicator}]/u.test(customEmoji[1])) return customEmoji[1];
  const regionFlags: Array<[string[], string]> = [
    [['香港'], '🇭🇰'], [['台湾', '台灣'], '🇹🇼'], [['日本'], '🇯🇵'], [['韩国', '韓國'], '🇰🇷'],
    [['新加坡'], '🇸🇬'], [['印度'], '🇮🇳'], [['泰国', '泰國'], '🇹🇭'], [['越南'], '🇻🇳'], [['菲律宾', '菲律賓'], '🇵🇭'],
    [['美国', '美國'], '🇺🇸'], [['加拿大'], '🇨🇦'], [['巴西'], '🇧🇷'], [['阿根廷'], '🇦🇷'], [['墨西哥'], '🇲🇽'],
    [['英国', '英國'], '🇬🇧'], [['法国', '法國'], '🇫🇷'], [['德国', '德國'], '🇩🇪'], [['荷兰', '荷蘭'], '🇳🇱'],
    [['瑞士'], '🇨🇭'], [['瑞典'], '🇸🇪'], [['挪威'], '🇳🇴'], [['芬兰', '芬蘭'], '🇫🇮'], [['丹麦', '丹麥'], '🇩🇰'],
    [['意大利'], '🇮🇹'], [['西班牙'], '🇪🇸'], [['葡萄牙'], '🇵🇹'], [['波兰', '波蘭'], '🇵🇱'], [['俄罗斯', '俄羅斯'], '🇷🇺'],
    [['土耳其'], '🇹🇷'], [['澳大利亚', '澳大利亞', '澳洲'], '🇦🇺'], [['新西兰', '新西蘭'], '🇳🇿'],
    [['南非'], '🇿🇦'], [['阿联酋', '阿聯酋'], '🇦🇪'], [['以色列'], '🇮🇱'],
  ];
  if (group.name === '全部节点' || group.name === '全部節點') return '🇺🇳';
  for (const [keywords, flag] of regionFlags) {
    if (keywords.some(keyword => group.name.includes(keyword))) return flag;
  }
  if (group.name === '全球直连') return '🎯';
  return group.type === 'Selector' ? '👆' : '🚀';
}
