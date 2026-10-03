"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useForm, type UseFormRegisterReturn } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { useAuth } from "@/contexts/AuthContext";
import { APIError } from "@/lib/api";

const registerSchema = z
  .object({
    name: z.string().trim().min(2).max(120),
    email: z.string().trim().email(),
    password: z.string().min(8).max(72),
    confirmPassword: z.string(),
  })
  .refine((values) => values.password === values.confirmPassword, {
    message: "Passwords do not match.",
    path: ["confirmPassword"],
  });

type RegisterForm = z.infer<typeof registerSchema>;

export default function RegisterPage() {
  const router = useRouter();
  const { register: createAccount } = useAuth();
  const [serverError, setServerError] = useState("");

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterForm>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      name: "",
      email: "",
      password: "",
      confirmPassword: "",
    },
  });

  async function onSubmit(values: RegisterForm) {
    setServerError("");

    try {
      await createAccount({
        name: values.name,
        email: values.email,
        password: values.password,
      });

      router.replace("/dashboard");
    } catch (error) {
      if (error instanceof APIError) {
        setServerError(error.message);
        return;
      }

      setServerError("Account creation is temporarily unavailable. You can explore the demo without an account.");
    }
  }

  return (
    <main
      id="main-content"
      className="flex min-h-screen items-center justify-center bg-slate-950 px-6 py-12 text-white"
    >
      <section className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-8">
        <Link href="/" className="text-sm font-semibold text-sky-400">
          Nimbus
        </Link>

        <h1 className="mt-4 text-3xl font-semibold">Create your account</h1>

        <p className="mt-2 text-sm text-slate-400">
          Start monitoring your deployed applications.
        </p>

        <Link href="/demo" className="mt-6 block rounded-lg border border-sky-800 bg-sky-950/40 px-4 py-3 text-center text-sm font-semibold text-sky-300 hover:bg-sky-950">
          Explore demo — no login required
        </Link>

        <form onSubmit={handleSubmit(onSubmit)} className="mt-8 space-y-5">
          <Field
            label="Name"
            type="text"
            registration={register("name")}
            error={errors.name?.message}
          />

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

          <Field
            label="Confirm password"
            type="password"
            registration={register("confirmPassword")}
            error={errors.confirmPassword?.message}
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
            {isSubmitting ? "Creating account..." : "Create account"}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-slate-400">
          Already have an account?{" "}
          <Link href="/login" className="font-semibold text-sky-400">
            Sign in
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

function Field({ label, type, error, registration }: FieldProps) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm font-medium">{label}</span>

      <input
        type={type}
        {...registration}
        className="w-full rounded-lg border border-slate-700 bg-slate-950 px-4 py-3 outline-none focus:border-sky-500"
      />

      {error ? (
        <span className="mt-2 block text-sm text-red-400">{error}</span>
      ) : null}
    </label>
  );
}
