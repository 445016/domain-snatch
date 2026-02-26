import axios from 'axios';
import { message } from 'antd';

// 优先使用环境变量，回退到当前域名或localhost
const getBaseURL = () => {
  // Vite 环境变量
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL;
  }
  // 生产环境：使用相对路径或同域API
  if (import.meta.env.PROD) {
    return '';
  }
  // 开发环境默认
  return 'http://localhost:8888';
};

const request = axios.create({
  baseURL: getBaseURL(),
  timeout: 120000, // 增加到120秒，处理极慢的域名
});

request.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

request.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
      message.error('登录已过期，请重新登录');
    } else {
      message.error(error.response?.data?.message || error.message || '请求失败');
    }
    return Promise.reject(error);
  }
);

export default request;
