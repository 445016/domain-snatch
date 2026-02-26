import request from './request';

// Auth
export const login = (data: { username: string; password: string }) =>
  request.post('/api/auth/login', data);

export const getUserInfo = () =>
  request.get('/api/auth/info');

// Domains
export const getDomainList = (params: {
  page?: number;
  pageSize?: number;
  status?: string;
  keyword?: string;
  monitor?: number;
  expiryStart?: string;
  expiryEnd?: string;
  sortField?: string;
  sortOrder?: string;
}) => request.get('/api/domains', { params });

export const addDomain = (data: { domain: string }) =>
  request.post('/api/domains', data);

export const deleteDomain = (id: number) =>
  request.delete(`/api/domains/${id}`);

export const toggleMonitor = (id: number, monitor: number) =>
  request.put(`/api/domains/${id}/monitor`, { monitor });

export const checkDomains = (data: { ids: number[] }) =>
  request.post('/api/domains/check', data);

export const importDomains = (file: File) => {
  const formData = new FormData();
  formData.append('file', file);
  return request.post('/api/domains/import', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
};

// Whois
export const whoisQuery = (data: { domain: string }) =>
  request.post('/api/whois/query', data);

// Snatch Tasks
export const getSnatchTaskList = (params: {
  page?: number;
  pageSize?: number;
  status?: string;
}) => request.get('/api/snatch/tasks', { params });

export const createSnatchTask = (data: {
  domainId: number;
  domain: string;
  priority?: number;
  targetRegistrar?: string;
  autoRegister?: number;  // 0=手动, 1=自动
}) => request.post('/api/snatch/tasks', data);

export const updateSnatchTask = (id: number, data: { status: string; result?: string }) =>
  request.put(`/api/snatch/tasks/${id}`, data);

export const deleteSnatchTask = (id: number) =>
  request.delete(`/api/snatch/tasks/${id}`);

// Notify
export const getNotifySettings = () =>
  request.get('/api/notify/settings');

export const updateNotifySettings = (data: {
  webhookUrl: string;
  expireDays: number;
  enabled: number;
}) => request.put('/api/notify/settings', data);

// Dashboard
export const getDashboardStats = () =>
  request.get('/api/dashboard/stats');
