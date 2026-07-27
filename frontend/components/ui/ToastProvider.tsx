"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

type ToastVariant = "success" | "error" | "info";

type Toast = {
  id: number;
  title: string;
  description?: string;
  variant: ToastVariant;
};

type ToastInput = Omit<Toast, "id">;

type ToastContextValue = {
  showToast: (toast: ToastInput) => void;
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

const variantStyles: Record<ToastVariant, string> = {
  success: "border-emerald-500/40 bg-emerald-950/95 text-emerald-100",
  error: "border-red-500/40 bg-red-950/95 text-red-100",
  info: "border-sky-500/40 bg-sky-950/95 text-sky-100",
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextID = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id);

    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }

    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const showToast = useCallback(
    (input: ToastInput) => {
      nextID.current += 1;

      const id = Date.now() + nextID.current;

      setToasts((current) => [
        ...current,
        {
          ...input,
          id,
        },
      ]);

      const timer = setTimeout(() => {
        dismiss(id);
      }, 4500);

      timers.current.set(id, timer);
    },
    [dismiss],
  );

  useEffect(() => {
    const currentTimers = timers.current;

    return () => {
      for (const timer of currentTimers.values()) {
        clearTimeout(timer);
      }

      currentTimers.clear();
    };
  }, []);

  const value = useMemo<ToastContextValue>(
    () => ({
      showToast,
      success: (title, description) =>
        showToast({
          title,
          description,
          variant: "success",
        }),
      error: (title, description) =>
        showToast({
          title,
          description,
          variant: "error",
        }),
      info: (title, description) =>
        showToast({
          title,
          description,
          variant: "info",
        }),
    }),
    [showToast],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}

      <div
        aria-atomic="false"
        aria-live="polite"
        className="pointer-events-none fixed right-4 top-4 z-[100] flex w-[calc(100%-2rem)] max-w-sm flex-col gap-3"
      >
        {toasts.map((toast) => (
          <div
            key={toast.id}
            role={toast.variant === "error" ? "alert" : "status"}
            className={`pointer-events-auto rounded-xl border p-4 shadow-2xl backdrop-blur ${variantStyles[toast.variant]}`}
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="font-semibold">{toast.title}</p>

                {toast.description ? (
                  <p className="mt-1 text-sm opacity-90">{toast.description}</p>
                ) : null}
              </div>

              <button
                type="button"
                aria-label="Dismiss notification"
                onClick={() => dismiss(toast.id)}
                className="rounded-md px-2 py-1 text-sm opacity-70 transition hover:bg-white/10 hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-white/70"
              >
                ×
              </button>
            </div>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const value = useContext(ToastContext);

  if (!value) {
    throw new Error("useToast must be used inside ToastProvider");
  }

  return value;
}
