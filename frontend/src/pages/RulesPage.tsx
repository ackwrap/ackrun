import React from 'react';
import { Cloud, Link2 } from 'lucide-react';

import { PageHeader } from '@/components/layout/PageHeader';
import { ConfirmDialog } from '@/components/ui/ConfirmDialog';
import { SyncScheduleControls } from '@/components/ui/SyncScheduleControls';
import { Toast } from '@/components/ui/Toast';
import { api } from '@/services/api';
import type { GeoAsset, RouteRule, RouteRulePreviewResponse, RouteRuleRequest, RouteRuleSubscription, RouteRuleSubscriptionRequest } from '@/services/types';
import { GeoDatabaseSection } from './rules/GeoDatabaseSection';
import { RuleListSection } from './rules/RuleListSection';
import { RuleSubscriptionActionsModal } from './rules/RuleSubscriptionActionsModal';
import { RouteRuleFormModal } from './rules/RouteRuleFormModal';

const ruleTypes = [
  { value: 'domain', label: '完整域名', hint: 'domain' },
  { value: 'domain_suffix', label: '域名后缀', hint: 'domain_suffix' },
  { value: 'domain_keyword', label: '域名关键词', hint: 'domain_keyword' },
  { value: 'ip_cidr', label: 'IP CIDR', hint: 'ip_cidr' },
  { value: 'geoip', label: 'GeoIP', hint: 'geoip' },
  { value: 'geosite', label: 'GeoSite', hint: 'geosite' },
  { value: 'rule_set', label: '规则集', hint: 'rule_set' },
  { value: 'mixed', label: '混合规则', hint: 'mixed' },
];

const ruleSetFormats = [
  { value: 'auto', label: '自动识别' },
  { value: 'binary', label: 'Binary (.srs)' },
  { value: 'source', label: 'Source JSON' },
  { value: 'clash', label: 'Clash YAML' },
];

const syncModes = [
  { value: 'off', label: '关闭' },
  { value: 'daily', label: '每天' },
  { value: 'weekly', label: '每周' },
  { value: 'monthly', label: '每月' },
];

const weekdays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'];

function syncScheduleLabel(mode: string, time: string, weekday: number) {
  const label = syncModes.find(item => item.value === mode)?.label || mode;
  if (mode === 'off') return label;
  if (mode === 'weekly') return `${label} ${time} ${weekdays[weekday] || ''}`;
  if (mode === 'monthly') return `${label} ${time} ${weekday || 1}号`;
  return `${label} ${time}`;
}
const geoIPCodes = new Set('private ad ae af ag ai al am ao aq ar as at au aw ax az ba bb bd be bf bg bh bi bj bl bm bn bo bq br bs bt bv bw by bz ca cc cd cf cg ch ci ck cl cm cn co cr cu cv cw cx cy cz de dj dk dm do dz ec ee eg eh er es et eu fi fj fk fm fo fr ga gb gd ge gf gg gh gi gl gm gn gp gq gr gs gt gu gw gy hk hm hn hr ht hu id ie il im in io iq ir is it je jm jo jp ke kg kh ki km kn kp kr kw ky kz la lb lc li lk lr ls lt lu lv ly ma mc md me mf mg mh mk ml mm mn mo mp mq mr ms mt mu mv mw mx my mz na nc ne nf ng ni nl no np nr nu nz om pa pe pf pg ph pk pl pm pn pr ps pt pw py qa re ro rs ru rw sa sb sc sd se sg sh si sj sk sl sm sn so sr ss st sv sx sy sz tc td tf tg th tj tk tl tm tn to tr tt tv tw tz ua ug um us uy uz va vc ve vg vi vn vu wf ws ye yt za zm zw'.split(' '));

function formatTime(value: number) {
  return value > 0 ? new Date(value).toLocaleString() : '--';
}

function ruleTypeLabel(value: string) {
  return ruleTypes.find(item => item.value === value)?.label || value;
}

function outboundClass(value: string) {
  if (value === 'direct') return 'border-emerald-400/25 bg-emerald-500/10 text-emerald-300';
  if (value === 'block') return 'border-red-400/25 bg-red-500/10 text-red-300';
  if (value === 'proxy') return 'border-violet-400/25 bg-violet-500/10 text-violet-200';
  return 'border-slate-400/25 bg-slate-500/10 text-slate-200';
}

function ruleSetFormatLabel(value: string) {
  return ruleSetFormats.find(item => item.value === value)?.label || value;
}

function syncStatusLabel(value: string) {
  if (value === 'syncing') return '更新中';
  if (value === 'updated') return '已更新';
  if (value === 'failed') return '失败';
  return '待更新';
}

function syncStatusClass(value: string) {
  if (value === 'syncing') return 'bg-blue-500/10 text-blue-200';
  if (value === 'failed') return 'bg-red-500/10 text-red-300';
  if (value === 'updated') return 'bg-emerald-500/10 text-emerald-300';
  return 'bg-yellow-500/10 text-yellow-300';
}

