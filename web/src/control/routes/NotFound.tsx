import { Alert } from "../components/Alert";
import { Shell } from "../components/Shell";

export function NotFound() {
  return (
    <Shell active="sessions">
      <Alert>Not found.</Alert>
    </Shell>
  );
}
