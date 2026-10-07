import React, { useEffect, useState } from "react";
import type { SourceActions as Actions } from "./cloud-types";
export function SourceActions({
  handle,
  request,
}: {
  handle: string;
  request: <T>(path: string, body?: unknown) => Promise<T>;
}) {
  const [actions, setActions] = useState<Actions>(),
    [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    setActions(undefined);
    setError("");
    request<Actions>("/api/v1/source-actions", { handle })
      .then((a) => {
        if (live) setActions(a);
      })
      .catch(() => {
        if (live)
          setError(
            "Source actions unavailable or stale; select current evidence again.",
          );
      });
    return () => {
      live = false;
    };
  }, [handle]);
  // Revalidate immediately before following any action. Render no stale href that
  // could bypass the backend validation after a workspace/generation change.
  async function open(kind: string) {
    try {
      const next = await request<Actions>("/api/v1/source-actions", { handle });
      setActions(next);
      const action = next.actions.find((a) => a.kind === kind);
      if (!action?.available || !action.url) {
        setError(action?.reason || "Source action no longer available.");
        return;
      }
      if (kind === "finder" || kind === "editor") {
        await request<{ opened: boolean }>("/api/v1/source-actions/open", {
          handle,
          kind,
        });
        setError(
          kind === "finder"
            ? "Finder reveal requested for the validated source."
            : "Editor open requested for the validated source.",
        );
      } else {
        window.location.assign(action.url);
      }
    } catch {
      setError(
        "Source action stale or unavailable. Select current evidence again.",
      );
    }
  }
  return (
    <section aria-label="Revision-aware source actions">
      <h3>Open captured source</h3>
      {actions && (
        <>
          <p>
            Validated generation {actions.generation} · revision{" "}
            {actions.revision}
          </p>
          <ul>
            {actions.actions.map((action) => (
              <li key={action.kind}>
                <button
                  disabled={!action.available}
                  onClick={() => void open(action.kind)}
                >
                  {action.label}
                </button>
                {!action.available && <span> {action.reason}</span>}
              </li>
            ))}
          </ul>
        </>
      )}
      {error && <p role="status">{error}</p>}
    </section>
  );
}
