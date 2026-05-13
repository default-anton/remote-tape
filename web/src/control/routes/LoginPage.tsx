import { useEffect, useState, type FormEvent } from "react";
import { useAuthSession, useLogin } from "../api/hooks";
import { Logo } from "../components/Shell";

function redirectToDashboard() {
  window.location.href = "/sessions";
}

export function LoginPage() {
  const auth = useAuthSession();
  const login = useLogin({
    onSuccess: redirectToDashboard,
  });
  const [password, setPassword] = useState("");

  useEffect(() => {
    if (auth.data?.authenticated) redirectToDashboard();
  }, [auth.data?.authenticated]);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    login.mutate(password);
  }

  return (
    <div className="login-page">
      <section className="login-brand">
        <Logo large />
        <h1>
          Secure recordings.
          <br />
          Anywhere, <span>anytime.</span>
        </h1>
        <ul>
          <li>
            <b>End-to-end encrypted transport</b>
            <small>All connections are encrypted in transit.</small>
          </li>
          <li>
            <b>Private & isolated</b>
            <small>Your recordings stay in your regions.</small>
          </li>
          <li>
            <b>Single-admin access</b>
            <small>Protected admin area with secure sign-in.</small>
          </li>
          <li>
            <b>Reliable by design</b>
            <small>Built for uptime, monitoring, and safety.</small>
          </li>
        </ul>
      </section>
      <section className="login-card panel">
        <div className="lock">▣</div>
        <h1>Admin sign in</h1>
        <p className="muted">Secure access to your remote-tape control plane.</p>
        <form onSubmit={submit}>
          <label>
            Password
            <input
              autoComplete="current-password"
              autoFocus
              disabled={login.isPending}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="Enter your password"
              required
              type="password"
              value={password}
            />
          </label>
          {login.error ? <p role="alert">{login.error.message}</p> : null}
          <button
            className="button primary"
            disabled={login.isPending || auth.isLoading}
            type="submit"
          >
            {login.isPending ? "Signing in…" : "Sign in →"}
          </button>
        </form>
        <p className="info-note">
          🛡 Secure, cookie-based session
          <br />
          <small>Your session is protected and encrypted.</small>
        </p>
      </section>
    </div>
  );
}
