"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  useForm,
  type UseFormRegisterReturn,
} from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { useAuth } from "@/contexts/AuthContext";
import { APIError } from "@/lib/api";

const loginSchema = z.object({
  email: z.string().trim().email(),
  password: z.string().min(1, "Password is required."),
});

type LoginForm = z.infer<typeof loginSchema>;

export default function LoginPage() {
  const router = useRouter();
  const { login } = useAuth();
  const [serverError, setServerError] = useState("");

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: "",
      password: "",
    },
  });

  async function onSubmit(values: LoginForm) {
    setServerError("");

    try {
      await login(values);
      router.replace("/dashboard");
    } catch (error) {
      if (error instanceof APIError) {
        setServerError(error.message);
        return;
      }

      setServerError("Nimbus could not sign you in.");
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-950 px-6 py-12 text-white">
      <section className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-8">
        <Link
          href="/"
          className="text-sm font-semibold text-sky-400"
        >
          Nimbus
        </Link>

        <h1 className="mt-4 text-3xl font-semibold">
          Welcome back
        </h1>

        <p className="mt-2 text-sm text-slate-400">
          Sign in to your reliability dashboard.
        </p>

        <form
          onSubmit={handleSubmit(onSubmit)}
          className="mt-8 space-y-5"
        >
          <Field
            label="Email"
            type="email"
            registration={register("email")}
            error={errors.email?.message}
          />

          <Field
            label="Password"
            type="password"
            registration={register("password")}
            error={errors.password?.message}
          />

          {serverError ? (
            <p className="rounded-lg border border-red-900 bg-red-950/50 px-4 py-3 text-sm text-red-300">
              {serverError}
            </p>
          ) : null}

          <button
            type="submit"
            disabled={isSubmitting}
            className="w-full rounded-lg bg-sky-500 px-4 py-3 font-semibold transition hover:bg-sky-400 disabled:opacity-60"
          >
            {isSubmitting ? "Signing in..." : "Sign in"}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-slate-400">
          Need an account?{" "}
          <Link
            href="/register"
            className="font-semibold text-sky-400"
          >
            Register
          </Link>
        </p>
      </section>
    </main>
  );
}

type FieldProps = {
  label: string;
  type: string;
  error?: string;
  registration: UseFormRegisterReturn;
};

function Field({
  label,
  type,
  error,
  registration,
}: FieldProps) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm font-medium">
        {label}
      </span>

      <input
        type={type}
        {...registration}
        className="w-full rounded-lg border border-slate-700 bg-slate-950 px-4 py-3 outline-none focus:border-sky-500"
      />

      {error ? (
        <span className="mt-2 block text-sm text-red-400">
          {error}
        </span>
      ) : null}
    </label>
  );
}