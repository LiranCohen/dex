export interface ChecklistItem {
  id: string;
  task_id: string;
  description: string;
  is_completed: boolean;
  is_optional: boolean;
}

interface ChecklistProps {
  items: ChecklistItem[];
  emptyMessage?: string;
}

export function Checklist({ items, emptyMessage = '// no checklist items' }: ChecklistProps) {
  if (items.length === 0) {
    return <p className="v2-empty-hint">{emptyMessage}</p>;
  }

  return (
    <div className="v2-card v2-checklist">
      {items.map((item) => (
        <div key={item.id} className="v2-checklist-item">
          <span className={`v2-checklist-item__icon ${item.is_completed ? 'v2-checklist-item__icon--complete' : 'v2-checklist-item__icon--pending'}`}>
            {item.is_completed ? '✓' : '◯'}
          </span>
          <span className={`v2-checklist-item__text ${item.is_completed ? 'v2-checklist-item__text--complete' : ''}`}>
            {item.description}
            {item.is_optional && <span className="v2-checklist-item__optional">(optional)</span>}
          </span>
        </div>
      ))}
    </div>
  );
}
