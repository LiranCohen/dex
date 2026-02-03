import { lazy, Suspense } from 'react';
import { Routes, Route } from 'react-router-dom';
import { ToastProvider, ErrorBoundary, SkeletonList } from './components';
import './styles/v2.css';

// Lazy load pages for code splitting
const Home = lazy(() => import('./pages/Home').then(m => ({ default: m.Home })));
const QuestDetail = lazy(() => import('./pages/QuestDetail').then(m => ({ default: m.QuestDetail })));
const ObjectiveDetail = lazy(() => import('./pages/ObjectiveDetail').then(m => ({ default: m.ObjectiveDetail })));
const Inbox = lazy(() => import('./pages/Inbox').then(m => ({ default: m.Inbox })));
const AllObjectives = lazy(() => import('./pages/AllObjectives').then(m => ({ default: m.AllObjectives })));
const NotFound = lazy(() => import('./pages/NotFound').then(m => ({ default: m.NotFound })));

function PageLoader() {
  return (
    <div className="v2-root">
      <div className="v2-content">
        <SkeletonList count={3} />
      </div>
    </div>
  );
}

export function V2App() {
  return (
    <ErrorBoundary>
      <ToastProvider>
        <Suspense fallback={<PageLoader />}>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/quests/:id" element={<QuestDetail />} />
            <Route path="/objectives/:id" element={<ObjectiveDetail />} />
            <Route path="/inbox" element={<Inbox />} />
            <Route path="/objectives" element={<AllObjectives />} />
            {/* 404 catch-all */}
            <Route path="*" element={<NotFound />} />
          </Routes>
        </Suspense>
      </ToastProvider>
    </ErrorBoundary>
  );
}
