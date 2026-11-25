import React from 'react';
import { describe, it, expect, beforeAll, afterEach, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import App from '../App';

const now = Date.now();

const workspaces = [
  {
    id: 'w1',
    name: 'Workspace One',
    isDefault: true,
    fps: 5,
    tokenUsage: 0,
    networkUsage: 0,
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
        message: 'Task one',
        sequenceNumber: 1,
        createdAt: now,
      },
    ],
  },
  {
    id: 'w2',
    name: 'Workspace Two',
    isDefault: false,
    fps: 5,
    tokenUsage: 0,
    networkUsage: 0,
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
        message: 'Task three',
        sequenceNumber: 1,
        createdAt: now,
      },
    ],
  },
];

const cancelCalls: string[] = [];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

beforeAll(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  );
  vi.stubGlobal(
    'WebSocket',
    class {
      close() {}
      addEventListener() {}
      removeEventListener() {}
      send() {}
    }
  );

  localStorage.setItem(
    'ua_auth',
    JSON.stringify({
      token: 'test-token',
      expiresAt: now + 86400000,
      user: { id: 'u1', username: 'tester', activeWorkspaceId: 'w1' },
    })
  );

  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/api/auth/me')) {
        return jsonResponse({
          user: { id: 'u1', username: 'tester', activeWorkspaceId: 'w1' },
          expiresAt: now + 86400000,
        });
      }
      if (url.includes('/api/workspaces') && !url.includes('/state')) {
        return jsonResponse({ workspaces });
      }
      if (url.includes('/api/inventory')) {
        return jsonResponse({ servers: [] });
      }
      if (url.includes('/api/capabilities')) {
        return jsonResponse({ disposableContainers: false });
      }
      if (url.includes('/execution-state')) {
        return jsonResponse({ tasks: [] });
      }
      if (url.includes('/task-cancel')) {
        const taskId = new URL(url).searchParams.get('taskId') || '';
        cancelCalls.push(taskId);
        return jsonResponse([{ result: 'Task canceled successfully', taskId }]);
      }
      if (url.includes('/models')) {
        return jsonResponse({ models: [] });
      }
      return jsonResponse({ result: 'ok' });
    })
  );
});

afterEach(() => {
  cancelCalls.length = 0;
});

describe('kill switch', () => {
  it('confirms, cancels tasks across workspaces, and reports the count', async () => {
    render(<App />);

    await screen.findByText('Workspace One');

    fireEvent.click(screen.getByRole('button', { name: /kill switch/i }));
    await screen.findByText('Cancel all tasks?');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel all tasks' }));

    await screen.findByText('Canceled 2 tasks');
    expect(screen.getByText('Across 2 workspaces.')).toBeTruthy();
    expect(cancelCalls.sort()).toEqual(['t1', 't3']);
  });
});
