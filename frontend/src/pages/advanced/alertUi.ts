import type {
  AlertChannelType,
  AlertEventType,
} from "@/services/advancedTypes";

export const alertEventOptions: Array<{
  value: AlertEventType;
  label: string;
}> = [
  { value: "circuit_open", label: "熔断" },
  { value: "recovered", label: "恢复" },
  { value: "subscription_failed", label: "订阅失败" },
];

export function alertEventLabel(value: string) {
  if (value === "test") return "测试发送";
  return alertEventOptions.find((item) => item.value === value)?.label || value;
}

export function alertChannelLabel(value: AlertChannelType | string) {
  return {
    webhook: "Webhook",
    telegram: "Telegram",
    email: "邮件",
  }[value] || value;
}

export function deliveryStatusLabel(value: string) {
  return {
    never: "尚未投递",
    success: "投递成功",
    failed: "投递失败",
  }[value] || value;
}
