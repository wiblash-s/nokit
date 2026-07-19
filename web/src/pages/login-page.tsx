import { useEffect, useState, type FormEvent } from "react"
import { useSearchParams } from "react-router-dom"

type Mode = "loading" | "local" | "oidc"

const ERROR_MESSAGES: Record<string, string> = {
  no_access:
    "Your account is not a member of any panel group. Ask an administrator to add you to a cs2-rcon-* group.",
  access_denied: "Sign-in was denied by the identity provider.",
  state: "Your sign-in session expired. Please try again.",
  exchange: "Could not complete sign-in with the identity provider.",
  verify: "Could not verify the identity provider response.",
  claims: "The identity provider response was malformed.",
  no_id_token: "The identity provider did not return an ID token.",
}

export function LoginPage() {
  const [params] = useSearchParams()
  const [mode, setMode] = useState<Mode>("loading")

  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const err = params.get("error")
    if (err) setError(ERROR_MESSAGES[err] ?? "Sign-in failed.")
  }, [params])

  useEffect(() => {
    fetch("/api/auth/config")
      .then((r) => r.json())
      .then((body) => setMode(body.mode === "oidc" ? "oidc" : "local"))
      .catch(() => setMode("local"))
  }, [])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError("")
    try {
      const r = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      })
      if (r.ok) {
        // Force a full reload so the AuthProvider re-fetches /api/me.
        window.location.assign("/")
      } else {
        const body = await r.json().catch(() => ({}))
        setError(body.error ?? "invalid credentials")
      }
    } catch {
      setError("could not reach server")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-sm">
        <div className="mb-8">
          <h1 className="font-mono text-2xl font-semibold tracking-tight">
            nokit
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">cs2 server panel</p>
        </div>

        {error && (
          <p className="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}

        {mode === "loading" && (
          <p className="text-sm text-muted-foreground">loading…</p>
        )}

        {mode === "oidc" && (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              Sign in with your single sign-on account.
            </p>
            <a
              href="/api/auth/login"
              className="flex h-10 w-full items-center justify-center rounded-md bg-foreground text-sm font-medium text-background transition-opacity hover:opacity-90"
            >
              Sign in with SSO
            </a>
          </div>
        )}

        {mode === "local" && (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-1.5">
              <label
                className="text-sm text-muted-foreground"
                htmlFor="username"
              >
                username
              </label>
              <input
                id="username"
                type="text"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring"
                required
              />
            </div>

            <div className="space-y-1.5">
              <label
                className="text-sm text-muted-foreground"
                htmlFor="password"
              >
                password
              </label>
              <input
                id="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-1 focus:ring-ring"
                required
              />
            </div>

            <button
              type="submit"
              disabled={busy}
              className="h-9 w-full rounded-md bg-foreground text-sm font-medium text-background disabled:opacity-50"
            >
              {busy ? "signing in…" : "sign in"}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
