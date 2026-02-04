import { render, type RenderOptions } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, type MemoryRouterProps } from 'react-router-dom';
import { ToastProvider } from '../app/components/Toast';
import type { ReactElement, ReactNode } from 'react';

// Create a fresh query client for each test
function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
        staleTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

interface WrapperOptions {
  initialEntries?: MemoryRouterProps['initialEntries'];
  withToast?: boolean;
}

function createWrapper(options: WrapperOptions = {}) {
  const { initialEntries = ['/'], withToast = true } = options;
  const queryClient = createTestQueryClient();

  return function Wrapper({ children }: { children: ReactNode }) {
    const content = (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>
          {children}
        </MemoryRouter>
      </QueryClientProvider>
    );

    return withToast ? <ToastProvider>{content}</ToastProvider> : content;
  };
}

interface CustomRenderOptions extends Omit<RenderOptions, 'wrapper'> {
  initialEntries?: MemoryRouterProps['initialEntries'];
  withToast?: boolean;
}

export function renderWithProviders(
  ui: ReactElement,
  options: CustomRenderOptions = {}
) {
  const { initialEntries, withToast, ...renderOptions } = options;

  return render(ui, {
    wrapper: createWrapper({ initialEntries, withToast }),
    ...renderOptions,
  });
}

// Re-export everything from testing-library
export * from '@testing-library/react';
export { default as userEvent } from '@testing-library/user-event';

// Export custom render as default
export { renderWithProviders as render };
