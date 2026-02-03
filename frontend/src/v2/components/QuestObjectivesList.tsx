import { Link } from 'react-router-dom';
import { StatusBar } from './StatusBar';
import { getTaskStatus } from '../utils/formatters';
import type { Task } from '../../lib/types';

interface QuestObjectivesListProps {
  tasks: Task[];
}

export function QuestObjectivesList({ tasks }: QuestObjectivesListProps) {
  if (tasks.length === 0) {
    return null;
  }

  return (
    <div className="v2-objectives-list">
      <div className="v2-label">Objectives</div>
      <div className="v2-objectives-list__items">
        {tasks.map((task) => (
          <Link
            key={task.ID}
            to={`/v2/objectives/${task.ID}`}
            className="v2-objective-link"
          >
            <StatusBar status={getTaskStatus(task.Status)} pulse={task.Status === 'running'} />
            <span className="v2-objective-link__title">{task.Title}</span>
            <span className="v2-label">{task.Status}</span>
          </Link>
        ))}
      </div>
    </div>
  );
}
