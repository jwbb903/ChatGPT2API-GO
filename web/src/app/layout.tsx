import type { Metadata, Viewport } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import { Toaster } from "sonner";
import "./globals.css";
import { PageTransition } from "@/components/page-transition";
import { RouteProgress } from "@/components/route-progress";
import { TopNav } from "@/components/top-nav";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "ChatGPT 号池管理",
  description: "ChatGPT account pool management dashboard",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 1,
  userScalable: false,
  themeColor: "#fbfbfd",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" className={`${inter.variable} ${jetbrainsMono.variable}`} suppressHydrationWarning>
      <body
        className="antialiased font-sans"
        style={{
          fontFamily:
            'var(--font-sans), "PingFang SC", "Microsoft YaHei", "Helvetica Neue", sans-serif',
        }}
      >
        <Toaster position="top-center" richColors offset={48} />
        <RouteProgress />
        <TopNav />
        <main className="h-screen overflow-x-hidden overflow-y-auto px-4 pt-12 pb-2 text-foreground [scrollbar-gutter:stable_both-edges] sm:px-6 sm:pt-14 lg:px-8">
          <div className="mx-auto box-border flex max-w-[1440px] flex-col pt-[env(safe-area-inset-top)]">
            <PageTransition>{children}</PageTransition>
          </div>
        </main>
      </body>
    </html>
  );
}
