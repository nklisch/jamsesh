import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PlaygroundChip from './PlaygroundChip.svelte';

describe('PlaygroundChip', () => {
  it('renders the playground label', () => {
    render(PlaygroundChip);
    expect(screen.getByText('playground')).toBeInTheDocument();
  });

  it('has a clock icon', () => {
    render(PlaygroundChip);
    // Icon span is aria-hidden but present in the DOM.
    const icon = document.querySelector('.icon');
    expect(icon).toBeInTheDocument();
    expect(icon?.textContent).toBe('⏳');
  });

  it('carries an aria-label identifying it as a playground session', () => {
    render(PlaygroundChip);
    const chip = screen.getByLabelText('Playground session');
    expect(chip).toBeInTheDocument();
  });

  it('has the playground-chip class', () => {
    render(PlaygroundChip);
    const chip = document.querySelector('.playground-chip');
    expect(chip).toBeInTheDocument();
  });
});
