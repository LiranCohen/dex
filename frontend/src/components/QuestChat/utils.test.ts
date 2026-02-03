import { describe, it, expect } from 'vitest';
import { parseObjectiveDrafts, parseQuestions, formatMessageContent, stripSignals } from './utils';

describe('parseObjectiveDrafts', () => {
  it('extracts a single draft', () => {
    const content = `Here's what I propose:
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test Feature","description":"Implement feature","hat":"coder","checklist":{"must_have":["Item 1"]},"auto_start":true}
Let me know if this works.`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0]).toEqual({
      draft_id: 'd1',
      title: 'Test Feature',
      description: 'Implement feature',
      hat: 'coder',
      checklist: { must_have: ['Item 1'] },
      auto_start: true,
    });
  });

  it('extracts multiple drafts', () => {
    const content = `OBJECTIVE_DRAFT:{"draft_id":"d1","title":"First","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}
Some text between.
OBJECTIVE_DRAFT:{"draft_id":"d2","title":"Second","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":false}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(2);
    expect(drafts[0].draft_id).toBe('d1');
    expect(drafts[0].title).toBe('First');
    expect(drafts[1].draft_id).toBe('d2');
    expect(drafts[1].title).toBe('Second');
    expect(drafts[1].auto_start).toBe(false);
  });

  it('handles nested braces in JSON', () => {
    const content = `OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"Use {nested} braces","hat":"coder","checklist":{"must_have":["Has {curly} braces"]},"auto_start":true}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0].description).toBe('Use {nested} braces');
    expect(drafts[0].checklist.must_have[0]).toBe('Has {curly} braces');
  });

  it('handles escaped quotes in strings', () => {
    const content = `OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Say \\"Hello\\"","description":"Test","hat":"coder","checklist":{"must_have":[]},"auto_start":true}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0].title).toBe('Say "Hello"');
  });

  it('skips malformed JSON', () => {
    const content = `OBJECTIVE_DRAFT:{invalid json here}
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Valid","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0].title).toBe('Valid');
  });

  it('skips drafts without required fields', () => {
    const content = `OBJECTIVE_DRAFT:{"description":"Missing title and draft_id"}
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Has both","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0].draft_id).toBe('d1');
  });

  it('defaults auto_start to true if not specified', () => {
    const content = `OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]}}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0].auto_start).toBe(true);
  });

  it('returns empty array when no drafts found', () => {
    const content = 'Just some regular text without any drafts.';
    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(0);
  });

  it('handles whitespace before JSON', () => {
    const content = `OBJECTIVE_DRAFT:
{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}`;

    const drafts = parseObjectiveDrafts(content);
    expect(drafts).toHaveLength(1);
    expect(drafts[0].draft_id).toBe('d1');
  });
});

describe('parseQuestions', () => {
  it('extracts a single question', () => {
    const content = `I have a question for you:
QUESTION: {"question":"What framework do you prefer?"}`;

    const questions = parseQuestions(content);
    expect(questions).toHaveLength(1);
    expect(questions[0].question).toBe('What framework do you prefer?');
  });

  it('extracts question with options', () => {
    const content = `QUESTION: {"question":"Choose a database:","options":["PostgreSQL","MySQL","SQLite"]}`;

    const questions = parseQuestions(content);
    expect(questions).toHaveLength(1);
    expect(questions[0].question).toBe('Choose a database:');
    expect(questions[0].options).toEqual(['PostgreSQL', 'MySQL', 'SQLite']);
  });

  it('extracts multiple questions', () => {
    const content = `QUESTION: {"question":"First question?"}
Some text.
QUESTION: {"question":"Second question?","options":["A","B"]}`;

    const questions = parseQuestions(content);
    expect(questions).toHaveLength(2);
    expect(questions[0].question).toBe('First question?');
    expect(questions[1].question).toBe('Second question?');
    expect(questions[1].options).toEqual(['A', 'B']);
  });

  it('skips malformed JSON', () => {
    const content = `QUESTION: {not valid json}
QUESTION: {"question":"Valid question"}`;

    const questions = parseQuestions(content);
    expect(questions).toHaveLength(1);
    expect(questions[0].question).toBe('Valid question');
  });

  it('skips questions without question field', () => {
    const content = `QUESTION: {"options":["A","B"]}
QUESTION: {"question":"Has question field"}`;

    const questions = parseQuestions(content);
    expect(questions).toHaveLength(1);
    expect(questions[0].question).toBe('Has question field');
  });

  it('returns empty array when no questions found', () => {
    const content = 'Just regular text.';
    const questions = parseQuestions(content);
    expect(questions).toHaveLength(0);
  });
});

