import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import type { Task } from '../../lib/types';

interface DependencyGraphProps {
  tasks: Task[];
  currentTaskId?: string;
}

interface TaskNode {
  task: Task;
  blockedBy: TaskNode[];
  blocks: TaskNode[];
  level: number;
}

// Build a map of task ID to tasks that it blocks
function buildBlocksMap(tasks: Task[]): Map<string, string[]> {
  const blocksMap = new Map<string, string[]>();

  tasks.forEach((task) => {
    if (task.BlockedBy && task.BlockedBy.length > 0) {
      task.BlockedBy.forEach((blockerId) => {
        const existing = blocksMap.get(blockerId) || [];
        existing.push(task.ID);
        blocksMap.set(blockerId, existing);
      });
    }
  });

  return blocksMap;
}

// Get status color class
function getStatusClass(status: string): string {
  switch (status) {
    case 'running':
      return 'app-dep-node--active';
    case 'completed':
      return 'app-dep-node--complete';
    case 'failed':
    case 'cancelled':
      return 'app-dep-node--error';
    default:
      return 'app-dep-node--pending';
  }
}

export function DependencyGraph({ tasks, currentTaskId }: DependencyGraphProps) {
  const { nodes, hasAnyDependencies } = useMemo(() => {
    const taskMap = new Map(tasks.map((t) => [t.ID, t]));
    const blocksMap = buildBlocksMap(tasks);

    // Find root nodes (tasks that aren't blocked by anything)
    const rootTasks = tasks.filter((t) => !t.BlockedBy || t.BlockedBy.length === 0);

    // Find tasks with dependencies
    const hasDeps = tasks.some((t) => t.BlockedBy && t.BlockedBy.length > 0);

    // Build tree structure
    const buildNode = (task: Task, level: number, visited: Set<string>): TaskNode | null => {
      if (visited.has(task.ID)) return null; // Prevent cycles
      visited.add(task.ID);

      const blockedByNodes: TaskNode[] = [];
      if (task.BlockedBy) {
        task.BlockedBy.forEach((blockerId) => {
          const blocker = taskMap.get(blockerId);
          if (blocker) {
            const node = buildNode(blocker, level + 1, new Set(visited));
            if (node) blockedByNodes.push(node);
          }
        });
      }

      const blocksNodes: TaskNode[] = [];
      const blockedTasks = blocksMap.get(task.ID) || [];
      blockedTasks.forEach((blockedId) => {
        const blocked = taskMap.get(blockedId);
        if (blocked) {
          const node = buildNode(blocked, level + 1, new Set(visited));
          if (node) blocksNodes.push(node);
        }
      });

      return {
        task,
        blockedBy: blockedByNodes,
        blocks: blocksNodes,
        level,
      };
    };

    // If viewing a specific task, center the graph on it
    if (currentTaskId && taskMap.has(currentTaskId)) {
      const currentTask = taskMap.get(currentTaskId)!;
      const node = buildNode(currentTask, 0, new Set());
      return { nodes: node ? [node] : [], hasAnyDependencies: hasDeps };
    }

    // Otherwise show from root nodes
    const rootNodes = rootTasks
      .map((t) => buildNode(t, 0, new Set()))
      .filter((n): n is TaskNode => n !== null);

    return { nodes: rootNodes, hasAnyDependencies: hasDeps };
  }, [tasks, currentTaskId]);

  if (!hasAnyDependencies) {
    return (
      <div className="app-dep-graph app-dep-graph--empty">
        <p className="app-empty-hint">No dependencies between objectives</p>
      </div>
    );
  }

  // Render a task node
  const renderNode = (node: TaskNode) => {
    const isCurrent = node.task.ID === currentTaskId;

    return (
      <div key={node.task.ID} className="app-dep-node-wrapper">
        {/* Upstream dependencies (what this task is blocked by) */}
        {node.blockedBy.length > 0 && (
          <div className="app-dep-upstream">
            <div className="app-dep-connector app-dep-connector--up" />
            <div className="app-dep-children">
              {node.blockedBy.map((child) => renderNode(child))}
            </div>
          </div>
        )}

        {/* The task node itself */}
        <Link
          to={`/objectives/${node.task.ID}`}
          className={`app-dep-node ${getStatusClass(node.task.Status)} ${isCurrent ? 'app-dep-node--current' : ''}`}
          title={node.task.Title}
        >
          <span className="app-dep-node__status" />
          <span className="app-dep-node__title">{node.task.Title}</span>
        </Link>

        {/* Downstream dependencies (what this task blocks) */}
        {node.blocks.length > 0 && (
          <div className="app-dep-downstream">
            <div className="app-dep-connector app-dep-connector--down" />
            <div className="app-dep-children">
              {node.blocks.map((child) => renderNode(child))}
            </div>
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="app-dep-graph">
      <div className="app-dep-legend">
        <span className="app-dep-legend__item">
          <span className="app-dep-legend__dot app-dep-legend__dot--active" />
          Running
        </span>
        <span className="app-dep-legend__item">
          <span className="app-dep-legend__dot app-dep-legend__dot--pending" />
          Pending
        </span>
        <span className="app-dep-legend__item">
          <span className="app-dep-legend__dot app-dep-legend__dot--complete" />
          Complete
        </span>
      </div>
      <div className="app-dep-tree">
        {nodes.map((node) => renderNode(node))}
      </div>
    </div>
  );
}
