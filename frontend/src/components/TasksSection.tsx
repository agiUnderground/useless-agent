import React, { useState, useEffect, useLayoutEffect, useRef } from 'react';
import './TasksSection.css';
import TaskCard from './TaskCard';

// Global task counter for sequence numbers
let taskSequenceNumber = 1;

// Task list scroll positions survive WorkspaceApp remounts (workspace switches)
const tasksScrollPositions = new Map<string, number>();

// Function for smart scrolling based on task position
export function smartScrollToTask(
  taskCardElement: HTMLElement,
  tasksContainerElement: HTMLElement,
  behavior: ScrollBehavior = 'smooth'
) {
  const taskCards = Array.from(tasksContainerElement.querySelectorAll('.task-card'));
  const taskIndex = taskCards.indexOf(taskCardElement);
  const totalTasks = taskCards.length;

  let targetScroll: number;
  if (taskIndex === 0) {
    // First task - scroll to top
    targetScroll = 0;
  } else if (taskIndex === totalTasks - 1) {
    // Last task - scroll to bottom
    targetScroll = tasksContainerElement.scrollHeight;
  } else {
    // Middle task - scroll to center (account for CSS transform scale when zoomed out)
    const taskRect = taskCardElement.getBoundingClientRect();
    const containerRect = tasksContainerElement.getBoundingClientRect();
    const scale =
      tasksContainerElement.clientHeight > 0 && containerRect.height > 0
        ? containerRect.height / tasksContainerElement.clientHeight
        : 1;
    const containerCenter = containerRect.height / 2;
    const taskCenter = taskRect.top - containerRect.top + taskRect.height / 2;
    const scrollOffset = (taskCenter - containerCenter) / scale;
    targetScroll = Math.max(0, tasksContainerElement.scrollTop + scrollOffset);
  }

  if (behavior === 'auto') {
    tasksContainerElement.scrollTop = targetScroll;
  } else if (typeof tasksContainerElement.scrollTo === 'function') {
    tasksContainerElement.scrollTo({ top: targetScroll, behavior });
  } else {
    tasksContainerElement.scrollTop = targetScroll;
  }
}

// Function to calculate user-assist connection path
function calculateUserAssistPath(taskCardElement: HTMLElement, chatFieldsetElement: HTMLElement): string {
  const taskRect = taskCardElement.getBoundingClientRect();
  const chatRect = chatFieldsetElement.getBoundingClientRect();
  const overlayRect = document.getElementById('connectionSvg')?.getBoundingClientRect();
  
  if (!overlayRect) return '';
  
  // Calculate positions relative to SVG overlay
  const taskBottomY = taskRect.bottom - overlayRect.top;
  const chatCenterX = chatRect.left + chatRect.width / 2 - overlayRect.left;
  const chatTopY = chatRect.top - overlayRect.top;
  
  // Straight line from bottom edge of task card to top center of chat
  return `M ${chatCenterX} ${chatTopY} L ${chatCenterX} ${taskBottomY}`;
}

interface Task {
  id: string;
  status: string;
  message: string;
  model?: string;
  sessionId?: string;
  sequenceNumber?: number;
  createdAt?: number;
  completedAt?: number; // Store when task was completed or cancelled
  timerInterval?: number;
  isPlaceholder?: boolean; // Flag to identify placeholder tasks
  originalMessage?: string; // Store original message to match with backend response
}

interface TasksSectionProps {
  tasks: Task[];
  onCancelTask: (taskId: string) => void;
  onActivateUserAssist: (taskId: string, event?: React.MouseEvent) => void;
  onOpenSessionWindow?: (sessionId: string, sessionIp: string) => void;
  userAssistActive: boolean;
  userAssistTaskCard: Task | null;
  onDeactivateUserAssist: () => void;
  sessions?: Array<{ id: string; ip: string }>;
  workspaceId?: string;
}