describe('formatMessageContent', () => {
  it('removes OBJECTIVE_DRAFT signals', () => {
    const content = `Before text.
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}
After text.`;

    const formatted = formatMessageContent(content);
    expect(formatted).toBe('Before text.\nAfter text.');
    expect(formatted).not.toContain('OBJECTIVE_DRAFT');
  });

  it('formats QUESTION signals as readable text', () => {
    const content = `QUESTION: {"question":"What do you prefer?","options":["Option A","Option B"]}`;

    const formatted = formatMessageContent(content);
    expect(formatted).toContain('**What do you prefer?**');
    expect(formatted).toContain('• Option A');
    expect(formatted).toContain('• Option B');
  });

  it('formats question without options', () => {
    const content = `QUESTION: {"question":"Please describe your needs."}`;

    const formatted = formatMessageContent(content);
    expect(formatted).toContain('**Please describe your needs.**');
  });

  it('removes QUEST_READY signals', () => {
    const content = `Some text. QUEST_READY:{"drafts":["d1","d2"]} More text.`;

    const formatted = formatMessageContent(content);
    expect(formatted).toBe('Some text.  More text.');
    expect(formatted).not.toContain('QUEST_READY');
  });

  it('handles multiple signal types in one message', () => {
    const content = `Intro text.
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}
QUESTION: {"question":"Agree?","options":["Yes","No"]}
QUEST_READY:{"drafts":["d1"]}
Closing text.`;

    const formatted = formatMessageContent(content);
    expect(formatted).not.toContain('OBJECTIVE_DRAFT');
    expect(formatted).not.toContain('QUEST_READY');
    expect(formatted).toContain('**Agree?**');
    expect(formatted).toContain('Intro text.');
    expect(formatted).toContain('Closing text.');
  });

  it('cleans up extra whitespace', () => {
    const content = `  Text with whitespace.  `;
    const formatted = formatMessageContent(content);
    expect(formatted).toBe('Text with whitespace.');
  });
});

describe('stripSignals', () => {
  it('removes OBJECTIVE_DRAFT signals', () => {
    const content = `Text before.
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}
Text after.`;

    const stripped = stripSignals(content);
    expect(stripped).not.toContain('OBJECTIVE_DRAFT');
    expect(stripped).toContain('Text before.');
    expect(stripped).toContain('Text after.');
  });

  it('removes QUESTION signals', () => {
    const content = `Text. QUESTION: {"question":"Question?","options":["A"]} More text.`;

    const stripped = stripSignals(content);
    expect(stripped).not.toContain('QUESTION');
    expect(stripped).toContain('Text.');
    expect(stripped).toContain('More text.');
  });

  it('removes QUEST_READY signals', () => {
    const content = `Text. QUEST_READY:{"drafts":["d1"]} End.`;

    const stripped = stripSignals(content);
    expect(stripped).not.toContain('QUEST_READY');
    expect(stripped).toBe('Text.  End.');
  });

  it('removes all signal types', () => {
    const content = `Start.
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}
QUESTION: {"question":"Q?"}
QUEST_READY:{"drafts":[]}
End.`;

    const stripped = stripSignals(content);
    expect(stripped).toBe('Start.\n\nEnd.');
  });

  it('cleans up excessive newlines', () => {
    const content = `Line 1.


OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"","hat":"coder","checklist":{"must_have":[]},"auto_start":true}


Line 2.`;

    const stripped = stripSignals(content);
    // Should collapse 3+ newlines to 2
    expect(stripped).not.toMatch(/\n{3,}/);
  });

  it('returns original text when no signals present', () => {
    const content = 'Just regular text without any signals.';
    const stripped = stripSignals(content);
    expect(stripped).toBe(content);
  });

  it('handles complex nested JSON in drafts', () => {
    const content = `Before.
OBJECTIVE_DRAFT:{"draft_id":"d1","title":"Test","description":"Has {nested: {objects}}","hat":"coder","checklist":{"must_have":["Item with {braces}"]},"auto_start":true}
After.`;

    const stripped = stripSignals(content);
    expect(stripped).not.toContain('OBJECTIVE_DRAFT');
    expect(stripped).not.toContain('{nested');
    expect(stripped).toContain('Before.');
    expect(stripped).toContain('After.');
  });
});
