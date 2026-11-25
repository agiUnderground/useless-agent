/**
 * Connection line routing.
 *
 * Draws the dashed line from the chat input to the selected session using only
 * free space: the vertical channel between the chat panel and the session
 * grid, the margin above the grid, and the gaps between session rows. Among
 * the routes that cross no other session it takes the shortest one, so the
 * line never takes a detour around the outside of the grid.
 *
 * Pure functions: no DOM, no React. ConnectionOverlay feeds it measured
 * rectangles and turns the returned points into an SVG path.
 */

export interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export interface Point {
  x: number;
  y: number;
}

export type RouteKind = 'left' | 'top' | 'bottom';

export interface RouteOptions {
  /** Where the line starts: the right edge of the chat input. */
  start: Point;
  /** x of the free vertical channel beside the session grid. */
  channelX: number;
  /** Bounding box of the session grid, including its inner padding. */
  stage: Rect;
  /** Every visible session. */
  sessions: Rect[];
  /** Index of the selected session in `sessions`. */
  targetIndex: number;
  /** Narrowest gap a line is routed through. */
  minChannel?: number;
  /** Clearance kept from session borders. */
  margin?: number;
  /** Extra cost for routing below the target, so lines prefer the top. */
  bottomBias?: number;
}

export interface Route {
  points: Point[];
  kind: RouteKind;
  /** How many (segment, other-session) intersections the route has. 0 is clean. */
  crossings: number;
  length: number;
}

// The top margin is the preferred place for the line; a route through a gap
// below the target has to be much shorter to be picked.
const DEFAULTS = { minChannel: 6, margin: 1, bottomBias: 1.6 };

interface Band {
  top: number;
  bottom: number;
}

/** Merges sessions that overlap vertically into rows (top to bottom). */
function rowBands(sessions: Rect[]): Band[] {
  const bands: Band[] = [];
  for (const s of [...sessions].sort((a, b) => a.top - b.top)) {
    const last = bands[bands.length - 1];
    if (last && s.top < last.bottom) {
      last.bottom = Math.max(last.bottom, s.bottom);
    } else {
      bands.push({ top: s.top, bottom: s.bottom });
    }
  }
  return bands;
}

function between(a: number, b: number, minChannel: number): number | null {
  return b - a >= minChannel ? (a + b) / 2 : null;
}

/** y of the free horizontal channel directly above row `index`, if wide enough. */
function channelAbove(bands: Band[], index: number, stage: Rect, minChannel: number): number | null {
  const band = bands[index];
  return index === 0 ? between(stage.top, band.top, minChannel) : between(bands[index - 1].bottom, band.top, minChannel);
}

/** y of the free horizontal channel directly below row `index`, if wide enough. */
function channelBelow(bands: Band[], index: number, stage: Rect, minChannel: number): number | null {
  const band = bands[index];
  return index === bands.length - 1
    ? between(band.bottom, stage.bottom, minChannel)
    : between(band.bottom, bands[index + 1].top, minChannel);
}

/** Whether the axis-aligned segment a-b passes through the rect's interior. */
function segmentHitsRect(a: Point, b: Point, r: Rect, margin: number): boolean {
  if (a.y === b.y) {
    return a.y > r.top + margin && a.y < r.bottom - margin && Math.max(a.x, b.x) > r.left + margin && Math.min(a.x, b.x) < r.right - margin;
  }
  if (a.x === b.x) {
    return a.x > r.left + margin && a.x < r.right - margin && Math.max(a.y, b.y) > r.top + margin && Math.min(a.y, b.y) < r.bottom - margin;
  }
  // Diagonal segments are not produced by this router; treat them conservatively.
  return true;
}

function countCrossings(points: Point[], obstacles: Rect[], margin: number): number {
  let crossings = 0;
  for (let i = 0; i + 1 < points.length; i++) {
    for (const o of obstacles) {
      if (segmentHitsRect(points[i], points[i + 1], o, margin)) crossings++;
    }
  }
  return crossings;
}

function routeLength(points: Point[]): number {
  let total = 0;
  for (let i = 0; i + 1 < points.length; i++) {
    total += Math.abs(points[i + 1].x - points[i].x) + Math.abs(points[i + 1].y - points[i].y);
  }
  return total;
}

/**
 * Routes the chat-to-session line. See the module comment.
 */
export function routeConnection(options: RouteOptions): Route {
  const { start, channelX, stage, sessions, targetIndex } = options;
  const minChannel = options.minChannel ?? DEFAULTS.minChannel;
  const margin = options.margin ?? DEFAULTS.margin;
  const bottomBias = options.bottomBias ?? DEFAULTS.bottomBias;

  const target = sessions[targetIndex];
  if (!target) {
    return { points: [start], kind: 'left', crossings: 0, length: 0 };
  }

  const others = sessions.filter((_, i) => i !== targetIndex);
  const cx = (target.left + target.right) / 2;
  const cy = (target.top + target.bottom) / 2;

  // Every route leaves the chat input horizontally into the vertical channel.
  const toChannel = (y: number): Point[] => [start, { x: channelX, y: start.y }, { x: channelX, y }];

  const candidates: Array<{ points: Point[]; kind: RouteKind }> = [
    // Into the left edge: only clean when nothing sits between channel and target.
    { kind: 'left', points: [...toChannel(cy), { x: target.left, y: cy }] },
  ];

  const bands = rowBands(sessions);
  let bandIndex = bands.findIndex((b) => cy >= b.top && cy <= b.bottom);
  if (bandIndex === -1) bandIndex = 0;

  const above = channelAbove(bands, bandIndex, stage, minChannel);
  if (above !== null) {
    candidates.push({ kind: 'top', points: [...toChannel(above), { x: cx, y: above }, { x: cx, y: target.top }] });
  }
  const below = channelBelow(bands, bandIndex, stage, minChannel);
  if (below !== null) {
    candidates.push({ kind: 'bottom', points: [...toChannel(below), { x: cx, y: below }, { x: cx, y: target.bottom }] });
  }

  let best: Route | null = null;
  let bestCost = Number.POSITIVE_INFINITY;
  for (const candidate of candidates) {
    const crossings = countCrossings(candidate.points, others, margin);
    const length = routeLength(candidate.points);
    // Clean routes always win; among equals the shorter one, with the bottom
    // route charged extra so lines prefer the top margin.
    const cost = crossings * 1e9 + length * (candidate.kind === 'bottom' ? bottomBias : 1);
    if (cost < bestCost) {
      bestCost = cost;
      best = { points: candidate.points, kind: candidate.kind, crossings, length };
    }
  }
  return best as Route;
}

/** Formats route points as an SVG path (`M x y L x y ...`). */
export function routeToPath(points: Point[]): string {
  return points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${Math.round(p.x * 10) / 10} ${Math.round(p.y * 10) / 10}`)
    .join(' ');
}
