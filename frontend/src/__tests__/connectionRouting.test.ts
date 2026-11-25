import { describe, expect, it } from 'vitest';
import { routeConnection, routeToPath, type Rect } from '../connectionRouting';
import { computeSessionGrid } from '../sessionLayout';

const STAGE: Rect = { left: 360, top: 22, right: 1900, bottom: 910 };
const START = { x: 333, y: 588 };
const CHANNEL_X = 352;
const TOP_PADDING = 10;
const ROW_GAP = 8;

/** Session boxes exactly as the CSS grid places them for the given count. */
function sessionRects(count: number): Rect[] {
  const grid = computeSessionGrid(count, STAGE.right - STAGE.left, STAGE.bottom - STAGE.top);
  const rowHeight = (STAGE.bottom - STAGE.top - TOP_PADDING - ROW_GAP * (grid.rows - 1)) / grid.rows;
  const unit = (STAGE.right - STAGE.left) / grid.gridCols;
  return grid.spans.map((span, i) => {
    const row = Math.floor(i / grid.cols);
    const colStart = grid.spans.slice(row * grid.cols, i).reduce((a, b) => a + b, 0);
    const left = STAGE.left + colStart * unit;
    const top = STAGE.top + TOP_PADDING + row * (rowHeight + ROW_GAP);
    return { left, top, right: left + span * unit, bottom: top + rowHeight };
  });
}

function route(count: number, targetIndex: number, start = START) {
  return routeConnection({ start, channelX: CHANNEL_X, stage: STAGE, sessions: sessionRects(count), targetIndex });
}

describe('routeConnection', () => {
  it('never crosses another session, for any count and any selected session', () => {
    for (let count = 1; count <= 12; count++) {
      for (let target = 0; target < count; target++) {
        const result = route(count, target);
        expect(result.crossings, `${count} sessions, target #${target}`).toBe(0);
        expect(result.points[0]).toEqual(START);
      }
    }
  });

  it('goes over the top and drops onto the selected session (bug6)', () => {
    const sessions = sessionRects(5);
    const target = sessions[1]; // top row, middle
    const result = route(5, 1);
    expect(result.kind).toBe('top');
    const points = result.points;
    // Out of the chat into the channel, up to the top margin, across, then down onto the session.
    expect(points.map((p) => p.x)).toEqual([START.x, CHANNEL_X, CHANNEL_X, (target.left + target.right) / 2, (target.left + target.right) / 2]);
    expect(points[2].y).toBeLessThan(target.top);
    expect(points[points.length - 1]).toEqual({ x: (target.left + target.right) / 2, y: target.top });
    // It must not run to the far right of the screen as the old path did.
    expect(Math.max(...points.map((p) => p.x))).toBeLessThan(target.right);
  });

  it('prefers the top margin even for a session in the last row', () => {
    const result = route(6, 4, { x: START.x, y: 100 }); // bottom row, middle; chat input near the top
    expect(result.kind).toBe('top');
    expect(result.crossings).toBe(0);
  });

  it('does not switch to a gap below just because it is a bit shorter', () => {
    const result = route(5, 1, { x: START.x, y: 367 }); // bug6: top row, chat input fairly high up
    expect(result.kind).toBe('top');
    expect(result.crossings).toBe(0);
  });

  it('uses the gap below when the top is blocked', () => {
    // No margin above the first row: the only clean way in is from below.
    const noTopMargin: Rect = { ...STAGE, top: 32 };
    const result = routeConnection({ start: START, channelX: CHANNEL_X, stage: noTopMargin, sessions: sessionRects(5), targetIndex: 1 });
    expect(result.kind).toBe('bottom');
    expect(result.crossings).toBe(0);
  });

  it('connects to the left edge when the session is in the first column', () => {
    const sessions = sessionRects(6);
    const target = sessions[3]; // bottom row, first column
    const result = route(6, 3);
    expect(result.kind).toBe('left');
    expect(result.points[result.points.length - 1]).toEqual({ x: target.left, y: (target.top + target.bottom) / 2 });
  });

  it('still works for stacked (mobile) sessions', () => {
    const sessions: Rect[] = [0, 1, 2].map((i) => ({ left: 360, top: 32 + i * 300, right: 1900, bottom: 332 + i * 300 }));
    const result = routeConnection({ start: START, channelX: CHANNEL_X, stage: STAGE, sessions, targetIndex: 1 });
    expect(result.kind).toBe('left');
    expect(result.crossings).toBe(0);
    expect(result.points[result.points.length - 1]).toEqual({ x: 360, y: 482 });
  });

  it('falls back to the shortest line when there is no free channel', () => {
    // Sessions touch each other and the stage: no gaps to route through.
    const sessions: Rect[] = [
      { left: 360, top: 22, right: 1130, bottom: 466 },
      { left: 1130, top: 22, right: 1900, bottom: 466 },
      { left: 360, top: 466, right: 1130, bottom: 910 },
      { left: 1130, top: 466, right: 1900, bottom: 910 },
    ];
    const result = routeConnection({ start: START, channelX: CHANNEL_X, stage: STAGE, sessions, targetIndex: 1 });
    expect(result.points.length).toBeGreaterThan(2);
  });

  it('formats a path', () => {
    expect(routeToPath([{ x: 1.04, y: 2 }, { x: 3, y: 2.5 }])).toBe('M 1 2 L 3 2.5');
  });
});
