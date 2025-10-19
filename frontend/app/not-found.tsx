"use client";

import Link from "next/link";
import { Button } from "@heroui/button";
import { useMemo } from "react";

type GlowProps = {
  className: string;
  seed: number;
};

function FloatingGlow({ className, seed }: GlowProps) {
  const style = useMemo(() => {
    const offsetX = Math.sin(seed) * 40;
    const offsetY = Math.cos(seed) * 40;
    const delay = (seed % 1) * 1200;
    return {
      transform: `translate3d(${offsetX}px, ${offsetY}px, 0)`,
      animationDelay: `${delay}ms`,
    };
  }, [seed]);

  return (
    <div
      className={`${className} animate-[pulse_9s_ease-in-out_infinite]`}
      style={style}
      aria-hidden="true"
    />
  );
}

export default function NotFound() {
  return (
    <section className="relative isolate overflow-hidden rounded-[2.75rem] border border-white/10 bg-gradient-to-br from-[#0d1627] via-[#060912] to-black px-8 py-20 text-white shadow-[0_60px_160px_-80px_rgba(15,23,42,0.9)] sm:px-12 lg:px-16">
      <FloatingGlow
        className="pointer-events-none absolute -left-36 top-0 -z-10 h-72 w-72 rounded-full bg-sky-500/20 blur-3xl sm:-left-24 sm:h-80 sm:w-80"
        seed={0.42}
      />
      <FloatingGlow
        className="pointer-events-none absolute -right-24 bottom-[-25%] -z-10 h-96 w-96 rounded-full bg-emerald-400/20 blur-3xl sm:-right-16"
        seed={0.78}
      />

      <div className="relative z-10 flex flex-col items-center text-center">
        <span className="text-xs font-semibold uppercase tracking-[0.5em] text-sky-200/80">
          404 / Not Found
        </span>
        <h1 className="mt-6 text-4xl font-semibold leading-tight tracking-tight sm:text-5xl">
          这条桥暂时没搭好
        </h1>
        <p className="mt-5 max-w-xl text-sm text-slate-200/80 sm:text-base">
          我们没有在代理网络中找到这个页面。也许链接写错了，或者它还在构建中。
          你可以返回首页继续浏览，或者前往仓库查看使用说明。
        </p>

        <div className="mt-10 flex flex-wrap justify-center gap-4">
          <Button
            as={Link}
            className="bg-sky-500/80 text-white shadow-lg shadow-sky-500/30 transition hover:bg-sky-400"
            href="/"
            radius="full"
            size="lg"
          >
            返回首页
          </Button>
          <Button
            as={Link}
            className="border border-white/20 bg-white/5 text-slate-100 backdrop-blur transition hover:border-white/40 hover:bg-white/10"
            href="https://github.com/gouryella/hfbridge"
            radius="full"
            size="lg"
            target="_blank"
            rel="noreferrer"
          >
            查看项目
          </Button>
        </div>
      </div>
    </section>
  );
}