function generatedGeoRuleSetTag(ruleType: string, value: string) {
  const normalized = value.trim().toLowerCase();
  if (!normalized) return '';
  return normalized.startsWith(`${ruleType}-`) ? normalized : `${ruleType}-${normalized}`;
}

function previewDraft(ruleType: string, values: string[], outbound: string, invert: boolean) {
  const draftValues = values.length > 0 ? values : [ruleType === 'geoip' ? 'cn' : ruleType === 'geosite' ? 'youtube' : 'example.com'];
  if (ruleType === 'mixed') {
    const mixedValues = values.length > 0 ? values : ['geosite:youtube', 'domain_suffix:youtube.com', 'domain:youtubei.googleapis.com'];
    const rules: Array<Record<string, unknown>> = [];
    const groupIndex = new Map<string, number>();
    const appendGrouped = (key: string, value: string) => {
      const index = groupIndex.get(key);
      if (index !== undefined) {
        (rules[index][key] as string[]).push(value);
        return;
      }
      const rule = { [key]: [value], outbound, ...(invert ? { invert: true } : {}) };
      groupIndex.set(key, rules.length);
      rules.push(rule);
    };
    mixedValues.forEach(line => {
      const separator = line.search(/[:=]/);
      const type = separator > 0 ? line.slice(0, separator).trim() : 'domain_suffix';
      const value = separator > 0 ? line.slice(separator + 1).trim() : line.trim();
      if (!value) return;
      if (type === 'geoip' || type === 'geosite') appendGrouped('rule_set', generatedGeoRuleSetTag(type, value));
      else if (type === 'rule_set') appendGrouped('rule_set', value);
      else appendGrouped(type, value);
    });
    return JSON.stringify(rules, null, 2);
  }
  if (ruleType === 'geoip' || ruleType === 'geosite') {
    return JSON.stringify([{ rule_set: draftValues.map(value => generatedGeoRuleSetTag(ruleType, value)).filter(Boolean), outbound, ...(invert ? { invert: true } : {}) }], null, 2);
  }
  return JSON.stringify([{ [ruleType]: draftValues, outbound, ...(invert ? { invert: true } : {}) }], null, 2);
}

function ruleValuePlaceholder(ruleType: string) {
  if (ruleType === 'rule_set') return 'geosite-cn\ngeoip-cn';
  if (ruleType === 'mixed') return 'geosite:youtube\ndomain_suffix:youtube.com\ndomain:youtubei.googleapis.com\ndomain_keyword:ytimg\nrule_set:geosite-google';
  if (ruleType === 'geosite') return 'youtube\ngoogle\nnetflix';
  if (ruleType === 'geoip') return 'cn\nprivate\nus';
  return 'google.com\ntelegram.org\ngithub.com';
}

function ruleValueHelp(ruleType: string) {
  if (ruleType === 'geosite') return '填写 geosite 分类名，例如 youtube；生成配置时会自动转成 rule_set geosite-youtube 并补 .srs 规则集。';
  if (ruleType === 'geoip') return '填写 geoip 区域名，例如 cn；生成配置时会自动转成 rule_set geoip-cn 并补 .srs 规则集。';
  if (ruleType === 'rule_set') return '填写已有规则集 tag，适合高级自定义 .srs/source/clash 规则订阅。';
  if (ruleType === 'mixed') return '一行一个条件，格式为 类型:值；生成配置时会展开为多条相邻规则，适合 geosite + 手动域名补漏。';
  return '一行一个匹配值，保存前会自动去重和去空行。';
}

function outboundOptionLabel(value: string, strategyName: string) {
  if (value === 'direct') return '直连（不走代理）';
  if (value === 'block') return '阻断（禁止访问）';
  if (value === 'proxy') return '策略（由策略组决定）';
  return `策略：${strategyName}`;
}

function isIPv4(value: string) {
  const parts = value.split('.');
  return parts.length === 4 && parts.every(part => /^\d+$/.test(part) && Number(part) >= 0 && Number(part) <= 255);
}

function isIPv6(value: string) {
  return value.includes(':') && /^[0-9a-f:]+$/i.test(value);
}

function isValidIPOrCIDR(value: string) {
  const [addr, prefix] = value.split('/');
  if (!addr || value.split('/').length > 2) return false;
  const isV4 = isIPv4(addr);
  const isV6 = isIPv6(addr);
  if (!isV4 && !isV6) return false;
  if (prefix === undefined) return true;
  if (!/^\d+$/.test(prefix)) return false;
  const bits = Number(prefix);
  return bits >= 0 && bits <= (isV4 ? 32 : 128);
}

function isValidGeoIPCode(value: string) {
  const code = value.trim().toLowerCase();
  return /^[a-z0-9_-]+$/i.test(code) && !/[\/.@:]/.test(code) && geoIPCodes.has(code);
}

function parseMixedValue(value: string) {
  const separator = value.search(/[:=]/);
  if (separator <= 0 || separator >= value.length - 1) return null;
  return { ruleType: value.slice(0, separator).trim(), value: value.slice(separator + 1).trim() };
}

