import React, { useMemo } from 'react';

interface JsonPreviewProps {
  data: unknown;
  maxHeight?: string;
  className?: string;
}

const BRACKET_COLORS: Record<string, string> = {
  '{': 'text-amber-300',
  '}': 'text-amber-300',
  '[': 'text-sky-300',
  ']': 'text-sky-300',
};

function tokenize(json: string): React.ReactNode[] {
  const lines = json.split('\n');
  const nodes: React.ReactNode[] = [];

  for (let lineIdx = 0; lineIdx < lines.length; lineIdx++) {
    const line = lines[lineIdx];
    const parts = tokenizeLine(line, lineIdx);
    nodes.push(...parts);
    if (lineIdx < lines.length - 1) {
      nodes.push('\n');
    }
  }
  return nodes;
}

function tokenizeLine(line: string, lineIdx: number): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  let i = 0;
  const indent = line.match(/^(\s*)/)?.[1] || '';

  if (indent) {
    nodes.push(<span key={`i-${lineIdx}`}>{indent}</span>);
    i = indent.length;
  }

  while (i < line.length) {
    const ch = line[i];

    if (ch === '"') {
      const endIdx = line.indexOf('"', i + 1);
      if (endIdx === -1) {
        nodes.push(<span key={`${lineIdx}-${i}`}>{line.slice(i)}</span>);
        break;
      }
      const str = line.slice(i, endIdx + 1);
      const colonPos = line.indexOf(':', endIdx + 1);
      const isKey = colonPos !== -1 && line.slice(endIdx + 1, colonPos).trim() === ':';

      if (isKey) {
        nodes.push(<span key={`${lineIdx}-${i}`} className="text-emerald-300">{str}</span>);
      } else {
        nodes.push(<span key={`${lineIdx}-${i}`} className="text-orange-300">{str}</span>);
      }
      i = endIdx + 1;
    } else if (ch === ':') {
      nodes.push(<span key={`${lineIdx}-${i}`} className="text-gray-500">: </span>);
      i += ch === ':' ? (line[i + 1] === ' ' ? 2 : 1) : 1;
    } else if (ch === '{' || ch === '}' || ch === '[' || ch === ']') {
      const colorClass = BRACKET_COLORS[ch] || 'text-gray-400';
      nodes.push(<span key={`${lineIdx}-${i}`} className={colorClass}>{ch}</span>);
      i++;
    } else if (ch === ',') {
      nodes.push(<span key={`${lineIdx}-${i}`} className="text-gray-500">,</span>);
      i++;
    } else if (ch === 't' || ch === 'f') {
      const boolStr = line.slice(i).startsWith('true') ? 'true' : 'false';
      nodes.push(<span key={`${lineIdx}-${i}`} className="text-purple-300">{boolStr}</span>);
      i += boolStr.length;
    } else if (ch === 'n') {
      nodes.push(<span key={`${lineIdx}-${i}`} className="text-gray-400">null</span>);
      i += 4;
    } else if (/\d/.test(ch) || ch === '-') {
      let numEnd = i + 1;
      while (numEnd < line.length && /[\d.eE+\-]/.test(line[numEnd])) numEnd++;
      nodes.push(<span key={`${lineIdx}-${i}`} className="text-cyan-300">{line.slice(i, numEnd)}</span>);
      i = numEnd;
    } else {
      nodes.push(<span key={`${lineIdx}-${i}`}>{ch}</span>);
      i++;
    }
  }
  return nodes;
}

export function JsonPreview({ data, maxHeight = '70vh', className = '' }: JsonPreviewProps) {
  const content = useMemo(() => {
    try {
      return JSON.stringify(data, null, 2);
    } catch {
      return String(data);
    }
  }, [data]);

  const highlighted = useMemo(() => tokenize(content), [content]);

  return (
    <div className={`h-full overflow-auto rounded-lg border border-[var(--border-default)] bg-black/30 ${className}`}>
      <pre className="p-4 text-xs leading-5 font-mono whitespace-pre" style={{ maxHeight }}>
        {highlighted}
      </pre>
    </div>
  );
}