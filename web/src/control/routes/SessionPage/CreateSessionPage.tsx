import { useLocation, useNavigate } from "react-router";
import { useCreateSession } from "../../api/hooks";
import { Shell } from "../../components/Shell";
import { CreateSessionForm } from "./components/CreateSessionForm";
import { ProvisionCard } from "./components/ProvisionCard";

export function CreateSessionPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const create = useCreateSession({
    onSuccess: (created) => {
      navigate(
        { pathname: `/sessions/${created.session.id}`, search: location.search },
        { state: { created } },
      );
    },
  });

  return (
    <Shell active="sessions">
      <div className="page-head narrow-head">
        <div>
          <h1>Create session</h1>
          <p className="lead">
            Session creation returns immediately. Provisioning continues in the background and your
            session will be ready in a few minutes.
          </p>
        </div>
      </div>
      <div className="create-grid">
        <CreateSessionForm
          busy={create.isPending}
          error={create.error}
          onSubmit={(input) => create.mutate(input)}
        />
        <ProvisionCard />
      </div>
    </Shell>
  );
}
