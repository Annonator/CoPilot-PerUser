import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Copilot AI Usage",
  description: "Self-service GitHub Copilot AI credit usage dashboard"
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
