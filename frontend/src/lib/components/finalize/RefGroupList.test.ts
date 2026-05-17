import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import RefGroupList from './RefGroupList.svelte';

type SourceCommit = {
  sha: string;
  author_name: string;
  author_id: string;
  subject: string;
};
type RefGroup = {
  ref: string;
  kind: 'draft' | 'isolated' | 'sync';
  tip_sha: string;
  commits: SourceCommit[];
};

function makeCommit(sha: string, subject: string): SourceCommit {
  return { sha, author_name: 'Alice', author_id: 'user-alice', subject };
}

function makeGroup(ref: string, kind: RefGroup['kind'], commits: SourceCommit[]): RefGroup {
  return { ref, kind, tip_sha: commits[0]?.sha ?? 'aaa', commits };
}

describe('RefGroupList', () => {
  it('1. empty state: no refs shows empty placeholder, no toggle possible', () => {
    const onToggle = vi.fn();
    render(RefGroupList, {
      props: { refs: [], selected: new Set(), onToggle },
    });
    expect(screen.getByText(/no refs available yet/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /add/i })).not.toBeInTheDocument();
    expect(onToggle).not.toHaveBeenCalled();
  });

  it('2. sync-only refs: renders sync group card', () => {
    const commit = makeCommit('aaaaaaa1111111111111111111111111111111aa', 'Add auth');
    const group = makeGroup('refs/heads/main', 'sync', [commit]);
    render(RefGroupList, {
      props: { refs: [group], selected: new Set(), onToggle: vi.fn() },
    });
    expect(screen.getByText('main')).toBeInTheDocument();
    expect(screen.getByText('sync')).toBeInTheDocument();
    expect(screen.getByText('Add auth')).toBeInTheDocument();
  });

  it('3. isolated-only refs: renders isolated group card', () => {
    const commit = makeCommit('bbbbbbb2222222222222222222222222222222bb', 'Wire middleware');
    const group = makeGroup('refs/heads/feature/auth', 'isolated', [commit]);
    render(RefGroupList, {
      props: { refs: [group], selected: new Set(), onToggle: vi.fn() },
    });
    expect(screen.getByText('feature/auth')).toBeInTheDocument();
    expect(screen.getByText('isolated')).toBeInTheDocument();
    expect(screen.getByText('Wire middleware')).toBeInTheDocument();
  });

  it('4. mixed sync + isolated: both group cards appear', () => {
    const syncCommit = makeCommit('aaaaaaa1111111111111111111111111111111aa', 'Add auth');
    const isolatedCommit = makeCommit('bbbbbbb2222222222222222222222222222222bb', 'Wire middleware');
    const syncGroup = makeGroup('refs/heads/main', 'sync', [syncCommit]);
    const isolatedGroup = makeGroup('refs/heads/feature/auth', 'isolated', [isolatedCommit]);
    render(RefGroupList, {
      props: { refs: [syncGroup, isolatedGroup], selected: new Set(), onToggle: vi.fn() },
    });
    expect(screen.getByText('sync')).toBeInTheDocument();
    expect(screen.getByText('isolated')).toBeInTheDocument();
    expect(screen.getByText('Add auth')).toBeInTheDocument();
    expect(screen.getByText('Wire middleware')).toBeInTheDocument();
  });

  it('5. toggle: clicking the add button fires onToggle with the correct SHA', async () => {
    const onToggle = vi.fn();
    const sha = 'aaaaaaa1111111111111111111111111111111aa';
    const commit = makeCommit(sha, 'Add auth');
    const group = makeGroup('refs/heads/main', 'sync', [commit]);
    render(RefGroupList, {
      props: { refs: [group], selected: new Set(), onToggle },
    });
    const addBtn = screen.getByRole('button', { name: /add aaaaaaa/i });
    await fireEvent.click(addBtn);
    expect(onToggle).toHaveBeenCalledOnce();
    expect(onToggle).toHaveBeenCalledWith(sha);
  });

  it('6. selected: SHA in selected set causes button to show remove label and row to be in-cart', () => {
    const sha = 'aaaaaaa1111111111111111111111111111111aa';
    const commit = makeCommit(sha, 'Add auth');
    const group = makeGroup('refs/heads/main', 'sync', [commit]);
    render(RefGroupList, {
      props: { refs: [group], selected: new Set([sha]), onToggle: vi.fn() },
    });
    // Button label changes to "Remove ..."
    expect(screen.getByRole('button', { name: /remove aaaaaaa/i })).toBeInTheDocument();
    // in-cart class applied to the row
    const row = screen.getByText('Add auth').closest('.commit-row');
    expect(row).toHaveClass('in-cart');
  });

  it('7. disabled: when disabled=true, add/remove buttons are disabled', () => {
    const onToggle = vi.fn();
    const sha = 'aaaaaaa1111111111111111111111111111111aa';
    const commit = makeCommit(sha, 'Add auth');
    const group = makeGroup('refs/heads/main', 'sync', [commit]);
    render(RefGroupList, {
      props: { refs: [group], selected: new Set(), onToggle, disabled: true },
    });
    const addBtn = screen.getByRole('button', { name: /add aaaaaaa/i });
    expect(addBtn).toBeDisabled();
  });
});
