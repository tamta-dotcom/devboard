
"use client";

import { useCallback, useEffect, useState } from "react";

type Project = {
  id: string;
  title: string;
  description: string;
  status: "active" | "archived";
  created_at: string;
  updated_at: string;
};

const API =
  process.env.NEXT_PUBLIC_PROJECTS_API_URL ?? "http://localhost:8082";

export default function ProjectDashboard({ token }: { token: string }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState<"all" | "active" | "archived">("all");

  const request = useCallback(
    async (path: string, init: RequestInit = {}) => {
      const response = await fetch(`${API}${path}`, {
        ...init,
        headers: {
          Authorization: `Bearer ${token}`,
          ...(init.body ? { "Content-Type": "application/json" } : {}),
          ...init.headers,
        },
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error ?? `Request failed (${response.status})`);
      }

      if (response.status === 204) return null;
      return response.json();
    },
    [token]
  );

  const loadProjects = useCallback(async () => {
    setLoading(true);
    setError("");

    try {
      const data = await request("/api/v1/projects");
      setProjects(data.projects);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load projects");
    } finally {
      setLoading(false);
    }
  }, [request]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadProjects();
  }, [loadProjects]);

  async function createProject(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!title.trim()) return;

    setBusy(true);
    setError("");

    try {
      const data = await request("/api/v1/projects", {
        method: "POST",
        body: JSON.stringify({ title, description }),
      });

      setProjects((current) => [data.project, ...current]);
      setTitle("");
      setDescription("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create project");
    } finally {
      setBusy(false);
    }
  }

  async function toggleStatus(project: Project) {
    const status = project.status === "active" ? "archived" : "active";
    setError("");

    try {
      const data = await request(`/api/v1/projects/${project.id}`, {
        method: "PATCH",
        body: JSON.stringify({ status }),
      });

      setProjects((current) =>
        current.map((p) => (p.id === project.id ? data.project : p))
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not update project");
    }
  }

  async function deleteProject(project: Project) {
    if (!window.confirm(`Delete "${project.title}" permanently?`)) return;

    setError("");

    try {
      await request(`/api/v1/projects/${project.id}`, {
        method: "DELETE",
      });

      setProjects((current) => current.filter((p) => p.id !== project.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not delete project");
    }
  }

  const visibleProjects = projects.filter(
    (p) => filter === "all" || p.status === filter
  );

  const activeCount = projects.filter((p) => p.status === "active").length;
  const archivedCount = projects.length - activeCount;

  return (
    <section className="mx-auto max-w-6xl px-6 pb-16">
      <div className="mb-8 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-sm text-indigo-400">YOUR WORKSPACE</p>
          <h2 className="mt-2 text-3xl font-bold tracking-tight">
            Projects
          </h2>
          <p className="mt-2 text-sm text-zinc-400">
            Organize your work and keep your team moving.
          </p>
        </div>

        <button
          onClick={() => document.getElementById("new-project-title")?.focus()}
          className="rounded-lg bg-indigo-500 px-4 py-2.5 text-sm font-medium hover:bg-indigo-400"
        >
          + New project
        </button>
      </div>

      <div className="mb-8 grid gap-4 sm:grid-cols-3">
        {[
          { label: "Total projects", value: projects.length },
          { label: "Active projects", value: activeCount },
          { label: "Archived projects", value: archivedCount },
        ].map((stat) => (
          <div
            key={stat.label}
            className="rounded-xl border border-white/10 bg-[#111113] p-5"
          >
            <p className="text-sm text-zinc-400">{stat.label}</p>
            <p className="mt-3 text-3xl font-semibold">{stat.value}</p>
          </div>
        ))}
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-[340px_1fr]">
        <form
          onSubmit={createProject}
          className="rounded-xl border border-white/10 bg-[#111113] p-5"
        >
          <h3 className="text-lg font-semibold">Create a project</h3>
          <p className="mt-1 text-sm text-zinc-500">
            Start with a name and a short description.
          </p>

          <label className="mt-5 block text-sm text-zinc-300">
            Project name
            <input
              id="new-project-title"
              required
              maxLength={120}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Website redesign"
              className="mt-2 w-full rounded-lg border border-white/10 bg-black/30 px-3 py-3 outline-none focus:border-indigo-400"
            />
          </label>

          <label className="mt-4 block text-sm text-zinc-300">
            Description
            <textarea
              maxLength={5000}
              rows={4}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What are you building?"
              className="mt-2 w-full resize-y rounded-lg border border-white/10 bg-black/30 px-3 py-3 outline-none focus:border-indigo-400"
            />
          </label>

          <button
            disabled={busy}
            className="mt-4 w-full rounded-lg bg-indigo-500 py-3 font-medium hover:bg-indigo-400 disabled:opacity-50"
          >
            {busy ? "Creating..." : "Create project"}
          </button>
        </form>

        <div>
          <div className="mb-4 flex flex-wrap gap-2">
            {(["all", "active", "archived"] as const).map((value) => (
              <button
                key={value}
                onClick={() => setFilter(value)}
                className={`rounded-lg px-3 py-2 text-sm capitalize ${
                  filter === value
                    ? "bg-zinc-100 text-zinc-900"
                    : "border border-white/10 text-zinc-400 hover:bg-white/5"
                }`}
              >
                {value}
              </button>
            ))}

            <button
              onClick={() => void loadProjects()}
              className="ml-auto rounded-lg border border-white/10 px-3 py-2 text-sm text-zinc-400 hover:bg-white/5"
            >
              Refresh
            </button>
          </div>

          {error && (
            <div role="alert" className="mb-4 rounded-lg border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-300">
              {error}
            </div>
          )}

          {loading ? (
            <div className="rounded-xl border border-white/10 p-8 text-center text-zinc-400">
              Loading projects...
            </div>
          ) : visibleProjects.length === 0 ? (
            <div className="rounded-xl border border-dashed border-white/15 p-10 text-center">
              <div className="text-3xl">◫</div>
              <p className="mt-3 font-medium">No projects here yet</p>
              <p className="mt-2 text-sm text-zinc-500">
                Create a project to see it appear in your workspace.
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {visibleProjects.map((project) => (
                <article
                  key={project.id}
                  className="rounded-xl border border-white/10 bg-[#111113] p-5 transition hover:border-white/20"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="h-2 w-2 rounded-full bg-indigo-400" />
                        <h3 className="break-words font-semibold">
                          {project.title}
                        </h3>
                        <span
                          className={`rounded-full px-2 py-1 text-xs ${
                            project.status === "active"
                              ? "bg-emerald-500/10 text-emerald-400"
                              : "bg-zinc-700/40 text-zinc-400"
                          }`}
                        >
                          {project.status}
                        </span>
                      </div>

                      <p className="mt-3 whitespace-pre-wrap break-words text-sm leading-6 text-zinc-400">
                        {project.description || "No description provided."}
                      </p>

                      <p className="mt-4 text-xs text-zinc-600">
                        Created{" "}
                        {new Date(project.created_at).toLocaleDateString()}
                      </p>
                    </div>

                    <div className="flex shrink-0 gap-2">
                      <button
                        onClick={() => void toggleStatus(project)}
                        className="rounded-lg border border-white/10 px-3 py-2 text-xs text-zinc-300 hover:bg-white/5"
                      >
                        {project.status === "active" ? "Archive" : "Restore"}
                      </button>
                      <button
                        onClick={() => void deleteProject(project)}
                        className="rounded-lg border border-red-500/20 px-3 py-2 text-xs text-red-300 hover:bg-red-500/10"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
