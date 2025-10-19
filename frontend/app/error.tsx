"use client";

import { useEffect } from "react";
import Link from "next/link";
import { Button } from "@heroui/button";
import { Snippet } from "@heroui/snippet";
import { useMemo } from "react";

type GlowProps = {
  className: string;
  seed: number;
};

function FloatingGlow({ className, seed }: GlowProps) {
  const style = useMemo(() => {
    const offsetX = Math.sin(seed) * 48;
    const offsetY = Math.cos(seed) * 48;
    const delay = (seed % 1) * 1400;
    return {
      transform: `translate3d(${offsetX}px, ${offsetY}px, 0)`,
      animationDelay: `${delay}ms`,
    };
  }, [seed]);

  return (
    <div
      className={`${className} animate-[pulse_8s_ease-in-out_infinite]`}
      style={style}
      aria-hidden="true"
    />
  );
}

export default function ErrorPage({
  error,
  reset,
}: {
  error: Error;
  reset: () => void;
}) {
  useEffect(() => {
    // Log the error to an error reporting service
    /* eslint-disable no-console */
    console.error(error);
  }, [error]);

  return (
    <section className="relative isolate overflow-hidden rounded-[2.75rem] border border-white/10 bg-gradient-to-br from-[#1a0b16] via-[#05060d] to-black px-8 py-20 text-white shadow-[0_60px_160px_-80px_rgba(15,23,42,0.9)] sm:px-12 lg:px-16">
      <FloatingGlow
        className="pointer-events-none absolute -left-20 top-0 -z-10 h-72 w-72 rounded-full bg-rose-500/25 blur-3xl sm:-left-10 sm:h-80 sm:w-80"
        seed={0.36}
      />
      <FloatingGlow
        className="pointer-events-none absolute -right-16 bottom-[-22%] -z-10 h-96 w-96 rounded-full bg-sky-500/25 blur-3xl sm:-right-10"
        seed={0.81}
      />

      <div className="relative z-10 flex flex-col items-center text-center">
        <span className="text-xs font-semibold uppercase tracking-[0.5em] text-rose-200/80">
          Error State
        </span>
        <h1 className="mt-6 text-4xl font-semibold leading-tight tracking-tight sm:text-5xl">
          代理通道暂时拥堵
        </h1>
        <p className="mt-5 max-w-2xl text-sm text-slate-200/80 sm:text-base">
          服务在处理中遇到了异常。你可以尝试重新加载当前段落，
          如果问题持续发生，建议回到首页或前往仓库提交 Issue。
        </p>

        {error?.message ? (
          <Snippet
            hideCopyButton
            className="mt-8 max-w-2xl border border-white/10 bg-white/5 text-left text-slate-100"
            symbol=""
            variant="flat"
          >
            {error.message}
          </Snippet>
        ) : null}

        <div className="mt-10 flex flex-wrap justify-center gap-4">
          <Button
            className="bg-rose-500/80 text-white shadow-lg shadow-rose-500/30 transition hover:bg-rose-400"
            onPress={() => reset()}
            radius="full"
            size="lg"
          >
            尝试恢复
          </Button>
          <Button
            as={Link}
            className="border border-white/20 bg-white/5 text-slate-100 backdrop-blur transition hover:border-white/40 hover:bg-white/10"
            href="/"
            radius="full"
            size="lg"
          >
            返回首页
          </Button>
          <Button
            as={Link}
            className="border border-white/10 bg-transparent text-slate-100 backdrop-blur transition hover:border-white/30 hover:bg-white/5"
            href="https://github.com/gouryella/hfbridge/issues"
            radius="full"
            size="lg"
            target="_blank"
            rel="noreferrer"
          >
            提交 Issue
          </Button>
        </div>
      </div>
    </section>
  );
}
