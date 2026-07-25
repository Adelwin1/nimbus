export type User = {
  id: string;
  name: string;
  email: string;
  created_at: string;
  updated_at: string;
};

export type AuthResponse = {
  user: User;
  access_token: string;
  refresh_token: string;
};

export type CurrentUserResponse = {
  user: User;
};

export type APIErrorResponse = {
  error?: {
    code?: string;
    message?: string;
    request_id?: string;
  };
};