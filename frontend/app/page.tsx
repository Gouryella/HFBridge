"use client";

import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import { Snippet } from "@heroui/snippet";
import { Card, CardHeader, CardBody, CardFooter } from "@heroui/card";
import { Input } from "@heroui/input";
import { GithubIcon, HuggingFaceIcon } from "@/components/icons";
import { Link } from "@heroui/link";

type UsageState = {
  totalCount: number;
  totalBytes: number;
  currentBps: number;
  windowSeconds: number;
};

const numberFormatter = new Intl.NumberFormat("zh-Hans");

function formatNumber(value: number): string {
  if (!Number.isFinite(value)) {
    return "0";
  }
  return numberFormatter.format(Math.max(0, Math.floor(value)));
}

type HumanReadable = {
  value: string;
  unit: string;
};

function formatDecimal(value: number): string {
  if (!Number.isFinite(value) || value === 0) {
    return "0";
  }
  if (value >= 100) {
    return value.toFixed(0);
  }
  if (value >= 10) {
    return value.toFixed(1);
  }
  return value.toFixed(2);
}

function formatBytes(bytes: number): HumanReadable {
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let value = Math.max(bytes, 0);
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return {
    value: formatDecimal(value),
    unit: units[unitIndex],
  };
}

function formatRate(bps: number): HumanReadable {
  const units = ["B/s", "KB/s", "MB/s", "GB/s", "TB/s"];
  let value = Math.max(bps, 0);
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return {
    value: formatDecimal(value),
    unit: units[unitIndex],
  };
}

function buildApiUrl(base: string, path: string): string {
  const normalizedBase = base.endsWith("/") ? base.slice(0, -1) : base;
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  return normalizedBase ? `${normalizedBase}${normalizedPath}` : normalizedPath;
}

type MovementOptions = {
  maxOffset?: number;
  minDurationMs?: number;
  maxDurationMs?: number;
  initialDelayMs?: number;
};

function useSlowRandomMovement({
  maxOffset = 120,
  minDurationMs = 6000,
  maxDurationMs = 14000,
  initialDelayMs = 400,
}: MovementOptions = {}): CSSProperties {
  const timeoutRef = useRef<number | null>(null);
  const [style, setStyle] = useState<CSSProperties>(() => ({
    transform: "translate3d(0, 0, 0)",
    transition: "none",
    willChange: "transform",
  }));

  useEffect(() => {
    let active = true;
    const safeOffset = Math.max(0, maxOffset);
    const safeMin = Math.max(3000, Math.floor(minDurationMs));
    const safeMax = Math.max(safeMin + 1000, Math.floor(maxDurationMs));
    const safeInitialDelay = Math.max(0, Math.floor(initialDelayMs));

    const nextOffsets = () => ({
      offsetX: (Math.random() * 2 - 1) * safeOffset,
      offsetY: (Math.random() * 2 - 1) * safeOffset,
    });

    const scheduleNext = (initial = false) => {
      if (!active) return;

      const duration = safeMin + Math.random() * (safeMax - safeMin);
      const { offsetX, offsetY } = nextOffsets();

      setStyle((prev) => ({
        ...prev,
        transform: `translate3d(${offsetX}px, ${offsetY}px, 0)`,
        transition: initial ? "none" : `transform ${duration}ms ease-in-out`,
      }));

      timeoutRef.current = window.setTimeout(
        () => scheduleNext(false),
        initial ? 40 : duration,
      );
    };

    timeoutRef.current = window.setTimeout(() => scheduleNext(true), safeInitialDelay);

    return () => {
      active = false;
      if (timeoutRef.current !== null) {
        window.clearTimeout(timeoutRef.current);
      }
    };
  }, [initialDelayMs, maxDurationMs, maxOffset, minDurationMs]);

  return style;
}

type FloatingGlowProps = MovementOptions & {
  className: string;
};

function FloatingGlow({ className, initialDelayMs, ...options }: FloatingGlowProps) {
  const resolvedDelay = useMemo(
    () => (initialDelayMs ?? 200 + Math.random() * 1200),
    [initialDelayMs],
  );
  const style = useSlowRandomMovement({ ...options, initialDelayMs: resolvedDelay });
  return <div className={className} style={style} aria-hidden="true" />;
}

