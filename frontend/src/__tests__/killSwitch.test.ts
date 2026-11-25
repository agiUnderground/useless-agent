import { describe, it, expect, vi } from 'vitest';
import { runKillSwitch } from '../killSwitch';
import { WorkspaceData } from '../api';

const now = Date.now();

function makeWorkspaces(): WorkspaceData[] {
  return [
    {
      id: 'w1',
      name: 'Workspace One',
      isDefault: true,
      createdAt: now,
      updatedAt: now,
      sessions: [
        { id: 's1', ip: '10.0.0.1', name: 'Session 1', selected: true, maximized: false },
      ],
      tasks: [
        {
          id: 't1',
          sessionId: 's1',
          sessionIp: '10.0.0.1',
          status: 'in-progress',
          message: 'Running',
        },
        {
          id: 't2',
          sessionId: 's1',
          sessionIp: '10.0.0.1',
          status: 'completed',
          message: 'Done',
        },
      ],
    },
    {
      id: 'w2',
      name: 'Workspace Two',
      isDefault: false,
      createdAt: now,
      updatedAt: now,
      sessions: [
        { id: 's2', ip: '10.0.0.2', name: 'Session 2', selected: true, maximized: false },
      ],
      tasks: [
        {
          id: 't3',
          sessionId: 's2',
          sessionIp: '10.0.0.2',
          status: 'in-the-queue',
          message: 'Queued',
        },
      ],
    },
  ];
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

type FetchHandler = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

function mockFetch(handler: FetchHandler) {
  return vi.fn(handler) as unknown as typeof fetch;
}

describe('runKillSwitch', () => {
  it('cancels active and unpersisted live tasks across all workspaces', async () => {
    const cancelCalls: string[] = [];
    const fetchMock = mockFetch(async (input, init) => {
      const url = String(input);
      if (url.includes('/execution-state')) {
        const ip = new URL(url).hostname;
        if (ip === '10.0.0.1') {
          return jsonResponse({
            tasks: [
              { id: 't1', status: 'in-progress', message: 'Running' },
              { id: 't9', status: 'in-progress', message: 'Runaway' },
            ],
          });
        }
        return jsonResponse({ tasks: [{ id: 't3', status: 'in-the-queue' }] });
      }
      if (url.includes('/task-cancel')) {
        const taskId = new URL(url).searchParams.get('taskId') || '';
        cancelCalls.push(taskId);
        expect(init?.method).toBe('POST');
        return jsonResponse([{ result: 'Task canceled successfully', taskId }]);
      }
      throw new Error(`Unexpected request: ${url}`);
    });

    const report = await runKillSwitch(makeWorkspaces(), fetchMock);

    expect(cancelCalls.sort()).toEqual(['t1', 't3', 't9']);
    expect(report.canceled.map((task) => task.taskId).sort()).toEqual(['t1', 't3', 't9']);
    expect(report.failed).toBe(0);
    expect(report.alreadyStopped).toBe(0);
    expect(report.workspaceCount).toBe(2);
    expect(report.canceled.find((task) => task.taskId === 't9')?.workspaceId).toBe('w1');
  });

  it('does not count tasks that had already stopped', async () => {
    const fetchMock = mockFetch(async (input) => {
      const url = String(input);
      if (url.includes('/execution-state')) {
        return jsonResponse({ tasks: [] });
      }
      const taskId = new URL(url).searchParams.get('taskId');
      return jsonResponse([
        { result: 'Task not found or already completed/canceled', taskId },
      ]);
    });

    const report = await runKillSwitch(makeWorkspaces(), fetchMock);

    expect(report.canceled).toHaveLength(0);
    expect(report.alreadyStopped).toBe(2);
    expect(report.failed).toBe(0);
    expect(report.workspaceCount).toBe(0);
  });

  it('reports failed cancellations without losing successful ones', async () => {
    const fetchMock = mockFetch(async (input) => {
      const url = String(input);
      if (url.includes('/execution-state')) {
        return jsonResponse({ tasks: [] });
      }
      const taskId = new URL(url).searchParams.get('taskId');
      if (taskId === 't3') {
        return jsonResponse({ error: 'boom' }, 500);
      }
      return jsonResponse([{ result: 'Task canceled successfully', taskId }]);
    });

    const report = await runKillSwitch(makeWorkspaces(), fetchMock);

    expect(report.canceled.map((task) => task.taskId)).toEqual(['t1']);
    expect(report.failed).toBe(1);
    expect(report.workspaceCount).toBe(1);
  });
});