const TasksSection: React.FC<TasksSectionProps> = ({
  tasks,
  onCancelTask,
  onActivateUserAssist,
  onOpenSessionWindow,
  userAssistActive,
  userAssistTaskCard,
  onDeactivateUserAssist,
  sessions = [],
  workspaceId
}) => {
  const tasksContainerRef = useRef<HTMLDivElement>(null);
  // Start with the initial length so a fresh workspace mount is not treated as
  // "new tasks added" (that would override the restored scroll position).
  const prevTasksLengthRef = useRef<number>(tasks.length);

  // Restore and persist the task list scroll position per workspace (both normal
  // and user-assist modes). WorkspaceApp is keyed by workspace id, so switching
  // workspaces remounts the task list.
  useLayoutEffect(() => {
    const container = tasksContainerRef.current;
    if (!container || !workspaceId) return;

    const savedScroll = tasksScrollPositions.get(workspaceId);
    let restoring = savedScroll !== undefined;
    let rafId = 0;

    if (savedScroll !== undefined) {
      container.scrollTop = savedScroll;
      rafId = requestAnimationFrame(() => {
        if (tasksContainerRef.current) {
          tasksContainerRef.current.scrollTop = savedScroll;
        }
        restoring = false;
      });
    }

    const saveScroll = () => {
      if (restoring) return;
      tasksScrollPositions.set(workspaceId, container.scrollTop);
    };
    container.addEventListener('scroll', saveScroll);
    return () => {
      cancelAnimationFrame(rafId);
      container.removeEventListener('scroll', saveScroll);
      if (!restoring && container.scrollHeight > container.clientHeight) {
        tasksScrollPositions.set(workspaceId, container.scrollTop);
      }
    };
  }, [workspaceId]);
  
  // Auto-scroll to bottom when a new task is added and not in user-assist mode
  useEffect(() => {
    // Check if a new task was added
    if (tasks.length > prevTasksLengthRef.current && !userAssistActive) {
      // Scroll to bottom after a short delay to ensure DOM has updated
      setTimeout(() => {
        if (tasksContainerRef.current) {
          tasksContainerRef.current.scrollTo({
            top: tasksContainerRef.current.scrollHeight,
            behavior: 'smooth'
          });
        }
      }, 100);
    }
    // Update the previous tasks length
    prevTasksLengthRef.current = tasks.length;
  }, [tasks, userAssistActive]);
  
  const handleCancelTask = (taskId: string) => {
    onCancelTask(taskId);
  };
  
  const handleTaskCardClick = (taskId: string, event?: React.MouseEvent) => {
    // Get the task card element and container
    let taskCardElement: HTMLElement | null = null;
    
    if (event?.currentTarget) {
      taskCardElement = event.currentTarget as HTMLElement;
    } else {
      // Fallback to query selector if event is not available
      taskCardElement = document.querySelector(`[data-task-id="${taskId}"]`) as HTMLElement;
    }
    
    const tasksContainerElement = tasksContainerRef.current;
    
    if (tasksContainerElement && taskCardElement) {
      // Smart scroll based on task position
      smartScrollToTask(taskCardElement, tasksContainerElement);
      
      // Toggle user-assist only for in-progress tasks
      const task = tasks.find(t => t.id === taskId);
      if (task && task.status === 'in-progress') {
        // Check if user-assist is already active for this same task
        const isSameTaskActive = userAssistActive && userAssistTaskCard?.id === taskId;
        if (isSameTaskActive) {
          // If clicking on the same task that's already active, deactivate user-assist
          onDeactivateUserAssist();
        } else {
          // Activate user-assist for different task
          onActivateUserAssist(taskId, event);
        }
      } else if (userAssistActive) {
        // If user-assist is already active and clicking on a non-in-progress task, deactivate it
        onDeactivateUserAssist();
      }
    }
  };

  return (
    <div
      id="tasksSection"
      className={`tasks-section ${tasks.length > 0 ? 'visible' : ''} ${userAssistActive ? 'maximized' : ''}`}
    >
      <fieldset>
        <legend>Tasks</legend>
        <div id="tasksContainer" ref={tasksContainerRef}>
          {tasks.map((task, index) => (
            <TaskCard
              key={task.id}
              task={{
                ...task,
                sequenceNumber: task.sequenceNumber || index + 1 // Ensure sequence numbers start from 1
              }}
              onCancel={handleCancelTask}
              onActivateUserAssist={handleTaskCardClick}
              onOpenSessionWindow={onOpenSessionWindow}
              userAssistActive={userAssistActive}
              isUserAssistTask={userAssistTaskCard?.id === task.id}
              sessions={sessions}
            />
          ))}
        </div>
      </fieldset>
      
      {/* User-assist connection line is handled by ConnectionOverlay component */}
    </div>
  );
};

export default TasksSection;
