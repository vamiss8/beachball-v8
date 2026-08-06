// keyboard handling. turns physical keys into the key state the server
// expects; no game logic lives here.

// several physical keys may map onto the same action
const BINDINGS = {
  KeyA: 'left',
  ArrowLeft: 'left',
  KeyD: 'right',
  ArrowRight: 'right',
  KeyW: 'jump',
  ArrowUp: 'jump',
  Space: 'jump',
  KeyS: 'block',
  ArrowDown: 'block',
};

export class Input {
  constructor() {
    // dashing has no key of its own: the server spots a walk key tapped
    // twice, so nothing here has to know that dashes exist
    this.keys = {
      left: false,
      right: false,
      jump: false,
      block: false,
    };

    window.addEventListener('keydown', (e) => this.handle(e, true));
    window.addEventListener('keyup', (e) => this.handle(e, false));
    // a key held while the tab loses focus would otherwise stay stuck down
    window.addEventListener('blur', () => this.releaseAll());
  }

  handle(event, pressed) {
    // typing into the lobby must not steer the player, and more importantly
    // the preventDefault below would otherwise swallow the letters a, d, w
    // and s, leaving half the alphabet untypeable in the name field. a
    // release is still let through: a key held while clicking into the field
    // is let go inside it, and ignoring that would leave the player walking
    // into the net for the rest of the match
    if (pressed && event.target instanceof HTMLInputElement) return;

    const action = BINDINGS[event.code];
    if (!action) return;

    // arrows and space scroll the page otherwise
    event.preventDefault();

    if (event.repeat) return;
    this.keys[action] = pressed;
  }

  releaseAll() {
    for (const action of Object.keys(this.keys)) {
      this.keys[action] = false;
    }
  }
}
