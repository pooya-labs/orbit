import type { NextConfig } from "next";

const isGitHubPages = process.env.GITHUB_ACTIONS === "true";
const basePath = isGitHubPages ? "/orbit" : "";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  // GitHub Pages hosts this repository at /orbit rather than the domain root.
  basePath,
  env: { NEXT_PUBLIC_ORBIT_BASE_PATH: basePath },
  output: "export",
  trailingSlash: true,
  images: {
    unoptimized: true,
  },
};

export default nextConfig;