function validateRuleValues(ruleType: string, values: string[]) {
  if (values.length === 0) return '请填写匹配值';
  if (ruleType === 'ip_cidr') {
    const invalid = values.find(value => !isValidIPOrCIDR(value));
    return invalid ? `IP CIDR 格式无效：${invalid}` : '';
  }
  if (ruleType === 'geoip') {
    const invalid = values.find(value => !isValidGeoIPCode(value));
    return invalid ? `GeoIP 应填写区域代码，例如 cn/us/private，不要填写 IP 或域名：${invalid}` : '';
  }
  if (ruleType === 'mixed') {
    for (const raw of values) {
      const parsed = parseMixedValue(raw);
      if (!parsed) return `混合规则格式应为 类型:值：${raw}`;
      if (parsed.ruleType === 'ip_cidr' && !isValidIPOrCIDR(parsed.value)) return `IP CIDR 格式无效：${parsed.value}`;
      if (parsed.ruleType === 'geoip' && !isValidGeoIPCode(parsed.value)) return `GeoIP 应填写区域代码，例如 cn/us/private，不要填写 IP 或域名：${parsed.value}`;
    }
  }
  return '';
}

export function RulesPage() {
  const [rules, setRules] = React.useState<RouteRule[]>([]);
  const [subscriptions, setSubscriptions] = React.useState<RouteRuleSubscription[]>([]);
  const [geoAssets, setGeoAssets] = React.useState<GeoAsset[]>([]);
  const [message, setMessage] = React.useState('');
  const [messageType, setMessageType] = React.useState<'success' | 'error'>('error');
  const [rulePendingDelete, setRulePendingDelete] = React.useState<RouteRule | null>(null);
  const [geoMessage, setGeoMessage] = React.useState('');
  const [geoMessageType, setGeoMessageType] = React.useState<'success' | 'error'>('success');
  const [geoSyncing, setGeoSyncing] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [editing, setEditing] = React.useState<RouteRule | null>(null);
  const [ruleFormOpen, setRuleFormOpen] = React.useState(false);
  const [editingSubscription, setEditingSubscription] = React.useState<RouteRuleSubscription | null>(null);
  const [preview, setPreview] = React.useState<RouteRulePreviewResponse | null>(null);
  const [subscriptionPreview, setSubscriptionPreview] = React.useState<{ title: string; content: string } | null>(null);
  const [subscriptionActions, setSubscriptionActions] = React.useState<RouteRuleSubscription | null>(null);
  const [name, setName] = React.useState('');
  const [enabled, setEnabled] = React.useState(true);
  const [ruleType, setRuleType] = React.useState('domain_suffix');
  const [valuesText, setValuesText] = React.useState('');
  const [outbound, setOutbound] = React.useState('direct');
  const [invert, setInvert] = React.useState(false);
  const [subscriptionName, setSubscriptionName] = React.useState('');
  const [subscriptionEnabled, setSubscriptionEnabled] = React.useState(true);
  const [subscriptionTag, setSubscriptionTag] = React.useState('');
  const [subscriptionURL, setSubscriptionURL] = React.useState('');
  const [subscriptionFormat, setSubscriptionFormat] = React.useState('auto');
  const [subscriptionUseProxy, setSubscriptionUseProxy] = React.useState(false);
  const [subscriptionSyncMode, setSubscriptionSyncMode] = React.useState('daily');
  const [subscriptionSyncTime, setSubscriptionSyncTime] = React.useState('04:00:00');
  const [subscriptionSyncWeekday, setSubscriptionSyncWeekday] = React.useState(0);
  const [generateReferenceRule, setGenerateReferenceRule] = React.useState(true);
  const [referenceOutbound, setReferenceOutbound] = React.useState('direct');
  const toastTimerRef = React.useRef<number | null>(null);
  const isGeoSyncing = geoSyncing || geoAssets.some(item => item.sync_status === 'syncing');

  const outboundOptions = React.useMemo(() => [
    { value: 'direct', label: outboundOptionLabel('direct', '') },
    { value: 'block', label: outboundOptionLabel('block', '') },
    { value: 'proxy', label: outboundOptionLabel('proxy', '') }
  ], []);

  const outboundLabel = React.useCallback((value: string) => {
    return outboundOptions.find(item => item.value === value)?.label || value;
  }, [outboundOptions]);

  const showMessage = (msg: string, type: 'success' | 'error' = 'error') => {
    setMessage(msg);
    setMessageType(type);
    if (toastTimerRef.current) window.clearTimeout(toastTimerRef.current);
    toastTimerRef.current = window.setTimeout(() => {
      setMessage('');
      toastTimerRef.current = null;
    }, type === 'success' ? 2600 : 4200);
  };

  React.useEffect(() => () => {
    if (toastTimerRef.current) window.clearTimeout(toastTimerRef.current);
  }, []);

  const load = React.useCallback(async () => {
    try {
      setLoading(true);
      const [ruleItems, subscriptionItems, geoItems] = await Promise.all([
        api.getRouteRules(),
        api.getRouteRuleSubscriptions(),
        api.getGeoAssets()
      ]);
      setRules(ruleItems);
      setSubscriptions(subscriptionItems);
      setGeoAssets(geoItems);
    } catch (e: any) {
      showMessage(`规则加载失败: ${e.message}`, 'error');
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { load(); }, [load]);
  React.useEffect(() => {
    if (!isGeoSyncing) return;
    const timer = window.setInterval(() => {
      load();
    }, 2000);
    return () => window.clearInterval(timer);
  }, [isGeoSyncing, load]);

  const resetForm = () => {
    setEditing(null);
    setName('');
    setEnabled(true);
    setRuleType('domain_suffix');
    setValuesText('');
    setOutbound('direct');
    setInvert(false);
  };

  const closeRuleForm = () => {
    setRuleFormOpen(false);
    resetForm();
  };

  const addRule = () => {
    resetForm();
    setRuleFormOpen(true);
  };

  const addGeoRule = () => {
    resetForm();
    setRuleType('geosite');
    setName('GeoSite 规则');
    setOutbound('proxy');
    setRuleFormOpen(true);
  };

  const editRule = (rule: RouteRule) => {
    setEditing(rule);
    setName(rule.name);
    setEnabled(rule.enabled);
    setRuleType(rule.rule_type);
    setValuesText(rule.values.join('\n'));
    setOutbound(rule.outbound);
    setInvert(rule.invert);
    setRuleFormOpen(true);
  };

  const resetSubscriptionForm = () => {
    setEditingSubscription(null);
    setSubscriptionName('');
    setSubscriptionEnabled(true);
    setSubscriptionTag('');
    setSubscriptionURL('');
    setSubscriptionFormat('auto');
    setSubscriptionUseProxy(false);
    setSubscriptionSyncMode('daily');
    setSubscriptionSyncTime('04:00:00');
    setSubscriptionSyncWeekday(0);
    setGenerateReferenceRule(true);
    setReferenceOutbound('proxy');
  };

  const editSubscription = (item: RouteRuleSubscription) => {
    setSubscriptionActions(null);
    setEditingSubscription(item);
    setSubscriptionName(item.name);
    setSubscriptionEnabled(item.enabled);
    setSubscriptionTag(item.tag);
    setSubscriptionURL(item.url);
    setSubscriptionFormat(item.format);
    setSubscriptionUseProxy(item.use_proxy);
    setSubscriptionSyncMode(item.sync_mode || 'daily');
    setSubscriptionSyncTime(item.sync_time || '04:00:00');
    setSubscriptionSyncWeekday(item.sync_weekday || 0);
  };

  const values = React.useMemo(() => Array.from(new Set(valuesText.split('\n').map(item => item.trim()).filter(Boolean))), [valuesText]);
  const buildRequest = (): RouteRuleRequest => ({
    name,
    enabled,
    priority: editing?.priority || 0,
    rule_type: ruleType,
    values,
    outbound,
    invert,
  });

  const buildSubscriptionRequest = (): RouteRuleSubscriptionRequest => ({
    name: subscriptionName,
    enabled: subscriptionEnabled,
    tag: subscriptionTag,
    url: subscriptionURL,
    format: subscriptionFormat,
    use_proxy: subscriptionUseProxy,
    sync_mode: subscriptionSyncMode,
    sync_time: subscriptionSyncTime,
    sync_weekday: subscriptionSyncWeekday,
  });

  const saveRule = async () => {
    try {
      const validationError = validateRuleValues(ruleType, values);
      if (validationError) {
        showMessage(validationError, 'error');
        return;
      }
      const payload = buildRequest();
      if (editing) {
        await api.updateRouteRule(editing.id, payload);
        setMessage('规则已更新');
      showMessage('规则已更新', 'success');
      } else {
        await api.createRouteRule(payload);
        setMessage('规则已添加');
      }
      resetForm();
      setRuleFormOpen(false);
      await load();
    } catch (e: any) {
      showMessage(`规则保存失败: ${e.message}`, 'error');
    }
  };

  const saveSubscription = async () => {
    try {
      const payload = buildSubscriptionRequest();
      if (editingSubscription) {
        await api.updateRouteRuleSubscription(editingSubscription.id, payload);
        setMessage('规则订阅已更新');
      } else {
        const created = await api.createRouteRuleSubscription(payload);
        if (generateReferenceRule) {
          await api.createRouteRule({ name: created.name, enabled: created.enabled, priority: 0, rule_type: 'rule_set', values: [created.tag], outbound: referenceOutbound, invert: false });
          showMessage(`规则订阅已添加，并已生成${outboundLabel(referenceOutbound)}引用规则`, 'success');
        } else {
          showMessage('规则订阅已添加', 'success');
        }
      }
      resetSubscriptionForm();
      await load();
    } catch (e: any) {
      showMessage(`规则订阅保存失败: ${e.message}`, 'error');
    }
  };

  const confirmRemoveRule = async () => {
    if (!rulePendingDelete) return;
    try {
      await api.deleteRouteRule(rulePendingDelete.id);
      showMessage('规则已删除', 'success');
      if (editing?.id === rulePendingDelete.id) resetForm();
      setRulePendingDelete(null);
      await load();
    } catch (e: any) {
      showMessage(`规则删除失败: ${e.message}`, 'error');
    }
  };

  const removeRule = (rule: RouteRule) => {
    setRulePendingDelete(rule);
  };

  const removeSubscription = async (item: RouteRuleSubscription) => {
    try {
      await api.deleteRouteRuleSubscription(item.id);
      setMessage('规则订阅已删除');
      if (editingSubscription?.id === item.id) resetSubscriptionForm();
      if (subscriptionActions?.id === item.id) setSubscriptionActions(null);
      await load();
    } catch (e: any) {
      showMessage(`规则订阅删除失败: ${e.message}`, 'error');
    }
  };

  const toggleRule = async (rule: RouteRule) => {
    try {
      await api.updateRouteRule(rule.id, { ...rule, enabled: !rule.enabled });
      await load();
    } catch (e: any) {
      showMessage(`规则状态更新失败: ${e.message}`, 'error');
    }
  };

  const toggleSubscription = async (item: RouteRuleSubscription) => {
    try {
      await api.updateRouteRuleSubscription(item.id, { ...item, enabled: !item.enabled });
      await load();
    } catch (e: any) {
      showMessage(`规则订阅状态更新失败: ${e.message}`, 'error');
    }
  };

  const syncSubscription = async (item: RouteRuleSubscription) => {
    try {
      await api.syncRouteRuleSubscription(item.id);
      showMessage(`规则订阅 ${item.name} 已开始更新`, 'success');
      await load();
    } catch (e: any) {
      showMessage(`规则订阅更新失败: ${e.message}`, 'error');
    }
  };

  const syncAllSubscriptions = async () => {
    try {
      await api.syncAllRouteRuleSubscriptions();
      showMessage('规则订阅已开始全部更新', 'success');
      await load();
    } catch (e: any) {
      showMessage(`规则订阅批量更新失败: ${e.message}`, 'error');
    }
  };

  const syncGeoAsset = async (item: GeoAsset) => {
    setGeoSyncing(true);
    try {
      await api.syncGeoAsset(item.id);
      setGeoMessage(`${item.name} 已开始更新`);
      setGeoMessageType('success');
      await load();
    } catch (e: any) {
      setGeoMessage(`Geo 数据库更新失败: ${e.message}`);
      setGeoMessageType('error');
    } finally {
      setGeoSyncing(false);
    }
  };

  const syncAllGeoAssets = async () => {
    setGeoSyncing(true);
    try {
      await api.syncAllGeoAssets();
      setGeoMessage('Geo 数据库已开始全部更新');
      setGeoMessageType('success');
      await load();
    } catch (e: any) {
      setGeoMessage(`Geo 数据库批量更新失败: ${e.message}`);
      setGeoMessageType('error');
    } finally {
      setGeoSyncing(false);
    }
  };

  const updateGeoAsset = async (item: GeoAsset, body: Parameters<typeof api.updateGeoAsset>[1]) => {
    try {
      await api.updateGeoAsset(item.id, body);
      setGeoMessage(`${item.name} 自动更新设置已保存`);
      setGeoMessageType('success');
      await load();
    } catch (e: any) {
      setGeoMessage(`Geo 自动更新设置保存失败: ${e.message}`);
      setGeoMessageType('error');
    }
  };

  const appendRuleSetTag = (tag: string) => {
    setRuleType('rule_set');
    const current = valuesText.split('\n').map(item => item.trim()).filter(Boolean);
    if (!current.includes(tag)) current.push(tag);
    setValuesText(current.join('\n'));
  };

  const createRuleFromSubscription = async (item: RouteRuleSubscription, outboundValue: string) => {
    try {
      const exists = rules.some(rule => rule.rule_type === 'rule_set' && rule.outbound === outboundValue && rule.values.includes(item.tag));
      if (exists) {
        setMessage(`已存在 ${item.tag} 的${outboundLabel(outboundValue)}引用规则`);
        return;
      }
      await api.createRouteRule({ name: item.name, enabled: item.enabled, priority: 0, rule_type: 'rule_set', values: [item.tag], outbound: outboundValue, invert: false });
      showMessage(`已生成 ${item.tag} 的${outboundLabel(outboundValue)}引用规则`, 'success');
      await load();
    } catch (e: any) {
      showMessage(`引用规则创建失败: ${e.message}`, 'error');
    }
  };

  const moveRule = async (index: number, direction: -1 | 1) => {
    const nextIndex = index + direction;
    if (nextIndex < 0 || nextIndex >= rules.length) return;
    const next = [...rules];
    [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
    setRules(next);
    try {
      await api.reorderRouteRules(next.map(item => item.id));
      await load();
    } catch (e: any) {
      showMessage(`规则排序失败: ${e.message}`, 'error');
      await load();
    }
  };

  const showPreview = async () => {
    try {
      setPreview(await api.previewRouteRules());
    } catch (e: any) {
      showMessage(`规则预览失败: ${e.message}`, 'error');
    }
  };

  const showSubscriptionPreview = async (item: RouteRuleSubscription) => {
    try {
      const content = await api.getRouteRuleSubscriptionContent(item.id);
      setSubscriptionPreview({ title: `${item.name} 转换结果`, content: JSON.stringify(content, null, 2) });
    } catch (e: any) {
      showMessage(`规则订阅预览失败: ${e.message}`, 'error');
    }
  };

  return (
    <div className="space-y-4">
      <PageHeader title="规则管理" />

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <div className="text-center">
            <div className="mx-auto h-10 w-10 animate-spin rounded-full border-4 border-blue-500/20 border-t-blue-500"></div>
            <div className="mt-4 text-sm text-[var(--text-secondary)]">加载中...</div>
          </div>
        </div>
      ) : (
        <>
      <GeoDatabaseSection
        geoAssets={geoAssets}
        syncModes={syncModes}
        weekdays={weekdays}
        onSyncAll={syncAllGeoAssets}
        onSyncOne={syncGeoAsset}
        onUpdate={updateGeoAsset}
        syncing={isGeoSyncing}
        message={geoMessage}
        messageType={geoMessageType}
        formatTime={formatTime}
        syncStatusLabel={syncStatusLabel}
        syncStatusClass={syncStatusClass}
      />

      <RuleListSection
        rules={rules}
        subscriptions={subscriptions}
        onRefresh={load}
        onAddGeo={addGeoRule}
        onAdd={addRule}
        onPreview={showPreview}
        onMove={moveRule}
        onToggle={toggleRule}
        onEdit={editRule}
        onRemove={removeRule}
        formatTime={formatTime}
        ruleTypeLabel={ruleTypeLabel}
        outboundLabel={outboundLabel}
        outboundClass={outboundClass}
      />

      <section className="rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.9),rgba(16,27,43,0.72))] p-5 shadow-[var(--shadow-card)]">
        <div className="mb-4 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-cyan-400/20 bg-cyan-500/10 text-cyan-200"><Cloud size={18} /></div>
            <div>
              <h3 className="text-sm font-semibold text-white">规则订阅</h3>
              <p className="mt-1 text-xs text-[var(--text-tertiary)]">高级自定义规则集入口；常见 GeoIP/GeoSite 分类请优先在上方路由规则里直接填写分类名。</p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <div className="text-xs text-[var(--text-tertiary)]">启用后会出现在预览的 <span className="font-mono text-cyan-100">route.rule_set</span> 中。</div>
            <button onClick={syncAllSubscriptions} className="h-8 rounded-md border border-cyan-400/25 bg-cyan-500/10 px-3 text-xs text-cyan-100 hover:bg-cyan-500/20">同步全部</button>
          </div>
        </div>

        <div className="grid gap-4 xl:grid-cols-[3fr_2fr]">
          <div className="rounded-xl border border-[var(--border-default)] bg-white/[0.03] p-4">
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <h4 className="text-sm font-semibold text-white">{editingSubscription ? '编辑订阅' : '新增订阅'}</h4>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">格式选自动时，.yml/.yaml 会按 Clash YAML 规则订阅处理。</p>
              </div>
              {editingSubscription && <button onClick={resetSubscriptionForm} className="rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-1.5 text-xs text-[var(--text-secondary)] hover:text-white">取消编辑</button>}
            </div>
            <div className="grid gap-3">
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">订阅名称</span>
                <input value={subscriptionName} onChange={e => setSubscriptionName(e.target.value)} placeholder="例如：GeoSite CN" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-cyan-400" />
              </label>
              <label className="block">
                <span className="text-xs text-[var(--text-tertiary)]">规则下载地址</span>
                <input value={subscriptionURL} onChange={e => setSubscriptionURL(e.target.value)} placeholder="https://example.com/geosite-cn.srs" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-white outline-none focus:border-cyan-400" />
              </label>
              <div className="grid gap-3 sm:grid-cols-2">
                <label className="block">
                  <span className="text-xs text-[var(--text-tertiary)]">规则集 tag</span>
                  <input value={subscriptionTag} onChange={e => setSubscriptionTag(e.target.value)} placeholder="geosite-cn" className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 font-mono text-sm text-white outline-none focus:border-cyan-400" />
                </label>
                <label className="block">
                  <span className="text-xs text-[var(--text-tertiary)]">格式</span>
                  <select value={subscriptionFormat} onChange={e => setSubscriptionFormat(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-cyan-400">
                    {ruleSetFormats.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
                  </select>
                </label>
              </div>
              <SyncScheduleControls
                value={{ sync_mode: subscriptionSyncMode, sync_time: subscriptionSyncTime, sync_weekday: subscriptionSyncWeekday }}
                syncModes={syncModes}
                weekdays={weekdays}
                onChange={patch => {
                  if (patch.sync_mode !== undefined) setSubscriptionSyncMode(patch.sync_mode);
                  if (patch.sync_time !== undefined) setSubscriptionSyncTime(patch.sync_time);
                  if (patch.sync_weekday !== undefined) setSubscriptionSyncWeekday(patch.sync_weekday);
                }}
              />
              {!editingSubscription && (
                <div className="rounded-lg border border-cyan-400/15 bg-cyan-500/[0.06] p-3">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
                    <label className="inline-flex items-center gap-2 text-sm text-cyan-100"><input type="checkbox" checked={generateReferenceRule} onChange={e => setGenerateReferenceRule(e.target.checked)} />同时生成引用规则</label>
                    <label className="block min-w-[180px]">
                      <span className="text-xs text-[var(--text-tertiary)]">引用规则出站</span>
                      <select value={referenceOutbound} disabled={!generateReferenceRule} onChange={e => setReferenceOutbound(e.target.value)} className="mt-1 w-full rounded-md border border-[var(--border-default)] bg-[#152235] px-3 py-2 text-sm text-white outline-none focus:border-cyan-400 disabled:opacity-50">
                        {outboundOptions.map(item => <option key={item.value} className="bg-[#152235] text-white" value={item.value}>{item.label}</option>)}
                      </select>
                    </label>
                  </div>
                  <p className="mt-2 text-xs text-[var(--text-tertiary)]">规则订阅只是 rule_set，必须有一条 route.rules 引用 tag 才会真正生效。</p>
                </div>
              )}
              <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                <div className="flex flex-wrap gap-2">
                  <label className="inline-flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-[var(--text-secondary)]"><input type="checkbox" checked={subscriptionEnabled} onChange={e => setSubscriptionEnabled(e.target.checked)} />启用</label>
                  <label className="inline-flex items-center gap-2 rounded-md border border-[var(--border-default)] bg-white/[0.04] px-3 py-2 text-sm text-[var(--text-secondary)]"><input type="checkbox" checked={subscriptionUseProxy} onChange={e => setSubscriptionUseProxy(e.target.checked)} />下载走代理</label>
                </div>
                <button onClick={saveSubscription} className="inline-flex h-9 items-center gap-2 rounded-md bg-cyan-600 px-4 text-sm font-medium text-white hover:bg-cyan-500"><Link2 size={15} />{editingSubscription ? '更新订阅' : '添加订阅'}</button>
              </div>
            </div>
          </div>

          <div className="overflow-hidden rounded-xl border border-[var(--border-default)]">
            {subscriptions.length === 0 ? (
              <div className="px-4 py-14 text-center">
                <div className="mx-auto mb-3 flex h-11 w-11 items-center justify-center rounded-xl border border-cyan-400/20 bg-cyan-500/10 text-cyan-200"><Cloud size={18} /></div>
                <div className="text-sm font-medium text-white">暂无规则订阅</div>
                <div className="mt-1 text-xs text-[var(--text-tertiary)]">添加远程规则集后可在手动规则里用 tag 引用。</div>
              </div>
            ) : (
              <div>
                {rules.length === 0 && <div className="border-b border-[var(--border-default)] bg-yellow-500/[0.06] px-4 py-3 text-xs text-yellow-100">当前只有规则订阅，还没有 route.rules 引用它。请点击下方“代理 / 直连 / 阻断”生成引用规则。</div>}
                <div className="divide-y divide-[var(--border-default)]">
                {subscriptions.map(item => (
                  <div key={item.id} className="grid gap-3 bg-white/[0.025] p-4 xl:grid-cols-[minmax(0,1fr)_auto] xl:items-center">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-white">{item.name}</span>
                        <span className="rounded border border-cyan-400/25 bg-cyan-500/10 px-2 py-0.5 font-mono text-xs text-cyan-100">{item.tag}</span>
                        <span className={`rounded px-2 py-0.5 text-xs ${item.enabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}`}>{item.enabled ? '启用' : '停用'}</span>
                        <span className="rounded bg-white/[0.05] px-2 py-0.5 text-xs text-[var(--text-secondary)]">{ruleSetFormatLabel(item.format)}</span>
                        <span className={`rounded px-2 py-0.5 text-xs ${item.use_proxy ? 'bg-blue-500/10 text-blue-200' : 'bg-emerald-500/10 text-emerald-300'}`}>{item.use_proxy ? '代理下载' : '直连下载'}</span>
                        <span className={`rounded px-2 py-0.5 text-xs ${syncStatusClass(item.sync_status)}`}>{syncStatusLabel(item.sync_status)}</span>
                      </div>
                      <div className="mt-2 truncate font-mono text-xs text-[var(--text-tertiary)]" title={item.url}>{item.url}</div>
                      <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[var(--text-tertiary)]">
                        <span>自动更新：{syncScheduleLabel(item.sync_mode, item.sync_time, item.sync_weekday)}</span>
                        <span>最后同步：{formatTime(item.last_sync_at)}</span>
                        <span>缓存：{formatTime(item.cached_updated_at)}</span>
                      </div>
                      {item.sync_error && <div className="mt-2 rounded border border-red-400/20 bg-red-500/10 px-2 py-1 text-xs text-red-300">{item.sync_error}</div>}
                    </div>
                    <div className="flex justify-end">
                      <button onClick={() => setSubscriptionActions(item)} className="rounded-md border border-cyan-400/25 bg-cyan-500/10 px-4 py-1.5 text-xs font-medium text-cyan-100 hover:bg-cyan-500/20">管理</button>
                    </div>
                  </div>
                ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </section>

      {ruleFormOpen && (
        <RouteRuleFormModal
          editing={editing}
          name={name}
          enabled={enabled}
          ruleType={ruleType}
          valuesText={valuesText}
          outbound={outbound}
          invert={invert}
          values={values}
          subscriptions={subscriptions}
          outboundOptions={outboundOptions}
          onNameChange={setName}
          onEnabledChange={setEnabled}
          onRuleTypeChange={setRuleType}
          onValuesTextChange={setValuesText}
          onOutboundChange={setOutbound}
          onInvertChange={setInvert}
          onAppendRuleSetTag={appendRuleSetTag}
          onClose={closeRuleForm}
          onSave={saveRule}
          ruleTypes={ruleTypes}
          ruleTypeLabel={ruleTypeLabel}
          outboundLabel={outboundLabel}
          outboundClass={outboundClass}
          previewDraft={previewDraft}
          ruleValueHelp={ruleValueHelp}
          ruleValuePlaceholder={ruleValuePlaceholder}
        />
      )}

      {subscriptionActions && (
        <RuleSubscriptionActionsModal
          item={subscriptionActions}
          ruleSetFormats={ruleSetFormats}
          syncModes={syncModes}
          weekdays={weekdays}
          onChange={setSubscriptionActions}
          onClose={() => setSubscriptionActions(null)}
          onReload={load}
          onCreateRule={createRuleFromSubscription}
          proxyOutbound="proxy"
          proxyOutboundLabel={outboundLabel('proxy')}
          onPreview={showSubscriptionPreview}
          onSync={syncSubscription}
          onAppendTag={appendRuleSetTag}
          onToggle={toggleSubscription}
          onEdit={editSubscription}
          onRemove={removeSubscription}
          formatTime={formatTime}
          syncStatusLabel={syncStatusLabel}
          syncStatusClass={syncStatusClass}
        />
      )}

      {subscriptionPreview && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
          <div className="max-h-[84vh] w-full max-w-3xl overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(13,24,40,0.98))] shadow-[var(--shadow-card)]">
            <div className="flex items-center justify-between border-b border-[var(--border-default)] px-5 py-4">
              <div>
                <h3 className="text-base font-semibold text-white">{subscriptionPreview.title}</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">这是后端转换后返回给 sing-box 下载的 source JSON。</p>
              </div>
              <button onClick={() => setSubscriptionPreview(null)} className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-tertiary)] transition-colors hover:border-red-400/30 hover:bg-red-500/10 hover:text-red-300" title="关闭"><svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2"><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg></button>
            </div>
            <div className="p-5">
              <pre className="max-h-[58vh] overflow-auto rounded-xl border border-[var(--border-default)] bg-[#07111f] p-4 font-mono text-xs leading-6 text-blue-50">{subscriptionPreview.content}</pre>
            </div>
          </div>
        </div>
      )}

      </>
      )}

      {preview && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
          <div className="max-h-[84vh] w-full max-w-3xl overflow-hidden rounded-[var(--radius-xl)] border border-[var(--border-default)] bg-[linear-gradient(180deg,rgba(20,33,52,0.98),rgba(13,24,40,0.98))] shadow-[var(--shadow-card)]">
            <div className="flex items-center justify-between border-b border-[var(--border-default)] px-5 py-4">
              <div>
                <h3 className="text-base font-semibold text-white">规则 JSON 预览</h3>
                <p className="mt-1 text-xs text-[var(--text-tertiary)]">展示已启用规则转换后的 sing-box route.rule_set 和 route.rules。</p>
              </div>
              <button onClick={() => setPreview(null)} className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-[var(--border-default)] bg-white/[0.04] text-[var(--text-tertiary)] transition-colors hover:border-red-400/30 hover:bg-red-500/10 hover:text-red-300" title="关闭"><svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2"><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg></button>
            </div>
            <div className="p-5">
              <pre className="max-h-[58vh] overflow-auto rounded-xl border border-[var(--border-default)] bg-[#07111f] p-4 font-mono text-xs leading-6 text-blue-50">{JSON.stringify({ rule_set: preview.rule_sets, rules: preview.rules }, null, 2)}</pre>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={!!rulePendingDelete}
        title="删除规则"
        message={rulePendingDelete ? `确定要删除规则「${rulePendingDelete.name}」吗？此操作不可撤销。` : ''}
        confirmText="删除"
        cancelText="取消"
        danger
        onConfirm={confirmRemoveRule}
        onCancel={() => setRulePendingDelete(null)}
      />

      <Toast message={message} type={messageType} />
    </div>
  );
}

export default RulesPage;
