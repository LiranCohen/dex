import { useState } from 'react';
import { Link } from 'react-router-dom';
import { StatusBar } from './StatusBar';
import { getTaskStatus } from '../utils/formatters';
import type { Task } from '../../lib/types';

interface QuestObjectivesListProps {
  tasks: Task[];
}

// Count tasks by status category
function getStatusCounts(tasks: Task[]) {
  const counts = {
    running: 0,
    pending: 0,
    completed: 0,
    blocked: 0,
    failed: 0,
  };

  for (const task of tasks) {
    switch (task.Status) {
      case 'running':
        counts.running++;
        break;
      case 'pending':
      case 'ready':
      case 'planning':
        counts.pending++;
        break;
      case 'completed':
      case 'completed_with_issues':
        counts.completed++;
        break;
      case 'blocked':
      case 'paused':
      case 'quarantined':
        counts.blocked++;
        break;
      case 'cancelled':
        counts.failed++;
        break;
    }
  }

  return counts;
}

export function QuestObjectivesList({ tasks }: QuestObjectivesListProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  if (tasks.length === 0) {
    return null;
  }

  const counts = getStatusCounts(tasks);

  // Build status summary parts
  const summaryParts: string[] = [];
  if (counts.running > 0) {
    summaryParts.push(`${counts.running} running`);
  }
  if (counts.pending > 0) {
    summaryParts.push(`${counts.pending} pending`);
  }
  if (counts.blocked > 0) {
    summaryParts.push(`${counts.blocked} blocked`);
  }
  if (counts.completed > 0) {
    summaryParts.push(`${counts.completed} done`);
  }
  if (counts.failed > 0) {
    summaryParts.push(`${counts.failed} cancelled`);
  }

  return (
    <div className="app-objectives-summary">
      {/* Collapsed summary bar */}
      <button
        type="button"
        className="app-objectives-summary__toggle"
        onClick={() => setIsExpanded(!isExpanded)}
        aria-expanded={isExpanded}
      >
        <div className="app-objectives-summary__status">
          {counts.running > 0 && (
            <span className="app-objectives-summary__indicator app-objectives-summary__indicator--running">
              <span className="app-objectives-summary__dot app-objectives-summary__dot--pulse" />
              {counts.running}
            </span>
          )}
          {counts.pending > 0 && (
            <span className="app-objectives-summary__indicator app-objectives-summary__indicator--pending">
              <span className="app-objectives-summary__dot" />
              {counts.pending}
            </span>
          )}
          {counts.blocked > 0 && (
            <span className="app-objectives-summary__indicator app-objectives-summary__indicator--blocked">
              <span className="app-objectives-summary__dot" />
              {counts.blocked}
            </span>
          )}
          {counts.completed > 0 && (
            <span className="app-objectives-summary__indicator app-objectives-summary__indicator--completed">
              ✓ {counts.completed}
            </span>
          )}
        </div>
        <span className="app-objectives-summary__label">
          {tasks.length} objective{tasks.length !== 1 ? 's' : ''}
        </span>
        <span className="app-objectives-summary__chevron">
          {isExpanded ? '▲' : '▼'}
        </span>
      </button>

      {/* Expanded list */}
      {isExpanded && (
        <div className="app-objectives-summary__list">
          {tasks.map((task) => {
            const isBlocked = task.IsBlocked || task.Status === 'blocked';
            return (
              <Link
                key={task.ID}
                to={`/objectives/${task.ID}`}
                className={`app-objectives-summary__item ${isBlocked ? 'app-objectives-summary__item--blocked' : ''}`}
              >
                <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
                <span className="app-objectives-summary__title">{task.Title}</span>
                {isBlocked && (
                  <span className="app-objectives-summary__blocked-icon" title="Waiting for dependencies">
                    ⛓
                  </span>
                )}
                <span className="app-label">{task.Status}</span>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
