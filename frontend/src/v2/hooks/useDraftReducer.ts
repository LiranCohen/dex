import { useReducer, useCallback } from 'react';
import type { ObjectiveDraft } from '../../lib/types';
import type { AcceptedDraft } from '../components';

// Draft state shape
export interface DraftState {
  pending: Map<string, ObjectiveDraft>;
  accepted: Map<string, AcceptedDraft>;
  rejected: Map<string, ObjectiveDraft>;
  accepting: Set<string>;
  acceptingAll: boolean;
}

// Action types
type DraftAction =
  | { type: 'SET_PENDING'; drafts: Map<string, ObjectiveDraft> }
  | { type: 'ADD_PENDING'; key: string; draft: ObjectiveDraft }
  | { type: 'REMOVE_PENDING'; key: string }
  | { type: 'CLEAR_PENDING' }
  | { type: 'START_ACCEPTING'; key: string }
  | { type: 'STOP_ACCEPTING'; key: string }
  | { type: 'START_ACCEPTING_ALL' }
  | { type: 'STOP_ACCEPTING_ALL' }
  | { type: 'ACCEPT'; key: string; draft: ObjectiveDraft; taskId?: string }
  | { type: 'ACCEPT_BATCH'; entries: Array<{ key: string; draft: ObjectiveDraft; taskId?: string }> }
  | { type: 'REJECT'; key: string; draft: ObjectiveDraft }
  | { type: 'REJECT_BATCH'; entries: Array<{ key: string; draft: ObjectiveDraft }> }
  | { type: 'UNDO_REJECT'; key: string }
  | { type: 'CLEAR_REJECTED'; key: string }
  | { type: 'RESTORE_PENDING'; key: string; draft: ObjectiveDraft };

// Initial state
const initialState: DraftState = {
  pending: new Map(),
  accepted: new Map(),
  rejected: new Map(),
  accepting: new Set(),
  acceptingAll: false,
};

// Reducer function
function draftReducer(state: DraftState, action: DraftAction): DraftState {
  switch (action.type) {
    case 'SET_PENDING':
      return { ...state, pending: action.drafts };

    case 'ADD_PENDING': {
      const pending = new Map(state.pending);
      pending.set(action.key, action.draft);
      return { ...state, pending };
    }

    case 'REMOVE_PENDING': {
      const pending = new Map(state.pending);
      pending.delete(action.key);
      return { ...state, pending };
    }

    case 'CLEAR_PENDING':
      return { ...state, pending: new Map() };

    case 'START_ACCEPTING': {
      const accepting = new Set(state.accepting);
      accepting.add(action.key);
      return { ...state, accepting };
    }

    case 'STOP_ACCEPTING': {
      const accepting = new Set(state.accepting);
      accepting.delete(action.key);
      return { ...state, accepting };
    }

    case 'START_ACCEPTING_ALL':
      return { ...state, acceptingAll: true };

    case 'STOP_ACCEPTING_ALL':
      return { ...state, acceptingAll: false };

    case 'ACCEPT': {
      const pending = new Map(state.pending);
      const accepted = new Map(state.accepted);
      const accepting = new Set(state.accepting);

      pending.delete(action.key);
      accepted.set(action.draft.draft_id, { draft: action.draft, taskId: action.taskId });
      accepting.delete(action.key);

      return { ...state, pending, accepted, accepting };
    }

    case 'ACCEPT_BATCH': {
      const pending = new Map(state.pending);
      const accepted = new Map(state.accepted);

      action.entries.forEach(({ key, draft, taskId }) => {
        pending.delete(key);
        accepted.set(draft.draft_id, { draft, taskId });
      });

      return { ...state, pending, accepted, acceptingAll: false };
    }

    case 'REJECT': {
      const pending = new Map(state.pending);
      const rejected = new Map(state.rejected);

      pending.delete(action.key);
      rejected.set(action.key, action.draft);

      return { ...state, pending, rejected };
    }

    case 'REJECT_BATCH': {
      const pending = new Map(state.pending);
      const rejected = new Map(state.rejected);

      action.entries.forEach(({ key, draft }) => {
        pending.delete(key);
        rejected.set(key, draft);
      });

      return { ...state, pending, rejected };
    }

    case 'UNDO_REJECT': {
      const draft = state.rejected.get(action.key);
      if (!draft) return state;

      const pending = new Map(state.pending);
      const rejected = new Map(state.rejected);

      rejected.delete(action.key);
      pending.set(action.key, draft);

      return { ...state, pending, rejected };
    }

    case 'CLEAR_REJECTED': {
      const rejected = new Map(state.rejected);
      rejected.delete(action.key);
      return { ...state, rejected };
    }

    case 'RESTORE_PENDING': {
      const pending = new Map(state.pending);
      pending.set(action.key, action.draft);
      return { ...state, pending };
    }

    default:
      return state;
  }
}

// Custom hook for draft management
export function useDraftReducer() {
  const [state, dispatch] = useReducer(draftReducer, initialState);

  // Convenience action creators
  const actions = {
    setPending: useCallback((drafts: Map<string, ObjectiveDraft>) => {
      dispatch({ type: 'SET_PENDING', drafts });
    }, []),

    addPending: useCallback((key: string, draft: ObjectiveDraft) => {
      dispatch({ type: 'ADD_PENDING', key, draft });
    }, []),

    removePending: useCallback((key: string) => {
      dispatch({ type: 'REMOVE_PENDING', key });
    }, []),

    clearPending: useCallback(() => {
      dispatch({ type: 'CLEAR_PENDING' });
    }, []),

    startAccepting: useCallback((key: string) => {
      dispatch({ type: 'START_ACCEPTING', key });
    }, []),

    stopAccepting: useCallback((key: string) => {
      dispatch({ type: 'STOP_ACCEPTING', key });
    }, []),

    startAcceptingAll: useCallback(() => {
      dispatch({ type: 'START_ACCEPTING_ALL' });
    }, []),

    stopAcceptingAll: useCallback(() => {
      dispatch({ type: 'STOP_ACCEPTING_ALL' });
    }, []),

    accept: useCallback((key: string, draft: ObjectiveDraft, taskId?: string) => {
      dispatch({ type: 'ACCEPT', key, draft, taskId });
    }, []),

    acceptBatch: useCallback((entries: Array<{ key: string; draft: ObjectiveDraft; taskId?: string }>) => {
      dispatch({ type: 'ACCEPT_BATCH', entries });
    }, []),

    reject: useCallback((key: string, draft: ObjectiveDraft) => {
      dispatch({ type: 'REJECT', key, draft });
    }, []),

    rejectBatch: useCallback((entries: Array<{ key: string; draft: ObjectiveDraft }>) => {
      dispatch({ type: 'REJECT_BATCH', entries });
    }, []),

    undoReject: useCallback((key: string) => {
      dispatch({ type: 'UNDO_REJECT', key });
    }, []),

    clearRejected: useCallback((key: string) => {
      dispatch({ type: 'CLEAR_REJECTED', key });
    }, []),

    restorePending: useCallback((key: string, draft: ObjectiveDraft) => {
      dispatch({ type: 'RESTORE_PENDING', key, draft });
    }, []),
  };

  return { state, actions, dispatch };
}
