import { create } from 'zustand';

interface UserInfo {
  id: number;
  username: string;
  role: string;
}

interface AuthState {
  token: string | null;
  user: UserInfo | null;
  setToken: (token: string) => void;
  setUser: (user: UserInfo) => void;
  logout: () => void;
  isLoggedIn: () => boolean;
}

// Simple state management without zustand dependency
let authState: AuthState;

export const useAuthStore = (): AuthState => {
  // Using localStorage-based simple state since we want to keep deps minimal
  const token = localStorage.getItem('token');
  return {
    token,
    user: null,
    setToken: (t: string) => localStorage.setItem('token', t),
    setUser: () => {},
    logout: () => {
      localStorage.removeItem('token');
      window.location.href = '/login';
    },
    isLoggedIn: () => !!localStorage.getItem('token'),
  };
};

export const isLoggedIn = () => !!localStorage.getItem('token');
export const getToken = () => localStorage.getItem('token');
export const setToken = (token: string) => localStorage.setItem('token', token);
export const removeToken = () => localStorage.removeItem('token');
