
"use client";

import ProjectDashboard from "./components/project-dashboard";
import Link from "next/link";
import { FormEvent, useState } from "react";

type User = {
  id: string;
  name: string;
  email: string;
};

const API =
  process.env.NEXT_PUBLIC_AUTH_API_URL ?? "http://localhost:8081";

export default function Home() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setBusy(true);

    try {
      const response = await fetch(
        `${API}/api/v1/auth/${mode === "login" ? "login" : "register"}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, email, password }),
        }
      );

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error ?? "Something went wrong");
      }

      if (mode === "register") {
        setMode("login");
        setPassword("");
        setError("Account created. Please log in.");
      } else {
        setUser(data.user);
        setToken(data.access_token);
      }
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not connect to the API"
      );
    } finally {
      setBusy(false);
    }
  }

  async function loadProfile() {
    setError("");

    try {
      const response = await fetch(`${API}/api/v1/auth/me`, {
        headers: { Authorization: `Bearer ${token}` },
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error ?? "Could not load profile");
      }

      setUser(data.user);
      setError("Profile verified by the Go authentication service.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed");
    }
  }

  function logout() {
    setToken("");
    setUser(null);
    setPassword("");
    setError("");
  }

  return (
    <main className="min-h-screen bg-[#09090b] text-zinc-100">
      <header className="flex items-center justify-between border-b border-white/10 px-6 py-5">
        <Link href="/" className="text-xl font-bold tracking-tight">
          <span className="text-indigo-400">◆</span> DevBoard
        </Link>
        <span className="text-sm text-zinc-500">
          Developer workspace
        </span>
      </header>

      <section className="mx-auto flex min-h-[80vh] max-w-6xl items-center justify-center gap-16 px-6 py-12">
        <div className="hidden max-w-md lg:block">
          <p className="mb-4 text-sm font-medium uppercase tracking-[0.2em] text-indigo-400">
            Build. Track. Ship.
          </p>
          <h1 className="text-5xl font-bold leading-tight tracking-tight">
            Your team s work, all in one place.
          </h1>
          <p className="mt-6 leading-7 text-zinc-400">
            A project management platform built with Next.js,
            Go microservices, and PostgreSQL.
          </p>
          <div className="mt-10 grid grid-cols-2 gap-3 text-sm">
            {["Go-powered APIs", "Secure password hashing",
              "PostgreSQL storage", "Microservices architecture"].map(
              (item) => (
                <div
                  key={item}
                  className="rounded-xl border border-white/10 bg-white/[0.03] p-4 text-zinc-300"
                >
                  <span className="mr-2 text-emerald-400">✓</span>
                  {item}
                </div>
              )
            )}
          </div>
        </div>

        <div className="w-full max-w-md rounded-2xl border border-white/10 bg-[#111113] p-7 shadow-2xl shadow-black/30">
          {user ? (
            <>
              <div className="mb-6 flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-500/15 text-xl text-indigo-300">
                ✓
              </div>
              <h2 className="text-2xl font-semibold">Welcome, {user.name}</h2>
              <p className="mt-2 text-sm text-zinc-400">{user.email}</p>

              <div className="mt-8 rounded-xl border border-white/10 p-4">
                <p className="text-sm text-zinc-400">Authentication status</p>
                <p className="mt-2 font-medium text-emerald-400">
                  Logged in successfully
                </p>
                <p className="mt-2 break-all text-xs text-zinc-500">
                  User ID: {user.id}
                </p>
              </div>

              <button
                onClick={loadProfile}
                className="mt-5 w-full rounded-lg border border-white/10 py-3 text-sm hover:bg-white/5"
              >
                Verify protected Go endpoint
              </button>
              <button
                onClick={logout}
                className="mt-3 w-full rounded-lg bg-indigo-500 py-3 font-medium hover:bg-indigo-400"
              >
                Log out
              </button>
            </>
          ) : (
            <>
              <h2 className="text-2xl font-semibold">
                {mode === "login" ? "Welcome back" : "Create your account"}
              </h2>
              <p className="mt-2 text-sm text-zinc-400">
                {mode === "login"
                  ? "Log in to your DevBoard workspace."
                  : "Get started with your workspace."}
              </p>

              <div className="mt-6 grid grid-cols-2 rounded-lg bg-black/40 p-1">
                {(["login", "register"] as const).map((item) => (
                  <button
                    key={item}
                    onClick={() => {
                      setMode(item);
                      setError("");
                    }}
                    className={`rounded-md py-2 text-sm capitalize ${
                      mode === item
                        ? "bg-zinc-800 text-white"
                        : "text-zinc-500 hover:text-white"
                    }`}
                  >
                    {item}
                  </button>
                ))}
              </div>

              <form onSubmit={submit} className="mt-6 space-y-4">
                {mode === "register" && (
                  <label className="block text-sm text-zinc-300">
                    Full name
                    <input
                      required
                      minLength={1}
                      maxLength={100}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      className="mt-2 w-full rounded-lg border border-white/10 bg-black/30 px-4 py-3 outline-none focus:border-indigo-400"
                      placeholder="Your name"
                    />
                  </label>
                )}

                <label className="block text-sm text-zinc-300">
                  Email address
                  <input
                    required
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="mt-2 w-full rounded-lg border border-white/10 bg-black/30 px-4 py-3 outline-none focus:border-indigo-400"
                    placeholder="you@example.com"
                  />
                </label>

                <label className="block text-sm text-zinc-300">
                  Password
                  <input
                    required
                    minLength={12}
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="mt-2 w-full rounded-lg border border-white/10 bg-black/30 px-4 py-3 outline-none focus:border-indigo-400"
                    placeholder="At least 12 characters"
                  />
                </label>

                {error && (
                  <p role="status" className="text-sm text-amber-300">
                    {error}
                  </p>
                )}

                <button
                  disabled={busy}
                  className="w-full rounded-lg bg-indigo-500 py-3 font-medium transition hover:bg-indigo-400 disabled:opacity-50"
                >
                  {busy
                    ? "Please wait..."
                    : mode === "login"
                      ? "Log in"
                      : "Create account"}
                </button>
              </form>
            </>
          )}

          {user && error && (
            <p role="status" className="mt-4 text-sm text-emerald-300">
              {error}
            </p>
          )}

          <p className="mt-6 text-center text-xs text-zinc-600">
            DevBoard · Built with Next.js and Go
          </p>
        </div>
      </section>
      {user && <ProjectDashboard token={token} />}
    </main>
  );
}
