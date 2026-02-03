import { formatTimeWithSeconds } from '../utils/formatters';

export interface Activity {
  id: string;
  type: string;
  content: string;
  created_at: string;
}

interface ActivityLogProps {
  items: Activity[];
  emptyMessage?: string;
}

export function ActivityLog({ items, emptyMessage = '// no activity yet' }: ActivityLogProps) {
  if (items.length === 0) {
    return <p className="v2-empty-hint">{emptyMessage}</p>;
  }

  return (
    <div className="v2-card v2-activity-log">
      {items.slice().reverse().map((item) => (
        <div key={item.id} className="v2-activity-item">
          <span className="v2-activity-item__time">{formatTimeWithSeconds(item.created_at)}</span>
          <span>{item.content || item.type}</span>
        </div>
      ))}
    </div>
  );
}
