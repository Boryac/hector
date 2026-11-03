'use client';

import React, { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react';
import { Icon } from './Icon';

type Tone = 'default' | 'ok' | 'warn' | 'err';

interface ToastItem {
  id: number;
  message: string;
  tone: Tone;
}

interface ToastApi {
  push: (message: string, tone?: Tone) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const counter = useRef(0);

  const remove = useCallback((id: number) => {
    setItems((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const push = useCallback(
    (message: string, tone: Tone = 'default') => {
      const id = ++counter.current;
      setItems((prev) => [...prev.slice(-3), { id, message, tone }]);
      window.setTimeout(() => remove(id), 4200);
    },
    [remove],
  );

  const api = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div className="toast-region" aria-live="polite" aria-atomic="false">
        {items.map((item) => (
          <div key={item.id} className={`toast toast-${item.tone}`} role="status">
            <span className="toast-mark" aria-hidden="true" />
            <span className="toast-text">{item.message}</span>
            <button type="button" className="toast-close" onClick={() => remove(item.id)} aria-label="关闭提示">
              <Icon name="x" size={13} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) return { push: () => undefined };
  return ctx;
}