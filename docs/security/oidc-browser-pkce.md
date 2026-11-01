# Browser OIDC authentication (M06)

The browser uses OpenID Connect Authorization Code with PKCE (S256). The browser client is public: **never configure or commit a client secret**.

## Build-time configuration

Set the following variables in the frontend build environment:

- NEXT_PUBLIC_ORBIT_OIDC_ISSUER: exact issuer string returned by the provider discovery document.
- NEXT_PUBLIC_ORBIT_OIDC_CLIENT_ID: public SPA client identifier.
- NEXT_PUBLIC_ORBIT_OIDC_AUDIENCE: optional API audience request parameter; it must match ORBIT_OIDC_AUDIENCE on the Go API.
- NEXT_PUBLIC_ORBIT_OIDC_SCOPE: optional scopes; defaults to openid profile email.
- NEXT_PUBLIC_ORBIT_OIDC_REDIRECT_URI: optional explicit redirect URI. Otherwise the app derives it from the current origin and configured Next.js base path.

For GitHub Pages, register this exact callback URL at the identity provider:

https://pooya16pg-spec.github.io/orbit/auth/callback/

The identity provider must advertise authorization-code response type and PKCE S256 support. If it includes the RFC 9207 authorization-response issuer parameter, the callback verifies that value against the configured issuer. Its token endpoint must allow browser CORS requests from https://pooya16pg-spec.github.io.

## Token handling

- The PKCE verifier, state and nonce are stored briefly in sessionStorage only to complete the redirect transaction.
- Access and refresh tokens remain in JavaScript memory. They are never written to localStorage, sessionStorage, cookies or URLs.
- The API client adds the access token as a Bearer token and refreshes it in memory when the provider returns a refresh token.
- Reloading the page clears the in-memory session and requires a new sign-in. This is intentional until a separately reviewed server-side session design exists.
- Sign-out clears the in-memory browser session only. Provider token revocation and global single logout are not implemented yet.

## Backend alignment

Configure the Go API with the same issuer and API audience:

- ORBIT_OIDC_ISSUER must exactly match the provider issuer.
- ORBIT_OIDC_AUDIENCE must match the audience requested by the browser and present in the API access token.
- ORBIT_ALLOWED_ORIGINS must include the exact browser origin (without a path): https://pooya16pg-spec.github.io.

This is a provider-neutral foundation. It is not production-integrated until a real identity provider/client is configured and the full browser-to-API flow is tested against it.
