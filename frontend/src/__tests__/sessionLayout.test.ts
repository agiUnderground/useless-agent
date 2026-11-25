import { describe, expect, it } from 'vitest';
import { computeSessionGrid } from '../sessionLayout';

/** Column span sum per row must be exactly the grid width: no empty cells. */
function rowFill(grid: ReturnType<typeof computeSessionGrid>): number[] {
  const fills: number[] = [];
  let current = 0;
  for (let i = 0; i < grid.spans.length; i++) {
    if (i > 0 && i % grid.cols === 0) {
      fills.push(current);
      current = 0;
    }
    current += grid.spans[i];
  }
  fills.push(current);
  return fills;
}

describe('computeSessionGrid', () => {
  it('fills every row completely for any session count', () => {
    for (let count = 1; count <= 24; count++) {
      for (const [w, h] of [[1550, 900], [900, 900], [2200, 700]]) {
        const grid = computeSessionGrid(count, w, h);
        expect(grid.spans, `${count} sessions on ${w}x${h}`).toHaveLength(count);
        expect(grid.rows).toBe(Math.ceil(count / grid.cols));
        for (const fill of rowFill(grid)) {
          expect(fill, `${count} sessions on ${w}x${h}`).toBe(grid.gridCols);
        }
      }
    }
  });

  it('places 5 sessions as 3 plus 2 instead of 3 + 1 + 1', () => {
    const grid = computeSessionGrid(5, 1550, 900);
    expect(grid.cols).toBe(3);
    expect(grid.rows).toBe(2);
    expect(grid.spans).toEqual([2, 2, 2, 3, 3]);
  });

  it('keeps the familiar layouts for 1 to 6 sessions', () => {
    expect(pick(computeSessionGrid(1, 1550, 900))).toEqual([1, 1, [1]]);
    expect(pick(computeSessionGrid(2, 1550, 900))).toEqual([2, 1, [1, 1]]);
    expect(pick(computeSessionGrid(3, 1550, 900))).toEqual([2, 2, [1, 1, 2]]);
    expect(pick(computeSessionGrid(4, 1550, 900))).toEqual([2, 2, [1, 1, 1, 1]]);
    expect(pick(computeSessionGrid(6, 1550, 900))).toEqual([3, 2, [1, 1, 1, 1, 1, 1]]);
  });

  it('gives all sessions of a full row the same size', () => {
    const grid = computeSessionGrid(7, 1550, 900);
    // 4 + 3: 12 grid columns, three wide each on top and four below.
    expect(grid.spans).toEqual([3, 3, 3, 3, 4, 4, 4]);
  });

  it('works without a measured stage size', () => {
    expect(() => computeSessionGrid(5)).not.toThrow();
    expect(computeSessionGrid(0).spans).toHaveLength(1);
  });
});

function pick(grid: ReturnType<typeof computeSessionGrid>) {
  return [grid.cols, grid.rows, grid.spans];
}
