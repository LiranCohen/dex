import { useState, useEffect, useCallback, useMemo } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Header, StatusBar, KeyboardShortcuts, LoadingState, useToast } from '../components';
import { fetchQuests, createQuest, fetchApprovals } from '../../lib/api';
import { useWebSocket } from '../../hooks/useWebSocket';
import { useKeyboardNavigation } from '../hooks/useKeyboardNavigation';
import type { Quest, Approval, WebSocketEvent } from '../../lib/types';

function getQuestStatus(quest: Quest): 'active' | 'pending' | 'complete' {
  if (quest.status === 'completed') return 'complete';
  // Could check if any objectives are running
  return 'active';
}

function formatProgress(quest: Quest): string {
  const total = quest.summary?.total_tasks || 0;
  const completed = quest.summary?.completed_tasks || 0;
  const running = quest.summary?.running_tasks || 0;

  if (total === 0) return 'No objectives yet';

  let text = `${completed}/${total} objectives`;
  if (running > 0) {
    text += ` · ${running} running`;
  }
  return text;
}

export function Home() {
  const [quests, setQuests] = useState<Quest[]>([]);
  const [approvalCount, setApprovalCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const navigate = useNavigate();
  const { subscribe } = useWebSocket();
  const { showToast } = useToast();

  const activeQuests = quests.filter((q) => q.status !== 'completed');

  // Keyboard navigation items
  const navItems = useMemo(() =>
    activeQuests.map((quest) => ({
      id: quest.id,
      onClick: () => navigate(`/v2/quests/${quest.id}`),
    })),
    [activeQuests, navigate]
  );

  const { selectedIndex } = useKeyboardNavigation({
    onHelp: () => setShowShortcuts(true),
    items: navItems,
    enabled: !loading,
  });

  const loadData = useCallback(async () => {
    try {
      const [questsData, approvalsData] = await Promise.all([
        fetchQuests('proj_default'),
        fetchApprovals(),
      ]);
      setQuests(questsData || []);
      setApprovalCount((approvalsData.approvals || []).filter((a: Approval) => a.status === 'pending').length);
    } catch (err) {
      console.error('Failed to load data:', err);
      showToast('Failed to load quests', 'error');
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // WebSocket updates
  useEffect(() => {
    const unsubscribe = subscribe((event: WebSocketEvent) => {
      if (event.type.startsWith('quest.') || event.type.startsWith('task.') || event.type.startsWith('approval.')) {
        loadData();
      }
    });
    return unsubscribe;
  }, [subscribe, loadData]);

  const handleNewQuest = async () => {
    setCreating(true);
    try {
      const quest = await createQuest('proj_default');
      navigate(`/v2/quests/${quest.id}`);
    } catch (err) {
      console.error('Failed to create quest:', err);
      showToast('Failed to create quest', 'error');
    } finally {
      setCreating(false);
    }
  };

  const completedQuests = quests.filter((q) => q.status === 'completed');

  if (loading) {
    return (
      <div className="v2-root">
        <Header inboxCount={0} />
        <main className="v2-content">
          <LoadingState message="Loading your quests..." size="large" />
        </main>
      </div>
    );
  }

  return (
    <div className="v2-root">
      <Header inboxCount={approvalCount} />

      <main className="v2-content">
        {/* Page header */}
        <div className="v2-home-header">
          <h1 className="v2-page-title">Quests</h1>
          <button
            type="button"
            className="v2-btn v2-btn--primary"
            onClick={handleNewQuest}
            disabled={creating}
          >
            + New Quest
          </button>
        </div>

        {/* Active quests */}
        {activeQuests.length === 0 && completedQuests.length === 0 ? (
          <div className="v2-empty-state">
            <p>No quests yet</p>
            <p className="v2-empty-state__hint">
              Start by creating a new quest
            </p>
          </div>
        ) : (
          <>
            {/* Active quests */}
            <div className="v2-quest-list">
              {activeQuests.map((quest, index) => (
                <Link
                  key={quest.id}
                  to={`/v2/quests/${quest.id}`}
                  className={`v2-card v2-card--interactive v2-quest-card ${selectedIndex === index ? 'v2-card--selected' : ''}`}
                >
                  <StatusBar
                    status={getQuestStatus(quest) === 'active' ? 'active' : 'pending'}
                    pulse={getQuestStatus(quest) === 'active'}
                  />
                  <div className="v2-quest-card__content">
                    <h2 className="v2-quest-card__title">
                      {quest.title || 'Untitled Quest'}
                    </h2>
                    <p className="v2-quest-card__progress">
                      {formatProgress(quest)}
                    </p>
                  </div>
                </Link>
              ))}
            </div>

            {/* Completed section */}
            {completedQuests.length > 0 && (
              <>
                <div className="v2-divider--text">
                  Completed
                </div>

                <div className="v2-quest-list">
                  {completedQuests.map((quest) => (
                    <Link
                      key={quest.id}
                      to={`/v2/quests/${quest.id}`}
                      className="v2-card v2-card--interactive v2-quest-card v2-quest-card--completed"
                    >
                      <StatusBar status="complete" />
                      <div className="v2-quest-card__content">
                        <h2 className="v2-quest-card__title">
                          {quest.title || 'Untitled Quest'}
                        </h2>
                        <p className="v2-quest-card__progress">
                          {formatProgress(quest)}
                        </p>
                      </div>
                    </Link>
                  ))}
                </div>
              </>
            )}
          </>
        )}
      </main>

      <KeyboardShortcuts isOpen={showShortcuts} onClose={() => setShowShortcuts(false)} />
    </div>
  );
}
