export const defaultFlag = '🇺🇳';

export function getFlagImageURL(flag: string) {
  const codepoints = Array.from(flag || defaultFlag)
    .map(char => char.codePointAt(0)?.toString(16))
    .filter(Boolean)
    .join('-');
  return `https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.2/assets/svg/${codepoints}.svg`;
}
