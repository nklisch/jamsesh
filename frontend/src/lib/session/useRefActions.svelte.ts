// Ref-action menu and dialog state.
//
// Manages which context menu is open (activeMenuRef) and which dialog is
// active (fork | mode-switch), along with the ref and mode it targets.
//
// Uses the wrapper-object-rune-store pattern via a factory so that each
// SessionViewShell instance gets its own isolated state.

export type ActiveDialog = null | 'fork' | 'mode-switch';

export function createRefActions() {
  let _activeMenuRef = $state<{ ref: string; x: number; y: number } | null>(null);
  let _activeDialog = $state<ActiveDialog>(null);
  let _activeDialogRef = $state<string>('');
  let _activeDialogRefMode = $state<'sync' | 'isolated' | undefined>(undefined);

  return {
    get activeMenuRef(): { ref: string; x: number; y: number } | null {
      return _activeMenuRef;
    },
    get activeDialog(): ActiveDialog {
      return _activeDialog;
    },
    get activeDialogRef(): string {
      return _activeDialogRef;
    },
    get activeDialogRefMode(): 'sync' | 'isolated' | undefined {
      return _activeDialogRefMode;
    },

    handleRefAction(event: { ref: string; action: 'menu'; x: number; y: number }) {
      _activeMenuRef = { ref: event.ref, x: event.x, y: event.y };
    },

    handleMenuAction(action: 'fork' | 'mode-switch', ref: string) {
      _activeMenuRef = null;
      _activeDialogRef = ref;
      _activeDialog = action;
      // Fetch current mode for the mode-switch dialog (best-effort).
      _activeDialogRefMode = undefined;
    },

    closeMenu() {
      _activeMenuRef = null;
    },

    closeDialog() {
      _activeDialog = null;
      _activeDialogRef = '';
      _activeDialogRefMode = undefined;
    },
  };
}
