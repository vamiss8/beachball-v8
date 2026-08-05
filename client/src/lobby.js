// the pre-match overlay: pick a name, say you are ready, watch for the other
// side to do the same. it reads the phase out of the snapshot and never
// decides anything itself, the same way the canvas does.

// how the client refers to a player who never typed a name. the server leaves
// the name empty on purpose so it does not have to know about colours
const DEFAULT_NAMES = { left: 'Green', right: 'Blue' };

export function displayName(player) {
  return player.name || DEFAULT_NAMES[player.side] || 'Player';
}

export class LobbyPanel {
  constructor(root, { onChange }) {
    this.root = root;
    this.onChange = onChange;

    this.nameInput = root.querySelector('[data-name]');
    this.readyButton = root.querySelector('[data-ready]');
    this.statusEl = root.querySelector('[data-lobby-status]');
    this.headingEl = root.querySelector('[data-lobby-heading]');

    this.ready = false;

    this.nameInput.addEventListener('input', () => this.publish());
    this.readyButton.addEventListener('click', () => {
      this.ready = !this.ready;
      this.publish();
    });

    // enter is what people press after typing a name
    this.nameInput.addEventListener('keydown', (event) => {
      if (event.key !== 'Enter') return;
      this.ready = true;
      this.publish();
    });
  }

  publish() {
    this.onChange({ name: this.nameInput.value, ready: this.ready });
  }

  // update mirrors the world back into the panel. the server owns readiness,
  // so a rematch that cleared it is reflected here rather than fought with
  update(world, view) {
    const showing = world.phase === 'lobby' || world.phase === 'finished';
    this.root.hidden = !showing || view.spectator;
    if (this.root.hidden) return;

    const me = world.players[view.playerId];
    if (me && me.ready !== this.ready) {
      this.ready = me.ready;
    }

    this.headingEl.textContent = world.phase === 'finished' ? 'rematch?' : 'ready up';
    this.readyButton.textContent = this.ready ? "waiting — i'm ready" : "i'm ready";
    this.readyButton.classList.toggle('is-ready', this.ready);
    this.statusEl.textContent = describeOpponent(world, view);
  }
}

// describeOpponent says what the other seat is doing, which is the only thing
// worth watching while stuck on this screen
function describeOpponent(world, view) {
  const others = Object.values(world.players).filter((p) => p.id !== view.playerId);
  if (others.length === 0) {
    return 'waiting for someone to join — send them the invite link';
  }

  const other = others[0];
  return other.ready
    ? `${displayName(other)} is ready`
    : `waiting for ${displayName(other)}`;
}
