// the dom overlay around the canvas: the room code and the invite button.
// kept out of render.js because this is html that changes once a match, not
// pixels that change sixty times a second.

// how long the button keeps showing its result before going back to normal
const FLASH_MS = 1600;

export class RoomBar {
  constructor(root) {
    this.root = root;
    this.codeEl = root.querySelector('[data-room-code]');
    this.button = root.querySelector('[data-copy-link]');
    this.buttonLabel = this.button.textContent;
    this.flashTimer = null;

    this.button.addEventListener('click', () => this.copyInviteLink());
  }

  show(code) {
    this.codeEl.textContent = code;
    this.root.hidden = false;
    rememberRoom(code);
  }

  async copyInviteLink() {
    try {
      await navigator.clipboard.writeText(location.href);
      this.flash('link copied');
    } catch {
      // the clipboard api needs a secure context and a gesture the browser
      // trusts, so it is unavailable over plain http on a lan. the address
      // bar already holds the same link, so point at that rather than fail
      // silently
      this.flash('copy the address bar');
    }
  }

  flash(message) {
    this.button.textContent = message;
    clearTimeout(this.flashTimer);
    this.flashTimer = setTimeout(() => {
      this.button.textContent = this.buttonLabel;
    }, FLASH_MS);
  }
}

// rememberRoom writes the code into the address bar without reloading, so the
// page someone shares already points at their match, and a reconnect after a
// dropped socket rejoins the same room instead of opening a new one.
function rememberRoom(code) {
  const url = new URL(location.href);
  if (url.searchParams.get('room') === code) return;

  url.searchParams.set('room', code);
  history.replaceState(null, '', url);
}
