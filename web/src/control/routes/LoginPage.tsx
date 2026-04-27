import { Logo } from "../components/Shell";

export function LoginPage() {
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
        <label>
          Username
          <input autoFocus placeholder="Enter your username" />
        </label>
        <label>
          Password
          <input type="password" placeholder="Enter your password" />
        </label>
        <button className="primary">Sign in →</button>
        <p className="info-note">
          🛡 Secure, cookie-based session
          <br />
          <small>Your session is protected and encrypted.</small>
        </p>
      </section>
    </div>
  );
}
