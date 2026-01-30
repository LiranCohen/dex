import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AuthState {
  token: string | null;
  userId: string | null;
  isAuthenticated: boolean;
  setToken: (token: string, userId?: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      userId: null,
      isAuthenticated: false,
      setToken: (token: string, userId?: string) => {
        localStorage.setItem('auth_token', token);
        set({
          token,
          userId: userId || null,
          isAuthenticated: true,
        });
      },
      logout: () => {
        localStorage.removeItem('auth_token');
        set({
          token: null,
          userId: null,
          isAuthenticated: false,
        });
      },
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        token: state.token,
        userId: state.userId,
        isAuthenticated: state.isAuthenticated,
      }),
    }
  )
);
