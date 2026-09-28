import React from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import App from './App';

function showFatalError(reason: unknown) {
  const root = document.getElementById('root');
  if (!root) return;
  const message = reason instanceof Error ? `${reason.name}: ${reason.message}` : String(reason);
  root.innerHTML = `<main class="shell"><div class="notice error"><strong>AgentPal could not start.</strong><br>${escapeHTML(message)}</div></main>`;
}

function escapeHTML(value: string) {
  const node = document.createElement('div');
  node.textContent = value;
  return node.innerHTML;
}

window.addEventListener('error', (event) => showFatalError(event.error ?? event.message));
window.addEventListener('unhandledrejection', (event) => showFatalError(event.reason));

const root = document.getElementById('root');
if (!root) throw new Error('Root element was not found');

createRoot(root).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
