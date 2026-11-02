// Next.js processes global CSS imports at build time. TypeScript 7 requires
// an explicit ambient module declaration for side-effect-only CSS imports.
declare module "*.css";
