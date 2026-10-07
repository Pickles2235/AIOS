import React, { useEffect, useRef, useState } from "react";
import type { Claim, Entity, Investigation } from "./runtime-types";
import type { ActivityView } from "./contextual-modules";
import { CloudRenderer } from "./cloud-renderer";
import {
  nodeIdentity,
  semanticColours,
  type CloudNode,
  type CloudPage,
} from "./cloud-types";
type Request = <T>(
  path: string,
  body?: unknown,
  signal?: AbortSignal,
) => Promise<T>;
const labels = {
  estate: "Repositories",
  repository: "Packages / directories",
  package: "Files",
  file: "Symbols",
};
export function KnowledgeCloud({
  request,
  selected,
  inspect,
  inspectClaim,
  onPromotion,
  investigation,
  resultHandles = [],
  activity,
}: {
  request: Request;
  selected?: string;
  inspect: (entity: Entity) => void;
  inspectClaim: (claim: Claim) => void;
  onPromotion: (invalidate: boolean) => void;
  investigation?: Investigation;
  resultHandles?: string[];
  activity: ActivityView;
}) {
  const [page, setPage] = useState<CloudPage>(),
    [error, setError] = useState(""),
    [graphics, setGraphics] = useState(""),
    [busy, setBusy] = useState(false),
    [accessible, setAccessible] = useState(true),
    [picked, setPicked] = useState<CloudNode>(),
    [hovered, setHovered] = useState<CloudNode>(),
    [reduced, setReduced] = useState(false),
    [motion, setMotion] = useState(""),
    [trail, setTrail] = useState<{ scope: string; label: string }[]>([]);
  const canvas = useRef<HTMLCanvasElement>(null),
    renderer = useRef<CloudRenderer>(),
    current = useRef<CloudPage>(),
    requestID = useRef(0),
    inFlight = useRef(false),
    pageCursor = useRef(""),
    comparable = useRef(true),
    drag = useRef<{
      x: number;
      y: number;
      startX: number;
      startY: number;
      pan: boolean;
    }>(),
    requestRef = useRef(request);
  requestRef.current = request;
  current.current = page;
  async function load(
    scope = "",
    snapshot = "",
    cursor = "",
    navigation = false,
    poll = false,
    entity = "",
  ) {
    if (poll && inFlight.current) return;
    inFlight.current = true;
    const id = ++requestID.current;
    setBusy(true);
    try {
      const query = new URLSearchParams({ scope, limit: "256" });
      if (snapshot) query.set("snapshot", snapshot);
      if (cursor) query.set("cursor", cursor);
      if (entity) query.set("entity", entity);
      const next = await requestRef.current<CloudPage>(
        "/api/v1/cloud?" + query,
      );
      if (id !== requestID.current) return;
      const previous = current.current;
      if (previous && previous.snapshot !== next.snapshot) onPromotion(false);
      if (poll && previous?.snapshot === next.snapshot) {
        setError("");
        return;
      }
      comparable.current = !pageCursor.current && !cursor && !entity;
      if (
        previous?.scope === next.scope &&
        previous.snapshot !== next.snapshot
      ) {
        const before = new Set(previous.nodes.map(nodeIdentity)),
          after = new Set(next.nodes.map(nodeIdentity));
        setMotion(
          !comparable.current
            ? "Generation changed. This bounded page was replaced; page differences are not a knowledge delta."
            : `Atomic promotion observed · ${next.nodes.filter((n) => !before.has(nodeIdentity(n))).length} added · ${previous.nodes.filter((n) => !after.has(nodeIdentity(n))).length} removed from these bounded pages. Counts do not describe unseen pages.`,
        );
        setPicked(undefined);
      } else if (navigation) setMotion("");
      pageCursor.current = cursor || (entity ? "focused" : "");
      setPage(next);
      setError("");
    } catch (e) {
      if (id !== requestID.current) return;
      // A removed scope cannot return a newer snapshot. Invalidate consumers
      // before attempting recovery so the inspector never keeps old evidence.
      onPromotion(true);
      setPicked(undefined);
      setError(
        `Cloud page unavailable or stale. ${e instanceof Error ? e.message : "Refresh to retry."}`,
      );
      if ((e as { status?: number }).status === 409 && scope) {
        try {
          const estate = await requestRef.current<CloudPage>(
            "/api/v1/cloud?limit=256",
          );
          if (id !== requestID.current) return;
          comparable.current = false;
          pageCursor.current = "";
          setPage(estate);
          setTrail([]);
          setError("");
          setMotion(
            "The selected scope changed or was removed. Returned to the current estate; previous evidence was cleared.",
          );
        } catch {
          /* Retain the labelled page with selection disabled. */
        }
      }
    } finally {
      if (id === requestID.current) {
        inFlight.current = false;
        setBusy(false);
      }
    }
  }
  useEffect(() => {
    void load();
    const timer = window.setInterval(() => {
      if (!document.hidden)
        void load(current.current?.scope || "", "", "", false, true);
    }, 4000);
    return () => {
      requestID.current++;
      clearInterval(timer);
    };
  }, []);
  useEffect(() => {
    const media = matchMedia("(prefers-reduced-motion: reduce)"),
      update = () => setReduced(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  useEffect(() => {
    if (
      selected &&
      !current.current?.nodes.some((node) => node.handle === selected)
    ) {
      setTrail([]);
      void load("", "", "", true, false, selected);
    }
  }, [selected]);
  const hasNodes = !!page?.nodes.length;
  useEffect(() => {
    if (!canvas.current) return;
    try {
      renderer.current = new CloudRenderer(canvas.current, setGraphics);
    } catch {
      setGraphics(
        "Graphics initialization failed. Evidence navigation remains available.",
      );
    }
    const resize = new ResizeObserver(() => renderer.current?.draw());
    resize.observe(canvas.current);
    return () => {
      resize.disconnect();
      renderer.current?.destroy();
      renderer.current = undefined;
    };
  }, [hasNodes]);
  useEffect(() => {
    if (page)
      renderer.current?.update(
        page,
        selected || picked?.handle || "",
        [
          ...resultHandles,
          ...(investigation?.findings.map((f) => f.entity.handle) || []),
        ],
        investigation?.relationships.map(
          (r) => (r as { handle?: string }).handle || "",
        ) || [],
        reduced,
        comparable.current,
      );
  }, [page, selected, picked, resultHandles.join("|"), investigation, reduced]);
  const choose = (node: CloudNode) => {
    if (error) return;
    setPicked(node);
    if (node.entity) inspect(node.entity);
  };
  async function expand(node: CloudNode) {
    if (!page || !node.child_scope || error) return;
    setTrail([...trail, { scope: page.scope, label: page.label }]);
    setPicked(undefined);
    await load(node.child_scope, page.snapshot, "", true);
  }
  const camera = (operation: (r: CloudRenderer) => void) => {
    if (renderer.current) operation(renderer.current);
  };
  const activities = activity.events.slice(-8),
    lastActivity = activities[activities.length - 1],
    staged = activities.filter((e) => !e.queryable && e.stage !== "failed");
  return (
    <section
      aria-label="Knowledge cloud"
      className="volumetric-cloud"
      data-motion={reduced ? "reduced" : "observed-deltas"}
    >
      <header>
        <div>
          <p className="eyebrow">Canonical knowledge</p>
          <h2>Knowledge cloud</h2>
        </div>
        <button
          onClick={() => {
            setTrail([]);
            void load("", "", "", true);
          }}
        >
          Estate overview
        </button>
        <button onClick={() => void load(page?.scope || "")}>
          Refresh cloud
        </button>
      </header>
      {!page && <p role="status">{error || "Reading active knowledge…"}</p>}
      {page && (
        <>
          <nav aria-label="Cloud hierarchy">
            {trail.map((step, i) => (
              <button
                key={step.scope}
                onClick={() => {
                  setTrail(trail.slice(0, i));
                  void load(step.scope, page.snapshot, "", true);
                }}
              >
                {step.label || "Estate"}
              </button>
            ))}
          </nav>
          <h3 aria-label="Cloud scope">
            {labels[page.level]} · {page.label || "Estate"}
          </h3>
          <p aria-label="Cloud membership disclosure">
            {page.nodes.length} of {page.total_nodes} items on this page ·{" "}
            {page.total_entities} canonical entities in {page.total_files}{" "}
            captured files in this scope. {page.count_scope}{" "}
            {page.truncated ? "More data is hidden by page bounds." : ""}
          </p>
          <details>
            <summary>Generation provenance and coverage</summary>
            {page.generations.map((g) => (
              <p key={g.id}>
                {g.id} · generation {g.generation} · revision {g.revision}
              </p>
            ))}
            <p>
              {page.coverage.complete
                ? "Complete captured scope"
                : "Partial captured coverage; missing knowledge is unknown"}
            </p>
            {page.coverage.uncertainty?.map((x) => (
              <p key={x}>{x}</p>
            ))}
          </details>
          {error && (
            <p role="alert">
              {error} The previous page is retained for reference; selection is
              disabled.
            </p>
          )}
          {motion && (
            <p role="status" aria-label="Cloud generation change">
              {motion}
              {reduced ? " Reduced motion: positions change immediately." : ""}
            </p>
          )}
          {!hasNodes ? (
            <p role="status">
              No active cloud in this scope. Staged work is not active evidence.
            </p>
          ) : (
            <>
              <div className="cloud-tools" aria-label="Cloud camera controls">
                <button
                  onClick={() =>
                    camera((r) =>
                      r.setCamera({ ...r.camera, yaw: r.camera.yaw - 0.2 }),
                    )
                  }
                >
                  Rotate left
                </button>
                <button
                  onClick={() =>
                    camera((r) =>
                      r.setCamera({ ...r.camera, yaw: r.camera.yaw + 0.2 }),
                    )
                  }
                >
                  Rotate right
                </button>
                <button
                  onClick={() =>
                    camera((r) =>
                      r.setCamera({ ...r.camera, panX: r.camera.panX + 0.1 }),
                    )
                  }
                >
                  Pan right
                </button>
                <button
                  onClick={() =>
                    camera((r) =>
                      r.setCamera({ ...r.camera, zoom: r.camera.zoom * 1.2 }),
                    )
                  }
                >
                  Zoom in
                </button>
                <button
                  onClick={() =>
                    camera((r) =>
                      r.setCamera({ ...r.camera, zoom: r.camera.zoom / 1.2 }),
                    )
                  }
                >
                  Zoom out
                </button>
                <button onClick={() => camera((r) => r.reset())}>
                  Reset camera
                </button>
              </div>
              <div className="cloud-viewport">
                <canvas
                  ref={canvas}
                  data-knowledge-cloud="canonical"
                  tabIndex={0}
                  role="application"
                  aria-label="Volumetric knowledge view"
                  aria-describedby="cloud-instructions"
                  onPointerDown={(e) => {
                    e.currentTarget.setPointerCapture(e.pointerId);
                    drag.current = {
                      x: e.clientX,
                      y: e.clientY,
                      startX: e.clientX,
                      startY: e.clientY,
                      pan: e.shiftKey || e.button === 1,
                    };
                  }}
                  onPointerMove={(e) => {
                    const d = drag.current;
                    if (!d) {
                      const box = e.currentTarget.getBoundingClientRect();
                      setHovered(
                        renderer.current?.pick(
                          e.clientX - box.left,
                          e.clientY - box.top,
                        ),
                      );
                      return;
                    }
                    const dx = e.clientX - d.x,
                      dy = e.clientY - d.y;
                    camera((r) =>
                      r.setCamera(
                        d.pan
                          ? {
                              ...r.camera,
                              panX:
                                r.camera.panX +
                                (dx / e.currentTarget.clientWidth) * 2,
                              panY:
                                r.camera.panY -
                                (dy / e.currentTarget.clientHeight) * 2,
                            }
                          : {
                              ...r.camera,
                              yaw: r.camera.yaw + dx * 0.009,
                              pitch: r.camera.pitch + dy * 0.009,
                            },
                      ),
                    );
                    d.x = e.clientX;
                    d.y = e.clientY;
                  }}
                  onPointerUp={(e) => {
                    const d = drag.current;
                    drag.current = undefined;
                    if (
                      d &&
                      Math.hypot(e.clientX - d.startX, e.clientY - d.startY) < 5
                    ) {
                      const box = e.currentTarget.getBoundingClientRect(),
                        node = renderer.current?.pick(
                          e.clientX - box.left,
                          e.clientY - box.top,
                        );
                      if (node) choose(node);
                    }
                  }}
                  onPointerCancel={() => {
                    drag.current = undefined;
                  }}
                  onWheel={(e) =>
                    camera((r) =>
                      r.setCamera({
                        ...r.camera,
                        zoom: r.camera.zoom * Math.exp(-e.deltaY * 0.001),
                      }),
                    )
                  }
                  onKeyDown={(e) => {
                    if (
                      [
                        "ArrowLeft",
                        "ArrowRight",
                        "ArrowUp",
                        "ArrowDown",
                        "+",
                        "-",
                        "Home",
                      ].includes(e.key)
                    ) {
                      e.preventDefault();
                      camera((r) => {
                        const c = { ...r.camera };
                        if (e.key === "Home") return r.reset();
                        if (e.key === "+") c.zoom *= 1.2;
                        else if (e.key === "-") c.zoom /= 1.2;
                        else if (e.shiftKey) {
                          c.panX +=
                            e.key === "ArrowRight"
                              ? 0.1
                              : e.key === "ArrowLeft"
                                ? -0.1
                                : 0;
                          c.panY +=
                            e.key === "ArrowUp"
                              ? 0.1
                              : e.key === "ArrowDown"
                                ? -0.1
                                : 0;
                        } else {
                          c.yaw +=
                            e.key === "ArrowRight"
                              ? 0.15
                              : e.key === "ArrowLeft"
                                ? -0.15
                                : 0;
                          c.pitch +=
                            e.key === "ArrowDown"
                              ? 0.15
                              : e.key === "ArrowUp"
                                ? -0.15
                                : 0;
                        }
                        r.setCamera(c);
                      });
                    }
                  }}
                />
                {hovered && (
                  <div className="cloud-hover">
                    {hovered.label}
                    <span>
                      {hovered.aggregate
                        ? `${hovered.member_count} canonical entities`
                        : hovered.entity?.kind}
                    </span>
                  </div>
                )}
                <div className="cloud-caption">
                  {labels[page.level]}
                  <span>{page.nodes.length} visible · perspective depth</span>
                </div>
              </div>
              <p id="cloud-instructions">
                Drag to rotate · Shift-drag to pan · Scroll to zoom · Arrow keys
                rotate, Shift-arrows pan, +/− zoom, Home resets. Select a point
                or use the evidence list. Geometry indicates layout, not causal
                distance.
              </p>
              <ul aria-label="Cloud colour legend" className="cloud-legend">
                {Object.entries(semanticColours).map(([kind, colour]) => (
                  <li key={kind}>
                    <span style={{ background: colour }} />
                    {kind === "directory"
                      ? "package / directory aggregate"
                      : kind}
                  </li>
                ))}
              </ul>
              <p aria-label="Cloud performance">
                Bounded detail: at most 256 nodes per page. Frames render on
                interaction or observed changes; no idle animation. Dense scopes
                use paging.
              </p>
              {graphics && <p role="status">{graphics}</p>}
              {(resultHandles.length > 0 || investigation) && (
                <p aria-label="Cloud query disclosure">
                  Pink marks actual returned entities and claims present on this
                  page. Results elsewhere remain in Spotlight. Matching results
                  do not establish a causal path.
                </p>
              )}
              <button
                aria-expanded={accessible}
                onClick={() => setAccessible(!accessible)}
              >
                Accessible evidence navigation
              </button>
              <div aria-label="Evidence navigation" hidden={!accessible}>
                <ul className="cloud-member-list">
                  {page.nodes.map((node) => (
                    <li key={node.handle}>
                      <button
                        disabled={!!error}
                        aria-pressed={
                          picked?.handle === node.handle ||
                          selected === node.handle
                        }
                        onClick={() => choose(node)}
                      >
                        {node.label}
                      </button>
                      <span>
                        {node.aggregate
                          ? `Explicit ${node.kind} aggregate · ${node.member_count} entities · ${node.file_count} files`
                          : node.entity?.kind}
                      </span>
                      {node.child_scope && (
                        <button
                          disabled={busy || !!error}
                          onClick={() => void expand(node)}
                        >
                          Explore {node.label}
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
              {picked && (
                <section aria-label="Cloud selection">
                  <h3>{picked.label}</h3>
                  <p>
                    {picked.aggregate
                      ? `Explicit aggregate; ${picked.member_count} canonical entity members in ${picked.file_count} captured files. Expand and page to enumerate membership.`
                      : "Canonical entity; source evidence appears in the inspector."}
                  </p>
                  <p>
                    {picked.repository}/{picked.path} · generation{" "}
                    {picked.generation}
                  </p>
                  {picked.child_scope && (
                    <button
                      disabled={!!error}
                      onClick={() => void expand(picked)}
                    >
                      Expand selection
                    </button>
                  )}
                </section>
              )}
              <details>
                <summary>Relationships ({page.edges.length} shown)</summary>
                <p>
                  {page.edge_count} canonical claims connect visible page
                  entities; {page.total_claims} canonical claims in the full
                  scope. Claims with hidden endpoints are not drawn. Aggregate
                  membership is not a causal relationship.
                  {page.edges_truncated &&
                    " The visible relationship payload is bounded; additional connecting claims are hidden."}
                </p>
                <ul>
                  {page.edges.map((edge) => (
                    <li key={edge.handle}>
                      {edge.predicate} · confidence {edge.confidence} ·{" "}
                      {edge.derivation} · canonical claim {edge.handle}
                      <button onClick={() => inspectClaim(edge)}>
                        Inspect relationship evidence
                      </button>
                    </li>
                  ))}
                </ul>
              </details>
              {page.next_cursor && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void load(page.scope, page.snapshot, page.next_cursor, true)
                  }
                >
                  Next cloud page
                </button>
              )}
            </>
          )}
        </>
      )}
      {(staged.length > 0 || activity.warning) && (
        <section aria-label="Staged cloud activity" className="cloud-staging">
          {activity.warning && <p role="alert">{activity.warning}</p>}
          <h3>
            {lastActivity?.queryable
              ? "Build activity history"
              : "Staging knowledge"}
          </h3>
          <p>
            Observed build events only. File counts are progress observations,
            not active entity counts. Staged data cannot answer queries.
          </p>
          <ul>
            {activities.map((event) => (
              <li key={event.sequence} data-stage={event.stage}>
                {event.repository} · {event.stage}
                {event.files > 0
                  ? ` · ${event.files} observed files`
                  : ""} ·{" "}
                {event.queryable
                  ? "promotion reported; active cloud verifies generation"
                  : "not queryable"}
              </li>
            ))}
          </ul>
        </section>
      )}
    </section>
  );
}
