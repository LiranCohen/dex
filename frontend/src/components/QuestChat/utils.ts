import type { ObjectiveDraft } from '../../lib/types';

// Question type for quest conversations
export interface QuestQuestion {
  question: string;
  options?: string[];
}

// Helper to parse objective drafts from message content
export function parseObjectiveDrafts(content: string): ObjectiveDraft[] {
  const drafts: ObjectiveDraft[] = [];
  const regex = /OBJECTIVE_DRAFT:\s*(\{[\s\S]*?\})\s*(?=OBJECTIVE_DRAFT:|QUESTION:|QUEST_READY:|$)/g;
  let match;

  while ((match = regex.exec(content)) !== null) {
    try {
      const draft = JSON.parse(match[1]);
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

// Helper to format signals for display
// Questions are formatted inline for history, drafts are shown in sidebar
export function formatMessageContent(content: string): string {
  // Remove OBJECTIVE_DRAFT signals (shown in sidebar)
  let formatted = content.replace(/OBJECTIVE_DRAFT:\s*\{[\s\S]*?\}\s*(?=OBJECTIVE_DRAFT:|QUESTION:|QUEST_READY:|$)/g, '');

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
  // Remove OBJECTIVE_DRAFT:...END_OBJECTIVE_DRAFT or JSON blocks
  let stripped = content.replace(/OBJECTIVE_DRAFT:[\s\S]*?(?:END_OBJECTIVE_DRAFT|\})\s*/g, '');

  // Remove QUESTION:...END_QUESTION or JSON blocks
  stripped = stripped.replace(/QUESTION:[\s\S]*?(?:END_QUESTION|\})\s*/g, '');

  // Remove QUEST_READY signals
  stripped = stripped.replace(/QUEST_READY:\s*\{[^}]*\}/g, '');

  // Clean up excessive whitespace
  stripped = stripped.replace(/\n{3,}/g, '\n\n').trim();

  return stripped;
}
