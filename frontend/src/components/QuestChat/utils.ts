import type { ObjectiveDraft } from '../../lib/types';

// Question type for quest conversations
export interface QuestQuestion {
  question: string;
  options?: string[];
}

// extractJSONObject extracts a JSON object from a string using balanced brace matching
// Returns the JSON string and the end index, or null if no valid JSON found
// This is a TypeScript port of the Go version in internal/quest/handler.go
function extractJSONObject(s: string): { json: string; endIndex: number } | null {
  // Skip whitespace
  let start = 0;
  while (start < s.length && (s[start] === ' ' || s[start] === '\t' || s[start] === '\n' || s[start] === '\r')) {
    start++;
  }

  if (start >= s.length || s[start] !== '{') {
    return null;
  }

  let depth = 0;
  let inString = false;
  let escaped = false;

  for (let i = start; i < s.length; i++) {
    const c = s[i];

    if (escaped) {
      escaped = false;
      continue;
    }

    if (c === '\\' && inString) {
      escaped = true;
      continue;
    }

    if (c === '"') {
      inString = !inString;
      continue;
    }

    if (inString) {
      continue;
    }

    if (c === '{') {
      depth++;
    } else if (c === '}') {
      depth--;
      if (depth === 0) {
        return { json: s.substring(start, i + 1), endIndex: i + 1 };
      }
    }
  }

  // Unbalanced braces
  return null;
}

// Helper to parse objective drafts from message content
export function parseObjectiveDrafts(content: string): ObjectiveDraft[] {
  const drafts: ObjectiveDraft[] = [];
  const marker = 'OBJECTIVE_DRAFT:';
  let remaining = content;

  while (true) {
    const idx = remaining.indexOf(marker);
    if (idx === -1) {
      break;
    }

    // Extract JSON portion using balanced brace matching
    const jsonStart = idx + marker.length;
    const result = extractJSONObject(remaining.substring(jsonStart));
    if (!result) {
      remaining = remaining.substring(jsonStart);
      continue;
    }

    try {
      const draft = JSON.parse(result.json);
      if (draft.title && draft.draft_id) {
        // Default auto_start to true if not specified
        if (draft.auto_start === undefined) {
          draft.auto_start = true;
        }
        drafts.push(draft);
      }
    } catch {
      // Skip malformed JSON
    }

    remaining = remaining.substring(jsonStart + result.endIndex);
  }

  return drafts;
}

// Helper to parse questions from message content
export function parseQuestions(content: string): QuestQuestion[] {
  const questions: QuestQuestion[] = [];
  const regex = /QUESTION:\s*(\{[^}]*\})/g;
  let match;

  while ((match = regex.exec(content)) !== null) {
    try {
      const q = JSON.parse(match[1]);
      if (q.question) {
        questions.push(q);
      }
    } catch {
      // Skip malformed JSON
    }
  }

  return questions;
}

// stripObjectiveDrafts removes all OBJECTIVE_DRAFT signals using balanced brace matching
function stripObjectiveDrafts(content: string): string {
  const marker = 'OBJECTIVE_DRAFT:';
  let result = '';
  let remaining = content;

  while (true) {
    const idx = remaining.indexOf(marker);
    if (idx === -1) {
      result += remaining;
      break;
    }

    // Add content before the marker
    result += remaining.substring(0, idx);

    // Extract JSON portion using balanced brace matching
    const jsonStart = idx + marker.length;
    const extracted = extractJSONObject(remaining.substring(jsonStart));
    if (!extracted) {
      // No valid JSON found, keep the marker text and continue
      result += marker;
      remaining = remaining.substring(jsonStart);
      continue;
    }

    // Skip the entire signal (marker + JSON) and any trailing whitespace
    let endPos = jsonStart + extracted.endIndex;
    while (endPos < remaining.length && (remaining[endPos] === ' ' || remaining[endPos] === '\t' || remaining[endPos] === '\n' || remaining[endPos] === '\r')) {
      endPos++;
    }
    remaining = remaining.substring(endPos);
  }

  return result;
}

// Helper to format signals for display
// Questions are formatted inline for history, drafts are shown in sidebar
export function formatMessageContent(content: string): string {
  // Remove OBJECTIVE_DRAFT signals (shown in sidebar) using balanced brace matching
  let formatted = stripObjectiveDrafts(content);

  // Format QUESTION signals as readable text for message history
  formatted = formatted.replace(/QUESTION:\s*(\{[^}]*\})/g, (_match, jsonStr) => {
    try {
      const q = JSON.parse(jsonStr);
      let questionText = `\n**${q.question}**`;
      if (q.options && q.options.length > 0) {
        questionText += '\n' + q.options.map((opt: string) => `• ${opt}`).join('\n');
      }
      return questionText;
    } catch {
      return '';
    }
  });

  // Remove QUEST_READY signals
  formatted = formatted.replace(/QUEST_READY:\s*\{[^}]*\}/g, '');
  // Clean up extra whitespace
  return formatted.trim();
}

// Strip signals from content entirely (for MessageBubble display)
export function stripSignals(content: string): string {
  // Remove OBJECTIVE_DRAFT signals using balanced brace matching
  let stripped = stripObjectiveDrafts(content);

  // Remove QUESTION signals (JSON blocks)
  stripped = stripped.replace(/QUESTION:\s*\{[^}]*\}\s*/g, '');

  // Remove QUEST_READY signals
  stripped = stripped.replace(/QUEST_READY:\s*\{[^}]*\}/g, '');

  // Clean up excessive whitespace
  stripped = stripped.replace(/\n{3,}/g, '\n\n').trim();

  return stripped;
}
