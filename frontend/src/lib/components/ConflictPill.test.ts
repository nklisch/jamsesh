import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ConflictPill from './ConflictPill.svelte';

describe('ConflictPill', () => {
  it('renders the text "conflict"', () => {
    render(ConflictPill, {});
    expect(screen.getByText('conflict')).toBeInTheDocument();
  });

  it('applies conflict styling via Badge variant', () => {
    render(ConflictPill, {});
    expect(screen.getByText('conflict')).toHaveClass('pill-conflict');
  });

  it('has the base pill class', () => {
    render(ConflictPill, {});
    expect(screen.getByText('conflict')).toHaveClass('pill');
  });
});
