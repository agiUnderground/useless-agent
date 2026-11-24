import React from 'react';
import { describe, it, expect, beforeAll, afterEach, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
      { id: 's1', ip: '127.0.0.1', name: 'Session 1', selected: true, maximized: false },
    ],
    tasks: [
      {
        id: 't1',
        sessionId: 's1',
        sessionIp: '127.0.0.1',
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
    sessions: [],
    tasks: [],
  },
];

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
        return jsonResponse({ user: { id: 'u1', username: 'tester', activeWorkspaceId: 'w1' }, expiresAt: now + 86400000 });
      }
      if (url.includes('/api/workspaces') && !url.includes('/state')) {
        return jsonResponse({ workspaces });
      }
      if (url.includes('/api/inventory')) {
        return jsonResponse({ servers: [] });
      }
      if (url.includes('/execution-state')) {
        return jsonResponse({ tasks: [] });
      }
      return jsonResponse({ result: 'ok' });
    })
  );
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('workspace switching', () => {
  it('preserves user-assist task selection, scroll, and golden line', async () => {
    const user = userEvent.setup();
    render(<App />);

    // Enter the expanded workspace.
    const frame = await waitFor(() => {
      const el = document.querySelector('.workspace-preview-frame') as HTMLElement | null;
      expect(el).not.toBeNull();
      return el as HTMLElement;
    });
    fireEvent.click(frame);
    await waitFor(() =>
      expect(document.querySelector('.workspace-expanded-root')).not.toBeNull()
    );
    const task = await screen.findByTestId('t1');
    await user.click(task);

    // Activation: golden main connection + golden user-assist line.
    await waitFor(() => {
      const paths = Array.from(document.querySelectorAll('#connectionSvg path'));
      expect(paths.some(p => p.getAttribute('stroke') === '#FFC107')).toBe(true);
    });
    expect(task.className).toContain('user-assist-task');

    // Switch away and back.
    await user.click(screen.getByRole('button', { name: /Workspace Two/i }));
    await waitFor(() => expect(screen.queryByTestId('t1')).toBeNull());
    await user.click(screen.getByRole('button', { name: /Workspace One/i }));

    const restoredTask = await screen.findByTestId('t1');
    await waitFor(() => {
      expect(restoredTask.className).toContain('user-assist-task');
      const paths = Array.from(document.querySelectorAll('#connectionSvg path'));
      expect(paths.some(p => p.getAttribute('stroke') === '#FFC107')).toBe(true);
    });
  });
});
