// the dom overlay around the canvas: the room code and the invite button.
// kept out of render.js because this is html that changes once a match, not
// pixels that change sixty times a second.

// how long the button keeps showing its result before going back to normal
const FLASH_MS = 1600;

export class RoomBar {
  constructor(root) {
    this.root = root;
    this.codeEl = root.querySelector('[data-room-code]');
    this.pingEl = root.querySelector('[data-ping]');
    this.button = root.querySelector('[data-copy-link]');
    this.buttonLabel = this.button.textContent;
    this.flashTimer = null;

    this.button.addEventListener('click', () => this.copyInviteLink());
  }

  show(code) {
    this.codeEl.textContent = code;
    this.root.hidden = false;
    // in the tab title too, so two rooms open side by side can be told apart
    // without switching between them
    document.title = `beachball · ${code}`;
    rememberRoom(code);
  }

  // showPing writes the latest round trip. the thresholds are about what the
  // game feels like rather than what a network engineer would call good: under
  // 60ms a rally plays clean, past 150ms you start swinging at where the ball
  // used to be
  showPing(rtt) {
    const ms = Math.round(rtt);
    this.pingEl.textContent = `${ms} ms`;

    this.pingEl.classList.toggle('is-good', ms < 60);
    this.pingEl.classList.toggle('is-fair', ms >= 60 && ms < 150);
    this.pingEl.classList.toggle('is-poor', ms >= 150);
  }

  // clearPing marks the readout stale, so a frozen number is never mistaken
  // for a live one while the socket is down
  clearPing() {
    this.pingEl.textContent = '—';
    this.pingEl.classList.remove('is-good', 'is-fair', 'is-poor');
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
