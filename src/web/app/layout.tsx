import type { Metadata } from "next";
import Script from "next/script";
import "./globals.css";

export const metadata: Metadata = {
  title: "Copilot AI Usage",
  description: "Self-service GitHub Copilot AI credit usage dashboard"
};

const themePreferenceScript = `
(() => {
  try {
    const preference = window.localStorage.getItem("copilot-usage-theme");
    if (preference === "light" || preference === "dark") {
      document.documentElement.dataset.theme = preference;
    } else {
      document.documentElement.removeAttribute("data-theme");
    }
  } catch {
    document.documentElement.removeAttribute("data-theme");
  }
})();
`;

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Script
          id="theme-preference"
          strategy="beforeInteractive"
          dangerouslySetInnerHTML={{ __html: themePreferenceScript }}
        />
        {children}
      </body>
    </html>
  );
}
