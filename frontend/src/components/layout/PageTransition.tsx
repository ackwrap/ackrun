import { useState, useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';

export function PageTransition({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const [displayChildren, setDisplayChildren] = useState(children);
  const [animating, setAnimating] = useState(false);
  const isFirstRender = useRef(true);

  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false;
      setDisplayChildren(children);
      return;
    }
    setAnimating(true);
    const timer = setTimeout(() => { setDisplayChildren(children); setAnimating(false); }, 50);
    return () => clearTimeout(timer);
  }, [location.pathname]);

  return <div className={animating ? 'animate-page-enter' : ''} key={location.pathname}>{displayChildren}</div>;
}