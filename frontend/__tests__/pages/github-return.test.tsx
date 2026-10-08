import { StrictMode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import GitHubReturnPage from "@/app/auth/github/page";
import { GITHUB_VERIFIER_KEY } from "@/lib/github-signin";
const mocks = vi.hoisted(() => ({ replace: vi.fn(), reloadUser: vi.fn(), saveTokens: vi.fn() }));
vi.mock("next/navigation", () => ({useRouter: () => ({replace: mocks.replace})}));
vi.mock("@/contexts/AuthContext", () => ({useAuth: () => ({reloadUser: mocks.reloadUser})}));
vi.mock("@/lib/token-storage", () => ({saveTokens: mocks.saveTokens}));
beforeEach(() => {
 vi.clearAllMocks(); sessionStorage.clear();
 window.history.replaceState(null,"","/auth/github?code=one-time-code");
 mocks.reloadUser.mockResolvedValue(undefined);
});
afterEach(() => {vi.unstubAllGlobals();window.history.replaceState(null,"","/");});
it("requires the initiating browser's verifier before exchanging a code", async () => {
 const request = vi.fn();vi.stubGlobal("fetch",request);
 render(<GitHubReturnPage />);
 expect(await screen.findByRole("alert")).toHaveTextContent("Start again from Nimbus");
 expect(request).not.toHaveBeenCalled();expect(mocks.saveTokens).not.toHaveBeenCalled();
 expect(window.location.search).toBe("");
});
it("exchanges once in Strict Mode and opens the connected private workspace", async () => {
 sessionStorage.setItem(GITHUB_VERIFIER_KEY,"browser-verifier");
 const request = vi.fn().mockResolvedValue({ok:true,json:async()=>({access_token:"access",refresh_token:"refresh"})});
 vi.stubGlobal("fetch",request);
 render(<StrictMode><GitHubReturnPage /></StrictMode>);
 await waitFor(()=>expect(mocks.replace).toHaveBeenCalledWith("/integrations"));
 expect(request).toHaveBeenCalledTimes(1);
 expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({code:"one-time-code",verifier:"browser-verifier"});
 expect(mocks.saveTokens).toHaveBeenCalledWith("access","refresh");
 expect(sessionStorage.getItem(GITHUB_VERIFIER_KEY)).toBeNull();
});
it("does not store tokens or redirect after a rejected exchange", async () => {
 sessionStorage.setItem(GITHUB_VERIFIER_KEY,"browser-verifier");
 vi.stubGlobal("fetch",vi.fn().mockResolvedValue({ok:false}));
 render(<GitHubReturnPage />);
 expect(await screen.findByRole("alert")).toBeVisible();
 expect(mocks.saveTokens).not.toHaveBeenCalled();expect(mocks.replace).not.toHaveBeenCalled();
});
