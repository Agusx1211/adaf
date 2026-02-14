import { createRoot } from 'react-dom/client';
import { AppProvider } from './state/store.js';
import { ToastProvider } from './components/common/Toast.jsx';
import App from './App.jsx';
import './global.css';

var root = document.getElementById('root');
if (root) {
  createRoot(root).render(
    <AppProvider>
      <ToastProvider>
        <App />
      </ToastProvider>
    </AppProvider>
  );
}
