import {
  initialCamera,
  nodeColour,
  nodeIdentity,
  position,
  project,
  type Camera,
  type CloudNode,
  type CloudPage,
} from "./cloud-types";
export type RenderStats = {
  frames: number;
  nodes: number;
  edges: number;
  last_ms: number;
  max_ms: number;
  context: string;
};
type Item = {
  node: CloudNode;
  point: [number, number, number];
  born: number;
  removed?: number;
};
// Geometry is a stable spatial index, not a claim about relationship distance.
// Only canonical claims supplied by the server are drawn as lines.
export class CloudRenderer {
  camera: Camera = { ...initialCamera };
  stats: RenderStats = {
    frames: 0,
    nodes: 0,
    edges: 0,
    last_ms: 0,
    max_ms: 0,
    context: "initializing",
  };
  private gl: WebGLRenderingContext | null = null;
  private program: WebGLProgram | null = null;
  private buffer: WebGLBuffer | null = null;
  private frame = 0;
  private items: Item[] = [];
  private page?: CloudPage;
  private selected = "";
  private highlights = new Set<string>();
  private claims = new Set<string>();
  private reduced = false;
  private focusedAt = 0;
  private previous?: CloudPage;
  constructor(
    private canvas: HTMLCanvasElement,
    private status: (message: string) => void,
  ) {
    canvas.addEventListener("webglcontextlost", this.lost);
    canvas.addEventListener("webglcontextrestored", this.restored);
    this.initialize();
  }
  private lost = (event: Event) => {
    event.preventDefault();
    cancelAnimationFrame(this.frame);
    this.stats.context = "lost";
    this.status(
      "Graphics context lost. Evidence navigation remains available; restoring graphics…",
    );
  };
  private restored = () => {
    this.initialize();
    this.status("Graphics restored from the current canonical page.");
    this.draw();
  };
  private initialize() {
    const gl = this.canvas.getContext("webgl", {
      alpha: false,
      antialias: true,
      preserveDrawingBuffer: true,
    });
    this.gl = gl;
    if (!gl) {
      this.stats.context = "unavailable";
      this.status("WebGL unavailable. Use evidence navigation below.");
      return;
    }
    const shader = (type: number, source: string) => {
      const s = gl.createShader(type)!;
      gl.shaderSource(s, source);
      gl.compileShader(s);
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS))
        throw new Error("Cloud shader unavailable");
      return s;
    };
    const p = gl.createProgram()!,
      v = shader(
        gl.VERTEX_SHADER,
        "attribute vec3 position; attribute vec4 colour; attribute float size; varying vec4 tint; void main(){gl_Position=vec4(position,1.0);gl_PointSize=size;tint=colour;}",
      ),
      f = shader(
        gl.FRAGMENT_SHADER,
        "precision mediump float; varying vec4 tint; uniform bool points; void main(){float a=1.0;if(points){float d=distance(gl_PointCoord,vec2(.5));if(d>.5)discard;a=1.0-smoothstep(.42,.5,d);}gl_FragColor=vec4(tint.rgb,tint.a*a);}",
      );
    gl.attachShader(p, v);
    gl.attachShader(p, f);
    gl.linkProgram(p);
    gl.deleteShader(v);
    gl.deleteShader(f);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS))
      throw new Error("Cloud program unavailable");
    this.program = p;
    this.buffer = gl.createBuffer();
    this.stats.context = "ready";
  }
  update(
    page: CloudPage,
    selected: string,
    entities: string[],
    claims: string[],
    reduced: boolean,
    comparable = true,
  ) {
    const now = performance.now(),
      sameScope = this.previous?.scope === page.scope,
      changed =
        comparable && sameScope && this.previous?.snapshot !== page.snapshot,
      old = new Map(
        this.items
          .filter((i) => !i.removed)
          .map((i) => [nodeIdentity(i.node), i]),
      );
    this.items = page.nodes.map((node) => {
      const existing = old.get(nodeIdentity(node));
      old.delete(nodeIdentity(node));
      return {
        node,
        point: position(node),
        born: changed && !existing ? now : 0,
      };
    });
    if (changed && !reduced)
      for (const item of old.values())
        this.items.push({ ...item, removed: now });
    this.page = page;
    this.previous = page;
    if (selected && this.selected !== selected) this.focusedAt = now;
    this.selected = selected;
    this.highlights = new Set(entities);
    this.claims = new Set(claims);
    this.reduced = reduced;
    this.draw();
  }
  setCamera(next: Camera) {
    this.camera = {
      ...next,
      zoom: Math.max(0.35, Math.min(5, next.zoom)),
      pitch: Math.max(-Math.PI / 2, Math.min(Math.PI / 2, next.pitch)),
      panX: Math.max(-2, Math.min(2, next.panX)),
      panY: Math.max(-2, Math.min(2, next.panY)),
    };
    this.draw();
  }
  reset() {
    this.setCamera({ ...initialCamera });
  }
  pick(x: number, y: number) {
    const box = this.canvas.getBoundingClientRect(),
      aspect = box.width / box.height;
    let best: CloudNode | undefined,
      distance = 24;
    for (const item of this.items) {
      if (item.removed) continue;
      const p = project(item.point, this.camera, aspect),
        d = Math.hypot(
          ((p.x + 1) * box.width) / 2 - x,
          ((1 - p.y) * box.height) / 2 - y,
        );
      if (d < distance) {
        distance = d;
        best = item.node;
      }
    }
    return best;
  }
  draw = () => {
    cancelAnimationFrame(this.frame);
    const gl = this.gl,
      p = this.program;
    if (!gl || !p || gl.isContextLost()) return;
    const start = performance.now(),
      box = this.canvas.getBoundingClientRect(),
      ratio = Math.min(devicePixelRatio, 2),
      w = Math.max(1, Math.round(box.width * ratio)),
      h = Math.max(1, Math.round(box.height * ratio));
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w;
      this.canvas.height = h;
    }
    gl.viewport(0, 0, w, h);
    gl.clearColor(0.035, 0.054, 0.095, 1);
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
    gl.enable(gl.DEPTH_TEST);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    gl.useProgram(p);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.buffer);
    const attr = (name: string, size: number, offset: number) => {
      const a = gl.getAttribLocation(p, name);
      gl.enableVertexAttribArray(a);
      gl.vertexAttribPointer(a, size, gl.FLOAT, false, 32, offset);
    };
    attr("position", 3, 0);
    attr("colour", 4, 12);
    attr("size", 1, 28);
    let moving =
      !this.reduced && !!this.selected && start - this.focusedAt < 600;
    const vertices: number[] = [],
      points = new Map<
        string,
        { x: number; y: number; z: number; scale: number }
      >();
    const colour = (hex: string) =>
      [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
    this.items = this.items.filter(
      (i) => !i.removed || start - i.removed < 600,
    );
    for (const item of this.items) {
      const age = this.reduced ? 1 : Math.min(1, (start - item.born) / 600),
        alpha = item.removed
          ? Math.max(0, 1 - (start - item.removed) / 600)
          : 1;
      moving ||= age < 1 || !!item.removed;
      const point = item.point.map((v) => v * (0.8 + 0.2 * age)) as [
          number,
          number,
          number,
        ],
        q = project(point, this.camera, w / h);
      points.set(item.node.handle, q);
      const selected = item.node.handle === this.selected,
        highlight = this.highlights.has(item.node.handle),
        rgb = colour(
          selected ? "#ffffff" : highlight ? "#ff80ac" : nodeColour(item.node),
        );
      vertices.push(
        q.x,
        q.y,
        q.z,
        ...rgb,
        alpha * (this.highlights.size && !highlight && !selected ? 0.3 : 1),
        (selected
          ? 24 +
            (this.reduced
              ? 0
              : Math.max(0, 1 - (start - this.focusedAt) / 600) * 12)
          : highlight
            ? 21
            : item.node.aggregate
              ? 19
              : 12) *
          ratio *
          (0.6 + q.scale),
      );
    }
    const lines: number[] = [];
    for (const edge of this.page?.edges || []) {
      const a = points.get(edge.subject),
        b = points.get(edge.object);
      if (!a || !b) continue;
      const rgb = this.claims.has(edge.handle)
        ? [1, 0.5, 0.67]
        : [0.38, 0.5, 0.65];
      for (const q of [a, b]) lines.push(q.x, q.y, q.z, ...rgb, 0.55, 1);
    }
    gl.uniform1i(gl.getUniformLocation(p, "points"), 0);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(lines), gl.DYNAMIC_DRAW);
    gl.drawArrays(gl.LINES, 0, lines.length / 8);
    gl.uniform1i(gl.getUniformLocation(p, "points"), 1);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(vertices), gl.DYNAMIC_DRAW);
    gl.drawArrays(gl.POINTS, 0, vertices.length / 8);
    gl.flush();
    this.stats = {
      frames: this.stats.frames + 1,
      nodes: this.page?.nodes.length || 0,
      edges: lines.length / 16,
      last_ms: performance.now() - start,
      max_ms: Math.max(this.stats.max_ms, performance.now() - start),
      context: "ready",
    };
    this.canvas.dataset.camera = JSON.stringify(this.camera);
    this.canvas.dataset.renderStats = JSON.stringify(this.stats);
    if (moving && !this.reduced) this.frame = requestAnimationFrame(this.draw);
  };
  destroy() {
    cancelAnimationFrame(this.frame);
    this.canvas.removeEventListener("webglcontextlost", this.lost);
    this.canvas.removeEventListener("webglcontextrestored", this.restored);
    this.gl?.deleteBuffer(this.buffer);
    this.gl?.deleteProgram(this.program);
  }
}
