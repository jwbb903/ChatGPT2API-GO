"use client";

import { LoaderCircle } from "lucide-react";

import { useAuthGuard } from "@/lib/use-auth-guard";

export default function RegisterPage() {
  const { isCheckingAuth, session } = useAuthGuard(["admin"]);

  if (isCheckingAuth || !session || session.role !== "admin") {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return (
    <main className="mx-auto flex max-w-3xl flex-col gap-4 px-6 py-12">
      <p className="text-xs font-semibold uppercase tracking-[0.28em] text-stone-400">Disabled Feature</p>
      <h1 className="text-3xl font-semibold tracking-tight text-stone-950">注册机已移除</h1>
      <p className="text-sm leading-6 text-stone-500">
        Go 版本按当前部署要求不再包含注册机功能。请在「号池管理」中手动导入已有账号。
      </p>
    </main>
  );
}
