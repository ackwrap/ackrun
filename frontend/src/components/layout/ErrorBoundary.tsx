import { Component, ReactNode, ErrorInfo } from 'react';
import { Button } from '@/components/ui/Button';

interface Props { children: ReactNode; fallback?: ReactNode }
interface State { hasError: boolean; error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) { super(props); this.state = { hasError: false, error: null }; }
  static getDerivedStateFromError(error: Error): State { return { hasError: true, error }; }
  componentDidCatch(error: Error, info: ErrorInfo) { console.error('[ErrorBoundary]', error, info); }
  handleRetry = () => { this.setState({ hasError: false, error: null }); };
  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <div className="flex flex-col items-center justify-center py-20 px-6">
          <div className="text-5xl mb-4">⚠️</div>
          <h2 className="text-xl font-semibold text-[var(--text-primary)] mb-2">页面出了点问题</h2>
          <p className="text-[var(--text-secondary)] mb-6 text-center max-w-md">{this.state.error?.message || '发生了未知错误，请重试。'}</p>
          <Button onClick={this.handleRetry}>重试</Button>
        </div>
      );
    }
    return this.props.children;
  }
}