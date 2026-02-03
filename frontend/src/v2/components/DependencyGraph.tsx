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
      return 'v2-dep-node--active';
    case 'completed':
      return 'v2-dep-node--complete';
    case 'failed':
    case 'cancelled':
      return 'v2-dep-node--error';
    default:
      return 'v2-dep-node--pending';
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
      <div className="v2-dep-graph v2-dep-graph--empty">
        <p className="v2-empty-hint">No dependencies between objectives</p>
      </div>
    );
  }

  // Render a task node
  const renderNode = (node: TaskNode) => {
    const isCurrent = node.task.ID === currentTaskId;

    return (
      <div key={node.task.ID} className="v2-dep-node-wrapper">
        {/* Upstream dependencies (what this task is blocked by) */}
        {node.blockedBy.length > 0 && (
          <div className="v2-dep-upstream">
            <div className="v2-dep-connector v2-dep-connector--up" />
            <div className="v2-dep-children">
              {node.blockedBy.map((child) => renderNode(child))}
            </div>
          </div>
        )}

        {/* The task node itself */}
        <Link
          to={`/v2/objectives/${node.task.ID}`}
          className={`v2-dep-node ${getStatusClass(node.task.Status)} ${isCurrent ? 'v2-dep-node--current' : ''}`}
          title={node.task.Title}
        >
          <span className="v2-dep-node__status" />
          <span className="v2-dep-node__title">{node.task.Title}</span>
        </Link>

        {/* Downstream dependencies (what this task blocks) */}
        {node.blocks.length > 0 && (
          <div className="v2-dep-downstream">
            <div className="v2-dep-connector v2-dep-connector--down" />
            <div className="v2-dep-children">
              {node.blocks.map((child) => renderNode(child))}
            </div>
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="v2-dep-graph">
      <div className="v2-dep-legend">
        <span className="v2-dep-legend__item">
          <span className="v2-dep-legend__dot v2-dep-legend__dot--active" />
          Running
        </span>
        <span className="v2-dep-legend__item">
          <span className="v2-dep-legend__dot v2-dep-legend__dot--pending" />
          Pending
        </span>
        <span className="v2-dep-legend__item">
          <span className="v2-dep-legend__dot v2-dep-legend__dot--complete" />
          Complete
        </span>
      </div>
      <div className="v2-dep-tree">
        {nodes.map((node) => renderNode(node))}
      </div>
    </div>
  );
}
