// Comment composer state: open/closed + selected line range.
//
// Drives the CommentComposer overlay: the composer opens automatically
// when a range is selected in the artifact pane (via handleRangeSelect)
// and stays open until the user submits or cancels.
//
// Uses the wrapper-object-rune-store pattern via a factory so that each
// SessionViewShell instance gets its own isolated state.

export function createCommentComposer() {
  let _open = $state(false);
  let _range = $state<{ start: number; end: number } | null>(null);

  return {
    get open(): boolean {
      return _open;
    },
    get range(): { start: number; end: number } | null {
      return _range;
    },

    handleRangeSelect(range: { start: number; end: number } | null) {
      _range = range;
      if (range) _open = true;
    },

    close() {
      _open = false;
    },
  };
}
