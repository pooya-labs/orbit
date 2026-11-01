import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "ORBIT — Systems Observatory",
  description:
    "An engineering-first observability workspace for distributed systems, available in English and Persian with right-to-left layout support.",
  applicationName: "ORBIT",
  authors: [{ name: "Pooya (@pooya16pg-spec)", url: "https://github.com/pooya16pg-spec" }],
  creator: "@pooya16pg-spec",
  publisher: "ORBIT · @pooya16pg-spec",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#090d12",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
