export const GITHUB_VERIFIER_KEY = "nimbus-github-signin-verifier";
const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1";
export function hex(bytes: Uint8Array): string {
  return Array.from(bytes, value => value.toString(16).padStart(2, "0")).join("");
}
export async function startGitHubSignIn(): Promise<string> {
  const verifier = hex(crypto.getRandomValues(new Uint8Array(32)));
  const challenge = hex(new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier))));
  sessionStorage.setItem(GITHUB_VERIFIER_KEY, verifier);
  return `${API_URL}/github/login?challenge=${challenge}`;
}
