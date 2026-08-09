// websocket transport. owns the connection and hands incoming snapshots to
// whoever subscribed; knows nothing about rendering or game rules.

// the first retry is quick, so a momentary blip is invisible, and each
// further one backs off. a server that is genuinely down, or a link with a
// room code the server rejects, would otherwise be hammered once a second
// for as long as the tab stays open
const RECONNECT_DELAY_MS = 500;
const RECONNECT_MAX_DELAY_MS = 10000;

// the room code lives in the query string, which makes the address bar the
// invite link. no code asks the server to open a fresh room and tell us which
function roomCodeFromURL() {
  return new URLSearchParams(location.search).get('room') ?? '';
}

// derives the socket url from the page url, so the same build works both
// behind the vite dev proxy and when the go server serves the bundle itself
function socketURL() {
  const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
  const code = roomCodeFromURL();
  const query = code ? `?room=${encodeURIComponent(code)}` : '';
  return `${scheme}://${location.host}/ws${query}`;
}

export class Connection {
  constructor({ onWelcome, onSnapshot, onStatus }) {
    this.onWelcome = onWelcome;
    this.onSnapshot = onSnapshot;
    this.onStatus = onStatus;

    this.socket = null;
    this.reconnectTimer = null;
    this.reconnectDelay = RECONNECT_DELAY_MS;
    this.lastSentKeys = null;
    this.lastSentLobby = null;
    this.lastLobby = null;
  }

  connect() {
    this.onStatus('connecting');

    const socket = new WebSocket(socketURL());
    this.socket = socket;

    socket.addEventListener('open', () => {
      this.onStatus('connected');
      this.reconnectDelay = RECONNECT_DELAY_MS;
      // the server keeps the last input it received, so a fresh connection
      // must resend the current key state instead of waiting for a change
      this.lastSentKeys = null;
      this.resendLobby();
    });

    socket.addEventListener('message', (event) => this.handleMessage(event.data));

    socket.addEventListener('close', () => {
      this.onStatus('disconnected');
      this.scheduleReconnect();
    });

    // close fires after error too, so reconnecting is handled in one place
    socket.addEventListener('error', () => socket.close());
  }

  scheduleReconnect() {
    if (this.reconnectTimer !== null) return;

    const delay = this.reconnectDelay;
    this.reconnectDelay = Math.min(delay * 2, RECONNECT_MAX_DELAY_MS);

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  handleMessage(raw) {
    let envelope;
    try {
      envelope = JSON.parse(raw);
    } catch {
      return;
    }

    switch (envelope.type) {
      case 'welcome':
        this.onWelcome(envelope.data);
        break;
      case 'state':
        this.onSnapshot(envelope.data.world);
        break;
      default:
        break;
    }
  }

  // sendInput only writes when something actually changed: the server holds
  // the last key state until told otherwise, so resending it every frame
  // would be 60 pointless messages per second
  sendInput(keys) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;

    const encoded = JSON.stringify(keys);
    if (encoded === this.lastSentKeys) return;
    this.lastSentKeys = encoded;

    this.socket.send(JSON.stringify({ type: 'input', data: { keys } }));
  }

  // sendLobby reports the name and readiness. also deduplicated, since typing
  // a name fires an event per keystroke, but remembered so a reconnect can
  // restore what the player had already chosen
  sendLobby(state) {
    this.lastLobby = state;

    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;

    const encoded = JSON.stringify(state);
    if (encoded === this.lastSentLobby) return;
    this.lastSentLobby = encoded;

    this.socket.send(JSON.stringify({ type: 'lobby', data: state }));
  }

  // resendLobby pushes the remembered lobby state onto a fresh socket. the
  // new connection is a new player as far as the server is concerned, so
  // without this a reconnect would land nameless and not ready
  resendLobby() {
    if (!this.lastLobby) return;
    this.lastSentLobby = null;
    this.sendLobby(this.lastLobby);
  }
}
