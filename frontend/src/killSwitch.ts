import { CONFIG } from './config';
import { WorkspaceData } from './api';

/** Task statuses that are still live and can be canceled. */
export const ACTIVE_TASK_STATUSES = ['in-the-queue', 'in-progress', 'paused'] as const;

export function isActiveTaskStatus(status: string): boolean {
  return (ACTIVE_TASK_STATUSES as readonly string[]).includes(status);
}

export interface KillSwitchTarget {
  taskId: string;
  sessionIp: string;
  workspaceId: string;
  sessionId?: string;
  message?: string;
  model?: string;
  thinkingLevel?: string;
  createdAt?: number;
}

export interface CanceledTask {
  taskId: string;
  workspaceId: string;
  sessionId?: string;
  sessionIp: string;
  message: string;
  model?: string;
  thinkingLevel?: string;
  createdAt?: number;
}

export interface KillSwitchReport {
  /** Tasks this kill switch actually canceled. */
  canceled: CanceledTask[];
  /** Tasks whose cancel request failed (server offline, network error, ...). */
  failed: number;
  /** Tasks that were already completed/canceled when the request arrived. */
  alreadyStopped: number;
  /** Number of workspaces that had at least one canceled task. */
  workspaceCount: number;
}

type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

/**
 * Cancels every queued/running/paused task in every workspace.
 *
 * Saved workspace tasks are used as the baseline, but each session server is
 * also asked for its live `/execution-state` so tasks that were never
 * persisted (runaway tasks) are killed too.
 */
export async function runKillSwitch(
  workspaces: WorkspaceData[],
  fetchImpl: FetchLike = fetch
): Promise<KillSwitchReport> {
  // Every known session server, mapped to a workspace/session that owns it.
  const sessionOwners = new Map<string, { workspaceId: string; sessionId: string }>();
  // Task id -> cancel target (deduped across workspaces).
  const targets = new Map<string, KillSwitchTarget>();

  for (const workspace of workspaces) {
    for (const session of workspace.sessions) {
      if (session.ip && !sessionOwners.has(session.ip)) {
        sessionOwners.set(session.ip, { workspaceId: workspace.id, sessionId: session.id });
      }
    }
    for (const task of workspace.tasks) {
      if (!task.id || !isActiveTaskStatus(task.status)) continue;
      const sessionIp =
        task.sessionIp ||
        workspace.sessions.find((session) => session.id === task.sessionId)?.ip;
      if (!sessionIp) continue;
      targets.set(task.id, {
        taskId: task.id,
        sessionIp,
        workspaceId: workspace.id,
        sessionId: task.sessionId,
        message: task.message,
        model: task.model,
        thinkingLevel: task.thinkingLevel,
        createdAt: task.createdAt,
      });
    }
  }

  // Ask every session server for its live task list too.
  const liveStates = await Promise.allSettled(
    [...sessionOwners.keys()].map(async (ip) => {
      const response = await fetchImpl(CONFIG.getHttpUrl(ip, '/execution-state'), {
        signal: AbortSignal.timeout(4000),
      });
      if (!response.ok) return null;
      const data = await response.json();
      return {
        ip,
        tasks: Array.isArray(data?.tasks) ? (data.tasks as Array<Record<string, unknown>>) : [],
      };
    })
  );

  for (const result of liveStates) {
    if (result.status !== 'fulfilled' || !result.value) continue;
    const owner = sessionOwners.get(result.value.ip);
    for (const task of result.value.tasks) {
      if (!task?.id || !isActiveTaskStatus(String(task.status)) || targets.has(String(task.id))) {
        continue;
      }
      const rawCreatedAt = task.createdAt;
      const createdAt =
        typeof rawCreatedAt === 'number'
          ? rawCreatedAt
          : rawCreatedAt
            ? Date.parse(String(rawCreatedAt)) || Date.now()
            : Date.now();
      targets.set(String(task.id), {
        taskId: String(task.id),
        sessionIp: result.value.ip,
        workspaceId: owner?.workspaceId || '',
        sessionId: (task.sessionId as string) || owner?.sessionId,
        message: (task.message as string) || '(Running task)',
        model: task.model as string | undefined,
        thinkingLevel: task.thinkingLevel as string | undefined,
        createdAt,
      });
    }
  }

  const targetList = [...targets.values()];
  const cancelResults = await Promise.allSettled(
    targetList.map(async (target) => {
      const response = await fetchImpl(
        CONFIG.getHttpUrl(
          target.sessionIp,
          `/task-cancel?taskId=${encodeURIComponent(target.taskId)}`
        ),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          signal: AbortSignal.timeout(10000),
        }
      );
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      // /task-cancel answers 200 even when the task was already gone, so read
      // the result text and only count tasks canceled by this kill switch.
      const payload = await response.json().catch(() => null);
      const entry = (Array.isArray(payload) ? payload[0] : payload) as
        | { result?: string }
        | null;
      const resultText = String(entry?.result ?? '').toLowerCase();
      const notCanceled =
        resultText.includes('not found') ||
        resultText.includes('already completed') ||
        resultText.includes('already canceled');
      return notCanceled ? 'already-stopped' : 'canceled';
    })
  );

  const report: KillSwitchReport = {
    canceled: [],
    failed: 0,
    alreadyStopped: 0,
    workspaceCount: 0,
  };
  const workspaceIds = new Set<string>();

  cancelResults.forEach((result, index) => {
    const target = targetList[index];
    if (result.status === 'rejected') {
      report.failed += 1;
      return;
    }
    if (result.value !== 'canceled') {
      report.alreadyStopped += 1;
      return;
    }
    report.canceled.push({
      taskId: target.taskId,
      workspaceId: target.workspaceId,
      sessionId: target.sessionId,
      sessionIp: target.sessionIp,
      message: target.message || 'Task canceled by kill switch',
      model: target.model,
      thinkingLevel: target.thinkingLevel,
      createdAt: target.createdAt,
    });
    if (target.workspaceId) workspaceIds.add(target.workspaceId);
  });
  report.workspaceCount = workspaceIds.size;

  return report;
}
