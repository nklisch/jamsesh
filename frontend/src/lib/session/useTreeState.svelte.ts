// Tree-state machine with localStorage persistence per session.
//
//  tree-collapsed → tree-expanded → tree-wide → tree-collapsed  (cyclic)
//
// Uses the wrapper-object-rune-store pattern: private $state held in a
// closure, exposed via a plain-object facade with getters and mutation
// methods so consumers get reactivity without direct $state export.

export type TreeState = 'tree-collapsed' | 'tree-expanded' | 'tree-wide';

const TREE_STATES: TreeState[] = ['tree-collapsed', 'tree-expanded', 'tree-wide'];

function treeStateKey(sessionId: string): string {
  return `jamsesh.tree-state.${sessionId}`;
}

function loadFromLS(sessionId: string): TreeState {
  const stored =
    typeof localStorage !== 'undefined' ? localStorage.getItem(treeStateKey(sessionId)) : null;
  if (stored && TREE_STATES.includes(stored as TreeState)) return stored as TreeState;
  return 'tree-collapsed';
}

export function createTreeState(sessionId: string) {
  let _state = $state<TreeState>(loadFromLS(sessionId));

  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(treeStateKey(sessionId), _state);
    }
  });

  return {
    get value(): TreeState {
      return _state;
    },
    cycle() {
      const idx = TREE_STATES.indexOf(_state);
      _state = TREE_STATES[(idx + 1) % TREE_STATES.length];
    },
  };
}
