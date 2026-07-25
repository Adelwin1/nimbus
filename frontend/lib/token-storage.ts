const ACCESS_TOKEN_KEY = "nimbus_access_token";
const REFRESH_TOKEN_KEY = "nimbus_refresh_token";

function browserStorageAvailable(): boolean {
  return typeof window !== "undefined";
}

export function getAccessToken(): string | null {
  if (!browserStorageAvailable()) {
    return null;
  }

  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  if (!browserStorageAvailable()) {
    return null;
  }

  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function saveTokens(
  accessToken: string,
  refreshToken: string,
): void {
  if (!browserStorageAvailable()) {
    return;
  }

  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

export function clearTokens(): void {
  if (!browserStorageAvailable()) {
    return;
  }

  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}