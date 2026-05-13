import { useCallback, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { useCreateSession, useSessions } from "../../api/hooks";
import { Shell } from "../../components/Shell";
import { CreateSessionForm } from "./components/CreateSessionForm";
import { ProvisionCard, type ProvisioningSelection } from "./components/ProvisionCard";

export function CreateSessionPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const sessions = useSessions();
  const [provisioningSelection, setProvisioningSelection] = useState<ProvisioningSelection>({
    region: "",
    size: "",
  });
  const updateProvisioningSelection = useCallback((selection: ProvisioningSelection) => {
    setProvisioningSelection(selection);
  }, []);
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
      <p className="back">
        <Link to={{ pathname: "/sessions", search: location.search }}>← Back to sessions</Link>
      </p>
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
          options={sessions.data?.provisioning_options}
          optionsError={sessions.error}
          optionsLoading={sessions.isLoading}
          onProvisioningSelectionChange={updateProvisioningSelection}
          onSubmit={(input) => create.mutate(input)}
        />
        <ProvisionCard
          options={sessions.data?.provisioning_options}
          selection={provisioningSelection}
        />
      </div>
    </Shell>
  );
}
