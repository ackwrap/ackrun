import type { ProxyGroup } from '@/services/clash';
import { getFlagImageURL } from '@/utils/nodeFlags';
import { proxyGroupIcon } from './monitorUtils';

interface ProxyGroupIconProps {
  group: ProxyGroup;
  className?: string;
}

export function ProxyGroupIcon({ group, className = 'h-5 w-5' }: ProxyGroupIconProps) {
  const icon = proxyGroupIcon(group);
  const isFlag = /^\p{Regional_Indicator}{2}$/u.test(icon);
  if (isFlag) {
    return <img src={getFlagImageURL(icon)} alt={icon} className={`${className} object-contain`} />;
  }
  return <span className={`flex items-center justify-center leading-none ${className}`}>{icon}</span>;
}
