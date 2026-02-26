import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import AppLayout from './components/Layout';
import LoginPage from './pages/login';
import DashboardPage from './pages/dashboard';
import DomainsPage from './pages/domains';
import SearchPage from './pages/search';
import SnatchPage from './pages/snatch';
import NotifyPage from './pages/notify';
import { isLoggedIn } from './store/auth';

const PrivateRoute: React.FC<{ children: React.ReactElement }> = ({ children }) => {
  if (!isLoggedIn()) {
    return <Navigate to="/login" replace />;
  }
  return children;
};

const App: React.FC = () => {
  return (
    <ConfigProvider locale={zhCN}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <PrivateRoute>
                <AppLayout />
              </PrivateRoute>
            }
          >
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<DashboardPage />} />
            <Route path="domains" element={<DomainsPage />} />
            <Route path="search" element={<SearchPage />} />
            <Route path="snatch" element={<SnatchPage />} />
            <Route path="notify" element={<NotifyPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </BrowserRouter>
    </ConfigProvider>
  );
};

export default App;
