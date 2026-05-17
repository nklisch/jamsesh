// theme-bootstrap.ts MUST be first — it sets data-theme on <html> before
// any paint, preventing FOUC when the user has a saved theme preference.
import '$lib/styles/theme-bootstrap.ts';
import './app.css';

import { mount } from 'svelte';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
