import type { ObjectiveDraft, Task } from '../../lib/types';
import type { AcceptedDraft } from '../components/chat/MessageList';

/**
 * Filters pending drafts to exclude:
 * 1. Drafts that already exist as tasks (by matching title)
 * 2. Drafts that have already been accepted (by draft_id)
 *
 * @param drafts - Array of parsed objective drafts
 * @param existingTasks - Array of existing tasks for the quest
 * @param acceptedDrafts - Map of already-accepted draft IDs to their data
 * @returns Filtered array of drafts that are still pending
 */
export function filterPendingDrafts(
  drafts: ObjectiveDraft[],
  existingTasks: Task[],
  acceptedDrafts: Map<string, AcceptedDraft>
): ObjectiveDraft[] {
  // Filter out drafts that already exist as tasks (by matching title)
  const existingTaskTitles = new Set(
    existingTasks.map((t) => t.Title?.toLowerCase())
  );
  const titleFiltered = drafts.filter(
    (d) => !existingTaskTitles.has(d.title?.toLowerCase())
  );

  // Also filter against already-accepted drafts (by draft_id)
  return titleFiltered.filter((d) => !acceptedDrafts.has(d.draft_id));
}

/**
 * Converts an array of drafts to a Map keyed by draft_id
 */
export function draftsToMap(drafts: ObjectiveDraft[]): Map<string, ObjectiveDraft> {
  const map = new Map<string, ObjectiveDraft>();
  drafts.forEach((d) => map.set(d.draft_id, d));
  return map;
}