export default function Home() {
  const [usage, setUsage] = useState<UsageState>({
    totalCount: 0,
    totalBytes: 0,
    currentBps: 0,
    windowSeconds: 1,
  });
  const [trend, setTrend] = useState<"up" | "down">("up");
  const [health, setHealth] = useState<"loading" | "ok" | "bad">("loading");
  const [repoUrl, setRepoUrl] = useState("");
  const [proxyBase, setProxyBase] = useState<string>("");
  const lastBpsRef = useRef(0);
  // Use same-origin relative API paths; no env needed

  useEffect(() => {
    if (typeof window !== "undefined") {
      setProxyBase(window.location.origin);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    const snapshotUrl = "/v1/usage";
    const streamUrl = "/v1/usage/stream";

    const fetchSnapshot = async () => {
      try {
        const res = await fetch(snapshotUrl, { method: "GET", cache: "no-store" });
        if (!res.ok || cancelled) {
          return;
        }
        const data = await res.json();
        if (cancelled) {
          return;
        }
        const nextBps = Number(data.current_bps) || 0;
        lastBpsRef.current = nextBps;
        const totalCountRaw = Number(data.total_count);
        const totalBytesRaw = Number(data.total_bytes);
        const windowSecondsRaw = Number(data.window_seconds);
        setUsage({
          totalCount: Number.isFinite(totalCountRaw) ? totalCountRaw : 0,
          totalBytes: Number.isFinite(totalBytesRaw) ? totalBytesRaw : 0,
          currentBps: nextBps,
          windowSeconds: Number.isFinite(windowSecondsRaw) && windowSecondsRaw > 0 ? windowSecondsRaw : 1,
        });
      } catch {
        // ignore
      }
    };

    fetchSnapshot();

    let source: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const openStream = () => {
      source?.close();
      source = new EventSource(streamUrl);
      source.onmessage = (event) => {
        if (cancelled) {
          return;
        }
        try {
          const payload = JSON.parse(event.data);
          const nextBps = Number(payload.current_bps) || 0;
          const previousBps = lastBpsRef.current;
          lastBpsRef.current = nextBps;
          setTrend(nextBps >= previousBps ? "up" : "down");
          setUsage((prev) => {
            const totalCount =
              payload.total_count === undefined || payload.total_count === null
                ? prev.totalCount
                : Number(payload.total_count) || 0;
            const totalBytes =
              payload.total_bytes === undefined || payload.total_bytes === null
                ? prev.totalBytes
                : Number(payload.total_bytes) || 0;
            const windowSecondsRaw =
              payload.window_seconds === undefined || payload.window_seconds === null
                ? prev.windowSeconds
                : Number(payload.window_seconds) || 1;
            const windowSeconds = windowSecondsRaw > 0 ? windowSecondsRaw : 1;
            return {
              totalCount,
              totalBytes,
              currentBps: nextBps,
              windowSeconds,
            };
          });
        } catch {
          // ignore malformed payloads
        }
      };
      source.onerror = () => {
        if (cancelled) {
          return;
        }
        source?.close();
        if (!retryTimer) {
          retryTimer = setTimeout(() => {
            retryTimer = null;
            openStream();
          }, 3000);
        }
      };
    };

    openStream();

    return () => {
      cancelled = true;
      if (retryTimer) {
        clearTimeout(retryTimer);
      }
      source?.close();
    };
  }, []);

  useEffect(() => {
    let mounted = true;
    const healthUrl = "/v1/healthz";
    const check = async () => {
      try {
        const res = await fetch(healthUrl, { method: "GET", cache: "no-store" });
        if (!mounted) return;
        setHealth(res.ok ? "ok" : "bad");
      } catch {
        if (!mounted) return;
        setHealth("bad");
      }
    };
    check();
    const id = setInterval(check, 5000);
    return () => {
      mounted = false;
      clearInterval(id);
    };
  }, []);

  const metrics = useMemo(() => {
    const totalBytesDisplay = formatBytes(usage.totalBytes);
    const throughputDisplay = formatRate(usage.currentBps);
    const windowSecondsDisplay = Math.max(1, Math.round(usage.windowSeconds));
    const throughputFootnote =
      usage.currentBps > 0
        ? trend === "up"
          ? `滑动 ${windowSecondsDisplay}s 窗口，吞吐稳定上升`
          : `滑动 ${windowSecondsDisplay}s 窗口，略有波动`
        : "等待活跃流量";

    return [
      {
        label: "累计请求量",
        primary: formatNumber(usage.totalCount),
        unit: "次",
        footnote: usage.totalCount > 0 ? "统计自代理启动以来" : "等待请求产生数据",
      },
      {
        label: "累计流量",
        primary: totalBytesDisplay.value,
        unit: totalBytesDisplay.unit,
        footnote: "含通过 LFS 下载的全部字节数",
      },
      {
        label: "实时吞吐速度",
        primary: throughputDisplay.value,
        unit: throughputDisplay.unit,
        footnote: throughputFootnote,
      },
    ];
  }, [trend, usage.totalBytes, usage.totalCount, usage.currentBps, usage.windowSeconds]);

  const usageSteps = useMemo(
    () => [
      {
        title: "方式一：完整 URL",
        description: "在代理根地址后直接拼接完整的 Hugging Face 链接。",
        code: `git clone ${proxyBase}/https://huggingface.co/google-bert/bert-base-uncased`,
      },
      {
        title: "方式二：相对路径",
        description: "在代理根地址后拼接仓库的相对路径（使用 DEFAULT_UPSTREAM 作为前缀）。",
        code: `git clone ${proxyBase}/google-bert/bert-base-uncased`,
      },
    ],
    [proxyBase],
  );

  const proxyUrl = useMemo(() => {
    if (!repoUrl) return "";

    if (repoUrl.includes("huggingface.co")) {
      const cleaned = repoUrl.replace(/^https?:\/\//, "");
      return `git clone ${proxyBase}/${cleaned}`;
    }

    const cleaned = repoUrl.replace(/^\//, "");
    return `git clone ${proxyBase}/${cleaned}`;
  }, [repoUrl, proxyBase]);

  const heartbeatColor =
    health === "loading"
      ? "rgba(156, 163, 175, 0.75)"
      : health === "ok"
        ? "rgba(16, 185, 129, 0.7)"
        : "rgba(244, 63, 94, 0.75)";

  return (
    <main className="relative min-h-screen flex flex-col gap-10 sm:gap-14 overflow-x-hidden">
        <section className="relative isolate overflow-hidden rounded-[2.75rem] border border-white/10 bg-gradient-to-br from-[#0c172a] via-[#05060d] to-black px-8 py-14 text-white shadow-[0_60px_160px_-80px_rgba(15,23,42,0.9)] sm:px-12 lg:px-16">
          <FloatingGlow
            className="pointer-events-none absolute -left-24 top-0 -z-10 h-72 w-72 rounded-full bg-sky-400/25 blur-3xl sm:h-80 sm:w-80 lg:-left-32"
            maxOffset={90}
            minDurationMs={7000}
            maxDurationMs={15000}
          />
          <FloatingGlow
            className="pointer-events-none absolute bottom-[-30%] right-[-20%] -z-10 h-96 w-96 rounded-full bg-emerald-400/20 blur-3xl sm:right-[-12%]"
            maxOffset={150}
            minDurationMs={9000}
            maxDurationMs={18000}
          />

          {/* 加 min-w-0，避免内部 flex 子项撑宽 */}
          <div className="relative z-10 flex min-w-0 flex-col gap-12 lg:flex-row lg:items-stretch lg:justify-between">
            <div className="max-w-2xl space-y-6 min-w-0">
              <p className="text-xs font-semibold uppercase tracking-[0.5em] text-sky-200/80">
                HF Bridge 实时看板
              </p>
              <div className="flex flex-col">
                <h1 className="text-5xl font-semibold leading-[1.05] tracking-tight md:text-6xl">
                  服务状态
                </h1>
                <div className="mt-8 flex items-center gap-2">
                  <span
                    className={
                      "inline-block h-3 w-3 rounded-full shadow-[0_0_0_3px_rgba(255,255,255,0.06)] animate-heartbeat " +
                      (health === "loading"
                        ? "bg-gray-400/80"
                        : health === "ok"
                          ? "bg-emerald-400 shadow-[0_0_20px_rgba(16,185,129,0.6)]"
                          : "bg-rose-500 shadow-[0_0_20px_rgba(244,63,94,0.6)]")
                    }
                    style={{ "--heartbeat-color": heartbeatColor } as CSSProperties}
                    aria-label={health === "ok" ? "服务正常" : health === "bad" ? "服务异常" : "检查中"}
                    title={health === "ok" ? "服务正常" : health === "bad" ? "服务异常" : "检查中"}
                  />
                  <span className="text-xs text-white/60">
                    {health === "ok" ? "运行正常" : health === "bad" ? "服务异常" : "检查中"}
                  </span>
                </div>
              </div>
            </div>

            <div className="flex w-full flex-1 flex-wrap gap-6 sm:gap-8 min-w-0">
              {metrics.map((m) => (
                <Card
                  key={m.label}
                  className="flex w-full sm:w-63 h-55 min-w-0 p-4 rounded-3xl border border-white/10 bg-white/5 backdrop-blur-xl shadow-[0_30px_80px_-60px_rgba(59,130,246,0.7)]"
                >
                  <CardHeader>
                    <p className="text-[11px] font-semibold uppercase tracking-[0.3em] text-white/60">
                      {m.label}
                    </p>
                  </CardHeader>
                  <CardBody>
                    <div className="flex items-baseline gap-3">
                      <span className="whitespace-nowrap mt-2 text-6xl font-semibold leading-none tracking-tight text-white tabular-nums">
                        {m.primary}
                      </span>
                      <span className="text-lg font-medium text-white/60 sm:text-xl whitespace-nowrap">
                        {m.unit}
                      </span>
                    </div>
                  </CardBody>
                  <CardFooter>
                    <p className="text-xs text-white/60">{m.footnote}</p>
                  </CardFooter>
                </Card>
              ))}
            </div>
          </div>
        </section>

        <section className="relative isolate overflow-hidden rounded-[2.75rem] border border-white/10 bg-gradient-to-br from-[#0c172a] via-[#05060d] to-black px-8 py-14 text-white shadow-[0_60px_160px_-80px_rgba(15,23,42,0.9)] sm:px-12 lg:px-16">
          <FloatingGlow
            className="pointer-events-none absolute -left-24 top-0 -z-10 h-72 w-72 rounded-full bg-sky-400/25 blur-3xl sm:h-80 sm:w-80 lg:-left-32"
            maxOffset={90}
            minDurationMs={7000}
            maxDurationMs={15000}
          />
          <FloatingGlow
            className="pointer-events-none absolute bottom-[-30%] right-[-20%] -z-10 h-96 w-96 rounded-full bg-emerald-400/20 blur-3xl sm:right-[-12%]"
            maxOffset={150}
            minDurationMs={9000}
            maxDurationMs={18000}
          />
          <Card className="rounded-3xl px-2 border border-white/10 bg-white/10 text-white shadow-[0_40px_120px_-80px_rgba(15,23,42,0.9)] backdrop-blur-xl">
            <CardHeader className="p-8">
              <h2 className="text-4xl font-semibold tracking-tight">如何使用</h2>
            </CardHeader>
            <CardBody className="mb-6 px-6">
              <ol className="space-y-6 text-sm text-white/75">
                {usageSteps.map((step, index) => (
                  <li key={step.title} className="flex flex-col gap-4 sm:flex-row min-w-0">
                    <span className="mt-1 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full border border-white/15 bg-white/10 text-sm font-semibold text-white">
                      {index + 1}
                    </span>
                    {/* 这里加 min-w-0，承载代码的容器要能收缩 */}
                    <div className="flex-1 min-w-0">
                      <p className="text-base font-medium text-white">
                        {step.title}
                      </p>
                      <p className="mt-2 leading-relaxed">{step.description}</p>
                      {!!proxyBase && (
                        <Snippet
                          color="default"
                          className="mt-2 w-full min-w-0 text-left text-xs sm:text-sm
                                     [&_code]:break-words [&_code]:whitespace-pre-wrap [&_code]:break-all
                                     [&_pre]:break-words  [&_pre]:whitespace-pre-wrap  [&_pre]:break-all"
                        >
                          {step.code}
                        </Snippet>
                      )}
                    </div>
                  </li>
                ))}
              </ol>

              <div className="mt-10 space-y-4 rounded-2xl border border-white/15 bg-white/5 p-6 backdrop-blur-sm min-w-0">
                <h3 className="text-base font-semibold text-white">快速生成代理链接</h3>
                <p className="text-sm text-white/70">
                  输入 Hugging Face 仓库地址或相对路径，自动生成代理命令
                </p>
                <Input
                  value={repoUrl}
                  onChange={(e) => setRepoUrl(e.target.value)}
                  placeholder="例如：https://huggingface.co/google-bert/bert-base-uncased 或 google-bert/bert-base-uncased"
                  classNames={{
                    inputWrapper: "bg-white/5 border text-xs sm:text-sm border-white/20 hover:bg-white/8 group-data-[focus=true]:bg-white/5 group-data-[focus=true]:border-sky-400/50",
                  }}
                  size="lg"
                />
                {proxyUrl && (
                  <div className="mt-4 min-w-0">
                    <p className="mb-2 text-sm font-medium text-white">生成的代理命令：</p>
                    <Snippet
                      color="success"
                      className="w-full min-w-0 text-left text-xs text-green-400 backdrop-blur-sm sm:text-sm
                                 [&_code]:break-words [&_code]:whitespace-pre-wrap [&_code]:break-all
                                 [&_pre]:break-words  [&_pre]:whitespace-pre-wrap  [&_pre]:break-all"
                    >
                      {proxyUrl}
                    </Snippet>
                  </div>
                )}
              </div>
            </CardBody>
          </Card>
        </section>

        <section className="relative isolate overflow-hidden rounded-[2.75rem] border border-white/10 bg-gradient-to-br from-[#0c172a] via-[#05060d] to-black px-8 py-14 text-white shadow-[0_60px_160px_-80px_rgba(15,23,42,0.9)] sm:px-12 lg:px-16">
          <FloatingGlow
            className="pointer-events-none absolute -left-24 top-0 -z-10 h-72 w-72 rounded-full bg-sky-400/25 blur-3xl sm:h-80 sm:w-80 lg:-left-32"
            maxOffset={90}
            minDurationMs={7000}
            maxDurationMs={15000}
          />
          <FloatingGlow
            className="pointer-events-none absolute bottom-[-30%] right-[-20%] -z-10 h-96 w-96 rounded-full bg-emerald-400/20 blur-3xl sm:right-[-12%]"
            maxOffset={150}
            minDurationMs={9000}
            maxDurationMs={18000}
          />
          <Card className="rounded-3xl p-8 border border-white/10 bg-white/10 text-white shadow-[0_40px_120px_-80px_rgba(15,23,42,0.9)] backdrop-blur-xl">
            <CardHeader>
              <h2 className="text-4xl font-semibold tracking-tight uppercase">⭐️ Star</h2>
            </CardHeader>
            <CardBody>
                <div className="flex flex-col items-start gap-2">
                  <p>如果你觉得 HF Bridge 对你有帮助，欢迎点个 ⭐️ Star 支持一下！</p>
                  <div className="flex md:flex-row flex-col gap-6 min-w-0">
                    <Link href="https://github.com/Gouryella/HFBridge" target="_blank" className="flex justify-center items-center mt-4 gap-2 rounded-4xl bg-gray-900/30 px-4 py-2 hover:bg-gray-900/50 transition">
                      <GithubIcon size={32} className="text-white/80 hover:text-white" />
                      <p className="text-white/80 hover:text-white font-semibold">GitHub 仓库</p>
                    </Link>
                    <Link href="https://huggingface.co/zhiqing" target="_blank" className="flex justify-center items-center mt-4 gap-2 rounded-4xl bg-gray-900/30 px-4 py-2 hover:bg-gray-900/50 transition">
                      <HuggingFaceIcon size={32} className="text-white/80 hover:text-white" />
                      <p className="text-white/80 hover:text-white font-semibold">Hugging Face 仓库</p>
                    </Link>
                  </div>
                </div>
            </CardBody>
          </Card>
        </section>
    </main>
  );
}
