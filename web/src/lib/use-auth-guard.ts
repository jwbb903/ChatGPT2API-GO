"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import {
  getCachedAuthSession,
  getValidatedAuthSession,
  hasValidatedAuthSession,
} from "@/lib/auth-session";
import {
  getDefaultRouteForRole,
  type AuthRole,
  type StoredAuthSession,
} from "@/store/auth";

type UseAuthGuardResult = {
  isCheckingAuth: boolean;
  session: StoredAuthSession | null;
};

export function useAuthGuard(allowedRoles?: AuthRole[]): UseAuthGuardResult {
  const router = useRouter();
  // 第一次进站没缓存 → 走 spinner；之后路由切换命中缓存，
  // 直接同步给出 session、isCheckingAuth=false，避免每页都闪一下加载圈。
  const initialCached = hasValidatedAuthSession() ? getCachedAuthSession() : null;
  const initialChecking = !hasValidatedAuthSession();
  const [session, setSession] = useState<StoredAuthSession | null>(initialCached);
  const [isCheckingAuth, setIsCheckingAuth] = useState(initialChecking);
  const allowedRolesKey = (allowedRoles || []).join(",");

  useEffect(() => {
    let active = true;
    const roleList = allowedRolesKey ? (allowedRolesKey.split(",") as AuthRole[]) : [];

    // 命中缓存时同步处理一次跳转/角色检查，不动 isCheckingAuth。
    if (hasValidatedAuthSession()) {
      const cached = getCachedAuthSession();
      if (!cached) {
        router.replace("/login");
      } else if (roleList.length > 0 && !roleList.includes(cached.role)) {
        router.replace(getDefaultRouteForRole(cached.role));
      }
    }

    // 不论命中与否，都在后台静默重新校验一次：
    //  - 第一次进站：拿到结果后关掉 spinner、写入 state；
    //  - 后续切换：仅在结果变化时更新 state，不闪 spinner。
    const load = async () => {
      const storedSession = await getValidatedAuthSession();
      if (!active) return;

      if (!storedSession) {
        setSession(null);
        setIsCheckingAuth(false);
        router.replace("/login");
        return;
      }

      if (roleList.length > 0 && !roleList.includes(storedSession.role)) {
        setSession(storedSession);
        setIsCheckingAuth(false);
        router.replace(getDefaultRouteForRole(storedSession.role));
        return;
      }

      setSession(storedSession);
      setIsCheckingAuth(false);
    };

    void load();
    return () => {
      active = false;
    };
  }, [allowedRolesKey, router]);

  return { isCheckingAuth, session };
}

export function useRedirectIfAuthenticated() {
  const router = useRouter();
  const [isCheckingAuth, setIsCheckingAuth] = useState(!hasValidatedAuthSession());

  useEffect(() => {
    let active = true;

    // 命中缓存：已登录则同步跳走。
    if (hasValidatedAuthSession()) {
      const cached = getCachedAuthSession();
      if (cached) {
        router.replace(getDefaultRouteForRole(cached.role));
      }
    }

    const load = async () => {
      const storedSession = await getValidatedAuthSession();
      if (!active) return;

      if (storedSession) {
        router.replace(getDefaultRouteForRole(storedSession.role));
        return;
      }

      setIsCheckingAuth(false);
    };

    void load();
    return () => {
      active = false;
    };
  }, [router]);

  return { isCheckingAuth };
}
