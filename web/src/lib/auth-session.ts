"use client";

import { login } from "@/lib/api";
import {
  clearStoredAuthSession,
  getStoredAuthSession,
  setStoredAuthSession,
  type StoredAuthSession,
} from "@/store/auth";

/**
 * 模块级会话缓存。
 *
 * 为什么需要：每个受 useAuthGuard 保护的页面在挂载时都会跑一次
 * getValidatedAuthSession()，里面有一发 login() 网络请求。校验回来之前
 * useAuthGuard 维持 isCheckingAuth=true，页面就吐一个 spinner——
 * 表现为路由切换后先闪一个加载图标，再出来真内容。
 *
 * 缓存策略：
 *  - getCachedAuthSession() 同步返回最近一次校验结果（命中即免 spinner）。
 *  - getValidatedAuthSession() 仍走网络重新校验，刷新缓存。
 *  - login / logout / 401 拦截会主动清缓存。
 */
let cachedSession: StoredAuthSession | null = null;
let hasValidatedOnce = false;
let validatedAt = 0;
let pendingValidation: Promise<StoredAuthSession | null> | null = null;

const AUTH_VALIDATION_TTL_MS = 30_000;

export function getCachedAuthSession(): StoredAuthSession | null {
  return cachedSession;
}

export function hasValidatedAuthSession(): boolean {
  return hasValidatedOnce;
}

export function primeAuthSessionCache(session: StoredAuthSession | null) {
  cachedSession = session;
  hasValidatedOnce = true;
  validatedAt = Date.now();
}

export function clearAuthSessionCache() {
  cachedSession = null;
  hasValidatedOnce = false;
  validatedAt = 0;
  pendingValidation = null;
}

export async function getValidatedAuthSession(): Promise<StoredAuthSession | null> {
  if (hasValidatedOnce && Date.now() - validatedAt < AUTH_VALIDATION_TTL_MS) {
    return cachedSession;
  }
  if (pendingValidation) {
    return pendingValidation;
  }

  pendingValidation = validateAuthSession();
  try {
    return await pendingValidation;
  } finally {
    pendingValidation = null;
  }
}

async function validateAuthSession(): Promise<StoredAuthSession | null> {
  const storedSession = await getStoredAuthSession();
  if (!storedSession) {
    primeAuthSessionCache(null);
    return null;
  }

  try {
    const data = await login(storedSession.key);
    const nextSession: StoredAuthSession = {
      key: storedSession.key,
      role: data.role,
      subjectId: data.subject_id,
      name: data.name,
    };
    await setStoredAuthSession(nextSession);
    primeAuthSessionCache(nextSession);
    return nextSession;
  } catch {
    await clearStoredAuthSession();
    primeAuthSessionCache(null);
    return null;
  }
}
