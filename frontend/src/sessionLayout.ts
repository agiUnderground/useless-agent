/**
 * Session grid layout.
 *
 * Decides how N sessions fill the workspace stage: how many go in a row, how
 * many rows there are, and how wide each session is. The last row may be
 * incomplete; its sessions are widened so that every row is completely filled
 * and the stage has no empty area (5 sessions become 3 on top and 2 wider ones
 * below, not 3 + 1 + 1).
 *
 * Pure functions: no DOM, no React. Used by WorkspaceApp and by tests.
 */

import type { CSSProperties } from 'react';

export interface SessionGridLayout {
  /** Sessions per full row. */
  cols: number;
  rows: number;
  /**
   * Number of CSS grid columns to declare. A multiple of both `cols` and the
   * last row's item count, so every session gets a whole-number span.
   */
  gridCols: number;
  /** Column span of each session, in order. */
  spans: number[];
}

/** Assumed stage size until the real one is measured. */
export const DEFAULT_STAGE_SIZE = { width: 1600, height: 900 };

/** Sessions show a 16:9 desktop stream, so prefer cells with that shape. */
const PREFERRED_CELL_ASPECT = 16 / 9;
/** Small cost per row, so equally good layouts stay wide rather than tall. */
const COST_PER_ROW = 0.15;

const gcd = (a: number, b: number): number => (b === 0 ? a : gcd(b, a % b));
const lcm = (a: number, b: number): number => (a / gcd(a, b)) * b;

/**
 * Computes the grid for `count` sessions on a stage of the given size.
 * Either size may be 0/omitted; a default is used then.
 */
export function computeSessionGrid(
  count: number,
  stageWidth: number = 0,
  stageHeight: number = 0,
): SessionGridLayout {
  const n = Math.max(1, Math.floor(count) || 1);
  const width = stageWidth > 0 ? stageWidth : DEFAULT_STAGE_SIZE.width;
  const height = stageHeight > 0 ? stageHeight : DEFAULT_STAGE_SIZE.height;

  let bestCols = 1;
  let bestScore = Number.POSITIVE_INFINITY;
  for (let cols = 1; cols <= n; cols++) {
    const rows = Math.ceil(n / cols);
    const lastRowCount = n - cols * (rows - 1);
    // An incomplete last row must still be at least half full, otherwise the
    // widened sessions look stretched (prefer 4 + 3 over 3 + 3 + 1).
    if (rows > 1 && lastRowCount < Math.ceil(cols / 2)) continue;

    const cellAspect = width / cols / (height / rows);
    const score = Math.abs(Math.log(cellAspect / PREFERRED_CELL_ASPECT)) + COST_PER_ROW * rows;
    if (score < bestScore) {
      bestScore = score;
      bestCols = cols;
    }
  }

  const cols = bestCols;
  const rows = Math.ceil(n / cols);
  const lastRowCount = n - cols * (rows - 1);
  const incompleteLastRow = lastRowCount < cols;
  const gridCols = incompleteLastRow ? lcm(cols, lastRowCount) : cols;

  const spans: number[] = [];
  for (let i = 0; i < n; i++) {
    const inLastRow = i >= cols * (rows - 1);
    spans.push(inLastRow ? gridCols / lastRowCount : gridCols / cols);
  }
  return { cols, rows, gridCols, spans };
}

/** Inline styles that apply a layout to the `.main-content` grid. */
export function gridStyle(layout: SessionGridLayout): CSSProperties {
  return {
    gridTemplateColumns: `repeat(${layout.gridCols}, minmax(0, 1fr))`,
    gridTemplateRows: `repeat(${layout.rows}, minmax(0, 1fr))`,
  };
}
